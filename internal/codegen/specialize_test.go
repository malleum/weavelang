package codegen

import (
	"strings"
	"testing"
)

// TestPrimitivesAreSpecialised checks that a known operand type removes the
// runtime's tag dispatch. `add` at Earth is an integer addition, and `gt` at
// Earth is a comparison rather than a call into w_compare.
func TestPrimitivesAreSpecialised(t *testing.T) {
	cases := []struct {
		name, src, want, gone string
	}{
		{"earth add", "add 1 2", "w_add_e", "wp_add"},
		{"earth mul", "mul 3 4", "w_mul_e", "wp_mul"},
		{"earth div keeps its zero check", "div 8 2", "w_div_e", "wp_div"},
		{"earth mod", "mod 8 3", "w_mod_e", "wp_mod"},
		{"water add", "add 1.5 2.5", "w_add_w", "wp_add"},
		{"earth comparison", "gt 1 2", "w_gt_e", "wp_gt"},
		{"water comparison", "lt 1.5 2.5", "w_lt_w", "wp_lt"},
		{"spark comparison", "eq 'a' 'b'", "w_eq_f", "wp_eq"},
		{"spirit logic", "and Light Shadow", "w_and_s", "wp_and"},
		{"even", "even 4", "w_even_e", "wp_even"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src+"\n")
			if !strings.Contains(c, tc.want+"(") {
				t.Errorf("expected %s, got:\n%s", tc.want, c)
			}
			if strings.Contains(c, tc.gone+"(") {
				t.Errorf("expected %s to be gone, got:\n%s", tc.gone, c)
			}
		})
	}
}

// TestPolymorphicCodeKeepsGeneralVerbs is the safety property: where the type
// is a variable rather than a primitive, nothing is specialised and the
// general verb runs, exactly as before.
func TestPolymorphicCodeKeepsGeneralVerbs(t *testing.T) {
	src := `
biggest a b is max a b
(biggest 1 2, biggest 1.5 2.5)
`
	c := generate(t, src)
	if !strings.Contains(c, "wp_max(") {
		t.Errorf("a polymorphic definition must keep the general verb:\n%s", c)
	}
}

// TestNonPrimitiveComparisonsStayGeneral covers the types with no typed helper:
// structural comparison still goes through w_compare.
func TestNonPrimitiveComparisonsStayGeneral(t *testing.T) {
	for _, src := range []string{`eq "a" "b"`, "eq (knot 1 2) (knot 3 4)", "eq [1] [2]"} {
		c := generate(t, src+"\n")
		if !strings.Contains(c, "wp_eq(") {
			t.Errorf("%s should use the general comparison:\n%s", src, c)
		}
	}
}

// TestFusedLoopsSpecialise checks the two places a chain can use a typed
// helper: a filter's bound and the fold a summing consumer performs.
func TestFusedLoopsSpecialise(t *testing.T) {
	c := generate(t, "span 1 20 | bend (x : mul x x) | sift (gt 50) | sum")
	for _, want := range []string{"w_mul_e(", "w_gt_e(", "w_add_e("} {
		if !strings.Contains(c, want) {
			t.Errorf("expected %s in the fused loop:\n%s", want, c)
		}
	}
	if strings.Contains(c, "w_compare(") {
		t.Errorf("no comparison in this loop needs w_compare:\n%s", c)
	}
}

// TestSpecialisedHelpersUseTheirOwnCallingConvention guards a bug that shipped
// in an earlier draft: the typed helpers are named w_something, and a fused
// stage mistook them for compiled Weave definitions, which take (env, args).
func TestSpecialisedHelpersUseTheirOwnCallingConvention(t *testing.T) {
	c := generate(t, "span 1 20 | bend (x : add x 1) | sift (gt 4) | len")
	if strings.Contains(c, "w_gt_e(NULL") {
		t.Errorf("a typed helper must not be called like a user function:\n%s", c)
	}
	if !strings.Contains(c, "w_gt_e(") {
		t.Errorf("expected the typed comparison:\n%s", c)
	}
}

// TestSpecialisationCanBeTurnedOff keeps the differential test's escape hatch
// working.
func TestSpecialisationCanBeTurnedOff(t *testing.T) {
	bag, file, info := parseAndCheck(t, "add 1 2\n")
	c := Generate(file, info, bag, Options{DisableSpecialize: true})
	if !strings.Contains(c, "wp_add(") {
		t.Errorf("expected the general verb when specialisation is off:\n%s", c)
	}
}

// A verb passed as a stage rather than applied still gets specialised: there is
// no operand to read a type off, but the checker recorded the type of the
// mention itself, and its argument side is what the loop feeds it.
func TestBareStageVerbsAreSpecialised(t *testing.T) {
	cases := []struct{ name, src, want, notWant string }{
		{"even as a filter", "span 1 20 | bend (x : mul x 3) | sift even | sum",
			"w_even_e(", "wp_even("},
		{"odd as a filter", "span 1 20 | bend (x : add x 1) | sift odd | len",
			"w_odd_e(", "wp_odd("},
		{"abs as a map", "span 1 20 | bend abs | sift even | sum",
			"w_abs_e(", "wp_abs("},
		{"even as a counter", "span 1 20 | bend (x : mul x 7) | count even",
			"w_even_e(", "wp_even("},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src+"\n")
			if !strings.Contains(c, tc.want) {
				t.Errorf("expected %s in:\n%s", tc.want, c)
			}
			if strings.Contains(c, tc.notWant) {
				t.Errorf("the general %s should not be called:\n%s", tc.notWant, c)
			}
		})
	}
}

// And where the element type is not a concrete primitive, nothing is
// specialised and the general verb runs exactly as before.
func TestPolymorphicStagesKeepTheGeneralVerb(t *testing.T) {
	c := generate(t, "same x is x\nspan 1 20 | bend same | sift even | sum\n")
	if !strings.Contains(c, "wu_same(") {
		t.Errorf("the program's own verb should be called:\n%s", c)
	}
}
