package codegen

import (
	"strings"
	"testing"
)

// TestConsumedParameterKeepsOwnership checks the shape the feature exists for:
// a helper that updates a collection and hands it back, called from a loop that
// puts the result straight into the slot the argument came from.
func TestConsumedParameterKeepsOwnership(t *testing.T) {
	src := `
fill t ks is ks | braid (v i : mend i 0 v) t
step 0 t acc is add acc (t | sum)
step n t acc is step (sub n 1) (fill t [0 1 2]) (add acc 1)
step 4 [9 9 9] 0
`
	c := generate(t, src)
	if !strings.Contains(c, "static Value wu_fill_move(") {
		t.Errorf("expected a second entry point for fill:\n%s", c)
	}
	if !strings.Contains(c, "return w_disown(wu_fill_move(env, args));") {
		t.Errorf("the ordinary entry point must still disown:\n%s", c)
	}
	if !strings.Contains(c, "wu_fill_move(NULL,") {
		t.Errorf("the loop's call should hand the Thread over:\n%s", c)
	}
}

// TestOrdinaryCallerStillCopies is the other half. `a` is bound to a name and
// read after `b` has been built from it, so the ownership must not cross that
// call — a `_move` there would let the second fold write through the first
// one's result.
func TestOrdinaryCallerStillCopies(t *testing.T) {
	src := `
fill t ks is ks | braid (v i : mend i 0 v) t
a is fill [1 2 3] [0 1]
b is fill a [2]
join "  " [air b, air a]
`
	c := generate(t, src)
	if strings.Contains(c, "wu_fill_move(NULL,") {
		t.Errorf("a top-level caller must use the disowning entry point:\n%s", c)
	}
}

// TestOwnershipComposesAlongAChain checks that a helper handed what another
// helper gave back writes through rather than copying once per link.
func TestOwnershipComposesAlongAChain(t *testing.T) {
	src := `
one t is mend 0 1 t
two t is mend 1 2 (one t)
step 0 t is t
step n t is step (sub n 1) (two t)
step 3 [9 9 9]
`
	c := generate(t, src)
	if !strings.Contains(c, "wp_mend_owned(w_earth(1LL), w_earth(2LL), wu_one_move(") {
		t.Errorf("the outer update should write through what `one` handed back:\n%s", c)
	}
}

// TestNotConsumedWhenItCouldBeKept walks the shapes that must not get a `_move`
// entry point at all, since each leaves a way for the caller's reference to
// outlive the call.
func TestNotConsumedWhenItCouldBeKept(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			// The result is not the collection, so there is nothing to hand the
			// ownership on to.
			"the result is a number",
			"fill t ks is len (ks | braid (v i : mend i 0 v) t)\nfill [1 2 3] [0]",
		},
		{
			// A memo table keeps its arguments for the rest of the program.
			"remembered",
			"remember fill t ks is ks | braid (v i : mend i 0 v) t\nair (fill [1 2 3] [0])",
		},
		{
			// Named twice: the collection reaches the result and is also read
			// into it, so one write would be seen through the other.
			"welded to itself",
			"fill t ks is weld (ks | braid (v i : mend i 0 v) t) t\nair (fill [1 2 3] [0])",
		},
		{
			// Nothing is written, so a second entry point would buy nothing.
			"only read",
			"fill t ks is take (len ks) t\nair (fill [1 2 3] [0])",
		},
		{
			// `rest` is a window on the argument's own array.
			"destructured",
			"fill [] ks is []\nfill [x ..rest] ks is mend 0 x rest\nair (fill [1 2 3] [0])",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src)
			if strings.Contains(c, "_move(") {
				t.Errorf("expected no consumed parameter:\n%s", c)
			}
		})
	}
}

// TestConsumingIsOffWithoutInPlace checks the flag the differential suite
// relies on: with in-place updating disabled there is one entry point and it
// copies, like everything else.
func TestConsumingIsOffWithoutInPlace(t *testing.T) {
	src := `
fill t ks is ks | braid (v i : mend i 0 v) t
step 0 t is t
step n t is step (sub n 1) (fill t [0 1 2])
step 4 [9 9 9]
`
	bag, file, info := parseAndCheck(t, src)
	c := Generate(file, info, bag, Options{DisableInPlace: true})
	if !bag.Empty() {
		t.Fatalf("codegen failed:\n%s", bag)
	}
	if strings.Contains(c, "_move(") {
		t.Errorf("-no-in-place must not emit a consuming entry point:\n%s", c)
	}
}
