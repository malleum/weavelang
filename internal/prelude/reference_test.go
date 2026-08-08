package prelude_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/prelude"
)

// Every verb has to be filed under a heading, or it would be missing from the
// reference without anyone noticing — which is how the spec's catalogue came
// to be short by ninety verbs.
func TestEveryVerbHasAGroup(t *testing.T) {
	known := map[string]bool{}
	for _, g := range prelude.Groups() {
		known[g.Name] = true
	}
	for _, e := range prelude.Values {
		if e.Group == "" {
			t.Errorf("`%s` has no Group, so it appears in no reference section", e.Name)
		}
	}
	// And every heading has prose, since a section of bare signatures explains
	// nothing.
	for name := range known {
		if !strings.Contains(prelude.Markdown("x", ""), "## "+name) {
			t.Errorf("group %q is missing from the rendered reference", name)
		}
	}
}

// The reference lists everything. This is the check the spec's sample cannot
// give: no verb is undocumented.
func TestReferenceCoversEveryVerb(t *testing.T) {
	doc := prelude.Markdown("x", "")
	for _, e := range prelude.Values {
		if !strings.Contains(doc, "`"+prelude.Signature(e)+"`") {
			t.Errorf("`%s` is not in the reference", e.Name)
		}
	}
	for _, c := range prelude.Ctors {
		if !strings.Contains(doc, "`"+c.Name+" :: "+c.Sig+"`") {
			t.Errorf("constructor `%s` is not in the reference", c.Name)
		}
	}
}

// docs/verbs.md is generated, so it goes stale the moment a verb is added
// without regenerating it.
func TestVerbsDocIsCurrent(t *testing.T) {
	path := filepath.Join("..", "..", "docs", "verbs.md")
	have, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command("go", "run", "../../cmd/weave", "verbs", "-md").Output()
	if err != nil {
		t.Fatalf("regenerating the reference failed: %v", err)
	}
	if string(have) != string(out) {
		t.Errorf("docs/verbs.md is out of date; run `make docs`")
	}
}
