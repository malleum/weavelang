package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/rt"
)

// cacheIn points the runtime cache at a directory of this test's own. These
// tests go through cmdTrace, which picks the cache up from the environment the
// way a person running the command would — and a build sandbox with no writable
// home has nowhere to put it. See defaultCacheDir.
func cacheIn(t *testing.T) {
	t.Helper()
	t.Setenv("WEAVE_CACHE", t.TempDir())
}

// A definition that will not finish used to keep the whole file quiet: the
// editor killed `weave trace` and threw away everything it had said. Now the
// slow one reports the hourglass and the rest of the file reports normally,
// which is the same bargain Salvage makes with a line that will not compile.
func TestTraceMarksTheLineThatRanOutOfTime(t *testing.T) {
	requireCC(t)
	cacheIn(t)

	src := strings.Join([]string{
		"quick is 1",
		"",
		// Slow and lean, so it is the clock that stops it and not the ceiling
		// below: an endless producer is compiled whole even in trace mode, so
		// this stays one fused loop holding one element at a time.
		"slow is cycle [1 2 3] through take 900000000 through sum",
		"",
		"after is add quick 2",
		"",
		"slow through inc",
		"",
		"after through inc",
		"",
	}, "\n")

	path := filepath.Join(t.TempDir(), "slow.weave")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := withStdio(t, "", func() error {
		return cmdTrace([]string{"-timeout", "2s", path})
	})
	if err != nil {
		t.Fatalf("trace failed: %v\n%s", err, out)
	}

	// The hourglass lands on the definition that ran out of time...
	if !strings.Contains(out, "3\tslow\t"+traceTimedOut) {
		t.Errorf("the slow definition was not marked:\n%s", out)
	}
	// ...the lines before it are untouched...
	if !strings.Contains(out, "1\tquick\t1") {
		t.Errorf("a line before the slow one stopped reporting:\n%s", out)
	}
	// ...it is the clock it ran out of, not the memory ceiling...
	if strings.Contains(out, traceOverMemory) {
		t.Errorf("running out of time was reported as running out of memory:\n%s", out)
	}
	// ...and, the point of the whole exercise, so are the lines after it.
	if !strings.Contains(out, "5\tafter\t3") || !strings.Contains(out, "9\t\t4") {
		t.Errorf("the lines after the slow one did not report:\n%s", out)
	}
	// The line that needed the slow definition cannot report a value, exactly
	// as it could not if the definition had failed to compile.
	if strings.Contains(out, "\n7\t") {
		t.Errorf("a line depending on the slow definition reported anyway:\n%s", out)
	}
}

// The limit only applies to a program that needs it. One that finishes reports
// every line as it always did, and pays for nothing.
func TestTraceUnderALimitLeavesAQuickProgramAlone(t *testing.T) {
	requireCC(t)
	cacheIn(t)

	src := "nums is [3 1 4]\n\ntotal is nums through sum\n\ntotal through inc\n"
	path := filepath.Join(t.TempDir(), "quick.weave")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := withStdio(t, "", func() error {
		return cmdTrace([]string{"-timeout", "30s", path})
	})
	if err != nil {
		t.Fatalf("trace failed: %v\n%s", err, out)
	}
	for _, want := range []string{"1\tnums\t[3 1 4]", "3\ttotal\t8", "5\t\t9"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in the records:\n%s", want, out)
		}
	}
	if strings.Contains(out, traceTimedOut) {
		t.Errorf("a program that finished was marked as timing out:\n%s", out)
	}
}

// A definition that means to allocate the whole machine is the same problem as
// one that means to run for ever, and gets the same answer: it costs its own
// line's ghost text and nothing else. Tracing runs a program nobody asked to
// run, on a file that is being typed into, and a half-written definition is as
// likely to do one as the other.
func TestTraceMarksTheLineThatWantedTooMuchMemory(t *testing.T) {
	requireCC(t)
	cacheIn(t)

	src := strings.Join([]string{
		"quick is 1",
		"",
		"hog is span 1 400000000 through bend inc through len",
		"",
		"after is add quick 2",
		"",
		"after through inc",
		"",
	}, "\n")

	path := filepath.Join(t.TempDir(), "hog.weave")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := withStdio(t, "", func() error {
		return cmdTrace([]string{"-memory", "256", path})
	})
	if err != nil {
		t.Fatalf("trace failed: %v\n%s", err, out)
	}

	if !strings.Contains(out, "3\thog\t"+traceOverMemory) {
		t.Errorf("the greedy definition was not marked:\n%s", out)
	}
	if strings.Contains(out, traceTimedOut) {
		t.Errorf("running out of memory was reported as running out of time:\n%s", out)
	}
	if !strings.Contains(out, "1\tquick\t1") || !strings.Contains(out, "5\tafter\t3") ||
		!strings.Contains(out, "7\t\t4") {
		t.Errorf("the other lines did not report:\n%s", out)
	}
}

// The ceiling is the program's own to keep — only it knows what it has taken —
// so the tracer learns about it from an exit code. The two ends of that have to
// agree, and they are written in different languages.
func TestTheOverMemoryExitCodeMatchesTheRuntime(t *testing.T) {
	header, ok := rt.Files()["weave.h"]
	if !ok {
		t.Fatal("the runtime has no weave.h")
	}
	want := fmt.Sprintf("#define W_EXIT_OVER_MEMORY %d", wOverMemory)
	if !strings.Contains(string(header), want) {
		t.Errorf("weave.h does not say %q, so the tracer would read an exit as an ordinary failure", want)
	}
}

// Ghost text answers "what does this line hold", and a line inside a function
// body holds a different thing on every call — so the inside of a recursion is
// the one place in a program with no ghost text. -watch records the calls
// instead: what each of the function's names held, on each call.
func TestTraceWatchesAFunctionsCalls(t *testing.T) {
	requireCC(t)
	cacheIn(t)

	// A `gentle` is the shape this is for, and it is the awkward one: the step
	// is lifted out to a name, and a name used as a fold's step is normally
	// inlined into the loop — where it would be exactly as invisible as it was
	// before anybody asked to watch it.
	src := strings.Join([]string{
		"step (seen, n) c is",
		"  weave next is add n n",
		"  pick (lt 5 seen) (Woven (add seen 1, next)) (Gentled n)",
		"",
		"gentle step (0, 1) (flow inc 1) failing 0",
		"",
	}, "\n")

	path := filepath.Join(t.TempDir(), "walk.weave")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := withStdio(t, "", func() error {
		return cmdTrace([]string{"-watch", "step", path})
	})
	if err != nil {
		t.Fatalf("trace failed: %v\n%s", err, out)
	}

	// A parameter, a `weave` binding and what the call answered, for the first
	// call and then the second, each under its own call number.
	for _, want := range []string{
		"@1\t1\tseen\t0",
		"@1\t1\tn\t1",
		"@2\t1\tnext\t2",
		"@1\t2\tseen\t1",
		"@2\t2\tnext\t4",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("expected the record %q:\n%s", want, out)
		}
	}
	// The count, which is an answer in its own right.
	if !strings.Contains(out, "@0\t0\tcalls\t") {
		t.Errorf("no count of the calls:\n%s", out)
	}
	// And the by-line records are still there, unchanged, beside them.
	if !strings.Contains(out, "5\t\t") {
		t.Errorf("watching a function stopped the ordinary records:\n%s", out)
	}
}

// Watching is opt-in per function, because recording per call costs the fusion
// inside the body. Nothing else in the file changes shape.
func TestTraceWithoutWatchRecordsNoCalls(t *testing.T) {
	requireCC(t)
	cacheIn(t)

	src := "double n is mul n 2\n\ndouble 21\n"
	path := filepath.Join(t.TempDir(), "plain.weave")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := withStdio(t, "", func() error { return cmdTrace([]string{path}) })
	if err != nil {
		t.Fatalf("trace failed: %v\n%s", err, out)
	}
	if strings.Contains(out, "@") {
		t.Errorf("a call record turned up without -watch:\n%s", out)
	}
	if !strings.Contains(out, "3\t\t42") {
		t.Errorf("the ordinary records are missing:\n%s", out)
	}
}

// The ghost text a recursion can have without asking for anything: a binding
// inside a function body reports the first value it ever holds. It is one value
// where there are many, and the first is the one you can reason about — it is
// the call you would have made by hand.
func TestTraceReportsTheFirstValueInsideAFunction(t *testing.T) {
	requireCC(t)
	cacheIn(t)

	src := strings.Join([]string{
		"fact n is",                              // 1
		"  weave lower is dec n",                 // 2
		"  pick (lt 2 n) 1 (mul n (fact lower))", // 3
		"",                                       // 4
		"fact 6",                                 // 5
		"",
	}, "\n")

	path := filepath.Join(t.TempDir(), "fact.weave")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := withStdio(t, "", func() error { return cmdTrace([]string{path}) })
	if err != nil {
		t.Fatalf("trace failed: %v\n%s", err, out)
	}

	// The first call's value, not the last: `fact 6` binds 5 before it binds 0.
	if !strings.Contains(out, "2\tlower\t5") {
		t.Errorf("the first value inside the function was not reported:\n%s", out)
	}
	if strings.Contains(out, "2\tlower\t0") {
		t.Errorf("a later call overwrote the first:\n%s", out)
	}
	// The definition still reports its type, and the program its answer.
	if !strings.Contains(out, "1\tfact\tEarth -> Earth") || !strings.Contains(out, "5\t\t720") {
		t.Errorf("the ordinary records changed:\n%s", out)
	}
}

// A `gentle` step lifted out to a name is inlined into the fused loop, which is
// where a body is hardest to see — so it has to report from in there too. The
// records carry the definition's own lines, not the call site's.
func TestTraceReportsInsideAnInlinedStep(t *testing.T) {
	requireCC(t)
	cacheIn(t)

	src := strings.Join([]string{
		"step (seen, n) c is",     // 1
		"  weave next is add n n", // 2
		"  pick (lt 5 seen) (Woven (add seen 1, next)) (Gentled n)", // 3
		"", // 4
		"gentle step (0, 1) (flow inc 1) failing 0", // 5
		"",
	}, "\n")

	path := filepath.Join(t.TempDir(), "walk.weave")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := withStdio(t, "", func() error { return cmdTrace([]string{path}) })
	if err != nil {
		t.Fatalf("trace failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "2\tnext\t2") {
		t.Errorf("an inlined body reported nothing:\n%s", out)
	}
}

// A lambda written out on the spot is a different case: its lines are the
// call's lines, and the chain already reports those. Reporting again would put
// two answers on one line, and the wrong one first.
func TestTraceLeavesAnInlineLambdaAlone(t *testing.T) {
	requireCC(t)
	cacheIn(t)

	src := "xs is [1 2 3]\n\ndoubled is xs through bend (n : mul n 2)\n\ndoubled through sum\n"
	path := filepath.Join(t.TempDir(), "inline.weave")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := withStdio(t, "", func() error { return cmdTrace([]string{path}) })
	if err != nil {
		t.Fatalf("trace failed: %v\n%s", err, out)
	}
	want := "1\txs\t[1 2 3]\n3\tdoubled\t[2 4 6]\n5\t\t12\n"
	if out != want {
		t.Errorf("the records changed:\ngot:\n%s\nwant:\n%s", out, want)
	}
}
