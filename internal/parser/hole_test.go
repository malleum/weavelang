package parser

import (
	"strings"
	"testing"

	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/diag"
)

// dumpOutput renders a program's output expression with whitespace collapsed.
func dumpOutput(t *testing.T, src string) string {
	t.Helper()
	f := parseOK(t, src)
	if f.Output() == nil {
		t.Fatalf("no output expression in %q", src)
	}
	return ast.Dump(f.Output())
}

func TestHoleInBracketsBecomesALambda(t *testing.T) {
	dumpEq(t, dumpOutput(t, "sift (mod _ 2) xs"),
		"(app sift (lambda (_) (app mod _ 2)) xs)")
}

func TestHoleInAPipelineStageBindsThePipedValue(t *testing.T) {
	// The piped value goes where the `_` is, and is bound rather than
	// substituted so it is evaluated once.
	dumpEq(t, dumpOutput(t, `w | get _ "a"`),
		`(let (weave _ w) (app get _ "a"))`)
}

func TestHoleAfterWhereMakesAPredicate(t *testing.T) {
	// `where` feeds the predicate, so a `_` there is a function, not the
	// filtered value.
	dumpEq(t, dumpOutput(t, "xs where mod _ 2"),
		"(app sift (lambda (_) (app mod _ 2)) xs)")
}

func TestEveryHoleInAGroupIsTheSameValue(t *testing.T) {
	dumpEq(t, dumpOutput(t, "bend (add _ _) xs"),
		"(app bend (lambda (_) (app add _ _)) xs)")
}

func TestNestedHoleBindsToTheInnerBrackets(t *testing.T) {
	// Deliberately the innermost group: the rule is the brackets, not "the
	// smallest sensible expression", so the mistake is a type error rather
	// than a different meaning.
	dumpEq(t, dumpOutput(t, "sift (gt 0 (mod _ 2)) xs"),
		"(app sift (app gt 0 (lambda (_) (app mod _ 2))) xs)")
}

func TestHoleInsideALambdaIsNotClaimed(t *testing.T) {
	failsWith(t, "bend (x : add x _) xs", "`_` has nothing to stand for")
}

func TestStrayHolesAreReported(t *testing.T) {
	for _, src := range []string{
		"a is add _ 1\na",
		"[_ 2]",
		"{_ : 1}",
	} {
		failsWith(t, src, "`_` has nothing to stand for")
	}
}

func TestHoleStaysAWildcardInPatterns(t *testing.T) {
	f := parseOK(t, "name _ is 1\n\nname 2\n")
	pat := f.Decls[0].Clauses[0].Params[0]
	if _, ok := pat.(*ast.PWild); !ok {
		t.Errorf("`_` in a pattern is %T, want *ast.PWild", pat)
	}
}

func failsWith(t *testing.T, src, want string) {
	t.Helper()
	bag := diag.New("test.weave", src)
	Parse(src, bag)
	if bag.Empty() {
		t.Fatalf("expected an error mentioning %q for %q", want, src)
	}
	if !strings.Contains(bag.String(), want) {
		t.Errorf("expected an error mentioning %q for %q, got:\n%s", want, src, bag)
	}
}
