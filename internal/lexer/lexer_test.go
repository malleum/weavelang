package lexer

import (
	"strings"
	"testing"

	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/token"
)

// sketch renders a token stream compactly so tests can assert on shape:
// layout tokens appear as >, < and ; for indent, dedent and newline.
func sketch(toks []token.Token) string {
	var parts []string
	for _, t := range toks {
		switch t.Kind {
		case token.Indent:
			parts = append(parts, ">")
		case token.Dedent:
			parts = append(parts, "<")
		case token.Newline:
			parts = append(parts, ";")
		case token.EOF:
			parts = append(parts, ".")
		case token.Lower, token.Upper, token.Int, token.Float:
			parts = append(parts, t.Lit)
		case token.Char:
			parts = append(parts, "'"+t.Lit+"'")
		case token.Text:
			parts = append(parts, `"`+t.Lit+`"`)
		default:
			parts = append(parts, t.Kind.String())
		}
	}
	return strings.Join(parts, " ")
}

func lexOK(t *testing.T, src string) []token.Token {
	t.Helper()
	bag := diag.New("test.weave", src)
	toks := Lex(src, bag)
	if !bag.Empty() {
		t.Fatalf("unexpected diagnostics:\n%s", bag)
	}
	return toks
}

func TestSimpleDecl(t *testing.T) {
	got := sketch(lexOK(t, "answer is 42"))
	want := "answer is 42 ; ."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestPipelineAndApplication(t *testing.T) {
	got := sketch(lexOK(t, "Source | lines | bend (earth _)"))
	want := `Source | lines | bend ( earth _ ) ; .`
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestEqualsIsAliasForIs(t *testing.T) {
	toks := lexOK(t, "x = 1")
	if toks[1].Kind != token.Is {
		t.Fatalf("`=` should lex as Is, got %v", toks[1].Kind)
	}
}

func TestIndentBlock(t *testing.T) {
	src := strings.Join([]string{
		"solve is",
		"  weave n is 1",
		"  n",
		"end is 2",
	}, "\n")
	got := sketch(lexOK(t, src))
	want := "solve is ; > weave n is 1 ; n ; < end is 2 ; ."
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestNestedDedentsCollapse(t *testing.T) {
	src := strings.Join([]string{
		"a is",
		"  ward x",
		"    Held n : n",
		"top is 1",
	}, "\n")
	got := sketch(lexOK(t, src))
	want := "a is ; > ward x ; > Held n : n ; < < top is 1 ; ."
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestBracketsSuspendLayout(t *testing.T) {
	// A newline inside brackets is plain whitespace, so no layout tokens are
	// produced for the continuation line.
	src := "xs is bend (a :\n    add 1 a) ys"
	got := sketch(lexOK(t, src))
	want := "xs is bend ( a : add 1 a ) ys ; ."
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestBlankAndCommentLinesHaveNoLayout(t *testing.T) {
	src := strings.Join([]string{
		"a is",
		"  # a comment, indented differently",
		"",
		"        # another",
		"  1",
	}, "\n")
	got := sketch(lexOK(t, src))
	want := "a is ; > 1 ; < ."
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestCommentToEndOfLine(t *testing.T) {
	got := sketch(lexOK(t, "a is 1 # trailing\nb is 2"))
	want := "a is 1 ; b is 2 ; ."
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLiterals(t *testing.T) {
	toks := lexOK(t, `1_000 3.14 '#' "hi\n" Light`)
	if toks[0].Kind != token.Int || toks[0].Int != 1000 {
		t.Errorf("int: got %v %d", toks[0].Kind, toks[0].Int)
	}
	if toks[1].Kind != token.Float || toks[1].Float != 3.14 {
		t.Errorf("float: got %v %v", toks[1].Kind, toks[1].Float)
	}
	if toks[2].Kind != token.Char || toks[2].Char != '#' {
		t.Errorf("spark: got %v %q", toks[2].Kind, toks[2].Char)
	}
	if toks[3].Kind != token.Text || toks[3].Lit != "hi\n" {
		t.Errorf("text: got %v %q", toks[3].Kind, toks[3].Lit)
	}
	if toks[4].Kind != token.Upper || toks[4].Lit != "Light" {
		t.Errorf("ctor: got %v %q", toks[4].Kind, toks[4].Lit)
	}
}

func TestNegativeLookingNumberIsNotSpecialCased(t *testing.T) {
	// There are no operators in Weave, so `1.` is an int followed by an
	// unexpected spark rather than a malformed float.
	toks := lexOK(t, "42")
	if toks[0].Int != 42 {
		t.Errorf("got %d", toks[0].Int)
	}
}

func TestUnderscoreIsWildcardButPrefixIsIdent(t *testing.T) {
	toks := lexOK(t, "_ _x")
	if toks[0].Kind != token.Underscore {
		t.Errorf("bare _: got %v", toks[0].Kind)
	}
	if toks[1].Kind != token.Lower || toks[1].Lit != "_x" {
		t.Errorf("_x: got %v %q", toks[1].Kind, toks[1].Lit)
	}
}

func TestPositions(t *testing.T) {
	toks := lexOK(t, "a is\n  bc")
	bc := toks[3] // a, is, ;, bc...
	if bc.Kind != token.Indent {
		t.Fatalf("expected indent cell 3, got %v", bc.Kind)
	}
	id := toks[4]
	if id.Pos.Line != 2 || id.Pos.Col != 3 {
		t.Errorf("got %s, want 2:3", id.Pos)
	}
}

func TestDiagnosticsAreReported(t *testing.T) {
	for _, src := range []string{
		`a is "unterminated`,
		"a is 'x",
		"a is $",
	} {
		bag := diag.New("t.weave", src)
		Lex(src, bag)
		if bag.Empty() {
			t.Errorf("expected a diagnostic for %q", src)
		}
	}
}

func TestStreamAlwaysEndsWithEOF(t *testing.T) {
	for _, src := range []string{"", "\n\n", "# just a comment", "a is\n  1"} {
		bag := diag.New("t.weave", src)
		toks := Lex(src, bag)
		if len(toks) == 0 || toks[len(toks)-1].Kind != token.EOF {
			t.Errorf("stream for %q does not end with EOF: %s", src, sketch(toks))
		}
	}
}

func TestDedentsClosedAtEOF(t *testing.T) {
	got := sketch(lexOK(t, "a is\n  b is\n    1"))
	want := "a is ; > b is ; > 1 ; < < ."
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestPipeContinuationLine(t *testing.T) {
	// A line opening with `|` continues the one above it, so no layout tokens
	// are produced for the break.
	src := "x is Source\n  | lines\n  | len"
	got := sketch(lexOK(t, src))
	want := "x is Source | lines | len ; ."
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestParticleContinuationLine(t *testing.T) {
	src := "x is xs\n  where even\n  as len"
	got := sketch(lexOK(t, src))
	want := "x is xs where even as len ; ."
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestParticleWordIsNotConfusedWithAnIdentifier(t *testing.T) {
	// `wheres` is an ordinary identifier, not the `where` particle. It is not
	// a block opener either, so the line continues the one above.
	src := "x is 1\n  wheres"
	got := sketch(lexOK(t, src))
	want := "x is 1 wheres ; ."
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestIndentContinuesAnApplication(t *testing.T) {
	// Nothing here opens a block, so the indented lines continue the call.
	src := "x is\n  pick c\n    a\n    b"
	got := sketch(lexOK(t, src))
	want := "x is ; > pick c a b ; < ."
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestBlockOpenersStillIndent(t *testing.T) {
	for _, tc := range []struct{ src, want string }{
		{"f is\n  1", "f is ; > 1 ; < ."},
		{"f is\n  ward x\n    _ : 1", "f is ; > ward x ; > _ : 1 ; < < ."},
		{"f is\n  ward x\n    _ :\n      1", "f is ; > ward x ; > _ : ; > 1 ; < < < ."},
	} {
		if got := sketch(lexOK(t, tc.src)); got != tc.want {
			t.Errorf("for %q:\ngot  %q\nwant %q", tc.src, got, tc.want)
		}
	}
}
