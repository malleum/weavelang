package weavenvim

import (
	"os/exec"
	"strings"
	"testing"
)

// TestAocSpec runs the plugin's own Lua checks, which cover the parts of it
// that are logic rather than editor: reading a year and a day out of a path,
// deciding which part the cursor is in, reading what the site said, turning the
// page into Markdown, and — in calls_spec — finding the definition the cursor
// is in and laying a watched function's calls out as a table.
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

	for _, spec := range []string{"test/aoc_spec.lua", "test/calls_spec.lua"} {
		args := []string{spec}
		if strings.HasSuffix(lua, "nvim") {
			args = []string{"-l", spec}
		}
		out, err := exec.Command(lua, args...).CombinedOutput()
		if err != nil || !strings.Contains(string(out), "all ok") {
			t.Errorf("%s failed: %v\n%s", spec, err, out)
		}
	}
}
