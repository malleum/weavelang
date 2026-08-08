package prelude_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/prelude"
)

// The spec's verb catalogue is a sample, not the source of truth, so it drifts
// unless something checks it. This reads the names out of §15's fenced block
// and requires each to be a verb the prelude actually declares — which is what
// caught `size` and `product` still being listed long after they were
// renamed.
func TestSpecCatalogueNamesExist(t *testing.T) {
	spec, err := os.ReadFile(filepath.Join("..", "..", "SPEC.md"))
	if err != nil {
		t.Fatal(err)
	}
	block := catalogue(t, string(spec))

	known := map[string]bool{}
	for _, e := range prelude.Values {
		known[e.Name] = true
	}
	for _, c := range prelude.Ctors {
		known[c.Name] = true
	}
	word := regexp.MustCompile(`^[a-z][A-Za-z0-9]*$`)
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		names, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		for _, name := range strings.Fields(names) {
			if !word.MatchString(name) {
				continue
			}
			if !known[name] {
				t.Errorf("SPEC.md §15 lists `%s`, which the prelude does not declare", name)
			}
		}
	}
}

// catalogue returns the fenced block of §15.
func catalogue(t *testing.T, spec string) string {
	t.Helper()
	i := strings.Index(spec, "## 15. Standard verbs")
	if i < 0 {
		t.Fatal("SPEC.md has no §15")
	}
	rest := spec[i:]
	start := strings.Index(rest, "```")
	if start < 0 {
		t.Fatal("§15 has no fenced block")
	}
	rest = rest[start+3:]
	end := strings.Index(rest, "```")
	if end < 0 {
		t.Fatal("§15's fenced block is not closed")
	}
	return rest[:end]
}
