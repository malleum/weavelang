package build_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/build"
	"github.com/malleum/weave/internal/diag"
)

// tallied compiles a program with the arena's bookkeeping turned on and returns
// what it printed and what the tally reported.
func tallied(t *testing.T, name, src, input string) (out, tally string) {
	t.Helper()
	requireCC(t)

	dir := t.TempDir()
	bag := diag.New(name, src)
	res, err := build.Compile(name, src, build.Options{
		Output: filepath.Join(dir, "program"),
		Tally:  true,
	}, bag)
	if err != nil {
		if !bag.Empty() {
			t.Fatalf("compiling %s failed:\n%s", name, bag)
		}
		t.Fatalf("compiling %s failed: %v", name, err)
	}

	cmd := exec.Command(res.Executable)
	cmd.Stdin = strings.NewReader(input)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("running %s failed: %v\nstderr: %s", name, err, stderr.String())
	}
	return stdout.String(), stderr.String()
}

// The tally is a debugging build, and a debugging build that changes the answer
// is worse than none at all: every measurement taken with it would be of a
// different program. The report goes to stderr precisely so that stdout is
// still the program's own output and nothing else.
func TestTallyDoesNotChangeTheProgram(t *testing.T) {
	const src = `grid is span 1 200 | bend (n : (n, mul n n)) | web

total is items grid | bend ((k, v) : add k v) | sum

[total, len (keys grid)] | bend air | join " "
`
	want := compileAndRun(t, "tally.weave", src, "")
	got, report := tallied(t, "tally.weave", src, "")
	if got != want {
		t.Errorf("a tallied build printed %q, an ordinary one %q", got, want)
	}
	if !strings.Contains(report, "high-water mark") {
		t.Errorf("no tally on stderr:\n%s", report)
	}
}

// An ordinary build must not carry any of it. The flag exists so that the cost
// — a hash insert and a hash delete around every allocation — is only paid when
// it is asked for.
func TestTallyIsSilentUnlessAskedFor(t *testing.T) {
	const src = "span 1 10 | sum\n"
	requireCC(t)

	dir := t.TempDir()
	bag := diag.New("quiet.weave", src)
	res, err := build.Compile("quiet.weave", src, build.Options{
		Output: filepath.Join(dir, "program"),
	}, bag)
	if err != nil {
		t.Fatalf("compiling failed: %v\n%s", err, bag)
	}
	cmd := exec.Command(res.Executable)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("running failed: %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("an untallied build wrote to stderr:\n%s", stderr.String())
	}
}

// The point of the whole exercise: a line that allocates and never lets go has
// to be findable by name. A heap profiler cannot do this, because the arena
// bump-allocates and every object in a chunk is charged to whichever call
// happened to ask for the chunk.
func TestTallyNamesTheLineThatHeldTheMemory(t *testing.T) {
	// Two hundred Webs of a thousand entries each, all kept. Nothing here can
	// be freed, so the flat tables have to be the top line of the report.
	const src = `one k is span 1 1000 | bend (n : (n, add n k)) | web

held is span 1 200 | bend one

len held
`
	out, report := tallied(t, "hold.weave", src, "")
	if strings.TrimSpace(out) != "200" {
		t.Fatalf("got %q, want 200", strings.TrimSpace(out))
	}
	rows := reportRows(t, report)
	if len(rows) == 0 {
		t.Fatalf("the report had no rows:\n%s", report)
	}
	if !strings.HasPrefix(rows[0], "collections.c:") {
		t.Errorf("the biggest holding was %q, want a flat table in collections.c\n%s",
			rows[0], report)
	}
}

// And the other half of being trustworthy: memory that *is* handed back has to
// leave the books. `mend` on an owned Thread writes through, and the loop below
// keeps exactly one Thread alive however many times it goes round.
func TestTallySeesMemoryComeBack(t *testing.T) {
	const src = `step 0 t is t
step n t is step (sub n 1) (mend (mod n 512) 1 t)

step 20000 (span 1 512 | bend (n : 0)) | sum
`
	out, report := tallied(t, "back.weave", src, "")
	// Every position ends up holding a 1, whichever order they were written in.
	if strings.TrimSpace(out) != "512" {
		t.Fatalf("got %q, want 512", strings.TrimSpace(out))
	}
	peak := reportPeak(t, report)
	if peak > 4<<20 {
		t.Errorf("peak was %d bytes for a loop over one 512-element Thread\n%s",
			peak, report)
	}
}

var (
	peakLine = regexp.MustCompile(`high-water mark ([0-9.]+) (B|KB|MB|GB)`)
	rowLine  = regexp.MustCompile(`^\s+\S+ \S+\s+\S+ \S+\s+\S+ \S+\s+\d+\s+(\S+)$`)
)

// reportPeak reads the high-water mark back out of the report, in bytes.
func reportPeak(t *testing.T, report string) int64 {
	t.Helper()
	m := peakLine.FindStringSubmatch(report)
	if m == nil {
		t.Fatalf("no high-water mark in:\n%s", report)
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("unreadable high-water mark %q", m[1])
	}
	for _, unit := range []string{"B", "KB", "MB", "GB"} {
		if unit == m[2] {
			break
		}
		n *= 1024
	}
	return int64(n)
}

// reportRows returns the site of each row of the breakdown, biggest first.
func reportRows(t *testing.T, report string) []string {
	t.Helper()
	var out []string
	for _, line := range strings.Split(report, "\n") {
		if m := rowLine.FindStringSubmatch(line); m != nil {
			out = append(out, m[1])
		}
	}
	return out
}
