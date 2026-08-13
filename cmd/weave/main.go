// Command weave is the compiler and toolchain for the Weave language.
//
// Usage:
//
//	weave check  file.weave    parse and check a program
//	weave lex    file.weave    print the token stream
//	weave parse  file.weave    print the syntax tree
//	weave build  file.weave    compile to a native executable
//	weave run    file.weave    compile and run, with stdin as Source
//	weave version
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/malleum/weave/internal/build"

	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/check"
	"github.com/malleum/weave/internal/codegen"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/docs"
	"github.com/malleum/weave/internal/format"
	"github.com/malleum/weave/internal/lexer"
	"github.com/malleum/weave/internal/lsp"
	"github.com/malleum/weave/internal/parser"
	"github.com/malleum/weave/internal/prelude"
	"github.com/malleum/weave/internal/style"
	"github.com/malleum/weave/internal/token"
	"github.com/malleum/weave/internal/types"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

// cmd is shorthand for one row of the table below, and gap is the blank line
// between groups of them.
func cmd(name, args, doc string) style.Command {
	return style.Command{Name: name, Args: args, Doc: doc}
}

func gap() style.Command { return style.Command{} }

// commands is the help screen, and the only place the command list lives.
var commands = []style.Command{
	cmd("run", "file.weave", "compile and run, feeding stdin to Source"),
	cmd("build", "file.weave", "compile to a native executable"),
	cmd("check", "file.weave", "parse and check, reporting any errors"),
	cmd("fmt", "file.weave", "print in canonical form (-w rewrites; - reads stdin)"),
	cmd("repl", "[input]", "evaluate definitions and expressions as you type them"),
	cmd("test", "file.weave", "run a program against the .in/.out files beside it"),
	cmd("trace", "file.weave", "print every definition's value, one record per line"),
	gap(),
	cmd("verbs", "[search]", "the built-in vocabulary, with types"),
	cmd("docs", "", "serve the reference as a page, with search (-o writes it out)"),
	cmd("lsp", "", "run the language server on stdin and stdout"),
	cmd("version", "", "print the compiler version"),
	gap(),
	cmd("parse", "file.weave", "print the syntax tree (for debugging the compiler)"),
	cmd("lex", "file.weave", "print the token stream (for debugging the compiler)"),
}

// help prints the title, the commands, and the one shorthand worth knowing.
func help(w io.Writer) {
	st := style.For(w)
	fmt.Fprint(w, st.Banner(version))
	fmt.Fprint(w, st.Help("commands", commands))
	fmt.Fprintf(w, "\n  %s is short for %s\n\n",
		st.Bold("weave file.weave"), st.Bold("weave run file.weave"))
}

// shortUsage is what an unrecognised command gets: the list, without the
// ceremony.
func shortUsage(w io.Writer) {
	st := style.For(w)
	fmt.Fprintf(w, "usage: %s\n\n", st.Bold("weave <command> [arguments]"))
	for _, c := range commands {
		if c.Name != "" {
			fmt.Fprintf(w, "  %s\n", c.Name)
		}
	}
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if !errors.Is(err, errReported) {
			fmt.Fprintf(os.Stderr, "weave: %v\n", err)
		}
		os.Exit(1)
	}
}

// errReported signals that diagnostics have already been printed, so main
// should exit non-zero without adding another message.
var errReported = errors.New("compilation failed")

func run(args []string) error {
	if len(args) == 0 {
		// Asking for help is not an error, so this goes to stdout and exits
		// zero. Getting a command wrong, below, does not.
		help(os.Stdout)
		return nil
	}

	cmd, rest := args[0], args[1:]
	switch cmd {
	case "check":
		return cmdCheck(rest)
	case "lex":
		return cmdLex(rest)
	case "parse":
		return cmdParse(rest)
	case "build":
		return cmdBuild(rest)
	case "run":
		return cmdRun(rest)
	case "trace":
		return cmdTrace(rest)
	case "repl":
		return cmdRepl(rest)
	case "test":
		return cmdTest(rest)
	case "fmt":
		return cmdFmt(rest)
	case "lsp":
		return lsp.Serve(os.Stdin, os.Stdout)
	case "version", "--version", "-v":
		fmt.Printf("weave %s\n", version)
		return nil
	case "verbs":
		return cmdVerbs(rest)
	case "docs":
		return cmdDocs(rest)
	case "help", "--help", "-h":
		help(os.Stdout)
		return nil
	default:
		// `weave file.weave` is `weave run file.weave`, since running a
		// program is the thing you want nine times out of ten.
		if looksLikeSource(cmd) {
			return cmdRun(args)
		}
		shortUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", cmd)
	}
}

// looksLikeSource reports whether an argument names a Weave program rather
// than mistyping a command.
func looksLikeSource(arg string) bool {
	if strings.HasPrefix(arg, "-") {
		return false
	}
	if filepath.Ext(arg) == ".weave" {
		return true
	}
	info, err := os.Stat(arg)
	return err == nil && !info.IsDir()
}

// source reads the single .weave file named by args.
func source(args []string, cmd string) (path, src string, err error) {
	return sourceWith(flag.NewFlagSet(cmd, flag.ContinueOnError), args, cmd)
}

// sourceWith is source for commands that take flags of their own.
func sourceWith(fs *flag.FlagSet, args []string, cmd string) (path, src string, err error) {
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return "", "", errReported
	}
	if fs.NArg() != 1 {
		return "", "", fmt.Errorf("usage: weave %s file.weave", cmd)
	}
	path = fs.Arg(0)
	if ext := filepath.Ext(path); ext != ".weave" && ext != "" {
		fmt.Fprintf(os.Stderr, "weave: warning: %s does not end in .weave\n", path)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	return path, string(b), nil
}

func cmdLex(args []string) error {
	path, src, err := source(args, "lex")
	if err != nil {
		return err
	}
	bag := diag.New(path, src)
	toks := lexer.Lex(src, bag)

	var sb strings.Builder
	depth := 0
	for _, t := range toks {
		switch t.Kind {
		case token.Indent:
			depth++
		case token.Dedent:
			depth--
		}
		fmt.Fprintf(&sb, "%-8s %s%s\n", t.Pos.String(), strings.Repeat("  ", max(depth, 0)), t)
	}
	fmt.Print(sb.String())
	return report(bag)
}

func cmdParse(args []string) error {
	path, src, err := source(args, "parse")
	if err != nil {
		return err
	}
	bag := diag.New(path, src)
	file := parser.Parse(src, bag)
	fmt.Println(ast.DumpFile(file))
	return report(bag)
}

func cmdCheck(args []string) error {
	fs := flag.NewFlagSet("check", flag.ContinueOnError)
	showTypes := fs.Bool("types", false, "print the inferred type of each definition")
	path, src, err := sourceWith(fs, args, "check")
	if err != nil {
		return err
	}

	bag := diag.New(path, src)
	file := parser.Parse(src, bag)
	if err := report(bag); err != nil {
		return err
	}

	info := check.File(file, bag)
	if err := report(bag); err != nil {
		return err
	}

	// Code generation catches what the type system cannot: an endless `flow`
	// with nothing to stop it, a verb the runtime has no implementation for.
	// Running it here is what makes `weave check` mean "this compiles".
	codegen.Generate(file, info, bag, codegen.Options{})
	if err := report(bag); err != nil {
		return err
	}

	if *showTypes || os.Getenv("WEAVE_TYPES") != "" {
		printTypes(file, info)
	}
	st := style.For(os.Stdout)
	if file.Output() == nil {
		fmt.Fprintf(os.Stderr, "%s %s has no output expression, so it prints nothing\n",
			style.For(os.Stderr).Yellow("warning:"), path)
	}
	fmt.Printf("%s %s\n", st.Green("ok"), st.Dim(path))
	return nil
}

// printTypes lists each definition's inferred type, in source order.
func printTypes(file *ast.File, info *check.Info) {
	st := style.For(os.Stdout)
	for _, d := range file.Decls {
		if sch, ok := info.Decls[d.Name]; ok {
			fmt.Printf("%s %s\n", st.Bold(d.Name), st.Cyan(":: "+types.SchemeString(sch)))
		}
	}
	if info.Output != nil {
		fmt.Printf("%s %s\n", st.Dim("(output)"), st.Cyan(":: "+types.String(info.Output)))
	}
}

// cmdFmt prints a program in canonical form. There are no options beyond where
// the result goes: one program, one layout.
func cmdFmt(args []string) error {
	fs := flag.NewFlagSet("fmt", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	write := fs.Bool("w", false, "rewrite the file in place")
	check := fs.Bool("check", false, "exit non-zero if the file is not formatted")
	terse := fs.Bool("terse", false, "print `=`, `:` and `|` rather than `is`, `gives` and `through`")
	if err := fs.Parse(args); err != nil {
		return errReported
	}
	style := format.Wordy
	if *terse {
		style = format.Terse
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: weave fmt [-w] [-check] file.weave... (or `-` for stdin)")
	}

	changed := false
	for _, path := range fs.Args() {
		// `-` formats standard input to standard output, so an editor can
		// format a buffer it has not written to disk.
		if path == "-" {
			src, err := io.ReadAll(os.Stdin)
			if err != nil {
				return err
			}
			out, err := format.SourceStyle(string(src), "<stdin>", style)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return errReported
			}
			if *check && out != string(src) {
				fmt.Fprintln(os.Stderr, "stdin is not formatted")
				changed = true
				continue
			}
			if !*check {
				fmt.Print(out)
			}
			continue
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out, err := format.SourceStyle(string(src), path, style)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return errReported
		}
		switch {
		case *check:
			if out != string(src) {
				fmt.Fprintf(os.Stderr, "%s is not formatted\n", path)
				changed = true
			}
		case *write:
			if out == string(src) {
				continue
			}
			if err := os.WriteFile(path, []byte(out), 0o644); err != nil {
				return err
			}
			fmt.Println(path)
		default:
			fmt.Print(out)
		}
	}
	if changed {
		return errReported
	}
	return nil
}

func cmdBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	opts := buildFlags(fs)
	path, src, err := sourceWith(fs, args, "build")
	if err != nil {
		return err
	}

	bag := diag.New(path, src)
	res, err := build.Compile(path, src, *opts, bag)
	if err != nil {
		if !bag.Empty() {
			return report(bag)
		}
		return err
	}
	fmt.Println(res.Executable)
	return nil
}

func cmdRun(args []string) error {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	opts := buildFlags(fs)
	path, src, err := sourceWith(fs, args, "run")
	if err != nil {
		return err
	}

	work, err := os.MkdirTemp("", "weave-run-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	opts.Output = filepath.Join(work, "program")

	bag := diag.New(path, src)
	res, err := build.Compile(path, src, *opts, bag)
	if err != nil {
		if !bag.Empty() {
			return report(bag)
		}
		return err
	}

	// The program reads Source from stdin and writes its output expression to
	// stdout, so it inherits this process's streams directly.
	cmd := exec.Command(res.Executable)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		return err
	}
	return nil
}

// cmdTrace compiles the program so that it reports every top-level
// definition's value rather than its output expression, then runs it. One
// tab-separated record per definition:
//
//	LINE<TAB>NAME<TAB>VALUE
//
// A definition with arguments has no value, so its inferred type is reported
// instead; the output expression is reported with an empty name. This is what
// the editor plugin shows as ghost text, and it is deliberately dull to parse.
//
// With -timeout, a definition that will not finish reports the hourglass
// instead of a value and the rest of the file is traced without it. See
// traceUnderLimit.
func cmdTrace(args []string) error {
	fs := flag.NewFlagSet("trace", flag.ContinueOnError)
	opts := buildFlags(fs)
	// The program is run once and thrown away, so the C compiler's own
	// optimiser is time that buys nothing.
	opts.Opt = "-O0"
	opts.Trace = true
	limit := fs.Duration("timeout", 0,
		"give up on a definition that runs longer than this and trace the rest without it")
	memory := fs.Int64("memory", 6144,
		"give up on a definition that wants more than this many megabytes; 0 for no ceiling")
	fs.StringVar(&opts.Watch, "watch", "",
		"also record what this function's names held on each call")
	path, src, err := sourceWith(fs, args, "trace")
	if err != nil {
		return err
	}

	work, err := os.MkdirTemp("", "weave-trace-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	opts.Output = filepath.Join(work, "program")

	// A file being edited does not compile most of the time. Reporting nothing
	// for that second would blink the editor's ghost text out on every
	// keystroke — exactly when the values are being looked at — so the
	// definitions the mistake did not reach are traced anyway. See
	// internal/build/salvage.go.
	kept, dropped := build.Salvage(path, src)

	bag := diag.New(path, kept)
	res, err := build.Compile(path, kept, *opts, bag)
	if err != nil {
		if !bag.Empty() {
			// Report against the file as written, not the salvaged copy, so
			// the caret lands where the mistake is.
			whole := diag.New(path, src)
			build.Compile(path, src, *opts, whole)
			if !whole.Empty() {
				return report(whole)
			}
			return report(bag)
		}
		return err
	}
	if dropped > 0 {
		// On stderr, so a plugin reading records on stdout is unaffected and a
		// person running this by hand still learns why some lines are quiet.
		fmt.Fprintf(os.Stderr, "weave trace: %d definition(s) left out; the rest is below\n", dropped)
	}

	if *limit > 0 || *memory > 0 {
		return traceUnderLimit(path, kept, *opts, res.Executable, *limit, *memory<<20)
	}

	cmd := exec.Command(res.Executable)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		return err
	}
	return nil
}

// What a line that was cut short reports instead of a value: the hourglass for
// one that ran out of time, and the slashed circle for one that asked for more
// memory than it was going to get.
//
// Neither is an emoji. An editor that renders one double-width pushes every
// other line's ghost text out of alignment, which rules out the stopwatch, the
// alarm clock and everything in that block.
const (
	traceTimedOut   = "⧖"
	traceOverMemory = "⊘"
)

// traceTurns is how many times the program may be run. The first go plus three
// more: a file with more slow definitions than that is one where the answer is
// to look at the program, not to spend the afternoon waiting on it.
const traceTurns = 4

// traceUnderLimit runs the traced program under limits, and keeps going when
// one of them stops it. Everything that reported is kept, the first item that
// did not gets the mark saying why, and then — exactly as Salvage does with the
// item an error is in — that item is blanked out of the source and the program
// is compiled and run again, so the lines below it report too.
//
// Time and memory are the same problem and get the same answer. A file being
// edited is full of definitions that are half written, and a half-written
// definition is as likely to ask for every byte in the machine as it is to loop
// for ever. Neither should cost more than its own line's ghost text: the tracer
// runs a program nobody asked to run.
//
// Each turn re-runs the lines above the blanked item, which is work already
// done; they reported inside the limits the first time, so it is cheap work,
// and it buys a program that needs no way of being resumed part way through.
func traceUnderLimit(path, src string, opts build.Options, exe string, limit time.Duration, memory int64) error {
	// The program is run more than once and reads Source from stdin, so stdin
	// has to be held rather than handed over.
	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	var out strings.Builder
	reported := map[int]bool{}
	after := 0
	for turn := 0; ; turn++ {
		stdout, cut, err := runFor(exe, in, limit, memory)
		if err != nil {
			return err
		}
		// A record for a line already reported is the same record, from the
		// same program on the same input. The first one is the one to keep.
		var seen []int
		for _, rec := range strings.Split(strings.TrimSuffix(stdout, "\n"), "\n") {
			if strings.HasPrefix(rec, "@") {
				// A watched call's records carry their own call number and
				// belong to no line, so there is nothing to keep them in step
				// with. Only the first turn's are kept; a later turn would
				// repeat the same calls under the same numbers.
				if turn == 0 {
					out.WriteString(rec)
					out.WriteByte('\n')
				}
				continue
			}
			line, ok := recordLine(rec)
			if !ok {
				continue
			}
			seen = append(seen, line)
			if !reported[line] {
				out.WriteString(rec)
				out.WriteByte('\n')
			}
		}
		// Marked after the whole run, so a line that legitimately reports twice
		// is not silenced the second time by its own first record.
		for _, line := range seen {
			reported[line] = true
		}

		if cut == "" {
			break
		}
		item, found := build.Unreported(build.Items(src), reported, after)
		if !found {
			// The program was cut short with every item accounted for, so there
			// is no line to blame and nothing a further turn would add.
			break
		}
		fmt.Fprintf(&out, "%d\t%s\t%s\n", item.Line, item.Name, cut)
		after = item.Line
		if turn+1 >= traceTurns {
			break
		}

		src = build.Blank(src, item.Line)
		// Whatever needed the blanked item cannot compile without it, and
		// Salvage is what takes those out.
		trimmed, _ := build.Salvage(path, src)
		bag := diag.New(path, trimmed)
		res, err := build.Compile(path, trimmed, opts, bag)
		if err != nil {
			// Nothing left that compiles. What has reported still stands.
			break
		}
		src, exe = trimmed, res.Executable
	}

	fmt.Print(out.String())
	return nil
}

// recordLine reads the line number off a trace record, and reports whether the
// text was one.
func recordLine(rec string) (int, bool) {
	tab := strings.IndexByte(rec, '\t')
	if tab <= 0 {
		return 0, false
	}
	line, err := strconv.Atoi(rec[:tab])
	if err != nil || line <= 0 {
		return 0, false
	}
	return line, true
}

// runFor runs the traced program with the given Source, and reports what it
// printed along with the mark for whatever cut it short — empty when nothing
// did. The runtime flushes every record as it writes it, so what arrived before
// the axe fell is what comes back.
//
// The two ceilings are enforced from different sides, because they have to be.
// Time is the tracer's to keep: a program that will not finish cannot be asked
// to notice. Memory is the program's own, since only it knows what it has
// taken — every byte it gets from the operating system goes through one place
// in the runtime, and that place counts. It comes in through the environment so
// that a traced program and a run one are the same program.
func runFor(exe string, in []byte, limit time.Duration, memory int64) (string, string, error) {
	ctx := context.Background()
	if limit > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, limit)
		defer cancel()
	}

	var stdout strings.Builder
	cmd := exec.CommandContext(ctx, exe)
	cmd.Stdin = bytes.NewReader(in)
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	if memory > 0 {
		cmd.Env = append(os.Environ(), fmt.Sprintf("WEAVE_MEM_CAP=%d", memory))
	}
	// Without this the wait sits on the pipe until whatever the killed program
	// left behind closes it.
	cmd.WaitDelay = time.Second

	err := cmd.Run()
	if ctx.Err() != nil {
		return stdout.String(), traceTimedOut, nil
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if exit.ExitCode() == wOverMemory {
				return stdout.String(), traceOverMemory, nil
			}
			// A program that stopped for a reason of its own has said why on
			// stderr, and what it reported before stopping is still worth
			// showing. It is not a line anybody can mark.
			return stdout.String(), "", nil
		}
		return "", "", err
	}
	return stdout.String(), "", nil
}

// wOverMemory is W_EXIT_OVER_MEMORY from the runtime: what a program exits with
// when it has gone past WEAVE_MEM_CAP. Kept in step by a test.
const wOverMemory = 9

// defaultCacheDir is where the compiled runtime is kept between runs. The
// cache is keyed by the runtime's contents and the flags it was built with, so
// a stale entry is impossible; WEAVE_CACHE overrides the location and an empty
// value turns caching off.
func defaultCacheDir() string {
	if dir, set := os.LookupEnv("WEAVE_CACHE"); set {
		return dir
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return ""
	}
	return filepath.Join(base, "weave")
}

// buildFlags registers the flags shared by build, run and repl.
func buildFlags(fs *flag.FlagSet) *build.Options {
	opts := &build.Options{CacheDir: defaultCacheDir()}
	fs.StringVar(&opts.Output, "o", "", "write the executable here")
	fs.StringVar(&opts.CC, "cc", "", "C compiler to use (default: clang)")
	fs.StringVar(&opts.Opt, "opt", "-O3", "optimisation level passed to the C compiler")
	fs.BoolVar(&opts.KeepC, "keep-c", false, "keep the generated C")
	fs.BoolVar(&opts.Verbose, "v", false, "print the C compiler command")
	fs.BoolVar(&opts.DisableFusion, "no-fuse", false,
		"compile Thread chains one runtime call per stage")
	fs.BoolVar(&opts.DisableSpecialize, "no-specialize", false,
		"use the general prelude verbs instead of typed primitive helpers")
	fs.BoolVar(&opts.CheckOverflow, "overflow", false,
		"stop the program when Earth arithmetic overflows, instead of wrapping")
	fs.BoolVar(&opts.DisableRegions, "no-regions", false,
		"keep a fused loop turn's storage instead of handing it back when the turn ends")
	fs.BoolVar(&opts.DisableInPlace, "no-in-place", false,
		"copy on every grid update instead of writing through when unshared")
	fs.BoolVar(&opts.DisableRelease, "no-release", false,
		"keep every Thread a function builds instead of freeing the dead ones")
	fs.BoolVar(&opts.Tally, "tally", false,
		"report, on exit, what was holding the heap at its largest and where it came from")
	return opts
}

// report prints any diagnostics and returns errReported if there were errors.
func report(bag *diag.Bag) error {
	if bag.Empty() {
		return nil
	}
	st := style.For(os.Stderr)
	fmt.Fprintln(os.Stderr, bag.Rendered(st))
	n := bag.Len()
	fmt.Fprintf(os.Stderr, "\n%s\n", st.Red(fmt.Sprintf("%d error%s", n, plural(n))))
	return errReported
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// cmdVerbs prints the built-in vocabulary. With an argument it filters by name,
// type or description, so `weave verbs Knot` answers "what works on
// coordinates" and `weave verbs Web` answers it for maps.
// cmdDocs serves the reference on localhost, or writes it out as one file.
//
// It is served rather than opened from disk so that a browser left on the page
// picks up a rebuilt compiler by reloading, which is what makes it worth
// running while working on the prelude.
func cmdDocs(args []string) error {
	fs := flag.NewFlagSet("docs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	port := fs.Int("port", 7373, "port to serve on; 0 chooses a free one")
	out := fs.String("o", "", "write the page to this file instead of serving it")
	if err := fs.Parse(args); err != nil {
		return errReported
	}

	if *out != "" {
		page, err := docs.Render()
		if err != nil {
			return err
		}
		return os.WriteFile(*out, []byte(page), 0o644)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", *port))
	if err != nil {
		return err
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		// Rendered per request, so a reload is enough to see a verb that was
		// added since the page was opened.
		page, err := docs.Render()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, page)
	})

	st := style.For(os.Stdout)
	fmt.Printf("%s http://%s\n", st.Accent("the reference is at"), ln.Addr())
	fmt.Printf("%s\n", st.Dim("press ctrl-c to stop"))
	return http.Serve(ln, mux)
}

func cmdVerbs(args []string) error {
	fs := flag.NewFlagSet("verbs", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	asMarkdown := fs.Bool("md", false, "write the reference as Markdown")
	if err := fs.Parse(args); err != nil {
		return errReported
	}
	query := strings.Join(fs.Args(), " ")

	if *asMarkdown {
		fmt.Print(prelude.Markdown(referenceTitle, referencePreamble))
		return nil
	}

	st := style.For(os.Stdout)
	shown := 0
	for _, g := range prelude.Groups() {
		var lines []prelude.Entry
		for _, e := range g.Entries {
			if prelude.Matches(e, query) {
				lines = append(lines, e)
			}
		}
		if len(lines) == 0 {
			continue
		}
		fmt.Printf("\n%s\n", st.Accent(g.Name))
		for _, e := range lines {
			shown++
			fmt.Printf("  %s %s\n    %s\n",
				st.Bold(e.Name),
				st.Cyan(":: "+withWhere(e)),
				st.Dim(e.Doc))
		}
	}
	if shown == 0 {
		fmt.Printf("no built-in matches %q\n", query)
		return errReported
	}
	fmt.Printf("\n%s\n", st.Dim(fmt.Sprintf("%d of %d built-ins", shown, len(prelude.Values))))
	return nil
}

func withWhere(e prelude.Entry) string {
	if e.Where == "" {
		return e.Sig
	}
	return e.Sig + "  where " + e.Where
}

const referenceTitle = "The Weave vocabulary"

const referencePreamble = `Every built-in, with its type. This file is generated — ` +
	"`weave verbs -md`" + ` — from
` + "`internal/prelude/prelude.go`" + `, which the compiler parses at start-up, so the
signatures here are the signatures the type checker uses and cannot drift from
them.

At a terminal, ` + "`weave verbs`" + ` prints the same thing, and ` + "`weave verbs Knot`" + ` filters
it — by name, by type, or by description, so searching for a type answers
"what works on one of these".

Two conventions run through the whole table, and the difference is deliberate.
**Sequence transforms are data-last**, so partial application composes with the
pipeline: ` + "`sift even`" + ` is a function still waiting for its Thread. **Keyed
collections take the collection first**, because a grid or a map is usually the
fixed thing being consulted rather than the thing flowing through. ` + "`_`" + ` bridges
the two when you want the second form in a chain: ` + "`w | get _ \"a\"`" + `.`
