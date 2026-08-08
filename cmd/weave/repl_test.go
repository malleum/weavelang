package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/build"
)

// session drives the REPL with a scripted set of lines and returns everything
// it printed.
func session(t *testing.T, lines ...string) string {
	t.Helper()
	requireCC(t)

	work := t.TempDir()
	var out bytes.Buffer
	in := strings.NewReader(strings.Join(append(lines, ":quit"), "\n") + "\n")

	opts := build.Options{
		Opt:      "-O0",
		CacheDir: filepath.Join(work, "cache"),
	}
	if err := runREPL(in, &out, opts, work, ""); err != nil {
		t.Fatalf("repl failed: %v", err)
	}
	return out.String()
}

func requireCC(t *testing.T) {
	t.Helper()
	for _, cc := range []string{"clang", "cc", "gcc"} {
		if _, err := exec.LookPath(cc); err == nil {
			return
		}
	}
	t.Skip("no C compiler available")
}

func TestREPLEvaluatesExpressions(t *testing.T) {
	out := session(t, "add 1 2", `join "-" ["a" "b"]`, "[1 2 3] | sum")
	for _, want := range []string{"3", "a-b", "6"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in the session:\n%s", want, out)
		}
	}
}

func TestREPLKeepsDefinitions(t *testing.T) {
	out := session(t, "double n is mul n 2", "double 21", "span 1 4 | bend double | sum")
	if !strings.Contains(out, "double :: Earth -> Earth") {
		t.Errorf("expected the definition's type to be reported:\n%s", out)
	}
	if !strings.Contains(out, "42") {
		t.Errorf("expected the definition to be usable:\n%s", out)
	}
	if !strings.Contains(out, "20") {
		t.Errorf("expected the definition to work in a pipeline:\n%s", out)
	}
}

func TestREPLReportsErrorsAndCarriesOn(t *testing.T) {
	out := session(t, "add 1 \"two\"", "add 1 2")
	if !strings.Contains(out, "Reckon Talent") {
		t.Errorf("expected a type error:\n%s", out)
	}
	if !strings.Contains(out, "3") {
		t.Errorf("the session should continue after an error:\n%s", out)
	}
}

func TestREPLRejectsABadDefinitionWithoutKeepingIt(t *testing.T) {
	// The broken definition must not poison every later line.
	out := session(t, "bad n is add n \"x\"", "add 1 2")
	if !strings.Contains(out, "3") {
		t.Errorf("a rejected definition should not break the session:\n%s", out)
	}
}

func TestREPLMultiLineDefinition(t *testing.T) {
	out := session(t,
		"classify n is",
		"  ward n",
		"    0 : \"zero\"",
		"    _ : \"other\"",
		"", // a blank line ends the block
		"classify 0",
	)
	if !strings.Contains(out, "zero") {
		t.Errorf("expected the multi-line definition to work:\n%s", out)
	}
}

func TestREPLTypeCommand(t *testing.T) {
	out := session(t, ":type bend", "double n is mul n 2", ":type double")
	if !strings.Contains(out, "(a -> b) -> Thread a -> Thread b") {
		t.Errorf("expected a builtin's type:\n%s", out)
	}
	if !strings.Contains(out, "double :: Earth -> Earth") {
		t.Errorf("expected a definition's type:\n%s", out)
	}
}

func TestREPLListDropAndClear(t *testing.T) {
	out := session(t, "a is 1", "b is 2", ":list", ":drop", ":list", ":clear", ":list")
	if !strings.Contains(out, "dropped the last definition") {
		t.Errorf("expected :drop to report:\n%s", out)
	}
	if !strings.Contains(out, "no definitions yet") {
		t.Errorf("expected :clear to empty the session:\n%s", out)
	}
}

func TestREPLSourceCommand(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "in.txt")
	if err := os.WriteFile(input, []byte("10\n20\n30\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := session(t,
		":source "+input,
		"Source | lines | bend earth | bend (otherwise 0) | sum",
	)
	if !strings.Contains(out, "60") {
		t.Errorf("expected Source to come from the file:\n%s", out)
	}
}

func TestREPLUnknownCommand(t *testing.T) {
	out := session(t, ":nope")
	if !strings.Contains(out, "unknown command") {
		t.Errorf("expected a complaint:\n%s", out)
	}
}

// TestLooksLikeSource covers the `weave file.weave` shorthand's guard: a
// mistyped command should still produce a usage message, not a file error.
func TestLooksLikeSource(t *testing.T) {
	dir := t.TempDir()
	prog := filepath.Join(dir, "p.weave")
	if err := os.WriteFile(prog, []byte("1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if !looksLikeSource(prog) {
		t.Error("a .weave file should be recognised")
	}
	if looksLikeSource("buidl") {
		t.Error("a mistyped command should not be treated as a file")
	}
	if looksLikeSource("-o") {
		t.Error("a flag should not be treated as a file")
	}
	if looksLikeSource(dir) {
		t.Error("a directory should not be treated as a file")
	}
}
