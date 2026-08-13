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

// A loop that only sometimes updates keeps the in-place path.
//
// Handing the collection straight back into its own slot is as single-threaded
// as updating it: the next turn holds exactly the reference this one did. The
// analysis used to read that bare mention as a second reference and refuse the
// whole loop — so a loop with a branch that skips the update, which is most
// loops, copied the collection on every turn including the turns that did
// update.
func TestASkippedUpdateKeepsTheInPlacePath(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"a map that skips what it already knows",
			"fill 0 w is w\nfill n w is pick (known w n) (fill (sub n 1) w) (fill (sub n 1) (put w n n))\n\nlen (fill 100 (web []))",
			"wp_put_owned(",
		},
		{
			"a set that skips what it has seen",
			"seen 0 c is len c\nseen n c is pick (member c n) (seen (sub n 1) c) (seen (sub n 1) (insert c n))\n\nseen 100 (circle [])",
			"wp_insert_owned(",
		},
		{
			"a Link that skips a pair already joined",
			"walk 0 l is len (clumped l)\nwalk n l is pick (bound l n 0) (walk (sub n 1) l) (walk (sub n 1) (bind l n 0))\n\nwalk 50 (link (span 0 50))",
			"wp_bind_owned(",
		},
		{
			"the same loop written with a ward",
			"fill xs w is\n  ward xs\n    [] : w\n    [k ..rest] : pick (known w k) (fill rest w) (fill rest (put w k k))\n\nlen (fill [1, 2, 3] (web []))",
			"wp_put_owned(",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src+"\n")
			if !strings.Contains(c, tc.want) {
				t.Errorf("expected %s in the loop:\n%s", tc.want, c)
			}
		})
	}
}

// A loop that never updates gains nothing and must not be given the owned form,
// since `w_disown` would then be emitted for a collection that was never owned.
func TestALoopThatNeverUpdatesIsNotOwned(t *testing.T) {
	src := `
count 0 w is len w
count n w is count (sub n 1) w

count 10 (web [(1, 2)])
`
	c := generate(t, src)
	if strings.Contains(c, "wp_put_owned(") || strings.Contains(c, "w_disown(") {
		t.Errorf("a loop that only reads should stay as it is:\n%s", c)
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
			// `most` hands back a key, but a Web of Webs would let the *value*
			// out, and the type is what says which one you have.
			"a value drawn out could be the map itself",
			"fill 0 w is len w\nfill n w is\n  weave inner is get w n | otherwise (web [])\n  add (len inner) (fill (sub n 1) (put w n (web [])))\n\nfill 5 (web [])\n",
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
			// A lambda capturing it can outlive the turn — unless it is handed
			// straight to a verb, which is the case below. Here it is bound to
			// a name first, so nothing says where it ends up.
			"captured by a lambda that is bound to a name",
			"w is [1 2] | braid (a k : weave f is (x : len a) into put a k (len (bend f [1]))) (web [])\nlen w\n",
		},
		{
			// `cells` hands back a Thread over the grid's own array, so the
			// Thread would see a write that came after it.
			"a verb that shares the storage",
			"g is [\"ab\"] | bend fires | weft ' '\n" +
				"step 0 p acc is acc\nstep n p acc is step (sub n 1) (set p (knot 0 0) '#') (add acc (len (cells p)))\n" +
				"step 5 g 0\n",
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

// The whitelist of blessed reading verbs is gone.
//
// It named about seventeen verbs per collection and refused the rest of the
// prelude, so a loop that asked its map anything unusual — or called one of the
// program's own helpers — copied on every turn with nothing in the source to
// say why. The rule now is a blacklist of the twelve verbs whose result can
// still reach the collection's storage, plus the type rule: the collection's
// own type constructor must not occur in the call's result type.
func TestReadingIsAllowedUnlessItCanKeepTheCollection(t *testing.T) {
	const loop = "fill 0 w is len w\nfill n w is fill (sub n 1) (put w n (%s))\n\nfill 30 (web [])\n"
	reads := []struct{ name, expr string }{
		{"len", "len w"},
		{"through keys", "len (keys w)"},
		{"through vals", "len (vals w)"},
		{"through items", "len (items w)"},
		{"through most", "most w | otherwise 0"},
		{"a membership test", "pick (known w n) 1 0"},
		{"a lookup with a default", "get w n | otherwise 0"},
		{"nested two deep", "len (sort (keys w))"},
		{"inside a pick", "pick (gt 0 (len w)) (len (keys w)) 0"},
		{"a comparison against another map", "pick (eq w (web [])) 1 0"},
		{"as an argument to the program's own function", "size2 w"},
	}
	for _, tc := range reads {
		t.Run(tc.name, func(t *testing.T) {
			src := strings.Replace(loop, "%s", tc.expr, 1)
			if strings.Contains(tc.expr, "size2") {
				src = "size2 m is add 1 (len m)\n" + src
			}
			c := generate(t, src)
			if !strings.Contains(c, "wp_put_owned(") {
				t.Errorf("reading with %s should not cost the in-place path:\n%s", tc.expr, c)
			}
		})
	}
}

// The other half: what must still copy, and why. Each of these can leave a
// second way to reach the collection that outlives the update.
func TestWhatStillHasToCopy(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			// `copies` puts the map inside a Thread, and the type says so:
			// `Thread (Web k v)` mentions Web.
			"stored inside what the verb returns",
			"fill 0 w is len w\nfill n w is\n  weave kept is copies 1 w\n  add (len kept) (fill (sub n 1) (put w n n))\n\nfill 5 (web [])\n",
		},
		{
			// A memo table keeps its arguments for the rest of the program, and
			// no type can say that.
			"handed to a remembered function",
			"remember size3 m is add 1 (len m)\n" +
				"fill 0 w is len w\nfill n w is fill (sub n 1) (put w n (size3 w))\n\nfill 5 (web [])\n",
		},
		{
			// `merge` builds a fresh map, but its *type* is `Web k v` and the
			// type is all the rule has to go on. Refusing here is the price of
			// a rule that cannot be wrong the other way.
			"a verb whose result type is the collection's own",
			"fill 0 w is len w\nfill n w is fill (sub n 1) (put w n (len (merge w (web []))))\n\nfill 5 (web [])\n",
		},
		{
			"named twice in one call",
			"fill 0 w is len w\nfill n w is fill (sub n 1) (put w n (len (merge w w)))\n\nfill 5 (web [])\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src)
			if strings.Contains(c, "wp_put_owned(") {
				t.Errorf("this can still reach the map, so it must copy:\n%s", c)
			}
		})
	}
}

// `gentle` is `braid` that may stop, and threads its accumulator identically.
// It was left out of the ownership machinery when it was written, so every
// `gentle` over a collection copied it once per element while the same loop
// spelled `braid` did not.
func TestGentleUpdatesInPlace(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"a Thread accumulator",
			"[1 2 3] | gentle (v n : Woven (mend 0 n v)) [9 9 9] | rescue [] | len\n",
			"wp_mend_owned(",
		},
		{
			"a Web accumulator",
			"[1 2 3] | gentle (w n : Woven (put w n n)) (web []) | rescue (web []) | len\n",
			"wp_put_owned(",
		},
		{
			"a step that may stop",
			"[1 2 3 4] | gentle (w n : pick (gt 2 n) (Gentled n) (Woven (put w n n))) (web []) | snag 0\n",
			"wp_put_owned(",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src)
			if !strings.Contains(c, tc.want) {
				t.Errorf("expected %s:\n%s", tc.want, c)
			}
			if !strings.Contains(c, "w_disown(") {
				t.Errorf("the accumulator leaves the fold, so it must be disowned:\n%s", c)
			}
		})
	}
}

// The collection must not leave a `gentle` still writable. `Gentled x` is the
// answer rather than the accumulator, so handing the collection out there is
// read like any other argument — which for a bare mention means copying.
func TestGentleHandingOutTheCollectionCopies(t *testing.T) {
	c := generate(t, "[1 2 3] | gentle (v n : pick (gt 2 n) (Gentled v) (Woven (mend 0 n v))) [9 9 9] | snag [] | len\n")
	if strings.Contains(c, "wp_mend_owned(") {
		t.Errorf("the accumulator escapes as the answer, so it must copy:\n%s", c)
	}
}

// A sibling argument of the updating tail call may not read the collection.
//
// The arguments are evaluated in order into the loop's slots, so the update
// writes through before a later argument is evaluated, and a later argument
// that reads the collection sees a write that has not happened yet. This was
// wrong from the day the analysis was written — `len w` was on the old
// whitelist — and the differential suite found it the moment the reading rule
// was widened enough to make it easy to write.
func TestASiblingArgumentMayNotReadTheCollection(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			"the count is taken after the update",
			"fill 0 w acc is acc\nfill n w acc is fill (sub n 1) (put w n n) (add acc (len w))\n\nfill 20 (web []) 0\n",
		},
		{
			"its keys are taken after the update",
			"fill 0 w acc is acc\nfill n w acc is fill (sub n 1) (put w n n) (add acc (len (keys w)))\n\nfill 20 (web []) 0\n",
		},
		{
			"a grid read after the update",
			"g is [\"ab\"] | bend fires | weft ' '\n" +
				"step 0 p acc is acc\nstep n p acc is step (sub n 1) (set p (knot 0 0) '#') (add acc (len (cells p)))\n\nstep 5 g 0\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src)
			if strings.Contains(c, "wp_put_owned(") || strings.Contains(c, "wp_set_owned(") {
				t.Errorf("a later argument reads it, so the update must copy:\n%s", c)
			}
		})
	}
}

// The same read is fine anywhere that runs before the tail call, which is
// everywhere else in the body.
func TestReadingBeforeTheTailCallIsFine(t *testing.T) {
	src := `
fill 0 w acc is acc
fill n w acc is
  weave here is len (keys w)
  fill (sub n 1) (put w n n) (add acc here)

fill 20 (web []) 0
`
	c := generate(t, src)
	if !strings.Contains(c, "wp_put_owned(") {
		t.Errorf("the read happens before the update, so it costs nothing:\n%s", c)
	}
}

// A collection carried as one half of a Twine accumulator.
//
// Carrying `(state, index)` through a fold is the natural way to write a walk,
// and it used to be the worst shape in the language: the whole collection was
// copied once per element, with nothing in the source to say why. A Twine the
// step takes apart on entry and rebuilds on exit has exactly one reference to
// each half, so the half that is a collection is as single-threaded as a bare
// accumulator would be.
func TestATwineAccumulatorUpdatesInPlace(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"a Thread and an index",
			"fst p is\n  ward p\n    (a, _) : a\n\n[1 2 3] | gentle ((v, i) n : Woven (mend i n v, add i 1)) ([9 9 9], 0) | rescue ([], 0) | fst | sum\n",
			"wp_mend_owned(",
		},
		{
			"the same through braid",
			"fst p is\n  ward p\n    (a, _) : a\n\n[1 2 3] | braid ((v, i) n : (mend i n v, add i 1)) ([9 9 9], 0) | fst | sum\n",
			"wp_mend_owned(",
		},
		{
			"the collection second",
			"snd p is\n  ward p\n    (_, b) : b\n\n[1 2 3] | braid ((i, v) n : (add i 1, mend i n v)) (0, [9 9 9]) | snd | sum\n",
			"wp_mend_owned(",
		},
		{
			"a Web and a counter",
			"fst p is\n  ward p\n    (a, _) : a\n\n[1 2 3] | braid ((w, i) n : (put w n i, add i 1)) (web [], 0) | fst | len\n",
			"wp_put_owned(",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src)
			if !strings.Contains(c, tc.want) {
				t.Errorf("expected %s:\n%s", tc.want, c)
			}
			if !strings.Contains(c, "w_disown(") {
				t.Errorf("the half that is a collection leaves the fold, so it must be disowned:\n%s", c)
			}
		})
	}
}

// The hazards a Twine accumulator brings with it.
func TestTwineAccumulatorHazardsCopy(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			// The slots are evaluated in order, so a later one that reads the
			// collection sees a write that has not happened.
			"another slot reads it after the update",
			"fst p is\n  ward p\n    (a, _) : a\n\n[1 2 3] | braid ((v, i) n : (mend i n v, add i (len v))) ([9 9 9], 0) | fst | sum\n",
		},
		{
			// Which of the two is being threaded is not clear.
			"two halves are both collections",
			"fst p is\n  ward p\n    (a, _) : a\n\n[1 2 3] | braid ((a, b) n : (mend 0 n a, mend 0 n b)) ([9 9], [9 9]) | fst | sum\n",
		},
		{
			"the Twine is rebuilt without the update",
			"fst p is\n  ward p\n    (a, _) : a\n\n[1 2 3] | braid ((v, i) n : (v, add i 1)) ([9 9 9], 0) | fst | sum\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src)
			if strings.Contains(c, "wp_mend_owned(") {
				t.Errorf("this shape cannot write through:\n%s", c)
			}
		})
	}
}

// A fold step lifted out to a name is the same lambda under another name, and
// is read back as one. Before this, lifting a step out — which is what you do
// the moment it grows past a line — cost the closure, the call per element, and
// the ownership analysis all at once.
func TestANamedStepIsReadBackAsALambda(t *testing.T) {
	src := `
step (v, i) n is Woven (mend i n v, add i 1)

fst p is
  ward p
    (a, _) : a

[1 2 3] | gentle step ([9 9 9], 0) | rescue ([], 0) | fst | sum
`
	c := generate(t, src)
	if !strings.Contains(c, "wp_mend_owned(") {
		t.Errorf("a named step should own its accumulator like a written-out one:\n%s", c)
	}
	if strings.Contains(c, "w_closure(") {
		t.Errorf("an inlined step needs no closure:\n%s", c)
	}
}

// Only the shapes where "a definition is a lambda" is exactly true.
func TestWhatIsNotReadBackAsALambda(t *testing.T) {
	cases := []struct{ name, src, mustCall string }{
		{
			"more than one clause",
			"step v 0 is v\nstep v n is mend 0 n v\n\n[1 2 3] | braid step [9 9 9] | len\n",
			"wu_step(",
		},
		{
			"a pattern that can fail to match",
			"step v (Held n) is mend 0 n v\nstep v Stilled is v\n\n[Held 1, Stilled] | braid step [9 9 9] | len\n",
			"wu_step(",
		},
		{
			"it calls itself",
			"step v n is pick (gt 0 n) (step v (sub n 1)) (mend 0 n v)\n\n[1 2 3] | braid step [9 9 9] | len\n",
			"wu_step(",
		},
		{
			"it is remembered",
			"remember step v n is mend 0 n v\n\n[1 2 3] | braid step [9 9 9] | len\n",
			"wu_step(",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := generate(t, tc.src)
			if !strings.Contains(c, tc.mustCall) {
				t.Errorf("this should stay a call rather than being inlined:\n%s", c)
			}
		})
	}
}

// A read of the collection through a lambda a verb is holding is a read even
// when the *call's* result is another collection of the same kind. The lambda
// cannot outlive the call, and the walk of its body has already refused every
// way it could hand the collection out — so all the result type has to rule out
// is a function coming back.
func TestAReadThroughALambdaInsideAChainIsStillARead(t *testing.T) {
	src := `
back [] wide xs is xs
back [(c, pr) ..rest] wide xs is
  weave used is
    span (add c 1) (sub wide 1) as (j : mul (nth j pr else 0) (nth j xs else 0)) | sum
  back rest wide (mend c used xs)

back [(0, [1 2]) (1, [3 4])] 2 [9 9] | sum
`
	c := generate(t, src)
	if !strings.Contains(c, "wp_mend_owned(") {
		t.Errorf("expected the update to write through:\n%s", c)
	}
}

// A collection the loop owns may leave inside a constructor, and codegen has to
// disown it there — the caller may keep what it is handed, exactly as for a
// bare mention.
func TestAConstructorCarriesTheCollectionOutDisowned(t *testing.T) {
	src := `
step 0 xs is Held xs
step n xs is step (sub n 1) (mend n n xs)

step 3 [9 9 9] else [] | sum
`
	c := generate(t, src)
	if !strings.Contains(c, "wp_mend_owned(") {
		t.Errorf("expected the update to write through:\n%s", c)
	}
	if !strings.Contains(c, "w_held(w_disown(") {
		t.Errorf("the Thread escapes inside the Held, so it must be disowned:\n%s", c)
	}
}

// The other half: a lambda whose call could hand a *function* back keeps the
// collection captured, so it is not a read.
func TestALambdaThatCouldEscapeIsNotARead(t *testing.T) {
	src := `
pickOne c f g is pick c f g
step 0 xs is sum xs
step n xs is
  weave f is pickOne (gt 0 n) (j : nth j xs else 0) (j : n)
  step (sub n 1) (mend 0 (f 0) xs)

step 5 [9 9 9]
`
	c := generate(t, src)
	if strings.Contains(c, "wp_mend_owned(") {
		t.Errorf("the lambda can leave inside the result, so this must copy:\n%s", c)
	}
}
