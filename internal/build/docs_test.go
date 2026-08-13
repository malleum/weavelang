package build_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/build"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/mdblock"
	"github.com/malleum/weave/internal/parser"
)

// Every ```weave block in the documentation is compiled, and where the block is
// followed by a fenced block of plain output, that output is checked too.
//
// Documentation that has quietly stopped working is worse than none: a reader
// cannot tell which half is wrong. This makes the tutorial's examples part of
// the test suite, so they cannot rot.
func TestDocumentationExamplesRun(t *testing.T) {
	requireCC(t)

	docs, err := filepath.Glob(filepath.Join("..", "..", "docs", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	// SPEC.md and README.md sit at the root rather than in docs/, and SPEC.md
	// was outside this check for long enough to grow examples that no longer
	// printed what they said. The whole point is that a reader cannot tell which
	// half of a stale example is wrong, and the spec is the document read most
	// carefully.
	docs = append(docs, filepath.Join("..", "..", "README.md"), filepath.Join("..", "..", "SPEC.md"))

	total := 0
	for _, path := range docs {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		blocks := mdblock.Blocks(string(src))
		if len(blocks) == 0 {
			continue
		}
		name := filepath.Base(path)
		t.Run(name, func(t *testing.T) {
			for _, b := range blocks {
				total++
				t.Run("line_"+itoa(b.Line+1), func(t *testing.T) { checkBlock(t, name, b) })
			}
		})
	}
	if total == 0 {
		t.Fatal("no weave examples found in the documentation")
	}
	t.Logf("checked %d examples", total)
}

func itoa(n int) string { return strconv.Itoa(n) }

func checkBlock(t *testing.T, doc string, b mdblock.Block) {
	t.Helper()
	name := doc + ":" + itoa(b.Line+1)

	// A ```weave-part block is an illustration written with names it never
	// defines, which is most of what a spec shows. Its syntax still has to be
	// right, so it is parsed; there is nothing to compile or run.
	if b.Fragment {
		bag := diag.New(name, b.Src)
		parser.Parse(b.Src, bag)
		if !bag.Empty() {
			t.Fatalf("%s does not parse:\n%s\n\nsource:\n%s", name, bag, b.Src)
		}
		return
	}

	dir := t.TempDir()
	bag := diag.New(name, b.Src)
	res, err := build.Compile(name, b.Src, build.Options{
		Output: filepath.Join(dir, "program"),
		Opt:    "-O0",
	}, bag)
	if err != nil {
		if !bag.Empty() {
			t.Fatalf("%s does not compile:\n%s\n\nsource:\n%s", name, bag, b.Src)
		}
		t.Fatalf("%s does not compile: %v\n\nsource:\n%s", name, err, b.Src)
	}
	if !b.HasWant {
		return
	}

	cmd := exec.Command(res.Executable)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stdin = strings.NewReader("")
	if err := cmd.Run(); err != nil {
		t.Fatalf("%s failed to run: %v", name, err)
	}
	got := strings.TrimRight(out.String(), "\n")
	if got != strings.TrimRight(b.Want, "\n") {
		t.Errorf("%s printed:\n%s\n\nbut the document says:\n%s\n\nsource:\n%s",
			name, got, b.Want, b.Src)
	}
}
