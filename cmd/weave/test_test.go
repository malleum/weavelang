package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/build"
	"github.com/malleum/weave/internal/diag"
)

// `weave test` is the loop an Advent of Code day is solved in, so it has to be
// right about which files are a case and what counts as a pass.
func TestCasesFor(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("day.weave", "Source | earths | sum\n")
	write("day.in", "1 2 3\n")
	write("day.out", "6\n")
	write("day.2.in", "10 20\n")
	write("day.2.out", "30\n")
	// An input with no expected output is not a case: it is the real puzzle
	// input, which nobody knows the answer to yet.
	write("day.real.in", "99\n")
	if err := os.MkdirAll(filepath.Join(dir, "testdata"), 0o755); err != nil {
		t.Fatal(err)
	}
	write(filepath.Join("testdata", "day.extra.in"), "7\n")
	write(filepath.Join("testdata", "day.extra.out"), "7\n")

	cases, err := casesFor(filepath.Join(dir, "day.weave"))
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, c := range cases {
		names = append(names, c.name)
	}
	got := strings.Join(names, " ")
	if got != "day day.2 day.extra" {
		t.Errorf("cases are %q, want %q", got, "day day.2 day.extra")
	}
}

func TestRunCaseComparesIgnoringTrailingNewlines(t *testing.T) {
	requireCC(t)
	dir := t.TempDir()
	src := filepath.Join(dir, "p.weave")
	if err := os.WriteFile(src, []byte("Source | earths | sum\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := buildForTest(t, src, dir)

	for _, c := range []struct {
		name string
		in   string
		want string
		pass bool
	}{
		{"exact", "1 2 3", "6", true},
		{"a trailing newline in the fixture", "1 2 3", "6\n", true},
		{"a wrong answer", "1 2 3", "7", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			ok, got, err := runCase(exe, testCase{name: c.name, in: c.in, want: c.want})
			if err != nil {
				t.Fatal(err)
			}
			if ok != c.pass {
				t.Errorf("pass=%v, want %v (got %q)", ok, c.pass, got)
			}
		})
	}
}

// buildForTest compiles a program and returns the executable.
func buildForTest(t *testing.T, src, dir string) string {
	t.Helper()
	text, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	bag := diag.New(src, string(text))
	res, err := build.Compile(src, string(text), build.Options{
		Output: filepath.Join(dir, "program"),
		Opt:    "-O0",
	}, bag)
	if err != nil {
		t.Fatalf("compile failed: %v\n%s", err, bag)
	}
	return res.Executable
}

// oneLine keeps a multi-line answer on one line, so a failure is two lines to
// compare rather than two blocks to scroll between.
func TestOneLine(t *testing.T) {
	if got := oneLine("a\nb\n"); got != "a ⏎ b" {
		t.Errorf("got %q", got)
	}
	if got := oneLine("6\n"); got != "6" {
		t.Errorf("got %q", got)
	}
}
