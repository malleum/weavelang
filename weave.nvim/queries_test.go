// Package weavenvim holds no Go code. This test keeps the plugin's copy of the
// tree-sitter queries identical to the grammar's, since nvim-treesitter looks
// for them on the runtimepath rather than inside the grammar derivation, and
// two copies that drift apart highlight differently depending on how the
// plugin was installed.
package weavenvim_test

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQueriesAreInSync(t *testing.T) {
	for _, name := range []string{"highlights.scm", "injections.scm"} {
		t.Run(name, func(t *testing.T) {
			want, err := os.ReadFile(filepath.Join("..", "tree-sitter-weave", "queries", name))
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join("queries", "weave", name))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Errorf("weave.nvim/queries/weave/%s is out of date; run `make grammar`", name)
			}
		})
	}
}
