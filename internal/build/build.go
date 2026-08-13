// Package build drives a whole compilation: parse, check, emit C, and hand the
// result to a C compiler.
package build

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/malleum/weave/internal/check"
	"github.com/malleum/weave/internal/codegen"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/parser"
	"github.com/malleum/weave/internal/rt"
)

// Options configure a build.
type Options struct {
	// Output is the executable to produce. When empty it is derived from the
	// source file name.
	Output string
	// KeepC leaves the generated C and the work directory in place.
	KeepC bool
	// CC overrides the C compiler. Empty means clang, then cc.
	CC string
	// Opt is the optimisation flag passed to the C compiler.
	Opt string
	// CheckOverflow compiles Earth arithmetic with an overflow check, so a
	// number too large for an int64 stops the program instead of wrapping
	// round to a wrong answer that looks like a right one.
	CheckOverflow bool
	// Verbose prints the C compiler command line.
	Verbose bool
	// Tally compiles the arena to keep its own books and print, on exit, what
	// was holding the heap at its high-water mark and where it was allocated.
	// The bookkeeping costs more than the allocation, so a tallied build says
	// nothing about speed.
	Tally bool
	// DisableFusion compiles Thread chains one runtime call per stage.
	DisableFusion bool
	// DisableSpecialize keeps the general prelude verbs instead of the typed
	// helpers the inferred types allow.
	DisableSpecialize bool
	// DisableRegions keeps a fused loop turn's storage instead of handing it
	// back when the turn ends.
	DisableRegions bool
	// DisableInPlace makes every grid update copy.
	DisableInPlace bool
	// DisableRelease keeps every Thread a function builds instead of handing
	// the dead ones back when it returns.
	DisableRelease bool
	// Trace compiles the program to report every top-level definition's value
	// rather than its output expression. See `weave trace`.
	Trace bool
	// Watch names one function whose calls are recorded, so that the inside of
	// a recursion can be read the way the outside already can. Trace only.
	Watch string
	// Lenient lets the build proceed past a soft diagnostic — a definition
	// that does not cover every case, which compiles to a runtime trap. The
	// REPL sets it so that a definition can be entered one clause at a time.
	Lenient bool
	// CacheDir, when set, holds the runtime compiled to object files. They are
	// built once and reused, which is what makes repeated small builds (the
	// REPL) fast: only the program itself is compiled each time.
	CacheDir string
}

// Result describes a completed build.
type Result struct {
	Executable string
	WorkDir    string
	CFile      string
}

// Compile builds src (already read from path) into an executable.
func Compile(path, src string, opts Options, bag *diag.Bag) (*Result, error) {
	bag.Lenient = bag.Lenient || opts.Lenient
	file := parser.Parse(src, bag)
	if !bag.Empty() {
		return nil, fmt.Errorf("parse failed")
	}

	info := check.File(file, bag)
	if !bag.Empty() {
		return nil, fmt.Errorf("check failed")
	}

	csrc := codegen.Generate(file, info, bag, codegen.Options{
		DisableFusion:     opts.DisableFusion,
		DisableSpecialize: opts.DisableSpecialize,
		DisableInPlace:    opts.DisableInPlace,
		DisableRelease:    opts.DisableRelease,
		DisableRegions:    opts.DisableRegions,
		Trace:             opts.Trace,
		Watch:             opts.Watch,
	})
	if !bag.Empty() {
		return nil, fmt.Errorf("code generation failed")
	}

	work, err := os.MkdirTemp("", "weave-build-")
	if err != nil {
		return nil, err
	}
	cleanup := func() {
		if !opts.KeepC {
			os.RemoveAll(work)
		}
	}

	cfile := filepath.Join(work, "program.c")
	if err := os.WriteFile(cfile, []byte(csrc), 0o644); err != nil {
		cleanup()
		return nil, err
	}

	out := opts.Output
	if out == "" {
		out = strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
		if out == "" {
			out = "a.out"
		}
	}
	if abs, err := filepath.Abs(out); err == nil {
		out = abs
	}

	cc := opts.CC
	if cc == "" {
		cc = findCC()
	}
	opt := opts.Opt
	if opt == "" {
		opt = "-O3"
	}

	include, runtimeInputs, err := runtimeFor(work, cc, opt, opts)
	if err != nil {
		cleanup()
		return nil, err
	}

	args := []string{opt, "-std=c11", "-I", include, "-o", out, cfile}
	args = append(args, defines(opts)...)
	args = append(args, runtimeInputs...)
	// The generated code is machine-written; its warnings are not the user's
	// problem, so keep the output clean but leave real errors visible.
	args = append(args, "-w", "-lm")

	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "weave: %s %s\n", cc, strings.Join(args, " "))
	}

	cmd := exec.Command(cc, args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		if !opts.KeepC {
			// A C compiler failure means a compiler bug, so keep the evidence.
			fmt.Fprintf(os.Stderr, "weave: the generated C is in %s\n", cfile)
			return nil, fmt.Errorf("the C compiler failed: %w", err)
		}
		cleanup()
		return nil, fmt.Errorf("the C compiler failed: %w", err)
	}

	if !opts.KeepC {
		defer cleanup()
	}
	return &Result{Executable: out, WorkDir: work, CFile: cfile}, nil
}

// runtimeFor makes the runtime available to the C compiler, returning the
// include directory and the inputs to add to the command line. Without a cache
// the sources are dropped next to the program and compiled with it; with one
// they are compiled to object files the first time and linked thereafter.
func runtimeFor(work, cc, opt string, opts Options) (string, []string, error) {
	files := rt.Files()
	defs := defines(opts)
	sources := rt.CSources
	if opts.Tally {
		sources = append(append([]string{}, sources...), rt.TallySource)
	}

	// A cache that cannot be made is not a reason to fail: caching is an
	// optimisation, and the build has a perfectly good uncached path. Sandboxes
	// that run the tests with no writable home — Nix's builder is the one that
	// found this — would otherwise fail every build for the sake of a directory
	// nothing needed.
	if opts.CacheDir != "" {
		if err := os.MkdirAll(opts.CacheDir, 0o755); err != nil {
			opts.CacheDir = ""
		}
	}

	if opts.CacheDir == "" {
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(work, name), content, 0o644); err != nil {
				return "", nil, err
			}
		}
		inputs := make([]string, 0, len(sources))
		for _, name := range sources {
			inputs = append(inputs, filepath.Join(work, name))
		}
		return work, inputs, nil
	}

	// The cache is keyed by the runtime's contents and the flags it was built
	// with, so changing either produces a fresh directory rather than stale
	// objects.
	dir := filepath.Join(opts.CacheDir, "rt-"+runtimeKey(files, cc, opt, defs))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil, err
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, content) {
			continue
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return "", nil, err
		}
	}

	inputs := make([]string, 0, len(sources))
	for _, name := range sources {
		obj := filepath.Join(dir, strings.TrimSuffix(name, ".c")+".o")
		if _, err := os.Stat(obj); err != nil {
			argv := append([]string{opt, "-std=c11", "-w", "-I", dir}, defs...)
			argv = append(argv, "-c", filepath.Join(dir, name), "-o", obj)
			cmd := exec.Command(cc, argv...)
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return "", nil, fmt.Errorf("compiling the runtime failed: %w", err)
			}
		}
		inputs = append(inputs, obj)
	}
	return dir, inputs, nil
}

// defines are the preprocessor flags the options ask for. They go to the
// program and to the runtime alike, and into the cache key, since a runtime
// built without them is not interchangeable with one built with them.
func defines(opts Options) []string {
	var out []string
	if opts.CheckOverflow {
		out = append(out, "-DWEAVE_CHECK_OVERFLOW")
	}
	if opts.Tally {
		out = append(out, "-DWEAVE_TALLY")
	}
	return out
}

// runtimeKey fingerprints the runtime sources and the flags used to build them.
func runtimeKey(files map[string][]byte, cc, opt string, defs []string) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)

	h := sha256.New()
	fmt.Fprintf(h, "%s\x00%s\x00%s\x00", cc, opt, strings.Join(defs, " "))
	for _, name := range names {
		fmt.Fprintf(h, "%s\x00", name)
		h.Write(files[name])
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// findCC picks a C compiler, preferring clang because Weave's performance
// story assumes LLVM's optimiser.
func findCC() string {
	for _, name := range []string{"clang", "cc", "gcc"} {
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return "cc"
}
