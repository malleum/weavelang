package weavenvim

import (
	"os/exec"
	"strings"
	"testing"
)

// TestAocSpec runs the plugin's own Lua checks, which cover the parts of
// lua/weave/aoc.lua that are logic rather than editor: reading a year and a
// day out of a path, deciding which part the cursor is in, reading what the
// site said, and turning the page into Markdown.
//
// It is skipped where no Lua is installed. The alternative — leaving the
// awkward parts of a plugin unchecked because the language they are written in
// is not the one the compiler is — is worse than a test that sometimes does
// not run.
func TestAocSpec(t *testing.T) {
	lua := ""
	for _, name := range []string{"lua5.1", "luajit", "lua", "nvim"} {
		if path, err := exec.LookPath(name); err == nil {
			lua = path
			break
		}
	}
	if lua == "" {
		t.Skip("no Lua on PATH")
	}

	args := []string{"test/aoc_spec.lua"}
	if strings.HasSuffix(lua, "nvim") {
		args = []string{"-l", "test/aoc_spec.lua"}
	}
	out, err := exec.Command(lua, args...).CombinedOutput()
	if err != nil || !strings.Contains(string(out), "all ok") {
		t.Errorf("the plugin's Lua checks failed: %v\n%s", err, out)
	}
}
