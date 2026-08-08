package parser_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/parser"
)

// TestExamplesParse keeps the programs in examples/ honest: every one of them
// must lex and parse without a diagnostic. They double as the language's
// regression suite for syntax changes.
func TestExamplesParse(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.weave"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no examples found")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			bag := diag.New(path, string(src))
			file := parser.Parse(string(src), bag)
			if !bag.Empty() {
				t.Fatalf("example does not parse:\n%s", bag)
			}
			if file.Output() == nil {
				t.Error("example has no output expression")
			}
		})
	}
}
