package codegen

import (
	"strings"
	"testing"

	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/check"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/parser"
)

// parseAndCheck runs the front end, failing the test on any diagnostic.
func parseAndCheck(t *testing.T, src string) (*diag.Bag, *ast.File, *check.Info) {
	t.Helper()
	bag := diag.New("test.weave", src)
	file := parser.Parse(src, bag)
	if !bag.Empty() {
		t.Fatalf("parse failed:\n%s", bag)
	}
	info := check.File(file, bag)
	if !bag.Empty() {
		t.Fatalf("check failed:\n%s", bag)
	}
	return bag, file, info
}

// generate compiles src to C with every optimisation on.
func generate(t *testing.T, src string) string {
	t.Helper()
	bag, file, info := parseAndCheck(t, src)
	out := Generate(file, info, bag, Options{})
	if !bag.Empty() {
		t.Fatalf("codegen failed:\n%s", bag)
	}
	return out
}

// TestChainsFuse checks that the runtime verbs really do disappear from the
// generated C. Without this, fusion could silently stop applying and every
// behavioural test would still pass — just slower.
func TestChainsFuse(t *testing.T) {
	cases := []struct {
		name, src string
		gone      []string
	}{
		{
			name: "map then fold",
			src:  "span 1 10 | bend (x : mul x x) | sum",
			gone: []string{"wp_bend", "wp_sum"},
		},
		{
			name: "map, filter, seek",
			src:  "span 1 10 | bend (x : mul x 2) | sift even | seek (gt 4)",
			gone: []string{"wp_bend", "wp_sift", "wp_seek"},
		},
		{
			name: "filter then count",
			src:  "span 1 10 | sift even | len",
			gone: []string{"wp_sift", "wp_len"},
		},
		{
			name: "two stages collecting to a Thread",
			src:  "span 1 10 | bend (x : add x 1) | sift even",
			gone: []string{"wp_bend", "wp_sift"},
		},
		{
			name: "fold with an explicit seed",
			src:  "span 1 10 | bend (x : mul x 2) | braid (a b : add a b) 0",
			gone: []string{"wp_bend", "wp_braid"},
		},
		{
			name: "any short-circuits",
			src:  "span 1 10 | bend (x : mul x 2) | any (gt 8)",
			gone: []string{"wp_bend", "wp_any"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src)
			for _, verb := range tc.gone {
				if strings.Contains(c, verb+"(") {
					t.Errorf("expected %s to be fused away, but it is still called:\n%s", verb, c)
				}
			}
			// A span-sourced chain generates its values in the loop header, so
			// the counter is an int64_t rather than an index.
			if !strings.Contains(c, "for (size_t") && !strings.Contains(c, "for (int64_t") {
				t.Errorf("expected a fused loop, got:\n%s", c)
			}
		})
	}
}

// TestLambdaStagesAreInlined checks the second half of the win: a lambda in a
// fused chain should become straight-line code, with no closure built and no
// indirect call.
func TestLambdaStagesAreInlined(t *testing.T) {
	c := generate(t, "span 1 10 | bend (x : mul x x) | sum")
	if strings.Contains(c, "w_closure(") {
		t.Errorf("the lambda should have been inlined, not turned into a closure:\n%s", c)
	}
	if strings.Contains(c, "w_call1(") {
		t.Errorf("the lambda should have been inlined, not called indirectly:\n%s", c)
	}
	// Specialisation may have turned it into the typed helper; either way the
	// multiplication has to be in the loop body rather than behind a call.
	if !strings.Contains(c, "wp_mul(") && !strings.Contains(c, "w_mul_e(") {
		t.Errorf("expected the multiplication inline in the loop:\n%s", c)
	}
}

// TestPartiallyAppliedBuiltinsAreDirect checks that `sift (gt 4)` calls the C
// function with a hoisted bound rather than allocating a closure per element.
func TestPartiallyAppliedBuiltinsAreDirect(t *testing.T) {
	c := generate(t, "span 1 10 | bend (x : add x 1) | sift (gt 4) | len")
	if strings.Contains(c, "w_closure(") {
		t.Errorf("`gt 4` should compile to a direct call:\n%s", c)
	}
	if !strings.Contains(c, "wp_gt(") && !strings.Contains(c, "w_gt_e(") {
		t.Errorf("expected a direct comparison call:\n%s", c)
	}
}

// TestShadowedVerbsAreNotFused makes sure fusion only ever applies to the real
// prelude verbs. A program that defines its own `bend` must get its own.
func TestShadowedVerbsAreNotFused(t *testing.T) {
	src := `
bend f xs is [1 2 3]
answer is span 1 10 | bend (x : mul x x) | sum
answer
`
	c := generate(t, src)
	if !strings.Contains(c, "wu_bend(") {
		t.Errorf("the program's own `bend` should be called:\n%s", c)
	}
}

// TestShadowedStageCallsTheProgramsDefinition covers the other half: a stage
// naming a prelude verb the program has redefined must call the program's.
// Before this check, `sign` in a fused chain compiled to the runtime's `sign`,
// which linked cleanly and gave the wrong answer.
func TestShadowedStageCallsTheProgramsDefinition(t *testing.T) {
	src := `
sign n is pick (gt 0 n) 1 0
answer is span 1 10 | bend sign | sift (eq 1) | len
answer
`
	c := generate(t, src)
	if strings.Contains(c, "wp_sign(") {
		t.Errorf("the prelude's `sign` should not be called:\n%s", c)
	}
	if !strings.Contains(c, "wu_sign(") {
		t.Errorf("the program's own `sign` should be called:\n%s", c)
	}
}

// A partially applied stage has the same hazard.
func TestShadowedPartialStageIsNotSpecialised(t *testing.T) {
	src := `
gt a b is add a b
answer is span 1 10 | bend (x : mul x 2) | sift (x : eq 0 (mod x 4)) | bend (gt 3) | sum
answer
`
	c := generate(t, src)
	if strings.Contains(c, "wp_gt(") || strings.Contains(c, "w_gt_e(") {
		t.Errorf("the prelude's `gt` should not be called:\n%s", c)
	}
}

// TestSingleStageIsNotFused documents the cutoff, and where it moved to.
//
// Rewriting one runtime verb as a loop wins nothing when the verb is going to
// be called anyway — so a lone stage over a Thread, with a function the loop
// would have to call through, stays a runtime call.
//
// Two things now cross that line. A *generated* producer — a span, or one of
// the grid walks — never builds the array the verb would have read, so one
// stage is already a saving. And a function written out on the spot is inlined
// by the loop, so what goes away is the closure built every time the enclosing
// function runs plus the indirect call per element; that is worth it with no
// stage to remove at all.
func TestSingleStageIsNotFused(t *testing.T) {
	t.Run("a lone stage over a Thread with a named function", func(t *testing.T) {
		c := generate(t, "square x is mul x x\nanswer is [1 2 3] | bend square\nanswer")
		if !strings.Contains(c, "wp_bend(") {
			t.Errorf("a lone bend should stay a runtime call:\n%s", c)
		}
	})

	t.Run("a lone stage over a span is fused: the array is the saving", func(t *testing.T) {
		c := generate(t, "square x is mul x x\nanswer is span 1 10 | bend square\nanswer")
		if strings.Contains(c, "wp_bend(") {
			t.Errorf("a span has no array to read, so this should fuse:\n%s", c)
		}
	})

	t.Run("a lone lambda stage is fused: the closure is the saving", func(t *testing.T) {
		c := generate(t, "answer is [1 2 3] | bend (x : mul x x)\nanswer")
		if strings.Contains(c, "wp_bend(") {
			t.Errorf("a lambda stage should be inlined into a loop:\n%s", c)
		}
		if strings.Contains(c, "w_closure(") {
			t.Errorf("the inlined lambda should need no closure:\n%s", c)
		}
	})

	t.Run("a lone lambda predicate is fused", func(t *testing.T) {
		c := generate(t, "answer is [1 2 3] | count (x : odd x)\nanswer")
		if strings.Contains(c, "wp_count(") {
			t.Errorf("a lambda predicate should be inlined into a loop:\n%s", c)
		}
	})
}

// TestSpanIsGeneratedNotBuilt checks that a range pipeline allocates nothing:
// the span becomes the loop bounds instead of a Thread.
func TestSpanIsGeneratedNotBuilt(t *testing.T) {
	c := generate(t, "span 1 1000 | bend (x : mul x x) | sum")
	if strings.Contains(c, "wp_span(") {
		t.Errorf("the span should be generated in the loop header:\n%s", c)
	}
	if !strings.Contains(c, "for (int64_t") {
		t.Errorf("expected a counted loop over the span:\n%s", c)
	}
	if strings.Contains(c, "w_alloc(") {
		t.Errorf("a folded span pipeline should allocate nothing:\n%s", c)
	}
}

// TestNonThreadSizeIsNotFused guards the Bulk Talent: `len` on a Circle is
// not a Thread pipeline.
func TestNonThreadSizeIsNotFused(t *testing.T) {
	c := generate(t, "len (circle [1 2 3])")
	if !strings.Contains(c, "wp_len(") {
		t.Errorf("len over a Circle must stay a runtime call:\n%s", c)
	}
}

// `drop` and `dropwhile` are stages, so the runtime verbs disappear from a
// fused chain the way `take` and `takewhile` already did — and, more to the
// point, an endless producer survives one.
func TestDropIsAStage(t *testing.T) {
	cases := []struct {
		name, src string
		gone      []string
	}{
		{"drop", "span 1 20 | drop 4 | sum", []string{"wp_drop", "wp_span", "wp_sum"}},
		{"dropwhile", "span 1 20 | dropwhile (gt 5) | sum", []string{"wp_dropwhile", "wp_sum"}},
		{"drop then take", "span 1 20 | drop 4 | take 3 | sum", []string{"wp_drop", "wp_take"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src)
			for _, verb := range tc.gone {
				if strings.Contains(c, verb+"(") {
					t.Errorf("expected %s to be fused away:\n%s", verb, c)
				}
			}
		})
	}
}

// Text is not something the loop fuser knows how to walk, so `drop` on it stays
// the verb it was — the same rule `take` follows.
func TestDropOnTextIsNotAStage(t *testing.T) {
	c := generate(t, "Source | drop 2 | len")
	if !strings.Contains(c, "wp_drop(") {
		t.Errorf("expected the verb to survive:\n%s", c)
	}
}
