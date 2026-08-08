// Package treesitter holds no Go code. This test exists so that `go test ./...`
// notices when the grammar stops agreeing with the compiler: every program the
// compiler accepts, the grammar must parse without an ERROR node.
//
// It is skipped when the tree-sitter CLI is not installed, so the rest of the
// suite still runs in a bare environment.
package treesitter_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/prelude"
)

func TestGrammarParsesEveryExample(t *testing.T) {
	requireTreeSitter(t)

	paths, err := filepath.Glob(filepath.Join("..", "examples", "*.weave"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no examples found")
	}

	for _, path := range paths {
		t.Run(strings.TrimSuffix(filepath.Base(path), ".weave"), func(t *testing.T) {
			abs, err := filepath.Abs(path)
			if err != nil {
				t.Fatal(err)
			}
			out, _ := exec.Command("tree-sitter", "parse", abs).CombinedOutput()
			if strings.Contains(string(out), "ERROR") || strings.Contains(string(out), "MISSING") {
				t.Errorf("the grammar does not parse %s:\n%s", path, out)
			}
		})
	}
}

func TestGrammarCorpus(t *testing.T) {
	requireTreeSitter(t)

	cmd := exec.Command("tree-sitter", "test")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Errorf("tree-sitter test failed: %v\n%s", err, out)
	}
}

// requireTreeSitter skips when the CLI or a generated parser is missing.
func requireTreeSitter(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("tree-sitter"); err != nil {
		t.Skip("tree-sitter CLI not installed")
	}
	if _, err := os.Stat("src/parser.c"); err != nil {
		t.Skip("parser not generated: run `tree-sitter generate` in tree-sitter-weave")
	}
}

// The highlighter's built-in list is written by hand, so it drifts: a verb is
// added or renamed and the editor quietly stops colouring it, or colours a name
// that no longer exists. This holds it to the prelude, which is the same table
// the compiler and `weave verbs` read.
func TestHighlightsListEveryBuiltin(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("queries", "highlights.scm"))
	if err != nil {
		t.Fatal(err)
	}
	// Only the verb list, not the keyword or type lists that follow it.
	block := string(src)
	start := strings.Index(block, "(#any-of? @function.builtin")
	if start < 0 {
		t.Fatal("highlights.scm has no @function.builtin list")
	}
	end := strings.Index(block[start:], "))")
	if end < 0 {
		t.Fatal("the @function.builtin list is not closed")
	}
	block = block[start : start+end]

	listed := map[string]bool{}
	for _, m := range regexp.MustCompile(`"([A-Za-z][A-Za-z0-9]*)"`).FindAllStringSubmatch(block, -1) {
		listed[m[1]] = true
	}
	// `knot` is a constructor spelled in lower case, so it reads as a verb and
	// belongs in this list even though it is not a Value.
	for _, extra := range []string{"knot"} {
		if !listed[extra] {
			t.Errorf("highlights.scm does not colour `%s`", extra)
		}
		delete(listed, extra)
	}

	known := map[string]bool{"knot": true}
	for _, e := range prelude.Values {
		// `Source` is a value, but it is capitalised and coloured as a
		// constant rather than as a verb.
		if e.Name == "Source" {
			known[e.Name] = true
			continue
		}
		known[e.Name] = true
		if !listed[e.Name] {
			t.Errorf("highlights.scm does not colour the built-in `%s`", e.Name)
		}
	}
	for name := range listed {
		if !known[name] {
			t.Errorf("highlights.scm colours `%s`, which is not a built-in", name)
		}
	}
}

// The plugin ships its own copy of the queries, and `make grammar` copies them
// across. If the two differ, an editor installing the plugin gets stale rules.
func TestPluginQueriesMatch(t *testing.T) {
	names, err := filepath.Glob(filepath.Join("queries", "*.scm"))
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range names {
		want, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		copied := filepath.Join("..", "weave.nvim", "queries", "weave", filepath.Base(path))
		got, err := os.ReadFile(copied)
		if err != nil {
			t.Fatalf("%s: %v (run `make grammar`)", copied, err)
		}
		if string(got) != string(want) {
			t.Errorf("%s is out of date; run `make grammar`", copied)
		}
	}
}
