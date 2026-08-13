package build_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/build"
	"github.com/malleum/weave/internal/diag"
)

// compileAndRun builds src and runs it with the given standard input.
func compileAndRun(t *testing.T, name, src, input string) string {
	t.Helper()
	requireCC(t)

	dir := t.TempDir()
	bag := diag.New(name, src)
	res, err := build.Compile(name, src, build.Options{
		Output: filepath.Join(dir, "program"),
	}, bag)
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

// requireCC skips the test when no C compiler is available, so the rest of the
// suite still runs in a bare environment.
func requireCC(t *testing.T) {
	t.Helper()
	for _, cc := range []string{"clang", "cc", "gcc"} {
		if _, err := exec.LookPath(cc); err == nil {
			return
		}
	}
	t.Skip("no C compiler available")
}

// TestExamplesProduceExpectedOutput compiles every program in examples/ and
// checks it against the fixtures in testdata/. These are the end-to-end tests:
// they exercise the lexer, parser, checker, code generator, C compiler and
// runtime in one go.
func TestExamplesProduceExpectedOutput(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.weave"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Fatal("no examples found")
	}

	for _, path := range paths {
		name := strings.TrimSuffix(filepath.Base(path), ".weave")
		t.Run(name, func(t *testing.T) {
			src, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			base := filepath.Join("..", "..", "testdata", name)
			input, err := os.ReadFile(base + ".in")
			if err != nil {
				t.Skipf("no input fixture for %s", name)
			}
			want, err := os.ReadFile(base + ".out")
			if err != nil {
				t.Skipf("no output fixture for %s", name)
			}

			got := compileAndRun(t, path, string(src), string(input))
			if strings.TrimRight(got, "\n") != strings.TrimRight(string(want), "\n") {
				t.Errorf("output mismatch\ngot:  %q\nwant: %q", got, string(want))
			}
		})
	}
}

// The cases below pin down language behaviour that the examples do not reach.
func TestLanguageBehaviour(t *testing.T) {
	cases := []struct {
		name, src, in, want string
	}{
		{
			name: "arithmetic without operators",
			src:  "answer is mul (add 2 3) (sub 10 4)\nanswer",
			want: "30",
		},
		{
			name: "water arithmetic",
			src:  "answer is div 7.0 2.0\nanswer",
			want: "3.5",
		},
		{
			name: "pipeline feeds the last argument",
			src:  "answer is [1 2 3 4] | sift even | sum\nanswer",
			want: "6",
		},
		{
			name: "where particle filters",
			src:  "answer is [1 2 3 4] where even | len\nanswer",
			want: "2",
		},
		{
			name: "ward on Hold",
			src:  "answer is\n  ward (first [7 8])\n    Held n : n\n    Stilled : 0\nanswer",
			want: "7",
		},
		{
			name: "ward on empty Thread takes Stilled",
			src:  "answer is\n  ward (first [])\n    Held n : n\n    Stilled : 0\nanswer",
			want: "0",
		},
		{
			name: "multi-clause dispatch",
			src:  "name 0 is \"none\"\nname 1 is \"one\"\nname _ is \"many\"\nname 5",
			want: "many",
		},
		{
			name: "closures capture locals",
			src:  "answer is\n  weave n is 10\n  [1 2 3] | bend (x : add n x) | sum\nanswer",
			want: "36",
		},
		{
			name: "local channel is callable",
			src:  "answer is\n  channel twice f x is f (f x)\n  twice (add 3) 1\nanswer",
			want: "7",
		},
		{
			name: "partial application of a builtin",
			src:  "answer is [1 5 9] | sift (gt 4) | len\nanswer",
			want: "2",
		},
		{
			name: "pick evaluates one branch",
			src:  "answer is pick (gt 1 2) \"big\" \"small\"\nanswer",
			want: "big",
		},
		{
			name: "tuples destructure",
			src:  "swap (a, b) is (b, a)\nswap (1, 2)",
			want: "(2, 1)",
		},
		{
			name: "recursion",
			src:  "fact 0 is 1\nfact n is mul n (fact (sub n 1))\nfact 10",
			want: "3628800",
		},
		{
			name: "text handling",
			src:  `answer is Source | lines | bend strip | join "-"` + "\nanswer",
			in:   " a \n b \n",
			want: "a-b",
		},
		{
			name: "fires and counting",
			src:  "answer is Source | fires | count isDigit\nanswer",
			in:   "a1b2c3",
			want: "3",
		},
		{
			name: "braid folds",
			src:  "answer is [1 2 3 4] | braid (acc x : add acc x) 0\nanswer",
			want: "10",
		},
		{
			name: "sort and zip",
			src:  "answer is zip (sort [3 1 2]) [\"a\" \"b\" \"c\"] | first\nanswer",
			want: `Held (1, "a")`,
		},
		{
			name: "pattern neighbours",
			src:  "g is Source through pattern\nanswer is nb4 g (knot 1 1) | count (eq '#')\nanswer",
			in:   ".#.\n#.#\n.#.\n",
			want: "4",
		},
		{
			name: "knot destructuring",
			src:  "sumk (knot r c) is add r c\nsumk (knot 3 4)",
			want: "7",
		},
		{
			name: "earth yields Stilled on junk",
			src:  "answer is earth \"abc\"\nanswer",
			want: "Stilled",
		},
		{
			name: "thread output prints one per line",
			src:  "[1 2 3]",
			want: "1\n2\n3",
		},
		{
			name: "span builds a range",
			src:  "answer is span 1 5 | sum\nanswer",
			want: "15",
		},
		{
			name: "inline let with into",
			src:  "answer is weave a is 2, b is 3 into mul a b\nanswer",
			want: "6",
		},
		{
			name: "top level values are order independent",
			src:  "a is add b 1\nb is 41\na",
			want: "42",
		},

		// Collections.
		{
			name: "web literal, get and put",
			// Keyed collections take the collection first, so they are called
			// rather than piped: `get w k`, not `w | get k`.
			src:  `w is {"a" : 1  "b" : 2}` + "\nget (put w \"c\" 3) \"b\" | otherwise 0",
			want: "2",
		},
		{
			name: "web get missing key is Stilled",
			src:  `get {"a" : 1} "zz"` + "\n",
			want: "Stilled",
		},
		{
			name: "web put replaces rather than duplicating",
			src:  `w is put (put (web []) "k" 1) "k" 2` + "\n(len w, get w \"k\" | otherwise 0)",
			want: "(1, 2)",
		},
		{
			name: "forget removes a key",
			src:  `w is forget {"a" : 1  "b" : 2} "a"` + "\n(len w, known w \"a\")",
			want: "(1, Shadow)",
		},
		{
			name: "freq counts occurrences",
			src:  `t is "abracadabra" | fires | freq` + "\nget t 'a' | otherwise 0",
			want: "5",
		},
		{
			name: "most finds the commonest",
			src:  `"abracadabra" | fires | freq | most`,
			want: "Held a",
		},
		{
			name: "circle keeps unique members",
			src:  "len (circle [1 2 3 2 1])",
			want: "3",
		},
		{
			name: "circle membership",
			src:  "c is circle [1 2 3]\n(member c 2, member c 9)",
			want: "(Light, Shadow)",
		},
		{
			name: "circle insert and remove",
			src:  "c is circle [1 2]\n(len (insert c 3), len (remove c 1))",
			want: "(3, 1)",
		},
		{
			name: "taveren yields its smallest",
			src: "h is taveren [5 1 9 3]\n" +
				"answer is\n  ward pop h\n    Stilled : 0\n    Held (v, _) : v\nanswer",
			want: "1",
		},
		{
			name: "web survives being stored and read back",
			// This is the regression test for Thread and tuple literals: a
			// collection keeps values past the frame that built them, so the
			// literal has to own its memory.
			src: "w is put (web []) \"k\" [1 2 3]\n" +
				"answer is\n  ward get w \"k\"\n    Stilled : 0\n    Held xs : sum xs\nanswer",
			want: "6",
		},
		{
			name: "a heap of tuples keeps them intact",
			src: "h is braid push (taveren []) [(2, \"b\") (1, \"a\")]\n" +
				"answer is\n  ward pop h\n    Stilled : \"none\"\n    Held ((_, s), _) : s\nanswer",
			want: "a",
		},

		// Newer sequence and text verbs.
		{
			name: "earths extracts integers, signs included",
			src:  "Source | earths | sum",
			in:   "moves 12 and -5 then 3",
			want: "10",
		},
		{
			name: "chunk and windows",
			src:  "(len (chunk 2 [1 2 3 4 5]), len (windows 2 [1 2 3 4 5]))",
			want: "(3, 4)",
		},
		{
			name: "pivot swaps rows and columns",
			src:  "pivot [[1 2] [3 4]] | bend sum",
			want: "4\n6",
		},
		{
			name: "gcd and lcm",
			src:  "(gcd 12 18, lcm 4 6)",
			want: "(6, 12)",
		},
		{
			name: "sortby orders by a derived key",
			src:  `["ccc" "a" "bb"] | sortby len | join ","`, // size counts runes
			want: "a,bb,ccc",
		},
		{
			name: "group gathers by a derived key",
			src:  "g is [1 2 3 4] | group even\nget g Light | otherwise [] | len",
			want: "2",
		},
		{
			name: "idx finds a position",
			src:  "(idx 3 [1 2 3], idx 9 [1 2 3])",
			want: "(Held 2, Stilled)",
		},
		{
			name: "contains searches text",
			src:  `(contains "ell" "hello", contains "zz" "hello")`,
			want: "(Light, Shadow)",
		},
		{
			name: "len works across collections",
			src:  `(len [1 2], len (circle [1 1]), len {"a" : 1}, len (taveren [1 2 3]), len "héllo")`,
			want: "(2, 1, 1, 3, 5)",
		},

		// Constructors as literals and as values. Nullary constructors used to
		// be miscompiled, and every test that passed anyway got its Spirits
		// back from a runtime function rather than from source.
		{
			name: "spirit literals are distinct",
			src:  "(Light, Shadow, eq Light Shadow, eq Light Light)",
			want: "(Light, Shadow, Shadow, Light)",
		},
		{
			name: "spirit literals work as web keys",
			src:  "w is put (put (web []) Light 1) Shadow 2\n(len w, get w Light | otherwise 0)",
			want: "(2, 1)",
		},
		{
			name: "Stilled is a value",
			src:  "answer is\n  ward Stilled\n    Stilled : \"empty\"\n    Held _ : \"full\"\nanswer",
			want: "empty",
		},
		{
			name: "a constructor can be passed as a function",
			src:  "[1 2] | bend Held",
			want: "Held 1\nHeld 2",
		},
		{
			name: "knot as a value",
			src:  "zip [1 2] [3 4] | bend (p : knot 0 0) | len",
			want: "2",
		},

		// Fused chains. These must produce exactly what the unfused runtime
		// verbs produce, including at the edges.
		{
			name: "fused chain over an empty Thread",
			src:  "([] | bend (x : mul x 2) | sift even | sum, [] | bend (x : x) | sift even)",
			want: "(0, [])",
		},
		{
			name: "fused filter that drops everything",
			src:  "[1 3 5] | bend (x : mul x 2) | sift odd | len",
			want: "0",
		},
		{
			// A Water always prints with a decimal point, so that it cannot
			// be mistaken for the Earth it is not.
			name: "fused sum keeps Water",
			src:  "[1.5 2.5] | bend (x : mul x 2.0) | sift (gt 0.0) | sum",
			want: "8.0",
		},
		{
			name: "fused prod",
			src:  "span 1 5 | bend (x : x) | prod",
			want: "120",
		},
		{
			name: "fused seek finds and stops",
			src:  "span 1 100 | bend (x : mul x 3) | seek (gt 10)",
			want: "Held 12",
		},
		{
			name: "fused seek that finds nothing",
			src:  "span 1 5 | bend (x : mul x 2) | seek (gt 1000)",
			want: "Stilled",
		},
		{
			name: "fused first",
			src:  "span 1 5 | bend (x : mul x 7) | sift even | first",
			want: "Held 14",
		},
		{
			name: "fused any and all",
			src:  "xs is span 1 5\n(xs | bend (x : mul x 2) | any (gt 8), xs | bend (x : mul x 2) | all (gt 1))",
			want: "(Light, Light)",
		},
		{
			name: "fused all that fails",
			src:  "span 1 5 | bend (x : mul x 2) | all (gt 100)",
			want: "Shadow",
		},
		{
			name: "fused count with a predicate",
			src:  "span 1 10 | bend (x : mul x 2) | count (gt 10)",
			want: "5",
		},
		{
			name: "fused braid with a seed",
			src:  "span 1 4 | bend (x : mul x x) | braid (a b : add a b) 100",
			want: "130",
		},
		{
			name: "fused collect keeps order and length",
			src:  `span 1 6 | bend (x : mul x 10) | sift (gt 20) | bend air | join ","`,
			want: "30,40,50,60",
		},
		{
			name: "chains nest through a non-stage verb",
			src:  "[[1 2] [3 4]] | bend (r : r | bend (x : mul x 2) | sum) | sift (gt 5) | sum",
			want: "20",
		},
		{
			name: "a fused chain reads a captured local",
			src:  "answer is\n  weave k is 10\n  span 1 5 | bend (x : mul x k) | sift (gt 20) | sum\nanswer",
			want: "120",
		},
		{
			name: "a fused stage may call a user function",
			src:  "twice n is mul n 2\nspan 1 5 | bend twice | sift (gt 4) | sum",
			want: "24",
		},
		{
			name: "a shadowed verb is not fused",
			src:  "bend f xs is [7]\nanswer is span 1 3 | bend (x : x) | sum\nanswer",
			want: "7",
		},
		{
			name: "destructuring lambda in a fused stage",
			src:  "zip [1 2] [10 20] | bend ((a, b) : add a b) | sift (gt 5) | sum",
			want: "33",
		},
		{
			name: "short-circuiting skips work a strict chain would do",
			// The third element would divide by zero. A lazy chain never
			// reaches it, which is the behaviour SPEC.md describes.
			src:  "[10 5 0] | bend (x : div 100 x) | seek (gt 5)",
			want: "Held 10",
		},

		// Tail calls. These recurse far deeper than the C stack would allow if
		// the calls were not turned into jumps.
		{
			name: "deep tail recursion with an accumulator",
			src:  "count 0 acc is acc\ncount n acc is count (sub n 1) (add acc n)\ncount 1000000 0",
			want: "500000500000",
		},
		{
			name: "deep tail recursion through ward",
			src:  "walk n is\n  ward n\n    0 : \"done\"\n    _ : walk (sub n 1)\nwalk 1000000",
			want: "done",
		},
		{
			name: "deep tail recursion through pick",
			src:  "down n is pick (eq 0 n) 0 (down (sub n 1))\ndown 1000000",
			want: "0",
		},
		{
			name: "tail recursion carrying a collection",
			src: "fill n acc is\n  ward n\n    0 : len acc\n    _ : fill (sub n 1) (insert acc n)\n" +
				"fill 20000 (circle [])",
			want: "20000",
		},
		{
			name: "tail call after a local binding",
			src: "walk n is\n  ward n\n    0 : 0\n    _ :\n      weave next is sub n 1\n      walk next\n" +
				"walk 500000",
			want: "0",
		},
		{
			name: "non-tail recursion still works",
			src:  "fact 0 is 1\nfact n is mul n (fact (sub n 1))\nfact 12",
			want: "479001600",
		},
		{
			name: "mutual recursion is left alone and still correct",
			src: "isEven n is\n  ward n\n    0 : Light\n    _ : isOdd (sub n 1)\n" +
				"isOdd n is\n  ward n\n    0 : Shadow\n    _ : isEven (sub n 1)\n" +
				"isEven 10000",
			want: "Light",
		},

		// The verbs added in the rask-alignment pass.
		{name: "second", src: "(first [1 2 3], drop 1 [1 2 3] | sum, second [1 2 3])",
			want: "(Held 1, 5, Held 2)"},
		{name: "none", src: "(none (gt 9) [1 2], none (gt 1) [1 2])", want: "(Light, Shadow)"},
		{name: "enum", src: "[\"a\" \"b\"] | enum | bend (p : p)", want: "(0, \"a\")\n(1, \"b\")"},
		{name: "scan", src: "[1 2 3] | scan (a b : add a b) 0", want: "1\n3\n6"},
		{name: "scan is sums", src: "[1 2 3] | scan add 0 | eq (sums [1 2 3])", want: "Light"},
		{name: "scan fuses as a stage", src: "[1 2 3 4] | scan add 0 | sift (gt 2) | sum", want: "19"},
		{name: "dupe", src: "[1 2 3 2 1] | dupe", want: "Held (3, 1, 2)"},
		// The second position is what makes a cycle measurable: the length is
		// the two subtracted, and before it was there the only way to it was a
		// second pass over a Thread `dupe` exists to walk once.
		{name: "dupe measures the cycle", src: "[1 2 3 2 1] | dupe | otherwise (0, 0, 0) | ((at, from, _) : sub at from)", want: "2"},
		{name: "dupe repeats the first element", src: "[7 1 2 7] | dupe", want: "Held (3, 0, 7)"},
		{name: "dupe repeats at once", src: "[4 4] | dupe", want: "Held (1, 0, 4)"},
		{name: "high and low", src: "(high [3 1 4], low [3 1 4])", want: "(Held 4, Held 1)"},
		{name: "highidx and lowidx", src: "(highidx [3 1 4], lowidx [3 1 4])", want: "(Held 2, Held 1)"},
		{name: "high of nothing", src: "high (span 1 0)", want: "Stilled"},
		{name: "seekidx", src: "seekidx (gt 2) [1 2 3 4]", want: "Held 2"},
		{name: "seekidx finds nothing", src: "seekidx (gt 9) [1 2 3]", want: "Stilled"},
		{name: "siftidx", src: "siftidx even [1 2 3 4] | air", want: "[1 3]"},
		{name: "idxs", src: "idxs 1 [3 1 4 1 5] | air", want: "[1 3]"},
		{name: "idxs finds nothing", src: "idxs 9 [1 2 3] | len", want: "0"},
		{name: "twist", src: "[1 2 3] | twist 1 inc", want: "1\n3\n3"},
		{name: "twist out of bounds", src: "[1 2 3] | twist 9 inc | sum", want: "6"},

		{name: "overlaps", src: "(overlaps (2, 8) (3, 7), overlaps (2, 8) (20, 30))", want: "(Light, Shadow)"},
		{name: "overlaps touching at one end", src: "overlaps (2, 8) (8, 30)", want: "Light"},
		{name: "overlapping", src: "(overlapping (2, 8) (3, 9), overlapping (2, 8) (20, 30))", want: "(Held (3, 8), Stilled)"},
		{name: "within", src: "(within (2, 8) (3, 7), within (3, 7) (2, 8))", want: "(Light, Shadow)"},
		{name: "spanning covers the gap", src: "spanning (2, 8) (20, 30)", want: "(2, 30)"},
		{name: "holding", src: "(holding (2, 8) 5, holding (2, 8) 9)", want: "(Light, Shadow)"},
		{name: "width", src: "(width (2, 8), width (8, 2))", want: "(7, 0)"},
		{name: "ranges are not Earth-only", src: "(overlaps (1.5, 2.5) (2.0, 9.0), overlaps ('a', 'm') ('k', 'z'))", want: "(Light, Light)"},
		{name: "snag takes the Gentled side", src: "[1 2 3] | gentle (a b : Gentled b) 0 | snag 9", want: "1"},
		{name: "snag falls back on a Woven", src: "[1 2 3] | gentle (a b : Woven b) 0 | snag 9", want: "9"},
		{name: "else is otherwise", src: "seek even [3 1 4] else 0", want: "4"},
		{name: "else on a Stilled", src: "seek even [3 1] else 0", want: "0"},
		{name: "failing is snag", src: "[1 2 3] | gentle (a b : Gentled b) 0 failing 9", want: "1"},
		{name: "dupe finds nothing", src: "[1 2 3] | dupe", want: "Stilled"},
		{
			name: "an endless cycle stopped by dupe",
			src:  "[1, neg 2, 3, 1] | cycle | scan add 0 | dupe",
			want: "Held (5, 2, 2)",
		},
		{
			name: "gentle stops when the step gentles",
			src:  "[1 2 3 4 5] | gentle (a b : pick (gt 3 b) (Gentled a) (Woven (add a b))) 0",
			want: "Gentled 6",
		},
		{
			name: "gentle runs out",
			src:  "[1 2 3] | gentle (a b : Woven (add a b)) 0",
			want: "Woven 6",
		},
		{name: "top and bot", src: "(top 2 [5 1 9 3] | sum, bot 2 [5 1 9 3] | sum)", want: "(14, 4)"},
		{name: "pairs", src: "[1 2 3] | pairs | len", want: "2"},
		{name: "cross", src: "cross [1 2] [3 4] | len", want: "4"},
		{name: "combos", src: "span 1 4 | combos 2 | len", want: "6"},
		{name: "compact", src: "[(Held 1) Stilled (Held 3)] | compact | sum", want: "4"},
		{name: "takewhile dropwhile", src: "xs is [1 2 9 1]\n(takewhile (lt 5) xs | len, dropwhile (lt 5) xs | len)",
			want: "(2, 2)"},
		{name: "mapcat", src: "[1 2] | mapcat (n : [n n]) | sum", want: "6"},
		{name: "nth over rows", src: "[[1 2] [3 4]] | glean (nth 1) | sum", want: "6"},
		{name: "nth and has", src: "([1 2 3] | nth 1, [1 2 3] | has 9)", want: "(Held 2, Shadow)"},
		{name: "cycle bounded by take", src: "cycle [1 2 3] | take 7 | sum", want: "13"},
		{name: "glean drops the Stilled", src: `["1" "x" "3"] | glean earth | sum`, want: "4"},
		{name: "weft builds a Pattern of Earths", src: "[[1 2] [3]] | weft 0 | cells | sum", want: "6"},
		{name: "digit", src: "'7' | digit | otherwise 0", want: "7"},
		{name: "maxby minby", src: `xs is ["a" "ccc" "bb"]` + "\n(maxby len xs, minby len xs)",
			want: `(Held "ccc", Held "a")`},
		{name: "blocks", src: "Source | blocks | len", in: "a\nb\n\nc\n\n\nd\n", want: "3"},
		{name: "upper lower", src: `(upper "aB", lower "aB")`, want: `("AB", "ab")`},
		{name: "pad", src: `(padl 4 '0' "7", padr 4 '.' "7")`, want: `("0007", "7...")`},
		{name: "starts ends cut", src: `(starts "he" "hello", ends "z" "hello", cutstart "he" "hello", cutend "lo" "hello")`,
			want: `(Light, Shadow, "llo", "hel")`},
		{name: "replace", src: `replace "l" "L" "hello"`, want: "heLLo"},
		{name: "ord spark", src: "(ord 'A', spark 66)", want: "(65, B)"},
		{name: "repeat", src: `repeat 3 "ab"`, want: "ababab"},
		// cbrt 27.0 really is 3.0000000000000004 here; `%g` used to round that
		// away, which is exactly the kind of thing a Water should not hide.
		{name: "sign sqrt roots", src: "(sign (neg 4), sqrt 16.0, cbrt 27.0)",
			want: "(-1, 4.0, 3.0000000000000004)"},
		{name: "rounding", src: "(ceil 2.1, floor 2.9, round 2.5, round 2.4)", want: "(3, 2, 3, 2)"},
		{name: "clamp", src: "(clamp 0 10 42, clamp 0 10 (neg 5), clamp 0 10 7)", want: "(10, 0, 7)"},
		{name: "pow", src: "(pow 2 10, pow 3 0)", want: "(1024, 1)"},
		{name: "bitwise", src: "(bor 12 10, band 12 10, bxor 12 10, shl 3 1, shr 1 8, base 2 10)",
			want: `(14, 8, 6, 8, 4, "1010")`},
		{name: "mdist", src: "mdist (knot 0 0) (knot 3 4)", want: "7"},
		{name: "constants", src: "gt 3.0 pi", want: "Light"},
		{name: "pattern shape and inb", src: "g is Source through pattern\n(shape g, inb g (knot 0 0), inb g (knot 9 9))",
			in: "..\n..\n", want: "((2, 2), Light, Shadow)"},
		{name: "dirs and around", src: "g is Source through pattern\n(len dirs4, len dirs8, around4 g (knot 0 0) | len)",
			in: "..\n..\n", want: "(4, 8, 2)"},
		{name: "set algebra", src: "a is circle [1 2 3]\nb is circle [2 3 4]\n(len (union a b), len (inter a b), len (diff a b))",
			want: "(4, 2, 1)"},
		{name: "mapvals", src: `w is {"a" : 1  "b" : 2}` + "\nmapvals (v : mul v 10) w | vals | sum",
			want: "30"},
		{name: "insert and remove", src: "c is circle [1 2]\n(len (insert c 3), len (remove c 1))", want: "(3, 1)"},
		{name: "renamed verbs still work", src: `(len [1 2], prod [2 3], rev [1 2] | first, flat [[1] [2]] | sum, fires "ab" | len, strip "  x  ", earths "a1b22" | sum, idx 2 [1 2 3])`,
			want: `(2, 6, Held 2, 3, 2, "x", 23, Held 1)`},

		// Layout.
		{
			name: "an application may span lines by indenting",
			src:  "answer is\n  add\n    1\n    2\nanswer",
			want: "3",
		},
		{
			name: "pick branches may span lines",
			src:  "answer is\n  pick (gt 1 2)\n    \"big\"\n    \"small\"\nanswer",
			want: "big",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := compileAndRun(t, tc.name+".weave", tc.src, tc.in)
			if strings.TrimRight(got, "\n") != tc.want {
				t.Errorf("got %q, want %q", strings.TrimRight(got, "\n"), tc.want)
			}
		})
	}
}
