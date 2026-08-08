package check

import (
	"strings"
	"testing"

	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/parser"
	"github.com/malleum/weave/internal/types"
)

// run checks src and returns the diagnostics as one string.
func run(t *testing.T, src string) (*Info, string) {
	t.Helper()
	bag := diag.New("test.weave", src)
	file := parser.Parse(src, bag)
	if !bag.Empty() {
		t.Fatalf("source does not parse:\n%s", bag)
	}
	info := File(file, bag)
	return info, bag.String()
}

// ok checks that src has no diagnostics and returns the inferred types.
func ok(t *testing.T, src string) *Info {
	t.Helper()
	info, errs := run(t, src)
	if errs != "" {
		t.Fatalf("unexpected errors for:\n%s\n\n%s", src, errs)
	}
	return info
}

// fails checks that src reports an error mentioning want.
func fails(t *testing.T, src, want string) {
	t.Helper()
	_, errs := run(t, src)
	if errs == "" {
		t.Fatalf("expected an error mentioning %q, but checking succeeded:\n%s", want, src)
	}
	if !strings.Contains(errs, want) {
		t.Errorf("expected an error mentioning %q, got:\n%s", want, errs)
	}
}

func declType(t *testing.T, info *Info, name string) string {
	t.Helper()
	sch, found := info.Decls[name]
	if !found {
		t.Fatalf("no type recorded for %q", name)
	}
	return types.SchemeString(sch)
}

// ------------------------------------------------------------------ literals

func TestLiteralTypes(t *testing.T) {
	info := ok(t, "a is 1\nb is 1.5\nc is 'x'\nd is \"hi\"\ne is Light\na")
	for name, want := range map[string]string{
		"a": "Earth", "b": "Water", "c": "Fire", "d": "Air", "e": "Spirit",
	} {
		if got := declType(t, info, name); got != want {
			t.Errorf("%s :: %s, want %s", name, got, want)
		}
	}
}

// --------------------------------------------------------------- application

func TestPipelineInfersThroughStdlib(t *testing.T) {
	info := ok(t, "nums is Source | lines | bend earth | bend (otherwise 0)\nnums | sum")
	if got := declType(t, info, "nums"); got != "Thread Earth" {
		t.Errorf("nums :: %s", got)
	}
	if got := types.String(info.Output); got != "Earth" {
		t.Errorf("output :: %s", got)
	}
}

func TestArgumentTypeMismatchIsCaught(t *testing.T) {
	// A number literal is not committed to a Power, so what fails here is the
	// Talent — which says more than a shape mismatch would.
	fails(t, `a is add 1 "two"`+"\na", "Air has no Reckon Talent")
	// With one side already committed the message is the ordinary shape one.
	fails(t, "a is add 1.0 'x'\na", "expects Water here, but found Fire")
}

func TestApplyingANonFunction(t *testing.T) {
	fails(t, "a is 1 2\na", "not a function")
}

func TestUnknownNameSuggests(t *testing.T) {
	_, errs := run(t, "a is bendd (add 1) [1 2]\na")
	if !strings.Contains(errs, "cannot find `bendd`") {
		t.Errorf("got:\n%s", errs)
	}
	if !strings.Contains(errs, "did you mean `bend`?") {
		t.Errorf("expected a suggestion, got:\n%s", errs)
	}
}

// ------------------------------------------------------------- polymorphism

func TestGeneralizationAllowsTwoUses(t *testing.T) {
	// `pair` is used at two element types, which only works if it was
	// generalised before its uses were checked.
	src := `
pair x is (x, x)
both is (pair 1, pair "a")
both
`
	ok(t, src)
}

func TestLocalBindingIsGeneralized(t *testing.T) {
	src := `
main is
  channel twice f x is f (f x)
  (twice (add 1) 0, twice not Light)
main
`
	ok(t, src)
}

func TestPolymorphicIdentityStaysPolymorphic(t *testing.T) {
	info := ok(t, "same x is x\nsame 1")
	if got := declType(t, info, "same"); got != "a -> a" {
		t.Errorf("same :: %s, want a -> a", got)
	}
}

func TestRecursionInfers(t *testing.T) {
	info := ok(t, "count2 n is\n  ward n\n    0 : 0\n    _ : count2 (sub n 1)\ncount2 5")
	if got := declType(t, info, "count2"); got != "Earth -> Earth" {
		t.Errorf("count2 :: %s", got)
	}
}

func TestMutualRecursionInfers(t *testing.T) {
	src := `
isEven n is
  ward n
    0 : Light
    _ : isOdd (sub n 1)

isOdd n is
  ward n
    0 : Shadow
    _ : isEven (sub n 1)

isEven 10
`
	info := ok(t, src)
	if got := declType(t, info, "isEven"); got != "Earth -> Spirit" {
		t.Errorf("isEven :: %s", got)
	}
}

func TestOccursCheckRejectsInfiniteType(t *testing.T) {
	fails(t, "a f is f f\na", "contain itself")
}

// -------------------------------------------------------------- annotations

func TestSignatureIsHonoured(t *testing.T) {
	info := ok(t, "twice :: Earth -> Earth\ntwice n is mul 2 n\ntwice 3")
	if got := declType(t, info, "twice"); got != "Earth -> Earth" {
		t.Errorf("twice :: %s", got)
	}
}

func TestSignatureMismatchIsCaught(t *testing.T) {
	// The signature says Air; `mul` needs Reckon, which Air does not have.
	fails(t, "twice :: Air -> Air\ntwice n is mul 2 n\ntwice \"x\"", "Air has no Reckon Talent")
}

func TestUnknownTypeInSignature(t *testing.T) {
	fails(t, "f :: Threed a -> Earth\nf xs is 1\nf [1]", "unknown type `Threed`")
}

func TestTypeConstructorArity(t *testing.T) {
	fails(t, "f :: Thread -> Earth\nf xs is 1\nf [1]", "takes 1 type argument")
}

// ------------------------------------------------------------------ talents

func TestReckonRejectsText(t *testing.T) {
	fails(t, `a is add "x" "y"`+"\na", "Reckon")
}

func TestReckonAcceptsWater(t *testing.T) {
	info := ok(t, "a is add 1.5 2.5\na")
	if got := declType(t, info, "a"); got != "Water" {
		t.Errorf("a :: %s", got)
	}
}

func TestOrdRejectsUnorderedType(t *testing.T) {
	// Spirit has Eq but not Ord, so it cannot be sorted.
	fails(t, "a is sort [Light Shadow]\na", "Ord")
}

func TestEqOnThreadRequiresEqElement(t *testing.T) {
	// The element type is what actually needs Eq.
	ok(t, "a is eq [1 2] [1 2]\na")
}

func TestOutputMustBeShowable(t *testing.T) {
	fails(t, "a is (n : n)\na", "Show")
}

func TestConstraintAppearsInScheme(t *testing.T) {
	info := ok(t, "double n is add n n\ndouble 1")
	got := declType(t, info, "double")
	if !strings.Contains(got, "Reckon") {
		t.Errorf("expected a Reckon constraint, got: %s", got)
	}
}

// ------------------------------------------------------------------- ward

func TestWardArmsMustAgree(t *testing.T) {
	fails(t, "a is\n  ward Light\n    Light : 1\n    Shadow : \"x\"\na", "ward arm")
}

func TestWardBindsPatternVariables(t *testing.T) {
	info := ok(t, "a is\n  ward (first [1 2])\n    Held n : n\n    Stilled : 0\na")
	if got := declType(t, info, "a"); got != "Earth" {
		t.Errorf("a :: %s", got)
	}
}

func TestPatternTypeMismatch(t *testing.T) {
	fails(t, "a is\n  ward 1\n    'x' : 1\n    _ : 2\na", "Fire")
}

func TestConstructorArityInPattern(t *testing.T) {
	fails(t, "a is\n  ward (first [1])\n    Held : 1\n    Stilled : 0\na", "carries 1 value")
}

// --------------------------------------------------------- exhaustiveness

func TestMissingStilledIsCaught(t *testing.T) {
	src := "a is\n  ward (first [1 2])\n    Held n : n\na"
	_, errs := run(t, src)
	if !strings.Contains(errs, "does not handle every case") {
		t.Fatalf("expected an exhaustiveness error, got:\n%s", errs)
	}
	if !strings.Contains(errs, "Stilled") {
		t.Errorf("witness should name Stilled, got:\n%s", errs)
	}
}

func TestMissingSpiritCaseIsCaught(t *testing.T) {
	src := "a is\n  ward Light\n    Light : 1\na"
	_, errs := run(t, src)
	if !strings.Contains(errs, "Shadow") {
		t.Errorf("expected Shadow to be reported missing, got:\n%s", errs)
	}
}

func TestWildcardMakesExhaustive(t *testing.T) {
	ok(t, "a is\n  ward (first [1 2])\n    Held n : n\n    _ : 0\na")
}

func TestVariableArmMakesExhaustive(t *testing.T) {
	ok(t, "a is\n  ward (first [1 2])\n    Held n : n\n    other : 0\na")
}

func TestLiteralMatchNeedsCatchAll(t *testing.T) {
	src := "a is\n  ward 3\n    0 : 1\n    1 : 2\na"
	_, errs := run(t, src)
	if !strings.Contains(errs, "does not handle every case") {
		t.Errorf("matching Earth literals needs a catch-all, got:\n%s", errs)
	}
}

func TestNestedPatternGapIsFound(t *testing.T) {
	// Held is covered, Stilled is covered, but `Held Shadow` is not.
	src := `
a a1 is
  ward a1
    Held Light : 1
    Stilled : 0
a (first [Light])
`
	_, errs := run(t, src)
	if !strings.Contains(errs, "Held Shadow") {
		t.Errorf("expected `Held Shadow` as the witness, got:\n%s", errs)
	}
}

func TestTupleExhaustiveness(t *testing.T) {
	src := `
a p is
  ward p
    (Light, Light) : 1
    (Light, Shadow) : 2
    (Shadow, Light) : 3
    (Shadow, Shadow) : 4
a (Light, Shadow)
`
	ok(t, src)
}

func TestTupleGapIsFound(t *testing.T) {
	src := `
a p is
  ward p
    (Light, Light) : 1
    (Shadow, Shadow) : 4
a (Light, Shadow)
`
	_, errs := run(t, src)
	if !strings.Contains(errs, "does not handle every case") {
		t.Errorf("expected a gap, got:\n%s", errs)
	}
}

func TestUnreachableArmIsReported(t *testing.T) {
	src := "a is\n  ward Light\n    _ : 1\n    Light : 2\na"
	_, errs := run(t, src)
	if !strings.Contains(errs, "can never match") {
		t.Errorf("expected an unreachable-arm warning, got:\n%s", errs)
	}
}

func TestDuplicateArmIsReported(t *testing.T) {
	src := "a is\n  ward Light\n    Light : 1\n    Light : 2\n    Shadow : 3\na"
	_, errs := run(t, src)
	if !strings.Contains(errs, "can never match") {
		t.Errorf("expected the duplicate arm to be reported, got:\n%s", errs)
	}
}

func TestMultiClauseCoverageIsChecked(t *testing.T) {
	src := "f Light is 1\nf 2"
	_, errs := run(t, src)
	if !strings.Contains(errs, "does not handle every case") {
		t.Errorf("expected clause coverage to be checked, got:\n%s", errs)
	}
}

func TestMultiClauseWithCatchAllIsFine(t *testing.T) {
	ok(t, "fib 0 is 0\nfib 1 is 1\nfib n is add (fib (sub n 1)) (fib (sub n 2))\nfib 10")
}

func TestUnreachableClauseIsReported(t *testing.T) {
	src := "f n is 1\nf 0 is 2\nf 3"
	_, errs := run(t, src)
	if !strings.Contains(errs, "can never match") {
		t.Errorf("expected the shadowed clause to be reported, got:\n%s", errs)
	}
}

// ------------------------------------------------ reading a Power from text

// The Power readers are ordinary verbs — that is the point of them. Each is
// Air -> Hold of its own Power, so a failure is a Stilled the caller has to
// handle rather than a second return value.
func TestPowerReadersAreOrdinaryVerbs(t *testing.T) {
	for _, c := range []struct{ verb, want string }{
		{"earth", "Hold Earth"},
		{"water", "Hold Water"},
		{"fire", "Hold Fire"},
	} {
		info := ok(t, "a is "+c.verb+" \"1\"\na")
		if got := declType(t, info, "a"); got != c.want {
			t.Errorf("%s :: Air -> %s, but a :: %s", c.verb, c.want, got)
		}
	}
}

func TestPowerReaderRejectsNonText(t *testing.T) {
	fails(t, "a is earth 3\na", "Air")
}

func TestPowerReaderPartialApplication(t *testing.T) {
	info := ok(t, "a is bend water (lines Source)\na")
	if got := declType(t, info, "a"); got != "Thread (Hold Water)" {
		t.Errorf("a :: %s", got)
	}
}

// ------------------------------------------------------------------- misc

func TestDuplicateTopLevelName(t *testing.T) {
	// Two clauses are fine; two separate definitions with different arity are
	// what the parser rejects. Same-name, same-arity clauses collect instead.
	ok(t, "f 0 is 1\nf _ is 2\nf 1")
}

func TestDuplicateBindingInPattern(t *testing.T) {
	fails(t, "f (x, x) is x\nf (1, 2)", "bound twice")
}

func TestWebLiteralKeysNeedEq(t *testing.T) {
	info := ok(t, `a is {"x" : 1  "y" : 2}`+"\nkeys a")
	if got := declType(t, info, "a"); got != "Web Air Earth" {
		t.Errorf("a :: %s", got)
	}
}

func TestThreadLiteralElementsMustAgree(t *testing.T) {
	fails(t, `a is [1 "x"]`+"\na", "Thread element")
}

func TestGridProgramChecks(t *testing.T) {
	src := `
busy g k is
  ward cell g k
    Held '#' : gte 2 (count (eq '#') (nb4 g k))
    _        : Shadow

sheet is Source through pattern

sheet | knots where busy sheet | len
`
	info := ok(t, src)
	if got := declType(t, info, "busy"); got != "Pattern Fire -> Knot -> Spirit" {
		t.Errorf("busy :: %s", got)
	}
}

// checkSource parses and checks src, returning the bag so a test can look at
// where a diagnostic landed rather than only at what it says.
func checkSource(t *testing.T, src string) *diag.Bag {
	t.Helper()
	bag := diag.New("test.weave", src)
	file := parser.Parse(src, bag)
	if !bag.Empty() {
		t.Fatalf("source does not parse:\n%s", bag)
	}
	File(file, bag)
	return bag
}

// ------------------------------------------------------- number literals

// `1` is not committed to a Power until the definition around it decides. That
// is what makes a Water function writable without spelling `1.0` everywhere —
// and what stops the error landing two lines from the mistake.
func TestNumberLiteralsTakeTheirPowerFromContext(t *testing.T) {
	info := ok(t, "fact 1.0 is 1.0\nfact n is mul n (fact (sub n 1))\nfact 5.0\n")
	if got := declType(t, info, "fact"); got != "Water -> Water" {
		t.Errorf("fact :: %s", got)
	}

	// A signature decides it where nothing else would.
	info = ok(t, "half :: Water -> Water\nhalf x is div x 2\nhalf 9.0\n")
	if got := declType(t, info, "half"); got != "Water -> Water" {
		t.Errorf("half :: %s", got)
	}

	// Inside a lambda over Waters, likewise.
	ok(t, "a is [1.5 2.5] | bend (x : mul x 2) | sum\na\n")
}

// Nothing that used to be an Earth stops being one. A literal the definition
// leaves undecided settles on Earth before it is generalised, which is both
// what a reader expects and what the compilation model requires: a value of
// type `a where Reckon a` has no representation.
func TestUndecidedNumberLiteralsAreEarths(t *testing.T) {
	for _, c := range []struct{ src, name, want string }{
		{"x is 1\nx\n", "x", "Earth"},
		{"f x is add x 1\nf 1\n", "f", "Earth -> Earth"},
		{"tri 0 is 0\ntri n is add n (tri (sub n 1))\ntri 3\n", "tri", "Earth -> Earth"},
		{"half x is div x 2\nhalf 4\n", "half", "Earth -> Earth"},
	} {
		info := ok(t, c.src)
		if got := declType(t, info, c.name); got != c.want {
			t.Errorf("%s :: %s, want %s", c.name, got, c.want)
		}
	}
}

// A literal cannot become something with no Reckon Talent.
func TestNumberLiteralsStayNumbers(t *testing.T) {
	fails(t, `a is add 1 "two"`+"\na", "Air has no Reckon Talent")
	fails(t, "a is 1 2\na", "not a function")
}

// ------------------------------------------------------- Thread patterns

// A Thread's length is not drawn from a finite set, so a list of fixed-length
// patterns is never exhaustive — but `[]` together with `[x ..rest]` names
// every shape there is, which is the pair that has to work without a `_`.
func TestThreadPatternExhaustiveness(t *testing.T) {
	ok(t, "total [] is 0\ntotal [x ..rest] is add x (total rest)\ntotal [1 2]\n")
	ok(t, "f [] is 0\nf [a] is a\nf [a ..r] is a\nf [1 2]\n")

	fails(t, "f [a b] is a\nf [1 2]\n", "does not handle every case")
	fails(t, "f [a ..r] is a\nf [1 2]\n", "does not handle every case")
}

// A shorter rest pattern shadows every longer fixed one.
func TestThreadPatternReachability(t *testing.T) {
	_, errs := run(t, "f [a ..r] is a\nf [a b] is a\nf [_ ..z] is 0\nf [1 2]\n")
	if !strings.Contains(errs, "can never match") {
		t.Errorf("expected the shadowed clause to be reported, got:\n%s", errs)
	}
}

func TestThreadPatternTypes(t *testing.T) {
	info := ok(t, "f [a b] is add a b\nf _ is 0\nf [1 2]\n")
	if got := declType(t, info, "f"); got != "Thread Earth -> Earth" {
		t.Errorf("f :: %s", got)
	}
	// The rest is a Thread of the element type, not the element type — and
	// nothing here pins the element down, so it stays polymorphic.
	info = ok(t, "g [x ..rest] is rest\ng _ is []\ng [1 2]\n")
	if got := declType(t, info, "g"); got != "Thread a -> Thread a" {
		t.Errorf("g :: %s", got)
	}
}

// ------------------------------------------------------- where a failure lands

// A pipeline's argument is everything to the left of it, so blaming the
// argument put the caret under the first stage for a mistake in the last. The
// stage that could not accept what it was handed is the interesting end.
func TestAPipelineBlamesTheStageThatFailed(t *testing.T) {
	cases := []struct {
		name, src string
		line, col int
	}{
		{
			// `pairs` yields tuples, so `sift` — expecting a Thread of
			// Threads — is where this goes wrong, at column 26.
			name: "the failing stage, not the first one",
			src:  "Source | fires | pairs | sift (p : eq (head p) (last p)) | len\n",
			line: 1, col: 26,
		},
		{
			// A stage written as a bare name has no position of its own, so
			// the `|` in front of it is the next best thing.
			name: "a bare stage is blamed on its pipe",
			src:  "[1 2 3] | fires | len\n",
			line: 1, col: 9,
		},
		{
			name: "through reads the same way",
			src:  "Source\n  through fires\n  through pairs\n  through sift (p : eq (head p) (last p))\n",
			line: 4, col: 11,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bag := checkSource(t, c.src)
			if bag.Empty() {
				t.Fatal("expected a type error")
			}
			got := bag.All()[0]
			if got.Pos.Line != c.line || got.Pos.Col != c.col {
				t.Errorf("reported at %d:%d, want %d:%d\n%s",
					got.Pos.Line, got.Pos.Col, c.line, c.col, bag)
			}
		})
	}
}

// An ordinary call still blames the argument, which is where the mistake is.
func TestAPlainCallBlamesTheArgument(t *testing.T) {
	bag := checkSource(t, "a is add 1 \"two\"\na\n")
	if bag.Empty() {
		t.Fatal("expected a type error")
	}
	if got := bag.All()[0]; got.Pos.Col != 12 {
		t.Errorf("reported at column %d, want 12\n%s", got.Pos.Col, bag)
	}
}
