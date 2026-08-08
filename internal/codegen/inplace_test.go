package codegen

import (
	"strings"
	"testing"
)

const gridPreamble = "sheet is Source through pattern\n"

// TestGridLoopUpdatesInPlace checks the case the analysis exists for.
func TestGridLoopUpdatesInPlace(t *testing.T) {
	src := gridPreamble + `
step g n is
  ward n
    0 : g
    _ : step (set g (knot 0 0) '#') (sub n 1)

step sheet 10
`
	c := generate(t, src)
	if !strings.Contains(c, "wp_set_owned(") {
		t.Errorf("expected an in-place update:\n%s", c)
	}
	if !strings.Contains(c, "w_disown(") {
		t.Errorf("the pattern escapes when the loop returns it, so it must be disowned:\n%s", c)
	}
}

// TestUnsafeShapesKeepCopying is the important half: each of these leaves a
// second way to reach the pattern, so the update has to copy.
func TestUnsafeShapesKeepCopying(t *testing.T) {
	cases := []struct{ name, body string }{
		{
			// `cells` hands out the pattern's own buffer, so naming it there means
			// the pattern can still be seen through the Thread.
			"pattern's cells taken during the loop",
			`
step g n acc is
  ward n
    0 : acc
    _ : step (set g (knot 0 0) '#') (sub n 1) (add acc (len (cells g)))

step sheet 10 0
`,
		},
		{
			"pattern bound to another name",
			`
step g n is
  ward n
    0 : g
    _ :
      weave keep is g
      step (set keep (knot 0 0) '#') (sub n 1)

step sheet 10
`,
		},
		{
			"pattern captured by a lambda",
			`
step g n is
  ward n
    0 : g
    _ : step (set g (knot 0 0) (pick (holds (seek (c : eq c '#') (cells g))) '.' '#')) (sub n 1)

step sheet 10
`,
		},
		{
			"pattern put into a tuple",
			`
step g n is
  ward n
    0 : g
    _ :
      weave kept is (g, n)
      step (set g (knot 0 0) '#') (sub n 1)

step sheet 10
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, gridPreamble+tc.body)
			if strings.Contains(c, "wp_set_owned(") {
				t.Errorf("this shape can still reach the pattern, so it must copy:\n%s", c)
			}
		})
	}
}

// TestThreadLoopUpdatesInPlace is the Thread half of the same proof. `mend`
// names its Thread last rather than first, which is the only thing that makes
// it different.
func TestThreadLoopUpdatesInPlace(t *testing.T) {
	src := `
step t n is
  ward n
    0 : t
    _ : step (mend 0 n t) (sub n 1)

step (span 1 10) 10
`
	c := generate(t, src)
	if !strings.Contains(c, "wp_mend_owned(") {
		t.Errorf("expected an in-place update:\n%s", c)
	}
	if !strings.Contains(c, "w_disown(") {
		t.Errorf("the Thread escapes when the loop returns it, so it must be disowned:\n%s", c)
	}
}

// TestThreadSlicesKeepCopying is the important half for a Thread: every verb
// that hands back a window on the buffer has to stop the update writing
// through, or the window would see writes that came after it.
func TestThreadSlicesKeepCopying(t *testing.T) {
	cases := []struct{ name, read string }{
		{"take", "sum (take 3 t)"},
		{"drop", "sum (drop 3 t)"},
		{"tail", "sum (tail t)"},
		{"strands", "sum (flat (strands even t))"},
		{"rev", "sum (rev t)"},
		{"sort", "sum (sort t)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			src := "\nstep t n acc is\n  ward n\n    0 : acc\n    _ : step (mend 0 n t) (sub n 1) (add acc (" +
				tc.read + "))\n\nstep (span 1 10) 10 0\n"
			c := generate(t, src)
			if strings.Contains(c, "wp_mend_owned(") {
				t.Errorf("%s leaves a second way to reach the buffer:\n%s", tc.name, c)
			}
		})
	}
}

// TestThreadBoundToAnotherNameKeepsCopying is the same point without a verb:
// a second name is a second owner.
func TestThreadBoundToAnotherNameKeepsCopying(t *testing.T) {
	src := `
step t n is
  ward n
    0 : t
    _ :
      weave keep is t
      step (mend 0 n keep) (sub n 1)

step (span 1 10) 10
`
	c := generate(t, src)
	if strings.Contains(c, "wp_mend_owned(") {
		t.Errorf("a second name is a second owner:\n%s", c)
	}
}

// TestPickIsTailPositionForOwnership pins the rule that `pick`'s branches are
// tail position here as they are in tailcall.go. Without it the two spellings
// of one loop compile to wildly different programs.
func TestPickIsTailPositionForOwnership(t *testing.T) {
	src := gridPreamble + `
step g n is
  pick (eq n 0) g (step (set g (knot 0 0) '#') (sub n 1))

step sheet 10
`
	c := generate(t, src)
	if !strings.Contains(c, "wp_set_owned(") {
		t.Errorf("a loop written with `pick` updates in place too:\n%s", c)
	}
}

// TestTakingOutIsAsCheapAsPuttingIn. Every ownable has a verb that removes as
// well as one that adds, and they do not all take the same number of
// arguments — `put w k v` against `forget w k` — which is what the arity check
// used to get wrong, silently leaving the removing half copying.
func TestTakingOutUpdatesInPlace(t *testing.T) {
	cases := []struct{ name, src, owned string }{
		{
			"forget from a Web",
			"w is web [(1, 1)]\nstep m n is\n  ward n\n    0 : len m\n    _ : step (forget m n) (sub n 1)\n\nstep w 10\n",
			"wp_forget_owned(",
		},
		{
			"remove from a Circle",
			"c is circle [1 2]\nstep s n is\n  ward n\n    0 : len s\n    _ : step (remove s n) (sub n 1)\n\nstep c 10\n",
			"wp_remove_owned(",
		},
		{
			"twist a Thread",
			"step t n is\n  ward n\n    0 : t\n    _ : step (twist 0 inc t) (sub n 1)\n\nstep (span 1 10) 10\n",
			"wp_twist_owned(",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src)
			if !strings.Contains(c, tc.owned) {
				t.Errorf("expected %s in:\n%s", tc.owned, c)
			}
		})
	}
}

// TestUpdateOutsideALoopCopies covers the plain case: without a loop threading
// the pattern there is nothing to prove.
func TestUpdateOutsideALoopCopies(t *testing.T) {
	c := generate(t, gridPreamble+"set sheet (knot 0 0) '#' | cells | len\n")
	if strings.Contains(c, "wp_set_owned(") {
		t.Errorf("a lone update must copy:\n%s", c)
	}
}

// TestNonGridParametersAreNeverDisowned guards the type restriction: w_disown
// only makes sense for a Pattern, and applying it to anything else would read a
// pointer out of an immediate.
func TestNonGridParametersAreNeverDisowned(t *testing.T) {
	src := `
count n acc is
  ward n
    0 : acc
    _ : count (sub n 1) (add acc 1)
count 10 0
`
	c := generate(t, src)
	if strings.Contains(c, "w_disown(") {
		t.Errorf("no pattern here, so nothing should be disowned:\n%s", c)
	}
}

// TestInPlaceCanBeTurnedOff keeps the differential test's escape hatch working.
func TestInPlaceCanBeTurnedOff(t *testing.T) {
	src := gridPreamble + `
step g n is
  ward n
    0 : g
    _ : step (set g (knot 0 0) '#') (sub n 1)

step sheet 10
`
	bag, file, info := parseAndCheck(t, src)
	c := Generate(file, info, bag, Options{DisableInPlace: true})
	if strings.Contains(c, "wp_set_owned(") {
		t.Errorf("expected copying when in-place updating is off:\n%s", c)
	}
}

// TestMapUpdatesWriteThrough covers the collections half: a Web threaded
// through a loop is updated in place rather than path-copied, which is what
// keeps a map built over a million steps from keeping a million paths.
func TestMapUpdatesWriteThrough(t *testing.T) {
	src := `
fill 0 w is w
fill n w is fill (sub n 1) (put w n n)

len (fill 100 (web []))
`
	c := generate(t, src)
	if !strings.Contains(c, "wp_put_owned(") {
		t.Errorf("the threaded map should be updated in place:\n%s", c)
	}
}

func TestSetUpdatesWriteThrough(t *testing.T) {
	src := `
seen 0 c is len c
seen n c is seen (sub n 1) (insert c n)

seen 100 (circle [])
`
	c := generate(t, src)
	if !strings.Contains(c, "wp_insert_owned(") {
		t.Errorf("the threaded set should be updated in place:\n%s", c)
	}
}

// The same hazards as for grids: anything that can leave a second reference
// behind has to fall back to copying.
func TestMapAliasingHazardsCopy(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			"bound to another name",
			"fill 0 w is len w\nfill n w is\n  weave old is w\n  add (len old) (fill (sub n 1) (put w n n))\n\nfill 5 (web [])\n",
		},
		{
			"its keys are taken",
			"fill 0 w acc is acc\nfill n w acc is fill (sub n 1) (put w n n) (add acc (len (keys w)))\n\nfill 5 (web []) 0\n",
		},
		{
			"put in a tuple",
			"fill 0 w is (w, 0)\nfill n w is fill (sub n 1) (put w n n)\n\nlen (first2 (fill 5 (web [])))\nfirst2 p is\n  ward p\n    (a, _) : a\n",
		},
		{
			"captured by a lambda",
			"fill 0 w is len w\nfill n w is add (braid (a _ : add a (len w)) 0 [1]) (fill (sub n 1) (put w n n))\n\nfill 5 (web [])\n",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src)
			if strings.Contains(c, "wp_put_owned(") {
				t.Errorf("a map that may be seen twice must be copied:\n%s", c)
			}
		})
	}
}

// ------------------------------------------------------ folds

// The case the fold analysis exists for: a map built with `braid`, which used
// to path-copy on every step because the accumulator lived inside the runtime.
func TestFoldAccumulatorUpdatesInPlace(t *testing.T) {
	c := generate(t, "w is [1 2 3] | braid (a k : put a k 1) (web [])\nlen w\n")
	if !strings.Contains(c, "wp_put_owned(") {
		t.Errorf("expected an in-place update:\n%s", c)
	}
	if !strings.Contains(c, "w_disown(") {
		t.Errorf("the accumulator leaves the loop, so it must be disowned:\n%s", c)
	}
}

func TestFoldInPlaceRespectsEveryHazard(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			// Binding the accumulator to another name leaves a second reader.
			"bound to another name",
			"w is [1 2] | braid (a k : weave b is a into put b k 1) (web [])\nlen w\n",
		},
		{
			// A lambda capturing it can outlive the turn.
			"captured by a lambda",
			"w is [1 2] | braid (a k : put a k (len ([1] | bend (x : len a)))) (web [])\nlen w\n",
		},
		{
			// `keys` hands back a Thread that shares the map's storage.
			"a verb that shares the storage",
			"w is [1 2] | braid (a k : put a (len (keys a)) 1) (web [])\nlen w\n",
		},
		{
			// The result is not the update, so the old map survives the step.
			"the update is not the result",
			"w is [1 2] | braid (a k : weave b is put a k 1 into a) (web [])\nlen w\n",
		},
		{
			// A fold over something that is not an ownable collection.
			"not a collection",
			"n is [1 2] | braid (a k : add a k) 0\nn\n",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := generate(t, c.src)
			if strings.Contains(out, "wp_put_owned(") {
				t.Errorf("the accumulator is not single-threaded here:\n%s", out)
			}
		})
	}
}

// The seed may be a map the caller still holds, so the first update has to
// copy. That is the runtime's job — the compiler only has to not disown it.
func TestFoldSeedIsNotAssumedOwned(t *testing.T) {
	c := generate(t, "base is web []\nw is [1 2] | braid (a k : put a k 1) base\n(len w, len base)\n")
	if !strings.Contains(c, "wp_put_owned(") {
		t.Errorf("expected the update to still be in place:\n%s", c)
	}
}

// `-no-in-place` turns it off, which is what the differential suite compares
// against.
func TestFoldInPlaceCanBeTurnedOff(t *testing.T) {
	src := "w is [1 2 3] | braid (a k : put a k 1) (web [])\nlen w\n"
	bag, file, info := parseAndCheck(t, src)
	c := Generate(file, info, bag, Options{DisableInPlace: true})
	if strings.Contains(c, "wp_put_owned(") {
		t.Errorf("expected copying when in-place updating is off:\n%s", c)
	}
}
