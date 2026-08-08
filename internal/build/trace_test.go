package build_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/build"
	"github.com/malleum/weave/internal/diag"
)

// `weave trace` reports every top-level definition rather than the output
// expression, which is what the editor plugin shows beside each line. The
// format is deliberately dull: LINE<TAB>NAME<TAB>VALUE, one record per line.
func TestTrace(t *testing.T) {
	requireCC(t)

	src := `nums is [1 2 3]

double n is mul n 2

total is nums | bend double | sum

grid2 is "ab\ncd" | pattern

total
`
	dir := t.TempDir()
	bag := diag.New("trace.weave", src)
	res, err := build.Compile("trace.weave", src, build.Options{
		Output: filepath.Join(dir, "program"),
		Trace:  true,
	}, bag)
	if err != nil {
		t.Fatalf("compiling failed: %v\n%s", err, bag)
	}

	cmd := exec.Command(res.Executable)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("running failed: %v", err)
	}

	want := []string{
		"1\tnums\t[1 2 3]",
		"3\tdouble\tEarth -> Earth", // a function has no value, so its type
		"5\ttotal\t12",
		"7\tgrid2\tab\\ncd\\n", // newlines escaped, so a record is one line
		"9\t\t12",              // the output expression, with an empty name
	}
	got := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if len(got) != len(want) {
		t.Fatalf("got %d records, want %d:\n%s", len(got), len(want), out.String())
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d: got %q, want %q", i, got[i], want[i])
		}
	}
}
