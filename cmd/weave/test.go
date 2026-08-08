package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/malleum/weave/internal/build"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/style"
)

// `weave test` runs a program against the sample inputs beside it.
//
// Advent of Code hands you a sample and its answer every single day, and the
// loop that follows is always the same: run the program on the sample, squint
// at the number, run it on the real input. Doing that by hand is what a
// terminal is for right up until the moment you change something and forget to
// re-check the sample.
//
// The convention is the one the compiler's own fixtures already use: `day05.in`
// beside `day05.weave` holds the input, `day05.out` holds what the program
// should print. A case may be numbered — `day05.1.in` and `day05.1.out` — so a
// day with two parts, or two samples, is two cases. A `testdata/` directory
// beside the program is searched as well, which is where this repository keeps
// its own.

func cmdTest(args []string) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	opts := buildFlags(fs)
	if err := fs.Parse(args); err != nil {
		return errReported
	}
	if fs.NArg() == 0 {
		return fmt.Errorf("usage: weave test file.weave [file.weave ...]")
	}

	st := style.For(os.Stdout)
	work, err := os.MkdirTemp("", "weave-test-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)

	passed, failed, skipped := 0, 0, 0
	for _, path := range fs.Args() {
		cases, err := casesFor(path)
		if err != nil {
			return err
		}
		if len(cases) == 0 {
			fmt.Printf("%s %s\n", st.Dim("?"), st.Dim(path+": no .in/.out beside it"))
			skipped++
			continue
		}

		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		o := *opts
		o.Output = filepath.Join(work, strings.TrimSuffix(filepath.Base(path), ".weave"))
		bag := diag.New(path, string(src))
		res, err := build.Compile(path, string(src), o, bag)
		if err != nil {
			if !bag.Empty() {
				report(bag)
			}
			fmt.Printf("%s %s\n", st.Red("FAIL"), path+": does not compile")
			failed += len(cases)
			continue
		}

		for _, c := range cases {
			ok, got, err := runCase(res.Executable, c)
			if err != nil {
				return err
			}
			if ok {
				passed++
				fmt.Printf("%s %s\n", st.Green("ok  "), st.Dim(c.name))
				continue
			}
			failed++
			fmt.Printf("%s %s\n", st.Red("FAIL"), c.name)
			fmt.Printf("     %s %s\n", st.Dim("expected"), oneLine(c.want))
			fmt.Printf("     %s %s\n", st.Dim("     got"), oneLine(got))
		}
	}

	summary := fmt.Sprintf("%d passed, %d failed", passed, failed)
	if skipped > 0 {
		summary += fmt.Sprintf(", %d without fixtures", skipped)
	}
	if failed > 0 {
		fmt.Printf("\n%s\n", st.Red(summary))
		return errReported
	}
	fmt.Printf("\n%s\n", st.Green(summary))
	return nil
}

// testCase is one input and the output it should produce.
type testCase struct {
	name string
	in   string
	want string
}

// casesFor finds the fixtures beside a program: `day05.in`/`day05.out`, and
// any numbered pair such as `day05.1.in`/`day05.1.out`.
func casesFor(path string) ([]testCase, error) {
	base := strings.TrimSuffix(filepath.Base(path), ".weave")
	stems := []string{
		filepath.Join(filepath.Dir(path), base),
		filepath.Join(filepath.Dir(path), "testdata", base),
		filepath.Join(filepath.Dir(path), "..", "testdata", base),
	}
	var ins []string
	for _, stem := range stems {
		found, err := filepath.Glob(stem + "*.in")
		if err != nil {
			return nil, err
		}
		ins = append(ins, found...)
	}
	var out []testCase
	for _, in := range ins {
		want := strings.TrimSuffix(in, ".in") + ".out"
		wantText, err := os.ReadFile(want)
		if err != nil {
			continue // an input with no expected output is not a case
		}
		inText, err := os.ReadFile(in)
		if err != nil {
			return nil, err
		}
		out = append(out, testCase{
			name: filepath.Base(in[:len(in)-len(".in")]),
			in:   string(inText),
			want: string(wantText),
		})
	}
	// By case name rather than by file name, so the plain `day` comes before
	// `day.2` — sorting the paths would put `day.2.in` first.
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}

// runCase feeds one input to the program and compares what it printed.
// Trailing newlines are not part of the comparison: whether a fixture file ends
// in one is an accident of how it was saved.
func runCase(exe string, c testCase) (bool, string, error) {
	cmd := exec.Command(exe)
	cmd.Stdin = strings.NewReader(c.in)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return false, "", err
		}
		// The program stopped: whatever it said on the way out is the answer
		// worth showing.
		return false, strings.TrimSpace(errOut.String()), nil
	}
	got := strings.TrimRight(out.String(), "\n")
	return got == strings.TrimRight(c.want, "\n"), got, nil
}

// oneLine renders a multi-line answer on one line, so a failure is a pair of
// lines to compare rather than two blocks to scroll between.
func oneLine(s string) string {
	s = strings.TrimRight(s, "\n")
	if !strings.Contains(s, "\n") {
		return s
	}
	return strings.ReplaceAll(s, "\n", " ⏎ ")
}
