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
