package parser

import (
	"strings"
	"testing"

	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/diag"
)

func parseOK(t *testing.T, src string) *ast.File {
	t.Helper()
	bag := diag.New("test.weave", src)
	f := Parse(src, bag)
	if !bag.Empty() {
		t.Fatalf("unexpected diagnostics for:\n%s\n\n%s", src, bag)
	}
	return f
}

// dumpEq compares a dump against a want string, ignoring leading indentation
// differences in the literal so tests stay readable.
func dumpEq(t *testing.T, got, want string) {
	t.Helper()
	norm := func(s string) string {
		var out []string
		for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
			out = append(out, strings.TrimSpace(line))
		}
		s = strings.Join(out, " ")
		// Collapse layout so a compact `want` matches the indented dump.
		for _, pair := range [][2]string{{"  ", " "}, {" )", ")"}, {"( ", "("}} {
			for strings.Contains(s, pair[0]) {
				s = strings.ReplaceAll(s, pair[0], pair[1])
			}
		}
		return s
	}
	if norm(got) != norm(want) {
		t.Errorf("got:\n%s\n\nwant:\n%s", got, want)
	}
}

func TestValueDecl(t *testing.T) {
	f := parseOK(t, "answer is 42")
	dumpEq(t, ast.DumpFile(f), `
(decl answer
  (clause
    42
  )
)`)
}

func TestEqualsAlias(t *testing.T) {
	f := parseOK(t, "answer = 42")
	if len(f.Decls) != 1 || f.Decls[0].Name != "answer" {
		t.Fatalf("got %s", ast.DumpFile(f))
	}
}

func TestApplicationIsJuxtaposition(t *testing.T) {
	f := parseOK(t, "x is add 1 2")
	dumpEq(t, ast.DumpFile(f), `
(decl x
  (clause
    (app add 1 2)
  )
)`)
}

func TestPipeFeedsLastArgument(t *testing.T) {
	// `xs | sift even` must become `sift even xs`, not `sift xs even`.
	f := parseOK(t, "x is xs | sift even")
	dumpEq(t, ast.DumpFile(f), `
(decl x
  (clause
    (app sift even xs)
  )
)`)
}

func TestPipeChain(t *testing.T) {
	f := parseOK(t, "x is Source | lines | len")
	dumpEq(t, ast.DumpFile(f), `
(decl x
  (clause
    (app len (app lines Source))
  )
)`)
}

func TestWhereParticleDesugarsToSift(t *testing.T) {
	f := parseOK(t, "x is xs where even")
	dumpEq(t, ast.DumpFile(f), `
(decl x
  (clause
    (app sift even xs)
  )
)`)
}

func TestThroughIsThePipe(t *testing.T) {
	a := parseOK(t, "x is Source through pattern")
	b := parseOK(t, "x is Source | pattern")
	if ast.DumpFile(a) != ast.DumpFile(b) {
		t.Errorf("`through` should match the pipe:\n%s\n%s", ast.DumpFile(a), ast.DumpFile(b))
	}
}

// `else` and `failing` carry verbs of their own too, but feed a value rather
// than a function.
func TestElseAndFailing(t *testing.T) {
	for _, tc := range []struct{ particle, verb string }{
		{"else", "otherwise"},
		{"failing", "snag"},
	} {
		a := parseOK(t, "x is h "+tc.particle+" 0")
		b := parseOK(t, "x is h | "+tc.verb+" 0")
		if ast.DumpFile(a) != ast.DumpFile(b) {
			t.Errorf("`%s d` should be `%s d`:\n%s\n%s",
				tc.particle, tc.verb, ast.DumpFile(a), ast.DumpFile(b))
		}
	}
}

// A ward's arms go in a block or bracketed on its own line, and the two are
// the same ward.
func TestWardArmsBracketedOnOneLine(t *testing.T) {
	a := parseOK(t, "f c is ward c (Light : 1) (Shadow : 0)\nf Light\n")
	b := parseOK(t, "f c is\n  ward c\n    Light : 1\n    Shadow : 0\nf Light\n")
	if ast.DumpFile(a) != ast.DumpFile(b) {
		t.Errorf("the two spellings should be one ward:\n%s\n%s", ast.DumpFile(a), ast.DumpFile(b))
	}
}

// The bracketed form is the only one an expression has room for, so it has to
// work inside one — which is what makes `ward` usable in a pipeline stage.
func TestWardInsideAnExpression(t *testing.T) {
	f := parseOK(t, "xs is [1 2] through bend (n gives ward (even n) (Light : n) (Shadow : 0))\nxs\n")
	if !strings.Contains(ast.DumpFile(f), "ward") {
		t.Errorf("expected a ward inside the lambda:\n%s", ast.DumpFile(f))
	}
}

// `as` is not the pipe: it carries a verb of its own, the way `where` does.
func TestAsIsBend(t *testing.T) {
	a := parseOK(t, "x is xs as inc")
	b := parseOK(t, "x is xs | bend inc")
	if ast.DumpFile(a) != ast.DumpFile(b) {
		t.Errorf("`as f` should be `bend f`:\n%s\n%s", ast.DumpFile(a), ast.DumpFile(b))
	}
}

func TestMultiClauseDeclCollects(t *testing.T) {
	src := strings.Join([]string{
		"fib 0 is 0",
		"fib 1 is 1",
		"fib n is add (fib (sub n 1)) (fib (sub n 2))",
	}, "\n")
	f := parseOK(t, src)
	if len(f.Decls) != 1 {
		t.Fatalf("expected 1 decl, got %d", len(f.Decls))
	}
	if got := len(f.Decls[0].Clauses); got != 3 {
		t.Fatalf("expected 3 clauses, got %d", got)
	}
	if f.Decls[0].Arity() != 1 {
		t.Errorf("arity: got %d", f.Decls[0].Arity())
	}
}

func TestClauseArityMismatchIsAnError(t *testing.T) {
	src := "f 0 is 0\nf a b is 1"
	bag := diag.New("t.weave", src)
	Parse(src, bag)
	if bag.Empty() {
		t.Fatal("expected an arity diagnostic")
	}
}

func TestDestructuringParams(t *testing.T) {
	f := parseOK(t, "dist (knot r c) is add r c")
	dumpEq(t, ast.DumpFile(f), `
(decl dist
  (clause (params (knot r c))
    (app add r c)
  )
)`)
}

func TestBlockWithLocals(t *testing.T) {
	src := strings.Join([]string{
		"solve is",
		"  weave nums is Source | lines",
		"  channel big n is gt 100 n",
		"  nums where big | len",
	}, "\n")
	f := parseOK(t, src)
	dumpEq(t, ast.DumpFile(f), `
(decl solve
  (clause
    (let
      (weave nums
        (app lines Source)
      )
      (channel big (n)
        (app gt 100 n)
      )
      (app len (app sift big nums))
    )
  )
)`)
}

func TestWardArms(t *testing.T) {
	src := strings.Join([]string{
		"f x is",
		"  ward x",
		"    Held n : n",
		"    Stilled : 0",
	}, "\n")
	f := parseOK(t, src)
	dumpEq(t, ast.DumpFile(f), `
(decl f
  (clause (params x)
    (ward
      x
      (arm (Held n)
        n
      )
      (arm Stilled
        0
      )
    )
  )
)`)
}

func TestWardWildcardArm(t *testing.T) {
	src := "f x is\n  ward x\n    0 : 1\n    _ : 2"
	f := parseOK(t, src)
	w := f.Decls[0].Clauses[0].Body.(*ast.Ward)
	if len(w.Arms) != 2 {
		t.Fatalf("got %d arms", len(w.Arms))
	}
	if _, ok := w.Arms[1].Pat.(*ast.PWild); !ok {
		t.Errorf("second arm should be a wildcard, got %T", w.Arms[1].Pat)
	}
}

func TestNestedWardInArm(t *testing.T) {
	src := strings.Join([]string{
		"f x is",
		"  ward x",
		"    Held n :",
		"      ward n",
		"        0 : 1",
		"        _ : 2",
		"    Stilled : 0",
	}, "\n")
	f := parseOK(t, src)
	w := f.Decls[0].Clauses[0].Body.(*ast.Ward)
	if len(w.Arms) != 2 {
		t.Fatalf("expected 2 arms, got %d", len(w.Arms))
	}
	if _, ok := w.Arms[0].Body.(*ast.Ward); !ok {
		t.Errorf("expected nested ward, got %T", w.Arms[0].Body)
	}
}

func TestLambda(t *testing.T) {
	f := parseOK(t, "x is bend (a : add 1 a) xs")
	dumpEq(t, ast.DumpFile(f), `
(decl x
  (clause
    (app bend (lambda (a) (app add 1 a)) xs)
  )
)`)
}

func TestMultiParamLambda(t *testing.T) {
	f := parseOK(t, "x is braid (acc n : add acc n) 0 xs")
	lam := f.Decls[0].Clauses[0].Body.(*ast.App).Args[0].(*ast.Lambda)
	if len(lam.Params) != 2 {
		t.Errorf("expected 2 params, got %d", len(lam.Params))
	}
}

func TestParenGroupingIsNotALambda(t *testing.T) {
	f := parseOK(t, "x is add (mul 2 3) 4")
	dumpEq(t, ast.DumpFile(f), `
(decl x
  (clause
    (app add (app mul 2 3) 4)
  )
)`)
}

func TestTuple(t *testing.T) {
	f := parseOK(t, `x is (1, "a")`)
	dumpEq(t, ast.DumpFile(f), `
(decl x
  (clause
    (tuple 1 "a")
  )
)`)
}

func TestThreadLiteral(t *testing.T) {
	f := parseOK(t, "x is [1 2 3]")
	dumpEq(t, ast.DumpFile(f), `
(decl x
  (clause
    (thread 1 2 3)
  )
)`)
}

func TestWebLiteral(t *testing.T) {
	f := parseOK(t, `x is {"a" : 1  "b" : 2}`)
	dumpEq(t, ast.DumpFile(f), `
(decl x
  (clause
    (web (pair "a" 1) (pair "b" 2))
  )
)`)
}

func TestInlineLet(t *testing.T) {
	f := parseOK(t, "x is weave a is 1, b is 2 into add a b")
	dumpEq(t, ast.DumpFile(f), `
(decl x
  (clause
    (let
      (weave a 1)
      (weave b 2)
      (app add a b)
    )
  )
)`)
}

func TestOutputExpression(t *testing.T) {
	src := "part1 is 42\npart1"
	f := parseOK(t, src)
	if f.Output() == nil {
		t.Fatal("expected an output expression")
	}
	if v, ok := f.Output().(*ast.Var); !ok || v.Name != "part1" {
		t.Errorf("got %T", f.Output())
	}
}

// Several bare expressions are allowed, so that things being tried out can sit
// side by side instead of taking turns behind a comment. The program prints the
// last; `weave trace` reports every one.
func TestSeveralOutputsAreKept(t *testing.T) {
	src := "a is 1\na\nadd a 1\nadd a 2\n"
	bag := diag.New("t.weave", src)
	f := Parse(src, bag)
	if !bag.Empty() {
		t.Fatalf("expected no diagnostics:\n%s", bag)
	}
	if len(f.Outputs) != 3 {
		t.Fatalf("expected three bare expressions, got %d", len(f.Outputs))
	}
	// The last one is what the program prints.
	if app, ok := f.Output().(*ast.App); !ok || len(app.Args) != 2 {
		t.Errorf("expected the last expression to be the output, got %T", f.Output())
	}
	// In source order, so the editor can put each beside its own line.
	for i, want := range []int{2, 3, 4} {
		if got := f.Outputs[i].Pos().Line; got != want {
			t.Errorf("output %d is on line %d, want %d", i, got, want)
		}
	}
}

func TestSignature(t *testing.T) {
	src := "len :: Thread a -> Earth\nlen xs is 0"
	f := parseOK(t, src)
	if f.Decls[0].Sig == nil {
		t.Fatal("signature was not attached")
	}
	dumpEq(t, ast.DumpFile(f), `
(decl len
  (sig (Thread a -> Earth))
  (clause (params xs)
    0
  )
)`)
}

func TestContinuationInsideBrackets(t *testing.T) {
	src := "x is bend (a :\n    add 1 a) xs"
	f := parseOK(t, src)
	dumpEq(t, ast.DumpFile(f), `
(decl x
  (clause
    (app bend (lambda (a) (app add 1 a)) xs)
  )
)`)
}

func TestWardInsideParensIsRejectedClearly(t *testing.T) {
	// Layout is suspended inside brackets, so a ward cannot seek its arms.
	// The parser must say so rather than fail obscurely.
	src := "x is add 1 (ward y\n  0 : 1)"
	bag := diag.New("t.weave", src)
	Parse(src, bag)
	if bag.Empty() {
		t.Fatal("expected a diagnostic")
	}
	if !strings.Contains(bag.String(), "ward") {
		t.Errorf("diagnostic should mention ward:\n%s", bag)
	}
}

func TestErrorsRecoverAndReportSeveral(t *testing.T) {
	src := "a is $\nb is $\nc is 1"
	bag := diag.New("t.weave", src)
	f := Parse(src, bag)
	if bag.Len() < 2 {
		t.Errorf("expected several diagnostics, got %d:\n%s", bag.Len(), bag)
	}
	// Parsing must still reach the last good declaration.
	found := false
	for _, d := range f.Decls {
		if d.Name == "c" {
			found = true
		}
	}
	if !found {
		t.Errorf("parser did not recover to reach `c`:\n%s", ast.DumpFile(f))
	}
}

func TestNoOperatorsDiagnostic(t *testing.T) {
	src := "a is 1 - 2"
	bag := diag.New("t.weave", src)
	Parse(src, bag)
	if !strings.Contains(bag.String(), "sub") {
		t.Errorf("expected a hint pointing cell `sub`:\n%s", bag)
	}
}

func TestParseTerminatesOnGarbage(t *testing.T) {
	// Guards against an infinite loop in error recovery.
	for _, src := range []string{"$", ")", "is", "((((", "ward", "weave", ":", "|"} {
		bag := diag.New("t.weave", src)
		Parse(src, bag)
	}
}

// A binding may take its value apart. A bare name is still a name — `weave x is
// 1` binds, it does not match — so only the bracketed shapes and `_` count.
func TestParsesADestructuringBinding(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"a twine", "f p is\n  weave (a, b) is p\n  a\n", "(weave-pattern (, a b)"},
		{"a thread", "f p is\n  weave [a, b] is p\n  a\n", "(weave-pattern (thread a b)"},
		{"a wildcard", "f p is\n  weave _ is p\n  1\n", "(weave-pattern _"},
		{"inline", "f p is weave (a, b) is p into a\n", "(weave-pattern (, a b)"},
		{"a plain name is still a name", "f p is\n  weave x is p\n  x\n", "(weave x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bag := diag.New("t.weave", tc.src)
			file := Parse(tc.src, bag)
			if !bag.Empty() {
				t.Fatalf("did not parse:\n%s", bag)
			}
			if got := ast.DumpFile(file); !strings.Contains(got, tc.want) {
				t.Errorf("expected %q in:\n%s", tc.want, got)
			}
		})
	}
}

// `channel` declares a function, and a function has a name.
func TestAChannelStillNeedsAName(t *testing.T) {
	src := "f p is\n  channel (a, b) is p\n  a\n"
	bag := diag.New("t.weave", src)
	Parse(src, bag)
	if bag.Empty() {
		t.Fatal("expected an error")
	}
	if !strings.Contains(bag.String(), "expected a name after `channel`") {
		t.Errorf("got:\n%s", bag)
	}
}
