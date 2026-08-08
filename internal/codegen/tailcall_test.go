package codegen

import (
	"strings"
	"testing"
)

// TestSelfTailCallBecomesALoop checks that a tail-recursive definition compiles
// to a jump rather than a call. Without this the C stack decides how deep a
// Weave program may recurse, and clang does not reliably do it for us.
func TestSelfTailCallBecomesALoop(t *testing.T) {
	src := `
count n acc is
  ward n
    0 : acc
    _ : count (sub n 1) (add acc 1)
count 10 0
`
	c := generate(t, src)
	fn := functionBody(t, c, "wu_count")
	if !strings.Contains(fn, "for (;;)") {
		t.Errorf("expected a loop:\n%s", fn)
	}
	if !strings.Contains(fn, "continue;") {
		t.Errorf("expected the tail call to become a continue:\n%s", fn)
	}
	if strings.Contains(fn, "wu_count(NULL") {
		t.Errorf("the tail call should not still be a call:\n%s", fn)
	}
}

// TestTailArgumentsAreEvaluatedBeforeRebinding is the subtle part: the new
// arguments usually read the parameters they replace, so every one must be
// computed before any is overwritten.
func TestTailArgumentsAreEvaluatedBeforeRebinding(t *testing.T) {
	src := `
walk a b is
  ward a
    0 : b
    _ : walk (sub a 1) (add b a)
walk 5 0
`
	c := generate(t, src)
	fn := functionBody(t, c, "wu_walk")

	// Look for the rebind specifically, not the parameter's declaration.
	lastTemp := strings.LastIndex(fn, "Value h")
	firstRebind := strings.Index(fn, "\np1 = ")
	if firstRebind < 0 {
		firstRebind = strings.Index(fn, "  p1 = h")
	}
	if lastTemp < 0 || firstRebind < 0 {
		t.Fatalf("expected temporaries and a rebind:\n%s", fn)
	}
	if lastTemp > firstRebind {
		t.Errorf("arguments must be computed before parameters are rebound:\n%s", fn)
	}
}

// TestPickBranchesAreTailPositions checks that the lazy conditional does not
// block the optimisation, since it is how most loops are written.
func TestPickBranchesAreTailPositions(t *testing.T) {
	src := `
down n is pick (eq 0 n) n (down (sub n 1))
down 10
`
	c := generate(t, src)
	fn := functionBody(t, c, "wu_down")
	if !strings.Contains(fn, "continue;") {
		t.Errorf("a tail call inside pick should jump:\n%s", fn)
	}
}

// TestNonTailRecursionKeepsTheCall guards against rewriting a call whose result
// is still needed.
func TestNonTailRecursionKeepsTheCall(t *testing.T) {
	src := `
fact 0 is 1
fact n is mul n (fact (sub n 1))
fact 5
`
	c := generate(t, src)
	fn := functionBody(t, c, "wu_fact")
	if strings.Contains(fn, "for (;;)") {
		t.Errorf("a non-tail call must not be turned into a loop:\n%s", fn)
	}
	if !strings.Contains(fn, "wu_fact(NULL") {
		t.Errorf("expected a real recursive call:\n%s", fn)
	}
}

// TestShadowedNameIsNotATailCall checks the guard: a local binding with the
// function's own name is not the function.
func TestShadowedNameIsNotATailCall(t *testing.T) {
	src := `
loop n is
  weave loop is 7
  loop
loop 1
`
	c := generate(t, src)
	fn := functionBody(t, c, "wu_loop")
	if strings.Contains(fn, "continue;") {
		t.Errorf("a shadowed name must not be treated as a tail call:\n%s", fn)
	}
}

// TestWrongArityIsNotATailCall covers a partial application of the function to
// itself, which produces a closure rather than a jump.
func TestWrongArityIsNotATailCall(t *testing.T) {
	src := `
pair a b is
  ward a
    0 : b
    _ : add b (len (bend (pair 1) [1 2]))
pair 0 1
`
	c := generate(t, src)
	fn := functionBody(t, c, "wu_pair")
	if strings.Contains(fn, "for (;;)") {
		t.Errorf("a partial application is not a tail call:\n%s", fn)
	}
}

// functionBody extracts one generated C function for inspection.
func functionBody(t *testing.T, c, name string) string {
	t.Helper()
	marker := "static Value " + name + "(Value *env, Value *args) {"
	i := strings.Index(c, marker)
	if i < 0 {
		t.Fatalf("no definition of %s in:\n%s", name, c)
	}
	rest := c[i:]
	if j := strings.Index(rest, "\n}\n"); j >= 0 {
		return rest[:j]
	}
	return rest
}

// TestMutualTailCallsBecomeOneLoop covers the other half of the promise: a set
// of definitions that tail-call each other is one loop too, not a chain of C
// frames. Before this, a pair like `even`/`odd` segfaulted at -O0 and only
// survived -O3 because clang happened to make the sibling call.
func TestMutualTailCallsBecomeOneLoop(t *testing.T) {
	src := `
even2 0 is Light
even2 n is odd2 (sub n 1)

odd2 0 is Shadow
odd2 n is even2 (sub n 1)

even2 10
`
	c := generate(t, src)
	// One loop, entered by each member at its own index.
	if !strings.Contains(c, "switch (which)") {
		t.Errorf("the two definitions should be merged into one loop:\n%s", c)
	}
	if !strings.Contains(c, "which = 1;") || !strings.Contains(c, "which = 0;") {
		t.Errorf("each tail call should select the other member:\n%s", c)
	}
	// And inside the loop neither calls the other: both are jumps. The entry
	// points and `main` still name them, which is the point of keeping those.
	loop := loopBody(t, c)
	for _, call := range []string{"wu_odd2(", "wu_even2("} {
		if strings.Contains(loop, call) {
			t.Errorf("%s inside the loop should have become a jump:\n%s", call, loop)
		}
	}
}

// loopBody returns the merged loop function's text.
func loopBody(t *testing.T, c string) string {
	t.Helper()
	i := strings.Index(c, "static Value wu__loop")
	for i >= 0 {
		rest := c[i:]
		// The forward declaration comes first; the definition is the one that
		// opens a brace.
		if end := strings.Index(rest, "\n"); end >= 0 && strings.Contains(rest[:end], "{") {
			if j := strings.Index(rest, "\n}\n"); j >= 0 {
				return rest[:j]
			}
			return rest
		}
		next := strings.Index(c[i+1:], "static Value wu__loop")
		if next < 0 {
			break
		}
		i += 1 + next
	}
	t.Fatalf("no merged loop in:\n%s", c)
	return ""
}

// A definition that is not part of any cycle keeps its plain shape: no loop,
// no switch, no shared slots.
func TestNonRecursiveDefinitionsAreUnchanged(t *testing.T) {
	c := generate(t, "twice n is mul n 2\n\ntwice 21\n")
	if strings.Contains(c, "for (;;)") || strings.Contains(c, "switch (which)") {
		t.Errorf("a definition that never calls itself needs no loop:\n%s", c)
	}
}

// A call that is not in tail position is not an edge, so the two are separate
// groups and the call stays a call.
func TestNonTailMutualCallsAreNotMerged(t *testing.T) {
	src := `
f 0 is 1
f n is add 1 (g (sub n 1))

g 0 is 1
g n is add 1 (f (sub n 1))

f 4
`
	c := generate(t, src)
	if strings.Contains(c, "switch (which)") {
		t.Errorf("calls that are not in tail position must not be merged:\n%s", c)
	}
}
