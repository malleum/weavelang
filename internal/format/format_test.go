package format

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/check"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/parser"
)

func fmtOK(t *testing.T, src string) string {
	t.Helper()
	out, err := Source(src, "test.weave")
	if err != nil {
		t.Fatalf("formatting failed: %v\nsource:\n%s", err, src)
	}
	return out
}

// TestCanonicalises pins the opinions: one spelling of each thing, and no
// parentheses that the parser would not need.
func TestCanonicalises(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"equals becomes is", "a   =  1\na\n", "a is 1\n\na\n"},
		{"spacing is normalised", "a is add   1    2\na\n", "a is add 1 2\n\na\n"},
		{"redundant parens go", "a is ((1))\na\n", "a is 1\n\na\n"},
		{"needed parens stay", "a is add (mul 2 3) 4\na\n", "a is add (mul 2 3) 4\n\na\n"},
		// The words are what the formatter prints; `|` and `through` are the
		// same token, so either spelling comes back as `through`.
		// `sift` and `bend` have particles of their own, and the words are what
		// the wordy style prints whichever way the author wrote them.
		{"a sift becomes where", "a is [1 2] | sift even | sum\na\n",
			"a is [1 2] where even through sum\n\na\n"},
		{"a bend becomes as", "a is [1 2] through bend inc\na\n",
			"a is [1 2] as inc\n\na\n"},
		{"a bracketed ward stays on its line", "a is ward Light (Light : 1) (_ : 0)\na\n",
			"a is ward Light (Light gives 1) (_ gives 0)\n\na\n"},
		{"a block ward stays a block",
			"a is\n  ward Light\n    Light : 1\n    _ : 0\na\n",
			"a is\n  ward Light\n    Light gives 1\n    _ gives 0\n\na\n"},
		{"an otherwise becomes else", "a is first [1 2] | otherwise 0\na\n",
			"a is first [1 2] else 0\n\na\n"},
		{"through stays through", "a is [1 2] through take 1\na\n",
			"a is [1 2] through take 1\n\na\n"},
		{"where survives", "a is [1 2] where even | sum\na\n",
			"a is [1 2] where even through sum\n\na\n"},
		// A one-off function is shorter written with the hole words, so that is
		// what comes back — but only where it reads as the same function.
		{"a lambda becomes a hole", "a is [1 2] | bend (x : add x 1)\na\n",
			"a is [1 2] as (add this 1)\n\na\n"},
		{"a lambda a hole cannot spell keeps its parameter",
			"a is [1 2] | bend (x : add x (mul x 2))\na\n",
			"a is [1 2] as (x gives add x (mul x 2))\n\na\n"},
		{"a lambda whose second parameter goes unused keeps both",
			"a is [1 2] | braid (x y : add x 1) 0\na\n",
			"a is [1 2] through braid (x y gives add x 1) 0\n\na\n"},
		{"a lambda arrow is spelled out", "a is [1 2] | braid (x y : add x y) 0\na\n",
			"a is [1 2] through braid (add this that) 0\n\na\n"},
		{"an equals is spelled out", "a = 1\na\n", "a is 1\n\na\n"},
		{"blank lines collapse", "a is 1\n\n\n\n\nb is 2\n\na\n",
			"a is 1\n\nb is 2\n\na\n"},
		{"indentation is normalised",
			"f x is\n      ward x\n            0 : 1\n            _ : 2\nf 0\n",
			"f x is\n  ward x\n    0 gives 1\n    _ gives 2\n\nf 0\n"},
		{"comments are kept",
			"# leading\na is 1  # trailing\na\n",
			"# leading\na is 1  # trailing\n\na\n"},
		{"a comment introduces what follows it",
			"a is 1\n\n# about b\nb is 2\n\na\n",
			"a is 1\n\n# about b\nb is 2\n\na\n"},
		{"a comment before the output expression stays with it",
			"a is 1\n# the answer\na\n",
			"a is 1\n\n# the answer\na\n"},
		{"remember goes once, on the first clause",
			"remember f 0 is 1\nremember f n is n\n\nf 1\n",
			"remember f 0 is 1\nf n is n\n\nf 1\n"},
		{"signatures are kept", "f :: Earth -> Earth\nf n is n\nf 1\n",
			"f :: Earth -> Earth\nf n is n\n\nf 1\n"},

		{"a sum type on one line", "Colour   is Red|Green |Blue\n\nRed\n",
			"Colour is Red | Green | Blue\n\nRed\n"},
		{"a sum type keeps its parameters",
			"Tree a is Leaf|Node (Tree a) a (Tree a)\n\nLeaf\n",
			"Tree a is Leaf | Node (Tree a) a (Tree a)\n\nLeaf\n"},
		{"a long sum type wraps",
			"Instruction is Acquire Air Earth | Release Air Earth | Await Air Earth | Restart Air | Halt\n\nHalt\n",
			"Instruction is\n  Acquire Air Earth\n  | Release Air Earth\n  | Await Air Earth\n  | Restart Air\n  | Halt\n\nHalt\n"},
		{"declarations keep their order",
			"a is 1\nColour is Red\nb is 2\n\na\n",
			"a is 1\n\nColour is Red\n\nb is 2\n\na\n"},

		{"atom threads stay space-separated", "a is [ 1,2 ,3 ]\na\n",
			"a is [1 2 3]\n\na\n"},
		{"threads of applications take commas",
			"Move is Step Earth | Rest\n\na is [Step 1, Rest]\na\n",
			"Move is Step Earth | Rest\n\na is [Step 1, Rest]\n\na\n"},

		{"a hole keeps its brackets", "a is [1 2] where (mod  _  2)\na\n",
			"a is [1 2] where (mod this 2)\n\na\n"},
		{"a piped hole stays a pipeline", "a is web [] | get  _  \"k\"\na\n",
			"a is web [] through get this \"k\"\n\na\n"},
		{"a piped `former` stays a pipeline", "a is (1, 2) | sub  former  latter\na\n",
			"a is (1, 2) through sub former latter\n\na\n"},
		{"a group holding `former` keeps its brackets",
			"a is [(1, 2)] | bend (add former latter)\na\n",
			"a is [(1, 2)] as (add former latter)\n\na\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fmtOK(t, tc.src); got != tc.want {
				t.Errorf("got:\n%q\nwant:\n%q", got, tc.want)
			}
		})
	}
}

// TestIdempotent is the property that makes a formatter trustworthy: running
// it twice changes nothing the second time.
func TestIdempotent(t *testing.T) {
	for _, src := range formatCorpus(t) {
		once := fmtOK(t, src)
		twice := fmtOK(t, once)
		if once != twice {
			t.Errorf("formatting is not idempotent\nfirst:\n%s\nsecond:\n%s", once, twice)
		}
	}
}

// TestFormattingPreservesTheProgram reformats every example and requires the
// result to parse and type-check to the same shape. A formatter that changes
// meaning is worse than none.
func TestFormattingPreservesTheProgram(t *testing.T) {
	for _, src := range formatCorpus(t) {
		before := shapeOf(t, src)
		after := shapeOf(t, fmtOK(t, src))
		if before != after {
			t.Errorf("formatting changed the program\nbefore:\n%s\nafter:\n%s", before, after)
		}
	}
}

// shapeOf renders the syntax tree, which ignores layout entirely, so any
// difference is a real change to the program.
//
// One difference is not: writing a lambda with the hole words renames its
// parameters, which no program can observe. Both trees get that renaming
// applied, so the comparison is up to the names of bound variables and about
// everything else.
func shapeOf(t *testing.T, src string) string {
	t.Helper()
	bag := diag.New("x.weave", src)
	file := parser.Parse(src, bag)
	if !bag.Empty() {
		t.Fatalf("does not parse:\n%s\n\n%s", src, bag)
	}
	check.File(file, bag)
	if !bag.Empty() {
		t.Fatalf("does not check:\n%s\n\n%s", src, bag)
	}
	walkLambdas(file, func(l *ast.Lambda) {
		if cand, _ := holeCandidate(l); cand != nil {
			l.Params = cand.Params
		}
	})
	return ast.DumpFile(file)
}

// formatCorpus is every example plus a few shapes the examples do not reach.
func formatCorpus(t *testing.T) []string {
	t.Helper()
	var out []string

	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.weave"))
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}

	out = append(out,
		"a is weave x is 1, y is 2 into add x y\na\n",
		"f (knot r c) is add r c\nf (knot 1 2)\n",
		"a is {\"k\" : 1  \"j\" : 2}\nkeys a | len\n",
		"a is [(1, 2) (3, 4)] | bend ((x, y) : add x y) | sum\na\n",
		"f x is\n  ward x\n    Held n :\n      ward n\n        0 : 1\n        _ : 2\n    Stilled : 0\nf (first [1])\n",
		"a is span 1 100 | bend (x : mul x x) | sift (gt 400) | bend (x : add x 1) | braid (p q : add p q) 0\na\n",
		"# only a comment\n",
		"a is 1.5\nb is 'x'\nc is \"t\"\n(a, b, c)\n",
		"Colour is Red | Green | Blue\n\nname Red is \"r\"\nname Green is \"g\"\nname Blue is \"b\"\n\nname Red\n",
		"Tree a is Leaf | Node (Tree a) a (Tree a)\n\ntotal Leaf is 0\ntotal (Node l v r) is add v (add (total l) (total r))\n\ntotal Leaf\n",
		"Move is Step Earth Earth | Rest\n\na is [Step 1 2, Rest, Step 3 4]\nlen a\n",
		"a is span 1 10 | sift (mod _ 3 | eq 0) | bend (mul _ _) | sum\na\n",
		"a is web [(\"k\", 1)] | get _ \"k\" | otherwise 0\na\n",
		"a is span 1 10 where (mod _ 2 | eq 0) | len\na\n",
		"a is [1 2] | bend (n : ward (even n) (Light : n) (Shadow : 0))\na\n",
		"a is ward Light (Light : \"a very long answer indeed for this one case here\") (_ : \"and another long one\")\na\n",
		"a is [1 2] | cycle | scan add 0 | gentle (s n : pick (member s n) (Gentled n) (Woven (insert s n))) (circle [0]) | snag 0\na\n",
		"a is [1 2] | sift (n : gt 100000000 (mul n (mul n (mul n (mul n n)))))\na\n",
		"f p is\n  weave (a, b) is p\n  weave (m, _) is (b, a)\n  add m a\nf (1, 2)\n",
		"g p is weave (a, b) is p into add a b\ng (1, 2)\n",
		// A Thread of one holding something that is not an atom. The comma
		// that separates elements is never written for a single one, so the
		// brackets round it are the only thing keeping it one element.
		"f x is [(inc x)]\nf 1\n",
		"a is [(1, 2)]\na\n",
		"a is [[1 2]]\na\n",
	)
	return out
}

// A piped hole word is a Ward or a Let by the time the formatter sees it — the
// parser desugars `xs | latter` to a match on the pair it opens — so a chain
// ending in one used to stop being a chain, and ran off the edge however long
// it got.
func TestAChainEndingInAHoleWordStillBreaks(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			"latter",
			"after p l is add p l\n\na is\n  [1, 2] through braid ((p, n) l gives " +
				"(after p l, add n (pick (eq 0 (after p l)) 1 0))) (50, 0) through latter\n\na\n",
		},
		{
			"the whole value",
			"a is\n  [1, 2] through braid (n l gives add n (mul l (add l (mul l " +
				"(add l 12345)))))  0 through _\n\na\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := fmtOK(t, tc.src)
			for _, line := range strings.Split(out, "\n") {
				if len(line) > Width {
					t.Errorf("line runs past the margin:\n%s", out)
				}
			}
			if again := fmtOK(t, out); again != out {
				t.Errorf("not idempotent:\n%s\n%s", out, again)
			}
		})
	}
}

// TestLongPipelineWraps checks that a chain past the margin is broken one
// stage per line rather than running off the edge.
func TestLongPipelineWraps(t *testing.T) {
	src := "a is span 1 100 | bend (x : mul x x) | sift (gt 400) | bend (x : add x 1) | braid (p q : add p q) 0\na\n"
	out := fmtOK(t, src)
	for _, line := range strings.Split(out, "\n") {
		if len(line) > Width {
			t.Errorf("line runs past the margin:\n%s", out)
		}
	}
	stages := 0
	for _, line := range strings.Split(out, "\n") {
		for _, particle := range []string{"through ", "where ", "as "} {
			if strings.HasPrefix(strings.TrimSpace(line), particle) {
				stages++
			}
		}
	}
	if stages < 2 {
		t.Errorf("expected the pipeline to be broken one stage per line:\n%s", out)
	}
}

// TestALongStageBreaksInside is the other half of the wrapping rule: once a
// pipeline is one stage to a line, a stage that still runs past the margin is
// broken at its arguments rather than left long.
func TestALongStageBreaksInside(t *testing.T) {
	src := "a is [1 2] | cycle | scan add 0 | gentle (s n : pick (member s n) (Gentled n) (Woven (insert s n))) (circle [0]) | snag 0\na\n"
	out := fmtOK(t, src)
	for _, line := range strings.Split(out, "\n") {
		if len(line) > Width {
			t.Errorf("line runs past the margin:\n%s", out)
		}
	}
	if !strings.Contains(out, "through gentle\n") {
		t.Errorf("expected the long stage broken at its arguments:\n%s", out)
	}
	// And it has to read back as the same program, which is what the corpus
	// tests check for everything else.
	if again := fmtOK(t, out); again != out {
		t.Errorf("breaking inside a stage is not idempotent:\n%s\n%s", out, again)
	}
}

// TestTerseIsTheOtherSpelling checks the flag that prints symbols. The two
// styles are the same tree, so formatting one and reading it back has to give
// the other unchanged.
func TestTerseIsTheOtherSpelling(t *testing.T) {
	src := "a is [1 2] as (add this 1) through sum\n\na\n"

	terse, err := SourceStyle(src, "t.weave", Terse)
	if err != nil {
		t.Fatalf("formatting failed: %v", err)
	}
	// Terse spells the particles out as the verbs they are, and the hole words
	// as the symbol where there is one. `that`, `former` and `latter` have
	// none, so they stay as they are in both styles.
	want := "a = [1 2] | bend (add _ 1) | sum\n\na\n"
	if terse != want {
		t.Errorf("terse:\ngot:  %q\nwant: %q", terse, want)
	}

	// And back again, which is the property that matters: the styles are two
	// spellings of one program, not two programs.
	wordy, err := SourceStyle(terse, "t.weave", Wordy)
	if err != nil {
		t.Fatalf("formatting the terse form failed: %v", err)
	}
	if wordy != src {
		t.Errorf("round trip:\ngot:  %q\nwant: %q", wordy, src)
	}
}

// A call that will not fit breaks at its arguments, the same seam a pipeline
// stage breaks at. Without this a nested `pick` — which is how a Weave program
// says "otherwise" — ran to whatever length it liked: 270 characters in the
// worst of the Advent of Code 2025 solutions, which is not a form anybody would
// choose, and so not one anybody would run the formatter to get.
func TestALongCallBreaksAtItsArguments(t *testing.T) {
	src := "f a b c d is\n" +
		"  pick (eq 0 a) Light (pick (lt 0 b) Shadow (pick (gte 10 c) Shadow " +
		"(pick (eq d a) (f a b c (add d 1)) (f a (sub b 1) c d))))\n\nf 1 2 3 4\n"
	out := fmtOK(t, src)
	for _, line := range strings.Split(out, "\n") {
		if len(line) > Width {
			t.Errorf("line runs past the margin:\n%s", out)
		}
	}
	// The condition and the first branch stay beside the verb; what did not fit
	// goes below, one argument to a line.
	if !strings.Contains(out, "pick (eq 0 a) Light\n") {
		t.Errorf("expected the call broken after what fits:\n%s", out)
	}
	if again := fmtOK(t, out); again != out {
		t.Errorf("breaking a call is not idempotent:\n%s\n%s", out, again)
	}
}

// The three things a too-long argument can be broken into, each inside the
// brackets it already sat in. The closing bracket has to land on the last line
// written, however deep the breaking went, or what comes back does not parse.
func TestALongArgumentBreaksInsideItsBrackets(t *testing.T) {
	cases := []struct{ name, src string }{
		{
			"a call",
			"f a b is\n  g (h a b (h b a (h a a (h b b (h a b (h b a (h a a 1)))))))\n\nf 1 2\n",
		},
		{
			"a pipeline",
			"f a is\n  add 1 (span 1 a through bend (n gives mul n n) through sift " +
				"(n gives gt 100 n) through sum)\n\nf 30\n",
		},
		{
			"a lambda",
			"f xs is\n  xs through bend (n gives add (mul n 1000) (add (mul n 100) " +
				"(add (mul n 10) (add n 12345678))))\n\nf [1, 2]\n",
		},
		{
			// A Twine literal has a comma to lead with, so it breaks one
			// element to a line the way a Thread literal already did.
			"a Twine literal",
			"f xs is\n  xs through bend (l gives (split \":\" l through first else \"\" " +
				"through strip, split \":\" l through second else \"\" through words))\n\nf [\"a: b\"]\n",
		},
		{
			// The same, written with the hole words: the lambda prints as its
			// body, so the seam is the body's.
			"a Twine literal under a hole word",
			"f xs is\n  xs as (split \":\" this through first else \"\" through strip, " +
				"split \":\" this through second else \"\" through words)\n\nf [\"a: b\"]\n",
		},
		{
			"a Thread literal",
			"f a is\n  weld [add a 100000, mul a 200000, sub a 300000, div a 400000, " +
				"mod a 500000] [1]\n\nf 7\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := fmtOK(t, tc.src)
			for _, line := range strings.Split(out, "\n") {
				if len(line) > Width {
					t.Errorf("line runs past the margin:\n%s", out)
				}
			}
			if again := fmtOK(t, out); again != out {
				t.Errorf("not idempotent:\n%s\n%s", out, again)
			}
		})
	}
}

// A binding that takes its value apart prints its pattern where a name would
// go, in both spellings, and reads back as itself.
func TestADestructuringBindingRoundTrips(t *testing.T) {
	src := "f p is\n" +
		"  weave (a, b) is p\n" +
		"  weave [x, y] is [a, b]\n" +
		"  weave (m, _) is (x, y)\n" +
		"  add m y\n" +
		"\n" +
		"g p is weave (a, b) is p into add a b\n" +
		"\n" +
		"join \" \" [f (1, 2) | air, g (3, 4) | air]\n"
	out := fmtOK(t, src)
	for _, want := range []string{"weave (a, b) is p", "weave [x y] is [a b]", "weave (m, _) is (x, y)"} {
		if !strings.Contains(out, want) {
			t.Errorf("expected %q in:\n%s", want, out)
		}
	}
	if again := fmtOK(t, out); again != out {
		t.Errorf("not idempotent:\n%s\n%s", out, again)
	}
	terse, err := SourceStyle(src, "test.weave", Terse)
	if err != nil {
		t.Fatalf("terse: %v", err)
	}
	if !strings.Contains(terse, "weave (a, b) = p") {
		t.Errorf("expected the terse binder:\n%s", terse)
	}
}

// A top-level definition that takes its value apart prints its pattern where a
// name would go. The formatter reads the parse tree, so it sees what was
// written rather than the projections the checker expands it into.
func TestATopLevelDestructuringPrintsAsWritten(t *testing.T) {
	src := "dims is (12, 5)\n\n(width, height) is dims\n\narea is mul width height\n\narea\n"
	out := fmtOK(t, src)
	if !strings.Contains(out, "(width, height) is dims") {
		t.Errorf("expected the pattern to print:\n%s", out)
	}
	if strings.Contains(out, "whole") {
		t.Errorf("the hidden name leaked into the output:\n%s", out)
	}
	if again := fmtOK(t, out); again != out {
		t.Errorf("not idempotent:\n%s\n%s", out, again)
	}
}
