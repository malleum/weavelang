package build_test

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/build"
	"github.com/malleum/weave/internal/diag"
)

// runWith compiles src with the given optimisations turned off and returns its
// output.
func runWith(t *testing.T, name, src, input string, o build.Options) string {
	t.Helper()
	requireCC(t)

	dir := t.TempDir()
	bag := diag.New(name, src)
	o.Output = filepath.Join(dir, "program")
	// The optimisations under test are Weave's, not the C compiler's, so this
	// matrix builds at -O0: it runs in a fraction of the time, and clang can no
	// longer paper over a difference between two of Weave's own code paths.
	o.Opt = "-O0"
	res, err := build.Compile(name, src, o, bag)
	if err != nil {
		if !bag.Empty() {
			t.Fatalf("compiling %s failed:\n%s", name, bag)
		}
		t.Fatalf("compiling %s failed: %v", name, err)
	}

	cmd := exec.Command(res.Executable)
	cmd.Stdin = strings.NewReader(input)
	var out, errOut bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errOut
	if err := cmd.Run(); err != nil {
		t.Fatalf("running %s failed: %v\nstderr: %s", name, err, errOut.String())
	}
	return out.String()
}

// TestOptimisationsPreserveMeaning compiles each program four ways — chains
// fused or not, primitives specialised or not — and requires every version to
// agree.
//
// This is the check that actually protects the optimisations: a hand-written
// expectation only proves the compiler agrees with whoever wrote the test,
// while this proves each fast path agrees with the simple one. It has already
// earned its place by catching a specialised comparison compiled with the
// calling convention of a user-defined function. New shapes should be added
// here rather than as fixed expectations.
// mapLoop threads a Web through a tail-recursive loop, adding one entry per
// turn: the shape in-place updating is for.
const mapLoop = "fill 0 w is w\nfill n w is fill (sub n 1) (put w n (mul n 2))\n"

func TestOptimisationsPreserveMeaning(t *testing.T) {
	// Every program here is compiled four times over and run four times over,
	// which is half an hour on a slow machine. That is the right price for the
	// check that protects every optimisation in the compiler, and the wrong
	// price for the test you run between two edits — so `-short` leaves it out
	// and continuous integration runs it on every push. See .github/workflows.
	if testing.Short() {
		t.Skip("the whole corpus compiled four ways; run without -short")
	}

	programs := []struct {
		name, src, in string
	}{
		{"map fold", "span 1 20 | bend (x : mul x x) | sum", ""},
		{"map filter fold", "span 1 50 | bend (x : mul x 3) | sift even | sum", ""},
		{"filter map filter", "span 1 40 | sift even | bend (x : add x 1) | sift (gt 20) | sum", ""},
		{"collect", "span 1 12 | bend (x : mul x 2) | sift (gt 6)", ""},
		{"collect three stages", "span 1 12 | bend (x : add x 1) | sift odd | bend (x : mul x x)", ""},
		{"len", "span 1 30 | bend (x : mul x 2) | sift (gt 10) | len", ""},
		{"count", "span 1 30 | bend (x : mul x 2) | count (gt 10)", ""},
		{"seek hit", "span 1 30 | bend (x : mul x 3) | seek (gt 20)", ""},
		{"seek miss", "span 1 10 | bend (x : mul x 3) | seek (gt 1000)", ""},
		{"first", "span 1 10 | bend (x : mul x 5) | sift even | first", ""},
		{"any true", "span 1 10 | bend (x : mul x 2) | any (gt 15)", ""},
		{"any false", "span 1 10 | bend (x : mul x 2) | any (gt 100)", ""},
		{"all true", "span 1 10 | bend (x : mul x 2) | all (gt 1)", ""},
		{"all false", "span 1 10 | bend (x : mul x 2) | all (gt 5)", ""},
		{"braid", "span 1 10 | bend (x : mul x x) | braid (a b : add a b) 7", ""},
		{"prod", "span 1 8 | bend (x : x) | sift (gt 1) | prod", ""},
		{"empty source", "[] | bend (x : mul x 2) | sift even | sum", ""},
		{"descending span is empty", "span 5 1 | bend (x : mul x 2) | sift even | sum", ""},
		{"descending span collects nothing", "span 5 1 | bend (x : mul x 2) | sift even", ""},
		{"span collect", "span 1 10 | bend (x : mul x 3) | sift even", ""},
		{"single element span", "span 7 7 | bend (x : mul x 2) | sift even | sum", ""},
		{"span bounds are computed once", "answer is\n  weave lo is add 1 1\n  span lo (mul lo 5) | bend (x : mul x x) | sum\nanswer", ""},
		{"filter drops everything", "span 1 10 | bend (x : mul x 2) | sift odd | len", ""},
		{"water", "[1.5 2.5 3.5] | bend (x : mul x 2.0) | sift (gt 4.0) | sum", ""},
		{"captured local", "answer is\n  weave k is 7\n  span 1 20 | bend (x : mul x k) | sift (gt 40) | sum\nanswer", ""},
		{"user function stage", "twice n is mul n 2\nspan 1 20 | bend twice | sift (gt 10) | sum", ""},
		{"constructor stage", "span 1 5 | bend Held | sift holds | len", ""},
		{"destructuring lambda", "zip (span 1 5) (span 10 14) | bend ((a, b) : add a b) | sift even | sum", ""},

		// zip and items produce pairs, and a pair that is taken apart on the
		// spot is never built. Every shape here has to agree with the version
		// that builds it.
		{"zip into a fold", `zip [1 2 3] ["a" "b" "c"] | braid (acc (n, t) : join "" [acc, t, air n]) ""`, ""},
		{"zip uneven lengths", `zip (span 1 9) ["a" "b"] | bend ((n, t) : join "" [t, air n]) | join ","`, ""},
		{"zip keeps the pair whole for a consumer that needs it", "zip (span 1 5) (span 10 14) | sift ((a, _) : even a) | len", ""},
		{"zip take", "zip (span 1 9) (span 10 18) | take 3 | bend ((a, b) : add a b) | sum", ""},
		{"zip nested pattern", "zip [(1, 2) (3, 4)] [10 20] | bend (((a, b), c) : add (add a b) c) | sum", ""},
		{
			"zip a pair the function does not open",
			"pair a b is add a b\n" +
				"total p is\n  ward p\n    (a, b) : pair a b\n" +
				"zip (span 1 5) (span 1 5) | bend total | sum", "",
		},
		{"zip empty", `zip [] ["a"] | bend ((n, t) : t) | len`, ""},
		{"items into a fold", "w is web [(1, 10) (2, 20) (3, 30)]\nitems w | braid (acc (k, v) : add acc (mul k v)) 0", ""},
		{"items mapped", "w is web [(1, \"a\") (2, \"b\")]\nitems w | bend ((k, v) : join \"\" [air k, v]) | join \",\"", ""},
		{"items filtered", "w is web [(1, 10) (2, 20) (3, 30)]\nitems w | sift ((k, _) : odd k) | bend ((_, v) : v) | sum", ""},
		{"items of an empty map", "w is web []\nitems w | braid (acc (k, v) : add acc k) 0", ""},
		{"items collected whole", "w is web [(2, 20) (1, 10)]\nitems w | sift ((k, _) : gt 0 k)", ""},
		{"items feeding the map it came from", "w is web [(1, 1) (2, 2)]\nitems w | braid (acc (k, v) : put acc k (mul v 10)) w | items", ""},
		{"members of a Circle is not a pair", "circle [3 1 2] | members | bend (x : mul x 2) | sum", ""},

		// The reading rule was widened from a whitelist of about seventeen
		// verbs to "anything that cannot keep the collection". Every one of
		// these now writes through where it used to copy, so every one has to
		// agree with the copying version — a read that secretly kept a
		// reference would show up here and nowhere else.
		{"reading its keys mid-loop", "fill 0 w acc is (len w, acc)\nfill n w acc is fill (sub n 1) (put w n n) (add acc (len (keys w)))\nfill 20 (web []) 0", ""},
		{"reading its vals mid-loop", "fill 0 w acc is (len w, acc)\nfill n w acc is fill (sub n 1) (put w n n) (add acc (sum (vals w)))\nfill 20 (web []) 0", ""},
		{"reading its items mid-loop", "fill 0 w acc is (len w, acc)\nfill n w acc is fill (sub n 1) (put w n n) (add acc (len (items w)))\nfill 20 (web []) 0", ""},
		{"reading a Circle's members mid-loop", "seen 0 c acc is (len c, acc)\nseen n c acc is seen (sub n 1) (insert c n) (add acc (len (members c)))\nseen 20 (circle []) 0", ""},
		{"sorting its keys mid-loop", "fill 0 w acc is (len w, acc)\nfill n w acc is fill (sub n 1) (put w n n) (add acc (sum (sort (keys w))))\nfill 20 (web []) 0", ""},
		{"a grid read through its knots mid-loop", "g is [\"abc\" \"def\"] | bend fires | weft ' '\nstep 0 p acc is (len (cells p), acc)\nstep n p acc is step (sub n 1) (set p (knot 0 0) '#') (add acc (len (knots p)))\nstep 10 g 0", ""},
		{"the program's own helper reading it", "size4 m is add 1 (len m)\nfill 0 w is len w\nfill n w is fill (sub n 1) (put w n (size4 w))\nfill 20 (web [])", ""},
		{"a Thread read every way mid-loop", "step 0 t acc is (sum t, acc)\nstep n t acc is step (sub n 1) (mend 0 n t) (add acc (add (len t) (sum t)))\nstep 20 (span 1 8) 0", ""},

		// The same reads, taken before the tail call, where the widened rule
		// does let the update write through. These are the ones that changed.
		{
			"keys read before the update",
			"fill 0 w acc is acc\n" +
				"fill n w acc is\n  weave k is len (keys w)\n  fill (sub n 1) (put w n n) (add acc k)\n" +
				"fill 20 (web []) 0", "",
		},
		{
			"vals summed before the update",
			"fill 0 w acc is acc\n" +
				"fill n w acc is\n  weave s is sum (vals w)\n  fill (sub n 1) (put w n n) (add acc s)\n" +
				"fill 20 (web []) 0", "",
		},
		{
			"a Circle's members before the update",
			"seen 0 c acc is acc\n" +
				"seen n c acc is\n  weave m is len (members c)\n  seen (sub n 1) (insert c n) (add acc m)\n" +
				"seen 20 (circle []) 0", "",
		},
		{
			"a grid's knots before the update",
			"g is [\"abc\" \"def\"] | bend fires | weft ' '\n" +
				"step 0 p acc is acc\n" +
				"step n p acc is\n  weave k is len (knots p)\n  step (sub n 1) (set p (knot 0 0) '#') (add acc k)\n" +
				"step 10 g 0", "",
		},
		{
			"a Thread read every way before the update",
			"step 0 t acc is acc\n" +
				"step n t acc is\n  weave r is add (len t) (sum t)\n  step (sub n 1) (mend 0 n t) (add acc r)\n" +
				"step 20 (span 1 8) 0", "",
		},
		{
			"the program's own helper reading it",
			"size4 m is add 1 (len m)\n" +
				"fill 0 w is len w\nfill n w is fill (sub n 1) (put w n (size4 w))\n" +
				"fill 20 (web [])", "",
		},

		// A collection carried as one half of a Twine accumulator, which is how
		// a walk carries its position. Every one of these writes through now,
		// so every one has to agree with the copying build — and the seed has
		// to survive, which is what the ones reading it afterwards check.
		{
			"a Thread and an index through gentle",
			"fst p is\n  ward p\n    (a, _) : a\n" +
				"[1 2 3] | gentle ((v, i) n : Woven (mend i n v, add i 1)) ([9 9 9], 0) | rescue ([], 0) | fst | air", "",
		},
		{
			"a Thread and an index through braid",
			"fst p is\n  ward p\n    (a, _) : a\n" +
				"[1 2 3] | braid ((v, i) n : (mend i n v, add i 1)) ([9 9 9], 0) | fst | air", "",
		},
		{
			"the collection is the second half",
			"snd p is\n  ward p\n    (_, b) : b\n" +
				"[1 2 3] | braid ((i, v) n : (add i 1, mend i n v)) (0, [9 9 9]) | snd | air", "",
		},
		{
			"a Web and a counter",
			"fst p is\n  ward p\n    (a, _) : a\n" +
				"[1 2 3] | braid ((w, i) n : (put w n i, add i 1)) (web [], 0) | fst | items", "",
		},
		{
			"the seed survives the fold",
			"fst p is\n  ward p\n    (a, _) : a\n" +
				"seed is [9 9 9]\n" +
				"done is [1 2 3] | braid ((v, i) n : (mend i n v, add i 1)) (seed, 0) | fst\n" +
				"join \",\" [air done, air seed]", "",
		},
		{
			"a walk that stops, carrying its position",
			"digits is [3 1 2 0 4]\n" +
				"step (v, i) c is\n" +
				"  weave n is nth i v\n" +
				"  pick (holds n) (Woven (mend i (inc (n | otherwise 0)) v, add (n | otherwise 0) i)) (Gentled c)\n" +
				"gentle step (digits, 0) (flow inc 0) | snag 0", "",
		},

		// A fold step lifted out to a name is the same lambda under another
		// name, and is inlined as one. These check the shapes that are and are
		// not read back that way.
		{
			"a named step",
			"step (v, i) n is (mend i n v, add i 1)\n" +
				"fst p is\n  ward p\n    (a, _) : a\n" +
				"[1 2 3] | braid step ([9 9 9], 0) | fst | air", "",
		},
		{
			"a named step of two clauses stays a call",
			"step v 0 is v\nstep v n is mend 0 n v\n[1 2 3] | braid step [9 9 9] | air", "",
		},
		{
			"a named step that calls itself stays a call",
			"step v n is pick (gt 1 n) (step v (sub n 1)) (mend 0 n v)\n[1 2 3] | braid step [9 9 9] | air", "",
		},
		{
			"a remembered step stays a call",
			"remember step v n is mend 0 n v\n[1 2 3] | braid step [9 9 9] | air", "",
		},
		{
			"a named stage function",
			"twice2 n is mul n 2\nspan 1 10 | bend twice2 | sift (gt 5) | sum", "",
		},
		{
			"a named predicate",
			"big n is gt 5 n\nspan 1 10 | count big", "",
		},

		// A lambda handed straight to a verb cannot outlive the call, so a
		// mention of the collection inside it is a read like any other.
		{
			"read through a lambda a verb is holding",
			"w is span 1 20 | braid (a k : put a k (len ([1] | bend (x : len a)))) (web [])\nlen w", "",
		},
		{
			"read through a lambda inside a stage",
			"walk xs i acc is\n" +
				"  ward nth i xs\n" +
				"    Stilled : acc\n" +
				"    Held d :\n" +
				"      weave used is span 0 i | bend (j : nth j xs | otherwise 0) | sum\n" +
				"      walk (mend i (add d used) xs) (add i 1) (add acc used)\n" +
				"walk [1 2 3 4 5] 0 0", "",
		},

		// Chained updates: each link hands its result to the next.
		{
			"two updates in one expression",
			"fill 0 w is len w\nfill n w is fill (sub n 1) (put (put w n n) (neg n) n)\nfill 20 (web [])", "",
		},
		{
			"three deep",
			"step 0 t is sum t\nstep n t is step (sub n 1) (mend 0 n (mend 1 n (mend 2 n t)))\nstep 20 [9 9 9]", "",
		},

		// The hazard the widening uncovered: a sibling argument of the updating
		// tail call is evaluated after the write, so it must not read.
		{"a sibling argument reads it after the update", "fill 0 w acc is acc\nfill n w acc is fill (sub n 1) (put w n n) (add acc (len w))\nfill 20 (web []) 0", ""},
		{"a sibling takes its keys after the update", "fill 0 w acc is acc\nfill n w acc is fill (sub n 1) (put w n n) (add acc (len (keys w)))\nfill 20 (web []) 0", ""},

		// `gentle` joined `braid`: it threads its accumulator identically and
		// had simply been left out.
		{"gentle over a Thread", "[1 2 3 4] | gentle (v n : Woven (mend 0 n v)) [9 9 9] | rescue [] | air", ""},

		// A fused `gentle` carries its accumulator in pieces and its Weaving in
		// a flag, so that neither is built on a turn nobody looks at them. Every
		// shape that decides whether it can is here, because the ones it cannot
		// take apart have to keep working exactly as they did.
		// A fused fold whose turn allocates and answers with a number hands its
		// storage back at the end of the turn. That is the one thing that helps
		// a backtracking search, which uses the collection it was handed once
		// per *option* and so can never be single-threaded — and it is also the
		// most dangerous thing in the compiler, because a turn that hands back
		// storage something still points at is silent corruption rather than a
		// crash. Every shape here is run with the release and without it.
		{
			"a fold whose turn allocates a Thread it throws away",
			"span 1 200 | braid (b v : max b (span 1 v | bend (x : mul x x) | sum)) 0", "",
		},
		{
			"a fold whose turn allocates text",
			"span 1 120 | braid (b v : max b (len (air v | repeat 3))) 0", "",
		},
		{
			"a fold whose turn builds a Web and a Circle",
			"span 1 80 | braid (b v : add b (span 1 v | braid (w n : put w n n) (web []) | keys | len)) 0", "",
		},
		{
			"a fold nested inside a fold, so a region sits inside a region",
			"span 1 40 | braid (b v : add b (span 1 v | braid (c n : add c (span 1 n | sum)) 0)) 0", "",
		},
		{
			"a fold over a search that copies the collection it is handed",
			"walk xs i best is\n" +
				"  ward i\n" +
				"    4 : min best (xs | sum)\n" +
				"    _ : span 0 2 | braid (b v : walk (mend i v xs) (add i 1) b) best\n" +
				"walk [9 9 9 9] 0 999", "",
		},
		{
			"a fold whose accumulator is text, which is not a shape it may release",
			"span 1 30 | braid (b v : weld b (air v)) \"\" | len", "",
		},
		{
			"a fold whose accumulator is a Thread, likewise",
			"span 1 30 | braid (b v : mend 0 v b) [0 0] | sum", "",
		},
		{
			"a fold reaching a remembered definition, likewise",
			"remember sq n is mul n n\n\nspan 1 60 | braid (b v : add b (sq v)) 0", "",
		},
		{
			"a fold reaching a top-level value, which is forced before the region",
			"nums is span 1 50\n\nspan 1 40 | braid (b v : add b (nth v nums else 0)) 0", "",
		},
		{
			"a gentle whose halves swap",
			"gentle ((a, b) n : pick (lt 8 a) (Woven (b, add a n)) (Gentled (add a b))) (0, 100) (span 1 100) failing 0", "",
		},
		{
			"a gentle whose second half reads the first",
			"gentle ((a, b) n : pick (lt 6 a) (Woven (add a n, add a b)) (Gentled b)) (0, 0) (span 1 100) failing 0", "",
		},
		{
			"a gentle carrying three",
			"gentle ((x, y, z) n : pick (lt 6 x) (Woven (inc x, add y n, z)) (Gentled (add y z))) (0, 0, 9) (span 1 50) failing 0", "",
		},
		{
			"a gentle whose accumulator is not a Twine",
			"gentle (s n : pick (lt 5 s) (Woven (add s 1)) (Gentled n)) 0 (flow inc 1) failing 0", "",
		},
		{
			"a gentle that never gentles",
			"gentle (s n : Woven (add s n)) 0 (span 1 10) failing 0", "",
		},
		{
			"a gentle ending in a ward, which is not a shape it can split",
			"gentle (s n : ward (gt 4 s) (Light : Woven (add s n)) (Shadow : Gentled s)) 0 (span 1 100) failing 0", "",
		},
		{
			"a gentle carrying a Circle it updates in place",
			"gentle ((s, k) n : pick (lt 20 k) (Woven (insert s n, inc k)) (Gentled (len s))) (circle [], 0) (span 1 100) failing 0", "",
		},
		{
			"a gentle carrying a Web and text",
			"gentle ((w, t, k) n : pick (lt 5 k) (Woven (put w n n, weld t \"x\", inc k)) (Gentled t)) (web [], \"\", 0) (span 1 50) failing \"?\"", "",
		},
		{"gentle over a Web", "[1 2 3] | gentle (w n : Woven (put w n n)) (web []) | rescue (web []) | items", ""},
		{"gentle that stops early", "[1 2 3 4 5] | gentle (w n : pick (gt 3 n) (Gentled n) (Woven (put w n n))) (web []) | snag 0", ""},
		{"gentle handing the accumulator out as the answer", "[1 2 3 4] | gentle (v n : pick (gt 2 n) (Gentled v) (Woven (mend 0 n v))) [9 9 9] | snag [] | air", ""},
		{"gentle whose seed is read afterwards", "t is [9 9 9]\njoin \",\" [air ([1 2] | gentle (v n : Woven (mend 0 n v)) t | rescue [] | sum), air (sum t)]", ""},

		// A loop that hands its collection straight back on the turns it does
		// not update now keeps the in-place path. Every one of these has to
		// agree with the copying version, since that is the whole risk: an
		// update written through where a second reference survived would show
		// up here and nowhere else.
		{
			"a map that skips what it already knows",
			"fill [] w is w\nfill [k ..rest] w is pick (known w k) (fill rest w) (fill rest (put w k (mul k 2)))\n" +
				"fill [1 2 1 3 2 4] (web []) | items", "",
		},
		{
			"a set that skips what it has seen",
			"seen [] c is c\nseen [x ..rest] c is pick (member c x) (seen rest c) (seen rest (insert c x))\n" +
				"seen [3 1 3 2 1] (circle []) | members | sort", "",
		},
		{
			"a Link that skips a pair already joined",
			"walk [] l is l\nwalk [(a, b) ..rest] l is pick (bound l a b) (walk rest l) (walk rest (bind l a b))\n" +
				"walk [(0, 1) (2, 3) (1, 2) (0, 3) (4, 5)] (link (span 0 5)) | clumped | bend len | sort", "",
		},
		{
			"a grid that skips the cells already right",
			"g is [\"ab\" \"cd\"] | bend fires | weft ' '\n" +
				"wipe [] p is p\nwipe [k ..rest] p is pick (eq (Held '.') (cell p k)) (wipe rest p) (wipe rest (set p k '.'))\n" +
				"wipe (knots g) g | cells | air", "",
		},
		{
			"a Thread that skips the positions already right",
			"zeroes [] xs is xs\nzeroes [i ..rest] xs is pick (eq (Held 0) (nth i xs)) (zeroes rest xs) (zeroes rest (mend i 0 xs))\n" +
				"zeroes [0 1 2 0 1] [5 0 7] | air", "",
		},
		{
			// The old value must still be there for whoever else held it, so
			// this reads the seed after the loop has run on it.
			"the collection the loop started from is unchanged",
			"w is web [(1, 1)]\nfill 0 m is m\nfill n m is fill (sub n 1) (put m n n)\n" +
				"join \",\" [air (len (fill 5 w)), air (len w)]", "",
		},
		{
			"a Link the loop started from is unchanged",
			"l is link [1 2 3]\njoin \",\" [air (len (clumped (bind l 1 2))), air (len (clumped l))]", "",
		},

		// A collection a loop owns is handed back to the caller, who may keep
		// it. These read the older one *after* the later loop has written, and
		// that ordering is the whole point: read it before and a Thread that
		// escaped still writable looks fine.
		{
			"a Thread out of one fold is the seed of the next",
			"fst p is\n  ward p\n    (a, _) : a\n" +
				"a is [1 2 3] | braid ((v, i) n : (mend i n v, add i 1)) ([9 9 9], 0) | fst\n" +
				"b is [7 7 7] | braid ((v, i) n : (mend i n v, add i 1)) (a, 0) | fst\n" +
				"join \"  \" [air b, air a]", "",
		},
		{
			"a Thread out of a helper is the seed of the next",
			"fill t ks is ks | braid (v i : mend i 0 v) t\n" +
				"a is fill [1 2 3] [0 1]\nb is fill a [2]\n" +
				"join \"  \" [air b, air a]", "",
		},
		{
			"a search that backtracks over a board",
			"fill board ks is ks | braid (b i : mend i 1 b) board\n" +
				"optionsAt pos is [[pos] [pos, add pos 1]]\n" +
				"free board ks is ks | all (i : eq (Held 0) (nth i board))\n" +
				"search board pos depth is\n" +
				"  pick (gt 2 depth) Shadow\n" +
				"    (pick (gte 4 pos) (eq 4 (board | sum))\n" +
				"      (optionsAt pos | any\n" +
				"        (ks : pick (free board ks)\n" +
				"          (search (fill board ks) (add pos 2) (add depth 1)) Shadow)))\n" +
				"join \" \" ([search (copies 6 0) 0 0, search (copies 6 0) 1 0] | bend air)", "",
		},

		// Consumed parameters: a helper handed the collection outright, so the
		// ownership crosses the call. The risk is the same one the disown
		// guards, one call deeper — a result that leaves still writable while
		// the caller kept the value it came from — so every one of these reads
		// the older collection after the later call has run.
		{
			"a helper that updates and hands back",
			"fill t ks is ks | braid (v i : mend i 0 v) t\n" +
				"step 0 t acc is add acc (t | sum)\n" +
				"step n t acc is step (sub n 1) (fill t [0 1 2]) (add acc 1)\n" +
				"step 4 [9 9 9] 0", "",
		},
		{
			"the collection a consuming helper was given survives",
			"fill t ks is ks | braid (v i : mend i 0 v) t\n" +
				"a is fill [1 2 3] [0 1]\nb is fill a [2]\n" +
				"join \"  \" [air b, air a]", "",
		},
		{
			"a consuming helper called from a fold",
			"fill w ks is ks | braid (m k : put m k 1) w\n" +
				"span 1 5 | braid (w n : fill w [n, neg n]) (web []) | len", "",
		},
		{
			"a consuming helper that recurses",
			"wipe [] p is p\nwipe [k ..rest] p is wipe rest (set p k '.')\n" +
				"g is [\"ab\" \"cd\"] | bend fires | weft ' '\n" +
				"h is wipe (knots g) g\njoin \"  \" [cells h | air, cells g | air]", "",
		},
		{
			"a consuming helper of two collection parameters",
			"pour c t is t | braid (a n : insert a n) c\n" +
				"seed is circle [9]\none is pour seed [1 2]\ntwo is pour one [3]\n" +
				"join \",\" [air (len (members two)), air (len (members one)), air (len (members seed))]", "",
		},
		{
			"a chain of two consuming helpers",
			"one t is mend 0 1 t\ntwo t is mend 1 2 (one t)\n" +
				"step 0 t is t\nstep n t is step (sub n 1) (two t)\n" +
				"seed is [9 9 9]\njoin \"  \" [air (step 3 seed), air seed]", "",
		},
		{
			"a consuming helper whose result is read twice",
			"grow l ps is ps | braid (m (a, b) : bind m a b) l\n" +
				"base is link (span 0 5)\njoined is grow base [(0, 1) (2, 3)]\n" +
				"more is grow joined [(1, 2)]\n" +
				"join \",\" [air (len (clumped more)), air (len (clumped joined)), air (len (clumped base))]", "",
		},

		// A collection the loop owns may leave inside a constructor, which is
		// how a search answers: `Held xs` for the vector it solved for. That is
		// the same escape a bare mention is, so the same disown has to happen —
		// and these read the older one after the later loop has written, which
		// is the only ordering that shows a missing one.
		{
			"a Thread handed out inside a Held",
			"setA 0 xs is Held xs\nsetA n xs is setA (sub n 1) (mend n (mul n 10) xs)\n" +
				"setB 0 xs is Held xs\nsetB n xs is setB (sub n 1) (mend n n xs)\n" +
				"a is setA 2 [9 9 9] else []\nb is setB 2 a else []\n" +
				"join \"  \" [air b, air a]", "",
		},
		{
			"a Web handed out inside a declared constructor",
			"Answer is Found (Web Earth Earth) | Missing\n\n" +
				"fill 0 w is Found w\nfill n w is fill (sub n 1) (put w n n)\n" +
				"seen a is\n  ward a\n    Found w : len (keys w)\n    Missing : 0\n" +
				"one is fill 4 (web [])\ntwo is\n  ward one\n    Found w : fill 9 w\n    Missing : Missing\n" +
				"join \",\" [air (seen two), air (seen one)]", "",
		},

		// A read of the collection through a lambda a verb is holding, inside a
		// call whose own result is a Thread. The lambda cannot outlive the
		// call and cannot hand the collection out, so this is a read — the
		// shape day 10's back-substitution is written in.
		{
			"a read through a lambda inside a chain that builds a Thread",
			"back [] wide xs is xs\n" +
				"back [(c, pr) ..rest] wide xs is\n" +
				"  weave used is\n" +
				"    span (add c 1) (sub wide 1)\n" +
				"      as (j : mul (nth j pr else 0) (nth j xs else 0))\n" +
				"      | sum\n" +
				"  back rest wide (mend c used xs)\n" +
				"back [(0, [1 2]) (1, [3 4])] 2 [9 9] | air", "",
		},
		{
			"the Thread that chain started from is unchanged",
			"step [] xs is xs\n" +
				"step [i ..rest] xs is\n" +
				"  weave n is span 0 2 | bend (j : nth j xs | otherwise 0) | sum\n" +
				"  step rest (mend i n xs)\n" +
				"seed is [1 2 3]\n" +
				"join \"  \" [air (step [0 1 2] seed), air seed]", "",
		},

		// A Held of something already on the heap keeps the value where it
		// stands rather than boxing it. Everything that reads a Hold has to
		// agree, including the one shape that still boxes: a Held of a Held.
		{"a Held Thread read back", "nth 0 [[1 2] [3 4]] | otherwise [] | air", ""},
		{"a Held Thread compared", "eq (nth 0 [[1 2]]) (nth 0 [[1 2]])", ""},
		{"a Held Thread ordered", "sort [nth 1 [[9] [1]], nth 0 [[9] [1]]] | bend (h : h | otherwise [] | air)", ""},
		{"a Held Thread shown", "air (nth 0 [[1 2] [3]])", ""},
		{"a Held Thread as a Web key", "w is web [(nth 0 [[1 2]], 7)]\nget w (nth 0 [[1 2]]) | otherwise 0", ""},
		{"a Held Web", "nth 0 [(web [(1, 2)])] | otherwise (web []) | items", ""},
		{"a Held text", "nth 0 [\"ab\" \"cd\"] | otherwise \"\"", ""},
		{"a Held of a Held", "air (nth 0 [(nth 0 [[1 2]])])", ""},
		{"a Stilled beside a Held Thread", "join \",\" [air (nth 9 [[1 2]]), air (nth 0 [[1 2]])]", ""},
		{"a remembered function keyed on a Held Thread", "remember size h is len (h | otherwise [])\nadd (size (nth 0 [[1 2 3]])) (size (nth 0 [[1 2 3]]))", ""},

		// The empty Thread is one object for the whole program, so everything
		// that could write to it or free it has to leave it alone.
		{"the empty Thread welded", "none is []\njoin \",\" [air (weld none [1 2]), air (weld [1 2] none), air none]", ""},
		{"the empty Thread mended", "join \",\" [air (mend 0 1 []), air ([] | len)]", ""},
		{"the empty Thread from two places", "none is []\nand (eq none (span 1 0)) (eq (len none) (len (span 1 0)))", ""},
		{"an empty default read many times", "xs is [[1 2] [3]]\nspan 0 20 | bend (i : nth i xs | otherwise [] | len) | sum", ""},
		{"a function releasing what it built beside an empty default", "xs is [[1 2] [3]]\n" +
			"look i is\n  weave got is nth i xs | otherwise []\n  len got\n" +
			"span 0 20 | bend look | sum", ""},

		// Sorting is Weave's own now — a stable merge sort, and a radix sort
		// past 256 elements when every key is an Earth. Stability is the part
		// that has to be checked at size: the radix path only starts there, and
		// a program that sorts and then walks reads the tie order.
		{"a stable sort by key, past the radix threshold",
			"span 0 999 | bend (i : (mod i 7, i)) | sortby (former) | bend (latter) | take 21 | air", ""},
		{"a stable sort by key, under it",
			"span 0 40 | bend (i : (mod i 7, i)) | sortby (former) | bend (latter) | air", ""},
		{"ties keep the order they were given",
			"[(2, 'a') (1, 'b') (2, 'c') (1, 'd')] | sortby (former) | bend (latter) | air", ""},
		{"negative keys", "[5, neg 3, 0, neg 100, 42, neg 3] | sort | air", ""},
		{"negative keys past the threshold",
			"span 0 600 | bend (i : sub (mul (mod i 13) 1000000) 6000000) | sort | take 20 | air", ""},
		{"one key for everything", "span 0 400 | bend (i : (7, i)) | sortby (former) | bend (latter) | take 8 | air", ""},
		{"keys that are not Earths", "[\"pear\" \"fig\" \"apple\"] | sortby len | air", ""},
		{"sorting Twines", "[(2, 1) (1, 9) (1, 2)] | sort | air", ""},
		{"sorting nothing", "none is []\njoin \",\" [air (sort none), air (sortby (x : x) none)]", ""},
		{"sorting one", "join \",\" [air (sort [3]), air (sortby (x : x) [3])]", ""},
		{"top and bot still read off a sort", "span 1 500 | bend (i : mod (mul i 37) 500) | top 4 | air", ""},

		// A binding may take its value apart, the way a parameter does. The
		// top-level form expands into one hidden definition holding the value
		// and a projection per name, so these check that the value is worked
		// out once and that the names come out of it in the right places.
		{"a local binding takes a Twine apart",
			"f p is\n  weave (a, b) is p\n  add a b\n\nf (1, 2)", ""},
		{"a local binding, nested",
			"f p is\n  weave ((a, b), c) is p\n  join \",\" [air a, air b, air c]\n\nf ((1, 2), 3)", ""},
		{"a local binding feeds later ones",
			"f p is\n  weave (a, b) is p\n  weave s is add a b\n  mul s 2\n\nf (3, 4)", ""},
		{"a local binding of a knot",
			"f k is\n  weave (knot r c) is k\n  add r c\n\nf (knot 3 4)", ""},
		{"a top-level definition takes a Twine apart",
			"(width, height) is (12, 5)\nmul width height", ""},
		{"a top-level definition used before it is written",
			"total is add w h\n(w, h) is (3, 4)\ntotal", ""},
		{"the value behind one is worked out once",
			"pair is span 1 4 | braid ((s, p) x : (add s x, mul p x)) (0, 1)\n" +
				"(s, p) is pair\njoin \",\" [air s, air p]", ""},
		{"two of them do not collide",
			"(a, b) is (1, 2)\n(c, d) is (3, 4)\nadd (add a b) (add c d)", ""},
		{"one inside a loop that owns its collection",
			"step 0 xs p is add (sum xs) (ward p ((a, _) : a))\n" +
				"step n xs p is\n" +
				"  weave (lo, hi) is p\n" +
				"  step (sub n 1) (mend 0 n xs) (add lo 1, hi)\n" +
				"step 5 [9 9 9] (0, 0)", ""},

		// The verbs that answer from one element. Fused they end the loop, so
		// each one has to agree with the runtime verb it replaces — including
		// about a position that is not there, which is where an off-by-one in
		// the count would show.
		{"idx over a generated producer", "span 1 40 | idx 7", ""},
		{"idx after a stage", "span 1 40 | bend (mul this 3) | idx 12", ""},
		{"idx finds nothing", "span 1 40 | bend (mul this 3) | idx 13", ""},
		{"idx counts past the stage", "span 1 40 | sift even | idx 12", ""},
		{"seekidx over a generated producer", "span 1 40 | seekidx (gt 30)", ""},
		{"seekidx finds nothing", "span 1 40 | seekidx (gt 99)", ""},
		{"seekidx counts past the stage", "span 1 40 | sift even | seekidx (gt 10)", ""},
		{"has over a generated producer", "span 1 40 | has 7", ""},
		{"has finds nothing", "span 1 40 | bend (mul this 3) | has 13", ""},
		{"none over a generated producer", "span 1 40 | none (gt 99)", ""},
		{"none finds one", "span 1 40 | none (gt 30)", ""},
		{"nth over a generated producer", "span 1 40 | nth 7", ""},
		{"nth after a stage", "span 1 40 | bend (mul this 3) | nth 7", ""},
		{"nth past the end", "span 1 4 | bend (mul this 3) | nth 9", ""},
		{"nth of a negative position", "span 1 4 | bend (mul this 3) | nth (neg 1)", ""},
		{"nth counts past the stage", "span 1 40 | sift even | nth 3", ""},
		{"second after a stage", "span 1 40 | bend (mul this 3) | second", ""},
		{"second of one element", "span 1 1 | bend (mul this 3) | second", ""},
		{"second of nothing", "span 1 0 | bend (mul this 3) | second", ""},
		// `nth` reads text as well as a Thread, and the loop only walks one.
		{"nth on text", "\"hello\" | nth 1", ""},
		{"nth on text past the end", "\"hi\" | nth 9", ""},

		// `drop` and `dropwhile` are stages now, the mirror of `take` and
		// `takewhile`. Being stages is what lets an endless producer survive
		// one: `cycle xs | drop 7 | first` used to be refused, because the
		// chain broke at the `drop` and the `cycle` had to be built.
		{"an endless chain dropped then read", "cycle [10 20 30] | drop 7 | first | otherwise 0", ""},
		{"a flow dropped then taken", "flow inc 1 | drop 5 | take 3 | air", ""},
		{"an endless chain skipped then read", "cycle [10 20 30] | dropwhile (eq 10) | first | otherwise 0", ""},
		{"drop then sum", "span 1 12 | drop 4 | sum", ""},
		{"drop none", "span 1 12 | drop 0 | sum", ""},
		{"drop past the end", "span 1 12 | drop 99 | sum", ""},
		{"drop a negative count", "span 1 12 | drop (neg 2) | sum", ""},
		{"drop then map then take", "span 1 12 | drop 3 | bend (mul this 2) | take 4 | air", ""},
		{"map then drop", "span 1 12 | bend (mul this 2) | drop 8 | air", ""},
		{"take then drop", "span 1 12 | take 6 | drop 2 | air", ""},
		{"drop twice", "span 1 12 | drop 3 | drop 3 | air", ""},
		{"dropwhile keeping everything", "span 1 12 | dropwhile (gt 99) | air", ""},
		{"dropwhile keeping nothing", "span 1 12 | dropwhile (gt 0) | air", ""},
		{"dropwhile lets a later failure through", "[1 9 1 9] | dropwhile (gt 5) | air", ""},
		{"dropwhile over pairs", "enum [10 20 30] | dropwhile ((i, _) : gt 1 i) | bend (latter) | air", ""},
		{"drop inside a fold", "span 1 20 | drop 5 | braid add 0", ""},
		{"drop is still a window on text", "\"abcdef\" | drop 2", ""},

		// `weld` never names the element type, so it carries `Ply` alongside
		// `take`, `drop`, `sever` and `rev`: text welds as readily as a Thread.
		{"welding text", `join "|" [weld "world" "hello ", weld "" "x", weld "y" ""]`, ""},
		{"welding windows of text", `weld (take 2 "abcdef") (drop 4 "abcdef")`, ""},
		{"welding Threads still works", "weld [3 4] [1 2] | air", ""},
		{"welding runes", `"abc" | fires | weld ['d'] | air`, ""},

		// `turn` shifts a Thread round and `wrap` indexes one that way. Both
		// take the count however far past the length it is, and negative, so
		// the edges are where they earn their keep.
		{"turn by one", "turn 1 [1 2 3 4 5] | air", ""},
		{"turn the other way", "turn (neg 1) [1 2 3 4 5] | air", ""},
		{"turn by none", "turn 0 [1 2 3 4 5] | air", ""},
		{"turn by the whole length", "turn 5 [1 2 3 4 5] | air", ""},
		{"turn further than the length", "join \",\" [air (turn 7 [1 2 3 4 5]), air (turn (neg 7) [1 2 3 4 5])]", ""},
		{"turn nothing", "turn 3 (take 0 [1 2 3]) | air", ""},
		{"turn text", "join \",\" [turn 2 \"abcdef\", turn (neg 1) \"abcdef\", turn 3 \"\"]", ""},
		{"turn text by rune", "turn 1 \"héllo\"", ""},
		{"turn is not a copy of its source", "xs is [1 2 3]\njoin \",\" [air (turn 1 xs), air xs]", ""},
		{"wrap inside", "wrap 2 [1 2 3 4 5] | otherwise 0", ""},
		{"wrap past the end", "wrap 7 [1 2 3 4 5] | otherwise 0", ""},
		{"wrap from the end", "join \",\" [air (wrap (neg 1) [1 2 3 4 5] | otherwise 0), air (wrap (neg 6) [1 2 3 4 5] | otherwise 0)]", ""},
		{"wrap nothing", "wrap 0 (take 0 [1 2 3]) | otherwise 99", ""},
		{"wrap over a turn", "span 0 6 | bend (i : wrap i (turn 2 [1 2 3]) | otherwise 0) | air", ""},

		// `repeat` lays a Thread or some text end to end, so `copies n xs | flat`
		// has one verb again — and it carries Ply, so both work.
		{"repeat a Thread", "repeat 3 [1 2] | air", ""},
		{"repeat text", "repeat 3 \"ab\"", ""},
		{"repeat none of it", "none is []\njoin \",\" [air (repeat 0 [1 2]), air (repeat 2 none), repeat 0 \"ab\"]", ""},
		{"repeat is copies flattened", "and (eq (repeat 4 [1 2 3]) (copies 4 [1 2 3] | flat)) (eq (repeat 4 \"ab\") (copies 4 \"ab\" | join \"\"))", ""},
		{"repeat a Thread of Threads", "repeat 2 [[1 2] [3]] | bend len | air", ""},
		{"repeat into a chain", "repeat 3 [1 2 3] | sum", ""},

		// `nth`, `first` and `last` read text as well as a Thread — by rune,
		// like every other text verb, so a multibyte character is one element.
		{"nth into text", "join \",\" [air (nth 1 \"hello\" else '?'), air (nth 9 \"hello\" else '?')]", ""},
		{"first and last of text", "join \",\" [air (first \"hello\" else '?'), air (last \"hello\" else '?')]", ""},
		{"of empty text", "join \",\" [air (first \"\" else '?'), air (last \"\" else '?'), air (nth 0 \"\" else '?')]", ""},
		{"a multibyte character", "s is \"h\u00e9llo\u2192\"\njoin \",\" [air (nth 1 s else '?'), air (last s else '?'), air (nth 5 s else '?')]", ""},
		{"text and Thread agree", "s is \"abc\"\nand (eq (nth 1 s) (fires s | nth 1)) (eq (last s) (fires s | last))", ""},
		{"a chain over text positions", "s is \"abcde\"\nspan 0 4 | bend (i : nth i s | otherwise '?') | rev | air", ""},
		{"still a Thread verb when nothing says otherwise", "[[1 2] [3]] | bend first | compact | bend air | join \",\"", ""},

		// `under n` is the span 0 to n-1 written the way anyone who wants "the
		// places of n things" writes it, and it is generated rather than built.
		{"under summed", "under 5 | sum", ""},
		{"under mapped", "under 4 | bend (i : mul i i) | air", ""},
		{"under filtered", "under 6 | sift even | air", ""},
		{"under folded", "under 5 | braid add 100", ""},
		{"under none", "join \",\" [air (under 0 | len), air (under (neg 3) | len), air (under 1 | sum)]", ""},
		{"under into a collect", "under 3 | bend inc | air", ""},
		{"under seeking", "under 100 | seek (i : gt 40 (mul i i)) | otherwise 0", ""},
		{"under counted", "under 20 | count (i : eq 0 (mod i 3))", ""},
		{"under with the bound worked out", "n is 9\nunder (div n 2) | sum", ""},
		{"under twice over", "under 3 | bend (i : under (add i 1) | sum) | air", ""},

		// `wind` is `mend` that goes round, the way `wrap` is `nth` that does.
		{"wind in range", "l is [1 2 3 4]\njoin \"  \" [air (wind 1 9 l), air l]", ""},
		{"wind from the end", "air (wind (neg 1) 9 [1 2 3 4])", ""},
		{"wind past the end", "join \",\" [air (wind 5 9 [1 2 3 4]), air (wind (neg 6) 9 [1 2 3 4])]", ""},
		{"wind nothing", "none is []\nair (wind 0 9 none)", ""},
		{"wind agrees with mend where they overlap",
			"l is [1 2 3 4]\nspan 0 3 | all (i : eq (wind i 9 l) (mend i 9 l))", ""},
		{"wind writes through a loop",
			"step 0 xs is sum xs\nstep n xs is step (sub n 1) (wind (neg n) n xs)\nstep 8 [0 0 0]", ""},
		{"a ring swap, read round and written round",
			"swap a b l is\n" +
				"  weave x is wrap a l else 0\n" +
				"  weave y is wrap b l else 0\n" +
				"  wind b x (wind a y l)\n" +
				"rvrs l i t is\n" +
				"  under (div t 2) | braid (m k : swap (add i k) (add i (sub t (add k 1))) m) l\n" +
				"rvrs [0 1 2 3 4] 3 4 | air", "",
		},

		// A Link built and read without a loop in sight.
		{"clumped after two binds", "bind (bind (link [1 2 3 4]) 1 2) 3 4 | clumped | bend len | sum", ""},
		{"bound over a chain", "bound (bind (bind (link [1 2 3]) 1 2) 2 3) 1 3", ""},

		// `couples` yields pairs for the same reason `zip` does, and every
		// caller takes the pair apart, so the Twine is never built. Its two
		// indices advance by hand, which is the part worth checking at the
		// edges: two elements, one, none.
		{"couples mapped", "couples [1 2 3 4] | bend ((a, b) : add a b) | sum", ""},
		{"couples filtered", "couples [1 2 3 4] | sift ((a, b) : eq 1 a) | len", ""},
		{"couples into a fold", "couples [1 2 3] | braid (n (a, b) : add n (mul a b)) 0", ""},
		{"couples collected whole", "couples [1 2 3] | sift ((a, _) : gt 0 a)", ""},
		{"couples counted", "couples (span 1 20) | count ((a, b) : eq 7 (add a b))", ""},
		{"couples of two", "couples [1 2] | bend ((a, b) : add a b) | sum", ""},
		{"couples of one", "couples [1] | bend ((a, b) : add a b) | len", ""},
		{"couples of none", "couples [] | bend ((a, b) : add a b) | len", ""},
		{"couples take", "couples (span 1 9) | take 4 | bend ((a, b) : add a b) | sum", ""},
		{"couples seeking", "couples (span 1 9) | seek ((a, b) : eq 12 (mul a b))", ""},
		{"couples of Twines", "couples [(1, 2) (3, 4) (5, 6)] | bend (((a, _), (c, _)) : add a c) | sum", ""},

		// `enum` is the most-written pair producer of the three, and its Twine
		// is nearly always taken apart by the very next stage.
		{"enum mapped", `enum ["x" "y" "z"] | bend ((i, v) : join ":" [air i, v]) | join ","`, ""},
		{"enum filtered", "enum [5 6 7] | sift ((i, _) : odd i) | bend ((_, v) : v) | sum", ""},
		{"enum into a fold", "enum [5 6 7] | braid (n (i, v) : add n (mul i v)) 0", ""},
		{"enum collected whole", "enum [5 6 7] | sift ((i, _) : gt 0 i)", ""},
		{"enum counted", "enum [5 6 7 8] | count ((i, v) : eq i (sub v 5))", ""},
		{"enum of nothing", "enum [] | bend ((i, v) : add i v) | len", ""},
		{"enum of one", "enum [9] | bend ((i, v) : add i v) | sum", ""},
		{"enum seeking", "enum [5 6 7] | seek ((i, v) : eq 7 v)", ""},
		{"enum of Twines", "enum [(1, 2) (3, 4)] | bend ((i, (a, b)) : add i (add a b)) | sum", ""},

		// A single stage or consumer whose function is written out on the spot
		// is fused for the closure and the indirect call, not for an array.
		{"one lambda stage collects", "[1 2 3] | bend (x : mul x 2)", ""},
		{"one lambda stage sifts", "[1 2 3 4] | sift (x : odd x)", ""},
		{"a named function stage is left alone", "twice n is mul n 2\n[1 2 3] | bend twice", ""},
		{"a lambda predicate counts", "[1 2 3 4] | count (x : odd x)", ""},
		{"a lambda predicate seeks", "[1 2 3 4] | seek (x : gt 2 x)", ""},
		{"a lambda predicate over nothing", "[] | all (x : gt 0 x)", ""},
		{"a lambda predicate that captures", "answer is\n  weave k is 3\n  [1 2 3 4] | any (x : eq k x)\nanswer", ""},

		// The grid producers. `knots` builds one Knot per cell, and a
		// neighbour verb builds a whole Thread per cell it is asked about —
		// which is why these fuse with no stages at all, and why they are the
		// one place a fused loop can differ from the verb in more than speed.
		// Every corner of the grid has to be checked, since the fused loop does
		// its own bounds test rather than borrowing the verb's.
		{"knots counted", "g is Source | pattern\nknots g | count (k : eq 0 (col k))", "abc\ndef\n"},
		{"knots sifted and collected", "g is Source | pattern\nknots g | sift (k : eq (row k) (col k)) | bend row", "abc\ndef\nghi\n"},
		{"knots into a fold", "g is Source | pattern\nknots g | braid (n k : add n (row k)) 0", "ab\ncd\n"},
		{"knots of a one-cell grid", "g is Source | pattern\nknots g | len", "x"},
		{"knots reading its own cells", "g is Source | pattern\nknots g | count (k : eq (Held '#') (cell g k))", ".#.\n##.\n"},
		{"nb4 counted at a corner", "g is Source | pattern\nnb4 g (knot 0 0) | count (eq '#')", "##\n#.\n"},
		{"nb8 counted at a corner", "g is Source | pattern\nnb8 g (knot 0 0) | count (eq '#')", "##\n##\n"},
		{"nb8 counted in the middle", "g is Source | pattern\nnb8 g (knot 1 1) | count (eq '#')", "###\n#.#\n###\n"},
		{"nb4 collected", "g is Source | pattern\nnb4 g (knot 1 1) | bend (c : c) | air", "abc\ndef\nghi\n"},
		{"nb8 of a one-cell grid", "g is Source | pattern\nnb8 g (knot 0 0) | len", "x"},
		{"nb4 outside the grid", "g is Source | pattern\nnb4 g (knot 9 9) | len", "ab\ncd\n"},
		{"nb4 at the far corner", "g is Source | pattern\nnb4 g (knot 1 1) | air", "ab\ncd\n"},
		{"around4 collected", "g is Source | pattern\naround4 g (knot 0 0) | bend col | sum", "abc\ndef\n"},
		{"around8 sifted", "g is Source | pattern\naround8 g (knot 1 1) | sift (k : eq 0 (row k)) | len", "abc\ndef\nghi\n"},
		{"around8 seeking", "g is Source | pattern\naround8 g (knot 1 1) | seek (k : eq (Held 'a') (cell g k))", "abc\ndef\nghi\n"},
		{
			// A neighbour walk under a fold is deliberately left unfused: the
			// accumulator may be the very grid being read, and an in-place
			// `set` would then be seen by the rest of the walk. The verb copies
			// its values out first, so this has to keep agreeing with it.
			"a neighbour walk folding into the grid it reads",
			"g is Source | pattern\n" +
				"nb4 g (knot 0 0) | braid (q c : set q (knot 1 1) c) g | cells | air",
			"ab\ncd\n",
		},
		{
			"a whole erosion, which is every grid verb at once",
			"g is Source | pattern\n" +
				"thin p is knots p | braid (q k : pick (lt 3 (nb4 p k | count (eq '#'))) (set q k '.') q) p\n" +
				"settle thin g | cells | count (eq '#')",
			"..#..\n.###.\n..#..\n",
		},

		// Threads a function builds and does not hand back are released at its
		// return. Each of these has to mean the same with that turned off.
		{
			"released locals",
			"tot xs is\n" +
				"  weave a is bend (x : mul x 2) xs\n" +
				"  weave b is zipwith (p q : add p q) a (drop 1 a)\n" +
				"  weave c is rev b\n" +
				"  braid (s x : add s x) 0 c\n" +
				"tot (span 1 40)", "",
		},
		{
			"a released local feeding a map that is returned",
			"seen xs is\n" +
				"  weave a is bend (x : mod x 7) xs\n" +
				"  weave b is rev a\n" +
				"  braid (w x : put w x (mul x 2)) (web []) b\n" +
				"span 1 30 | seen | items", "",
		},
		{
			"a local that escapes in the result is kept",
			"keepit xs is\n  weave a is bend (x : mul x 3) xs\n  a\n" +
				"keepit (span 1 10)", "",
		},
		{
			"a slice of a released local",
			"tot xs is\n" +
				"  weave a is bend (x : add x 1) xs\n" +
				"  add (sum (drop 2 a)) (sum (take 3 a))\n" +
				"tot (span 1 20)", "",
		},
		{
			"released locals in a clause that is not the first",
			"pick2 0 xs is 0\n" +
				"pick2 n xs is\n" +
				"  weave a is bend (x : mul x n) xs\n" +
				"  sum (rev a)\n" +
				"pick2 3 (span 1 10)", "",
		},
		// A `Held` of one of the Powers keeps the value where it stands rather
		// than boxing it, which means `Held 0` and `Stilled` are told apart by
		// something other than the pointer field being zero. Every one of these
		// would have gone wrong if a Held 0 still looked like a Stilled.
		{"a Held zero is not Stilled", "get (web [(1, 0)]) 1 | otherwise 99", ""},
		{"a missing key still is", "get (web [(1, 0)]) 9 | otherwise 99", ""},
		{"nth of a zero", "[0 1 2] | nth 0 | otherwise 99", ""},
		{"seek finding a zero", "[0 0 0] | seek (x : eq 0 x) | otherwise 99", ""},
		{"cell holding a zero", `cell (pattern "012") (knot 0 0) | otherwise 'z'`, ""},
		{"compact keeps zeroes", "[(Held 0), Stilled, (Held 5)] | compact", ""},
		{"glean keeps zeroes", `["0" "x" "5"] | glean earth`, ""},
		{"holds a zero", "(holds (Held 0), holds Stilled)", ""},
		{"harvest over a zero", `["0" "5"] | harvest earth | air`, ""},
		{"harvest that fails", `["0" "z"] | harvest earth | air`, ""},
		{"a Hold renders", "(air (Held 0), air Stilled, air (Held 1.5))", ""},
		{"Holds compare", "(eq (Held 0) (Held 0), eq (Held 0) Stilled, lt (Held 1) (Held 0))", ""},
		{"Holds as map keys", `web [((Held 0), "a"), (Stilled, "b")] | items | air`, ""},
		{"a Held Water", "Held 1.5 | otherwise 0.0", ""},
		{"a Held Knot", "Held (knot 2 3) | otherwise (knot 0 0) | air", ""},
		{"a Held Spirit", "(Held Shadow | otherwise Light, Held Light | otherwise Shadow)", ""},
		{"a Held Thread still boxes", "Held [1 2] | otherwise []", ""},
		{"a Held Air still boxes", `Held "text" | otherwise ""`, ""},
		{"first of nothing", "[] | first | otherwise 42", ""},
		{"a ward over a Held zero", "answer is\n  ward get (web [(1, 0)]) 1\n    Held n : n\n    Stilled : 99\nanswer", ""},

		// A fused sum or product over Earths starts at its identity and adds
		// unconditionally; anything else waits for the first element, because
		// `sum` of an empty Thread answers an Earth 0 whatever the element
		// type. These pin both halves of that, and the empty cases especially.
		{"sum of nothing", "[] | bend (x : mul x 2) | sum", ""},
		{"product of nothing", "[] | bend (x : mul x 2) | prod", ""},
		{"sum of Waters", "[1.5 2.5] | bend (x : mul x 2.0) | sum", ""},
		{"sum of no Waters", "[] | bend (x : mul x 2.0) | sum", ""},
		{"product of no Waters", "[] | bend (x : mul x 2.0) | prod", ""},
		{"sum filtered down to nothing", "span 1 10 | sift (x : gt 100 x) | sum", ""},
		{"product filtered down to nothing", "span 1 10 | sift (x : gt 100 x) | prod", ""},
		{"sum with negatives", "[(neg 3) 5 (neg 7)] | bend (x : x) | sum", ""},
		{"sum over a span", "span 1 100 | bend (x : mul x x) | sift even | sum", ""},
		{"product over a span", "span 1 8 | bend (x : x) | sift odd | prod", ""},

		// Building and editing a Thread. `weld` is also how you append and
		// prepend; `mend` leaves the Thread alone when the position is not
		// there; `sever` and `strands` share the original's storage, which is
		// why neither is on the escape analysis's fresh-array list.
		{"weld two Threads", "[1 2 3] | weld [10 20]", ""},
		{"weld appends one", "span 1 4 | weld [99]", ""},
		{"weld prepends one", "[0] | weld (span 1 4)", ""},
		{"weld with an empty side", "([] | weld (span 1 3), span 1 3 | weld [])", ""},
		{"weld then fold", "span 1 5 | weld (span 6 10) | sum", ""},
		{"mend a position", "span 1 5 | mend 2 99", ""},
		{"mend past the end leaves it alone", "span 1 5 | mend 9 99", ""},
		{"mend the first", "span 1 5 | mend 0 7 | sum", ""},
		{"sever in two", "span 1 5 | sever 2 | air", ""},
		{"sever at the ends", "(span 1 5 | sever 0 | air, span 1 5 | sever 99 | air)", ""},
		{"strands of equal neighbours", "[1 1 2 2 2 3 1] | strands (x : x) | bend len", ""},
		{"strands over text", `"aaabbc" | fires | strands (c : c) | bend len`, ""},
		{"strands of nothing", "[] | strands (x : x) | len", ""},
		{"strands by a derived key", "span 1 12 | strands (x : div x 4) | bend len", ""},
		{"plait two Threads", "[1 2 3] | plait [10 20]", ""},
		{"plait then sum", "span 1 6 | plait (span 10 15) | sum", ""},
		{"cull alone", "span 1 10 | cull even", ""},
		{"cull fuses like sift", "span 1 40 | cull even | bend (x : mul x 3) | sum", ""},
		{"cull then sift", "span 1 40 | cull (gt 30) | sift even | len", ""},
		{"cull everything", "span 1 10 | cull (x : Light) | len", ""},

		// `split ""` gives one piece per character, as text. It used to hand
		// back `fires`, which is a Thread of Fires wearing the type of a Thread
		// of Air, and the first verb to treat one as text read a character code
		// as a pointer.
		{"split on nothing is text", `Source | split "" | join ","`, "1234567"},
		{"split on nothing then read a number", `Source | split "" | first | otherwise "0" | earth | otherwise 0`, "1234567"},
		{"split on nothing sums its digits", `Source | split "" | glean earth | sum`, "1234567"},
		{"split on nothing counts characters", `Source | split "" | len`, "héllo"},
		{"split on nothing keeps characters whole", `Source | split "" | join "|"`, "héllo"},
		{"split on nothing over empty input", `Source | split "" | len`, ""},
		{"split on nothing compares as text", `Source | split "" | sift (c : eq "l" c) | len`, "héllo"},

		// `zipwith` is a producer that reads two Threads and combines on the
		// spot, so its function is inlined rather than called through a closure.
		{"zipwith alone", "zipwith (p q : add p q) [1 2 3 4 5] [10 20 30]", ""},
		{"zipwith uneven the other way", "zipwith (p q : add p q) [10 20] (span 1 9)", ""},
		{"zipwith into a fold", "zipwith (p q : mul p q) (span 1 20) (span 5 24) | sum", ""},
		{"zipwith then a filter", "zipwith (p q : add p q) (span 1 30) (span 1 30) | sift (gt 20) | len", ""},
		{"zipwith with a bare verb", "zipwith add (span 1 10) (span 1 10)", ""},
		{"zipwith with a user function", "both p q is add (mul p 10) q\nzipwith both (span 1 6) (span 1 6) | sum", ""},
		{"zipwith over itself shifted", "answer is\n  weave xs is span 1 12 | bend (x : mul x x)\n  zipwith (p q : sub q p) xs (drop 1 xs)\nanswer", ""},
		{"zipwith making pairs", `zipwith (p q : (p, q)) [1 2 3] ["a" "b" "c"] | air`, ""},
		{"zipwith on empty", "zipwith (p q : add p q) [] (span 1 5) | len", ""},
		{"zipwith with take", "zipwith (p q : add p q) (span 1 99) (span 1 99) | take 4 | prod", ""},
		{"zipwith into a seek", "zipwith (p q : mul p q) (span 1 40) (span 1 40) | seek (gt 100)", ""},
		{"zipwith over Waters", "zipwith (p q : add p q) [1.5 2.5] [0.25 0.75] | sum", ""},
		{"zipwith of text", `zipwith (p q : join "" [p, q]) ["a" "b"] ["x" "y"] | join ","`, ""},
		{"zipwith feeding another zipwith", "answer is\n  weave a is span 1 10\n  weave b is zipwith (p q : add p q) a a\n  zipwith (p q : sub q p) b a | sum\nanswer", ""},

		// A map's entries come back in ascending key order, whichever
		// representation it used — which is the whole point of sorting them, so
		// the flat table and the trie have to agree on every key type.
		{"order of Earth keys", "span 1 200 | braid (w x : put w (mod (mul x 7919) 1000) x) (web []) | keys", ""},
		{"order of negative Earth keys", "[(neg 5), 3, (neg 100), 0, 77] | braid (w x : put w x (mul x x)) (web []) | items", ""},
		{"order of Knot keys", "[(knot 2 1), (knot 0 5), (knot 2 0), (knot (neg 1) 3)] | braid (w k : put w k 1) (web []) | keys", ""},
		{"order of Fire keys", `"the quick brown fox" | fires | braid (w c : put w c 1) (web []) | keys`, ""},
		{"order of Air keys", `["pear", "apple", "fig"] | braid (w t : put w t (len t)) (web []) | items`, ""},
		{"order of Twine keys", "[(2, 1), (1, 9), (2, 0)] | braid (w k : put w k 1) (web []) | keys", ""},
		{"order of Circle members", "span 1 50 | braid (c x : insert c (mod (mul x 31) 17)) (circle []) | members", ""},
		{"order of a one-entry map", `web [(42, "x")] | items`, ""},
		{"order of an empty map", "web [] | keys | len", ""},
		{"order of keys that share their low bytes", "[256, 512, 65536, 1, 16777216] | braid (w x : put w x 1) (web []) | keys", ""},
		{"order of vals follows the keys", "span 1 40 | braid (w x : put w (mod (mul x 37) 41) (mul x 10)) (web []) | vals", ""},

		// delve: `{}` keeps a run, everything else has to match, and the shape
		// has to account for the whole line.
		{"delve two runs", `"Game 11: 3 blue, 4 red" | delve "Game {}: {}" | air`, ""},
		{"delve a separator", `"3-5" | delve "{}-{}" | air`, ""},
		{"delve stops at the first match", `"a-b-c" | delve "{}-{}" | air`, ""},
		{"delve that does not match", `"nope" | delve "{}-{}" | air`, ""},
		{"delve an exact shape", `("exact" | delve "exact" | air, "wrong" | delve "exact" | air)`, ""},
		{"delve anchors the end", `("5 red" | delve "{} red" | air, "5 redx" | delve "{} red" | air)`, ""},
		{"delve empty shape", `("" | delve "" | air, "x" | delve "" | air)`, ""},
		{"delve the whole rest", `"trailing" | delve "trail{}" | air`, ""},
		{"delve three runs", `"1,2,3" | delve "{},{},{}" | air`, ""},
		{"delve runs with nothing between", `"ab" | delve "{}{}" | air`, ""},
		{"delve over a file", "Source\n  | lines\n  | glean (l : delve \"{}-{}\" l)\n  | bend (p : p | harvest earth | rescue [] | sum)\n  | sum", "3-5\nnot a range\n8-1\n"},

		{
			"a local handed to a remembered function is kept",
			"remember tally xs is sum xs\n" +
				"go n is\n  weave a is bend (x : mul x n) (span 1 10)\n  tally a\n" +
				"add (go 2) (go 2)", "",
		},
		{"nested chains", "[[1 2 3] [4 5 6]] | bend (r : r | bend (x : mul x 2) | sum) | sift (gt 10) | sum", ""},
		{"chain over input", "Source | lines | bend len | sift (gt 2) | sum", "a\nbbb\ncccc\n"},
		{"chain over fires", "Source | fires | sift isDigit | bend (c : 1) | sum", "a1b22c333"},
		{"chain feeding a collection", "span 1 20 | bend (x : mod x 5) | sift even | freq | len", ""},
		{"text chain", `Source | lines | bend strip | sift (l : gt 0 (len l)) | bend air | join ","`, " a \n\n b \n"},

		// Shapes that exercise the typed primitive helpers.
		{"earth arithmetic", "answer is sub (mul (add 3 4) 5) (div 20 4)\nanswer", ""},
		{"earth division and remainder", "(div 17 5, mod 17 5, div (neg 17) 5, mod (neg 17) 5)", ""},
		{"water arithmetic", "(add 1.5 2.25, sub 1.5 2.25, mul 1.5 2.0, div 1.5 0.5)", ""},
		{"earth comparisons", "n is 5\n(lt 10 n, lte 5 n, gt 1 n, gte 5 n, eq 5 n, neq 5 n)", ""},
		{"water comparisons", "n is 1.5\n(lt 2.0 n, gt 1.0 n, eq 1.5 n)", ""},
		{"spark comparisons", "c is 'm'\n(lt 'z' c, gt 'a' c, eq 'm' c)", ""},
		{"spirit logic", "(and Light Shadow, or Light Shadow, not Light, eq Light Light)", ""},
		{"min max and abs", "(min 3 7, max 3 7, abs (neg 9), min 1.5 0.5, max 1.5 0.5)", ""},
		{"even odd divBy", "(even 4, odd 4, divBy 3 9, divBy 3 10)", ""},
		{"mixed comparison in a fold", "span 1 30 | sift (gt 10) | bend (x : sub x 1) | braid (a b : max a b) 0", ""},
		{"polymorphic function keeps the general verbs", "same x is x\n(same 1, same 1.5, same 'c')", ""},
		{"comparison inside a user function", "big n is gt 100 n\nspan 1 200 | sift big | len", ""},
		{"knot comparison stays general", "eq (knot 1 2) (knot 1 2)", ""},
		{"air comparison stays general", `(eq "ab" "ab", lt "b" "a")`, ""},

		// take and takewhile as fused stages, which is also what bounds a flow.
		{"take", "span 1 100 | bend (x : mul x 2) | take 5", ""},
		{"take more than there is", "span 1 3 | bend (x : mul x 2) | take 10", ""},
		{"take none", "span 1 10 | bend (x : mul x 2) | take 0", ""},
		{"take after a filter", "span 1 100 | sift even | bend (x : mul x 3) | take 4", ""},
		{"take before a filter", "span 1 100 | take 10 | sift even | sum", ""},
		{"takewhile", "span 1 100 | bend (x : mul x x) | takewhile (gt 200) | sum", ""},
		{"takewhile stops at once", "span 1 10 | bend (x : mul x x) | takewhile (gt 0) | len", ""},
		{"takewhile then take", "span 1 100 | takewhile (gt 50) | take 3 | prod", ""},

		// `flow`, which only exists as a fused loop.
		{"flow take", "flow (add 10) 0 | take 5", ""},
		{"flow map take", "flow (add 1) 1 | bend (x : mul x x) | take 6 | sum", ""},
		{"flow filter take", "flow (add 1) 1 | sift even | take 4", ""},
		{"flow seek", "flow (mul 2) 1 | seek (gt 1000)", ""},
		{"flow first", "flow (add 3) 2 | sift (x : eq 0 (mod x 5)) | first", ""},
		{"flow any", "flow (add 1) 1 | any (gt 99)", ""},
		{"flow takewhile", "flow (mul 3) 1 | takewhile (gt 500) | len", ""},
		{
			"flow over a user function",
			"next n is pick (even n) (div n 2) (add 1 (mul 3 n))\n" +
				"flow next 27 | takewhile (neq 1) | len", "",
		},
		{
			"flow over a pair, which is how a Fibonacci sequence is written",
			"fst p is\n  ward p\n    (a, _) : a\n" +
				"flow ((a, b) : (b, add a b)) (0, 1) | take 12 | bend fst",
			"",
		},

		// `_` standing in for an argument.
		{"hole in a filter", "span 1 20 | sift (mod _ 3 | eq 0) | sum", ""},
		{"hole in a map", "span 1 20 | bend (mul _ _) | sift (gt 100 _) | sum", ""},
		{"hole after where", "span 1 20 where (mod _ 4 | eq 0) | len", ""},
		{"hole placing the piped value", `web [("a", 1) ("b", 2)] | get _ "b" | otherwise 0`, ""},
		{"hole used twice", "span 1 10 | bend (add _ _) | sum", ""},

		// Declared sum types, through the same chains.
		{
			"declared constructor stage",
			"Box is Wrap Earth\nspan 1 10 | bend Wrap | len", "",
		},
		{
			"declared type through a fused chain",
			"Sign is Pos | Zero | Neg\n" +
				"sign n is pick (gt 0 n) Pos (pick (lt 0 n) Neg Zero)\n" +
				"span (neg 5) 5 | bend sign | sift (eq Pos) | len", "",
		},
		{
			"matching a declared type inside a fold",
			"Move is Step Earth Earth | Rest\n" +
				"dist m is\n  ward m\n    Step a b : add (abs a) (abs b)\n    Rest : 0\n" +
				"[Step 1 2, Rest, Step 3 (neg 4)] | bend dist | braid (a b : add a b) 0", "",
		},
		{
			"declared types as collection keys",
			"Colour is Red | Green | Blue\n" +
				"[Red, Blue, Red, Green] | freq | items | sort", "",
		},
		{
			"a recursive type over the input",
			"Tree is Leaf | Node Tree Earth Tree\n" +
				"build 0 is Leaf\nbuild n is Node (build (sub n 1)) n (build (sub n 1))\n" +
				"total Leaf is 0\ntotal (Node l v r) is add v (add (total l) (total r))\n" +
				"total (build 5)", "",
		},

		// A verb standing on its own as a stage, which has no operand to read a
		// type off and so needs the type of the mention itself.
		{"bare even as a filter", "span 1 200 | bend (x : mul x 3) | sift even | sum", ""},
		{"bare odd as a filter", "span 1 200 | bend (x : add x 1) | sift odd | len", ""},
		{"bare abs as a map", "span (neg 50) 50 | bend abs | sift even | sum", ""},
		{"bare neg as a map", "span 1 50 | bend neg | sift (lt 0) | sum", ""},
		{"bare not over Spirits", "span 1 50 | bend even | bend not | count (eq Light)", ""},
		{"bare even as a counter", "span 1 200 | bend (x : mul x 7) | count even", ""},
		{"bare odd inside any", "span 1 200 | bend (x : mul x 2) | any odd", ""},
		{"bare even over Water stays general", "[1.5 2.5] | bend (x : mul x 2.0) | sift (gt 2.0) | sum", ""},
		{
			"a stage whose type is polymorphic keeps the general verb",
			"same x is x\nspan 1 50 | bend same | sift even | sum", "",
		},

		// Map and set updates, which are path-copied unless the compiler can
		// prove the collection is threaded without ever being duplicated. Every
		// shape here can still see the collection it started from.
		{"map loop", mapLoop + "len (fill 200 (web []))", ""},
		{
			// The map handed in must survive the loop untouched.
			"the source map is untouched", mapLoop +
				"base is web [(0, 0)]\nafter is fill 50 base\n(len base, len after)", "",
		},
		{
			// Two loops from the same starting map must not interfere.
			"two loops from one map", mapLoop +
				"base is web [(0, 0)]\na is fill 20 base\nb is fill 40 base\n" +
				"(len base, len a, len b)", "",
		},
		{
			// A map the loop returned is shared again, so updating it must not
			// disturb what the first result holds.
			"a returned map is shared again", mapLoop +
				"a is fill 20 (web [])\nb is fill 40 a\n(len a, len b)", "",
		},
		{
			// Reading the keys out shares the map's storage, so from there on
			// the loop has to copy.
			"taking the keys shares the map",
			"walk 0 w acc is acc\n" +
				"walk n w acc is walk (sub n 1) (put w n n) (add acc (len (keys w)))\n" +
				"walk 40 (web []) 0", "",
		},
		{
			// Binding the map to another name gives it a second owner, so the
			// analysis has to give up.
			"binding the map to another name",
			"walk 0 w is len w\n" +
				"walk n w is\n  weave old is w\n  add (len old) (walk (sub n 1) (put w n n))\n" +
				"walk 30 (web [])", "",
		},
		{
			"set loop",
			"seen 0 c is len c\nseen n c is seen (sub n 1) (insert c n)\n" +
				"seen 500 (circle [])", "",
		},
		{
			"the source set is untouched",
			"seen 0 c is c\nseen n c is seen (sub n 1) (insert c n)\n" +
				"base is circle [0]\nafter is seen 50 base\n(len base, len after)", "",
		},
		{
			// Values that are themselves collections are shared through the
			// outer map, so nothing may write through them.
			"a map of maps",
			"walk 0 w is len (keys w)\n" +
				"walk n w is walk (sub n 1) (put w n (web [(n, n)]))\n" +
				"walk 60 (web [])", "",
		},
		{
			// Every value read back has to be the one that was put in.
			"every entry reads back",
			"walk 0 w is w\nwalk n w is walk (sub n 1) (put w n (mul n 3))\n" +
				"w is walk 300 (web [])\n" +
				"span 1 300 | bend (k : get w k | otherwise 0) | sum", "",
		},
		{
			// Replacing an existing key must not grow the map.
			"replacing keys",
			"walk 0 w is (len w, get w 5 | otherwise 0)\n" +
				"walk n w is walk (sub n 1) (put w (mod n 10) n)\n" +
				"walk 400 (web [])", "",
		},
		{
			// Removing from a map the loop owns.
			"removing while owned",
			"walk 0 w is len w\n" +
				"walk n w is walk (sub n 1) (forget (put w n n) (sub n 1))\n" +
				"walk 200 (web [])", "",
		},

		// Grid updates. These are the shapes in-place updating must not break:
		// every one of them can still see the pattern it started from.
		{
			"pattern loop", gridLoop + "cells (step sheet 40) | count (eq '#')",
			gridInput,
		},
		{
			// The memoised pattern must survive being threaded through the loop.
			"the source pattern is untouched", gridLoop +
				"after is step sheet 40\n" +
				"(cells sheet | count (eq '#'), cells after | count (eq '#'))",
			gridInput,
		},
		{
			// Two loops over the same starting pattern must not interfere.
			"two loops from one pattern", gridLoop +
				"a is step sheet 20\nb is step sheet 40\n" +
				"(cells a | count (eq '#'), cells b | count (eq '#'), cells sheet | count (eq '#'))",
			gridInput,
		},
		{
			// A pattern the loop returned is shared again, so updating it must not
			// disturb what the first result holds.
			"a returned pattern is shared again", gridLoop +
				"a is step sheet 20\nb is step a 40\n" +
				"(cells a | count (eq '#'), cells b | count (eq '#'))",
			gridInput,
		},
		{
			// Reading during the loop has to see the updates so far.
			"loop that reads as it writes", `
sheet is Source through pattern

step g n is
  ward n
    0 : g
    _ :
      weave here is cell g (knot (mod n 7) (mod n 7)) | otherwise '.'
      step (set g (knot (mod n 7) (mod n 7)) (pick (eq '#' here) '.' '#')) (sub n 1)

cells (step sheet 40) | count (eq '#')
`, gridInput,
		},
		{
			// `cells` shares the buffer, so the pattern must stop being writable.
			"cells taken during the loop", `
sheet is Source through pattern

step g n acc is
  ward n
    0 : acc
    _ : step (set g (knot 0 (mod n 5)) '#') (sub n 1) (add acc (count (eq '#') (cells g)))

step sheet 20 0
`, gridInput,
		},
		{
			// A pattern named twice in the call cannot be updated in place.
			"pattern used twice in the tail call", `
sheet is Source through pattern

step g n acc is
  ward n
    0 : acc
    _ : step (set g (knot 0 0) '#') (sub n 1) (add acc (rows g))

step sheet 10 0
`, gridInput,
		},
		{
			// The Thread half of the same proof. Reading during the loop has to
			// see the updates so far, and the answer must be what copying gives.
			"thread mended through a loop", `
step t n is
  ward n
    0 : t
    _ :
      weave here is nth (mod n 7) t | otherwise 0
      step (mend (mod n 7) (add here n) t) (sub n 1)

step (span 1 20) 60 | sum
`, "",
		},
		{
			// `take` hands back a window on the same buffer, so the Thread must
			// stop being writable the moment one is named.
			"a slice taken during the loop", `
step t n acc is
  ward n
    0 : acc
    _ : step (mend 0 n t) (sub n 1) (add acc (sum (take 3 t)))

step (span 1 20) 20 0
`, "",
		},
		{
			// A Thread named twice in the call cannot be updated in place.
			"thread used twice in the tail call", `
step t n acc is
  ward n
    0 : acc
    _ : step (mend 0 n t) (sub n 1) (add acc (len t))

step (span 1 20) 20 0
`, "",
		},
		{
			"thread mended outside any loop", `
a is mend 0 99 (span 1 5)
b is mend 1 88 (span 1 5)
c is span 1 5
(sum a, sum b, sum c)
`, "",
		},
		{
			// Taking keys out of a map in a loop writes through, on the same
			// terms putting them in does. The flat table has to survive it:
			// turning the map into a trie to remove one key would cost the
			// whole map and slow every lookup after.
			"keys forgotten through a loop", `
step m n is
  ward n
    0 : len m
    _ : step (forget m (mod n 7)) (sub n 1)

step (span 1 20 | bend (k : (k, k)) | web) 20
`, "",
		},
		{
			"values removed from a circle through a loop", `
step c n is
  ward n
    0 : members c | sum
    _ : step (remove c (mod n 7)) (sub n 1)

step (circle (span 1 20)) 20
`, "",
		},
		{
			// A key that was never there must not lose the map its ownership,
			// nor quietly copy it every turn.
			"forgetting what was never there", `
step m n is
  ward n
    0 : len m
    _ : step (forget m (add n 1000)) (sub n 1)

step (span 1 20 | bend (k : (k, k)) | web) 20
`, "",
		},
		{
			"pattern updated outside any loop", `
sheet is Source through pattern
a is set sheet (knot 0 0) '#'
b is set sheet (knot 1 1) '#'
(cells a | count (eq '#'), cells b | count (eq '#'), cells sheet | count (eq '#'))
`, gridInput,
		},
	}

	// Everything off is the reference; each optimisation alone, and all of them
	// together, must agree with it.
	off := build.Options{
		DisableFusion: true, DisableSpecialize: true,
		DisableInPlace: true, DisableRelease: true,
	}
	settings := []struct {
		label string
		opts  build.Options
	}{
		{"all optimisations", build.Options{}},
		{"fusion only", build.Options{DisableSpecialize: true, DisableInPlace: true, DisableRelease: true}},
		{"specialisation only", build.Options{DisableFusion: true, DisableInPlace: true, DisableRelease: true}},
		{"in-place only", build.Options{DisableFusion: true, DisableSpecialize: true, DisableRelease: true}},
		{"releasing only", build.Options{DisableFusion: true, DisableSpecialize: true, DisableInPlace: true}},
	}

	for _, p := range programs {
		t.Run(p.name, func(t *testing.T) {
			// Each program is six compilations and six runs, and there are a
			// good many programs. They do not touch each other, so they run
			// side by side; sequentially this outgrew `go test`'s ten minutes.
			t.Parallel()
			want := runWith(t, p.name+".weave", p.src, p.in, off)
			for _, s := range settings {
				got := runWith(t, p.name+".weave", p.src, p.in, s.opts)
				if got != want {
					t.Errorf("%s disagrees with the unoptimised build\ngot:  %q\nwant: %q\nsource:\n%s",
						s.label, got, want, p.src)
				}
			}
		})
	}
}

// gridLoop is the shared preamble for the pattern cases: a loop that threads a
// pattern through repeated updates.
const gridLoop = `
sheet is Source through pattern

step g n is
  ward n
    0 : g
    _ : step (set g (knot (mod n 5) (mod n 5)) '#') (sub n 1)

`

const gridInput = ".....\n.....\n.....\n.....\n.....\n"

// TestMutualTailRecursionRunsInOneFrame is the end-to-end half: a pair of
// definitions that hand control back and forth fifty million times must finish,
// not exhaust the C stack. It builds at -O0 deliberately — at -O3 clang
// sometimes makes the sibling call itself, which would hide a regression.
func TestMutualTailRecursionRunsInOneFrame(t *testing.T) {
	requireCC(t)

	cases := []struct{ name, src, want string }{
		{
			"two members, same arity",
			"even2 0 is Light\neven2 n is odd2 (sub n 1)\n\n" +
				"odd2 0 is Shadow\nodd2 n is even2 (sub n 1)\n\n" +
				"even2 50000000\n",
			"Light",
		},
		{
			"two members, different arities",
			"ping 0 total is total\nping n total is pong (sub n 1) (add total 1)\n\n" +
				"pong n total is ping n (add total 2)\n\n" +
				"ping 5000000 0\n",
			"15000000",
		},
		{
			"three members, entered from the smallest",
			"a 0 is 0\na n is b (sub n 1) 7\n\n" +
				"b 0 _ is 1\nb n acc is c (sub n 1) acc (mul acc 2)\n\n" +
				"c 0 _ _ is 2\nc n x y is a (sub n 1)\n\n" +
				"a 3000000\n",
			"0", // 3000000 is divisible by 3, so the cycle lands on `a 0`
		},
		{
			// A cycle through `pick`, whose branches are tail positions.
			"a cycle through pick",
			"up n is pick (gt 1000000 n) n (down (add n 1))\n\n" +
				"down n is up (add n 1)\n\n" +
				"up 0\n",
			"1000002", // up sees only even n, so the first past the bound is 1000002
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runWith(t, tc.name+".weave", tc.src, "", build.Options{})
			if strings.TrimRight(got, "\n") != tc.want {
				t.Errorf("got %q, want %q", strings.TrimRight(got, "\n"), tc.want)
			}
		})
	}
}
