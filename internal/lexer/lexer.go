// Package lexer turns Weave source text into a token stream.
//
// Weave is indentation-sensitive: blocks are introduced by indenting, so the
// lexer synthesises Newline, Indent and Dedent tokens (the "layout" tokens)
// alongside the ones that appear literally in the source. Layout is suspended
// inside brackets, which lets an expression span lines freely:
//
//	xs | seek (a :
//	  gt 10 a)
package lexer

import (
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/token"
)

// IndentWidth is the number of spaces per indentation level that `weave fmt`
// emits. The lexer itself accepts any consistent widths.
const IndentWidth = 2

type lexer struct {
	src      string
	bag      *diag.Bag
	toks     []token.Token
	comments []token.Comment

	off  int // byte offset of the next rune to read
	line int
	col  int

	depth   int   // bracket nesting depth; >0 suspends layout
	indents []int // stack of active indentation columns, always starting at 0
	atLine  bool  // true when positioned at the start of a logical line
	emitted bool  // whether the current logical line produced any token

	// Tracking for the layout rule below: whether the logical line just ended
	// was one that opens a block.
	lastKind token.Kind // last significant token of the current logical line
	hasWard  bool       // the current logical line contains a top-level `ward`
	prevKind token.Kind // the same, for the line that just ended
	prevWard bool
}

// Lex scans src and returns its tokens. Lexical errors are recorded in bag;
// the returned stream is still well-formed (it always ends with EOF) so the
// parser can run and report further problems in one pass.
func Lex(src string, bag *diag.Bag) []token.Token {
	toks, _ := LexAll(src, bag)
	return toks
}

// LexAll scans src and returns its tokens along with the comments, which the
// formatter needs and every other phase ignores.
func LexAll(src string, bag *diag.Bag) ([]token.Token, []token.Comment) {
	l := &lexer{
		src:     src,
		bag:     bag,
		line:    1,
		col:     1,
		indents: []int{0},
		atLine:  true,
	}
	l.run()
	return l.toks, l.comments
}

func (l *lexer) run() {
	for {
		if l.atLine && l.depth == 0 {
			if !l.layout() {
				break
			}
		}
		l.skipSpaceAndComments()
		if l.eof() {
			break
		}
		if l.peek() == '\n' {
			l.newlineToken()
			continue
		}
		l.scanToken()
	}
	l.finish()
}

// layout handles the start of a physical line: it measures indentation and
// emits Indent or Dedent tokens. Blank and comment-only lines carry no layout
// meaning and are skipped. It reports false at end of input.
func (l *lexer) layout() bool {
	for {
		start := l.off
		width, ok := l.measureIndent()
		if !ok {
			return false
		}
		if l.eof() {
			return false
		}
		// Blank or comment-only lines have no layout significance.
		if l.peek() == '\n' {
			l.advance()
			continue
		}
		if l.peek() == '#' {
			l.skipComment()
			continue
		}
		// A line that opens with `|` or a particle continues the line above,
		// so that a long pipeline can be broken across lines:
		//
		//	Source
		//	  | lines
		//	  | bend (parse Earth)
		if l.startsContinuation() {
			l.atLine = false
			l.joinPreviousLine()
			return true
		}

		// Indentation only opens a block after something that syntactically
		// wants one. Everywhere else a deeper line simply continues the line
		// above, which is what lets an application span lines:
		//
		//	pick (member seen k)
		//	  (walk rest seen)
		//	  (walk (push rest k) (join seen k))
		if width > l.indents[len(l.indents)-1] && !l.prevOpensBlock() {
			l.atLine = false
			l.joinPreviousLine()
			return true
		}

		l.atLine = false
		l.applyIndent(width, start)
		return true
	}
}

// measureIndent consumes leading whitespace and returns its visual width.
func (l *lexer) measureIndent() (int, bool) {
	width := 0
	for !l.eof() {
		switch l.peek() {
		case ' ':
			width++
			l.advance()
		case '\t':
			l.bag.AddHint(l.pos(), "indent with spaces", "tab used for indentation")
			width += IndentWidth
			l.advance()
		case '\r':
			l.advance()
		default:
			return width, true
		}
	}
	return width, false
}

// applyIndent compares width against the indentation stack and emits the
// Indent or Dedent tokens needed to reconcile them.
func (l *lexer) applyIndent(width, lineStart int) {
	pos := l.pos()
	top := l.indents[len(l.indents)-1]
	switch {
	case width > top:
		l.indents = append(l.indents, width)
		l.push(token.Token{Kind: token.Indent, Pos: pos})
	case width < top:
		for len(l.indents) > 1 && l.indents[len(l.indents)-1] > width {
			l.indents = l.indents[:len(l.indents)-1]
			l.push(token.Token{Kind: token.Dedent, Pos: pos})
		}
		if l.indents[len(l.indents)-1] != width {
			l.bag.AddHint(pos, "align this line with an enclosing block",
				"inconsistent indentation: no enclosing block starts at column %d", width+1)
			l.indents = append(l.indents, width)
		}
	}
	_ = lineStart
}

// prevOpensBlock reports whether the logical line that just ended is one that
// introduces an indented block: a binding (`... is`), a ward arm (`... :`), or
// the `ward` line whose arms follow.
func (l *lexer) prevOpensBlock() bool {
	return l.prevKind == token.Is || l.prevKind == token.Colon || l.prevWard
}

// continuationWords are the infix particles that may open a continuation line,
// alongside the pipeline symbol itself.
var continuationWords = []string{"where", "as", "through", "else", "failing"}

// CompletionKeywords is the reserved words worth offering in an editor, read
// from the token table so that a word cannot be reserved without the editor
// knowing it.
//
// `_` is left out because it is not a word; `this` is the spelling of it that
// `weave fmt` prints.
func CompletionKeywords() []string {
	var out []string
	for name := range token.Keywords() {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// startsContinuation reports whether the text at the cursor begins with a
// token that can only continue the previous logical line.
func (l *lexer) startsContinuation() bool {
	if l.peek() == '|' {
		return true
	}
	rest := l.src[l.off:]
	for _, w := range continuationWords {
		if strings.HasPrefix(rest, w) && !isIdentRune(byteAt(rest, len(w))) {
			return true
		}
	}
	return false
}

// joinPreviousLine undoes the line break before a continuation line by
// dropping the Newline the lexer already emitted.
func (l *lexer) joinPreviousLine() {
	if n := len(l.toks); n > 0 && l.toks[n-1].Kind == token.Newline {
		l.toks = l.toks[:n-1]
		l.emitted = true
		// The two lines are one logical line again, so restore what the layout
		// rule knows about it.
		l.lastKind = l.prevKind
		l.hasWard = l.prevWard
	}
}

func byteAt(s string, i int) byte {
	if i >= len(s) {
		return 0
	}
	return s[i]
}

// newlineToken ends a logical line.
func (l *lexer) newlineToken() {
	pos := l.pos()
	l.advance()
	if l.emitted {
		l.push(token.Token{Kind: token.Newline, Pos: pos})
		l.emitted = false
		l.prevKind, l.prevWard = l.lastKind, l.hasWard
		l.lastKind, l.hasWard = token.EOF, false
	}
	l.atLine = true
}

// finish closes out the token stream at end of input.
func (l *lexer) finish() {
	pos := l.pos()
	if l.emitted {
		l.push(token.Token{Kind: token.Newline, Pos: pos})
	}
	for len(l.indents) > 1 {
		l.indents = l.indents[:len(l.indents)-1]
		l.push(token.Token{Kind: token.Dedent, Pos: pos})
	}
	l.push(token.Token{Kind: token.EOF, Pos: pos})
}

func (l *lexer) scanToken() {
	pos := l.pos()
	c := l.peek()

	switch {
	case isDigit(c):
		l.scanNumber(pos)
		return
	case c == '_' && !isIdentRune(l.peekAt(1)):
		l.advance()
		l.push(token.Token{Kind: token.Underscore, Pos: pos, Lit: "_"})
		return
	case isIdentStart(c):
		l.scanIdent(pos)
		return
	case c == '\'':
		l.scanChar(pos)
		return
	case c == '"':
		l.scanText(pos)
		return
	}

	// `..` opens a rest pattern. A lone `.` is only ever a decimal point, which
	// scanNumber has already taken by the time we get here.
	if c == '.' && l.peekAt(1) == '.' {
		l.advance()
		l.advance()
		l.push(token.Token{Kind: token.DotDot, Pos: pos})
		return
	}

	l.advance()
	switch c {
	case '|':
		l.push(token.Token{Kind: token.Pipe, Pos: pos})
	case ':':
		if !l.eof() && l.peek() == ':' {
			l.advance()
			l.push(token.Token{Kind: token.ColonColon, Pos: pos})
		} else {
			l.push(token.Token{Kind: token.Colon, Pos: pos})
		}
	case '-':
		if !l.eof() && l.peek() == '>' {
			l.advance()
			l.push(token.Token{Kind: token.Arrow, Pos: pos})
		} else {
			l.bag.AddHint(pos, "Weave has no operators; negate with `neg` and subtract with `sub`",
				"unexpected `-`")
		}
	case ',':
		l.push(token.Token{Kind: token.Comma, Pos: pos})
	case '=':
		// `=` is an accepted alias for `is`.
		l.push(token.Token{Kind: token.Is, Pos: pos, Lit: "="})
	case '(':
		l.depth++
		l.push(token.Token{Kind: token.LParen, Pos: pos})
	case ')':
		l.closeBracket()
		l.push(token.Token{Kind: token.RParen, Pos: pos})
	case '[':
		l.depth++
		l.push(token.Token{Kind: token.LBracket, Pos: pos})
	case ']':
		l.closeBracket()
		l.push(token.Token{Kind: token.RBracket, Pos: pos})
	case '{':
		l.depth++
		l.push(token.Token{Kind: token.LBrace, Pos: pos})
	case '}':
		l.closeBracket()
		l.push(token.Token{Kind: token.RBrace, Pos: pos})
	default:
		l.bag.Add(pos, "unexpected character %q", string(c))
	}
}

func (l *lexer) closeBracket() {
	if l.depth > 0 {
		l.depth--
	}
}

func (l *lexer) scanIdent(pos token.Pos) {
	start := l.off
	for !l.eof() && isIdentRune(l.peek()) {
		l.advance()
	}
	name := l.src[start:l.off]
	first, _ := utf8.DecodeRuneInString(name)
	kind := token.Upper
	if !unicode.IsUpper(first) {
		kind = token.LookupIdent(name)
	}
	l.push(token.Token{Kind: kind, Pos: pos, Lit: name})
}

func (l *lexer) scanNumber(pos token.Pos) {
	start := l.off
	for !l.eof() && (isDigit(l.peek()) || l.peek() == '_') {
		l.advance()
	}
	isFloat := false
	// A '.' is only a decimal point when a digit follows it.
	if !l.eof() && l.peek() == '.' && isDigit(l.peekAt(1)) {
		isFloat = true
		l.advance()
		for !l.eof() && (isDigit(l.peek()) || l.peek() == '_') {
			l.advance()
		}
	}
	lit := l.src[start:l.off]
	clean := strings.ReplaceAll(lit, "_", "")
	if isFloat {
		f, err := strconv.ParseFloat(clean, 64)
		if err != nil {
			l.bag.Add(pos, "malformed Water literal %q", lit)
		}
		l.push(token.Token{Kind: token.Float, Pos: pos, Lit: lit, Float: f})
		return
	}
	n, err := strconv.ParseInt(clean, 10, 64)
	if err != nil {
		l.bag.AddHint(pos, "Earth values are 64-bit", "Earth literal %q is out of range", lit)
	}
	l.push(token.Token{Kind: token.Int, Pos: pos, Lit: lit, Int: n})
}

func (l *lexer) scanChar(pos token.Pos) {
	l.advance() // opening quote
	if l.eof() || l.peek() == '\n' {
		l.bag.Add(pos, "unterminated Fire literal")
		l.push(token.Token{Kind: token.Char, Pos: pos})
		return
	}
	var r rune
	if l.peek() == '\\' {
		l.advance()
		r = l.readEscape(pos)
	} else {
		r, _ = utf8.DecodeRuneInString(l.src[l.off:])
		l.advanceRune(r)
	}
	if l.eof() || l.peek() != '\'' {
		l.bag.AddHint(pos, "Fire holds exactly one rune; use \"...\" for Air",
			"unterminated Fire literal")
	} else {
		l.advance()
	}
	l.push(token.Token{Kind: token.Char, Pos: pos, Lit: string(r), Char: r})
}

func (l *lexer) scanText(pos token.Pos) {
	l.advance() // opening quote
	var sb strings.Builder
	for {
		if l.eof() || l.peek() == '\n' {
			l.bag.Add(pos, "unterminated Air literal")
			break
		}
		if l.peek() == '"' {
			l.advance()
			break
		}
		if l.peek() == '\\' {
			l.advance()
			sb.WriteRune(l.readEscape(pos))
			continue
		}
		r, _ := utf8.DecodeRuneInString(l.src[l.off:])
		l.advanceRune(r)
		sb.WriteRune(r)
	}
	l.push(token.Token{Kind: token.Text, Pos: pos, Lit: sb.String()})
}

func (l *lexer) readEscape(pos token.Pos) rune {
	if l.eof() {
		l.bag.Add(pos, "unterminated escape sequence")
		return 0
	}
	c := l.peek()
	l.advance()
	switch c {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'r':
		return '\r'
	case '0':
		return 0
	case '\\':
		return '\\'
	case '\'':
		return '\''
	case '"':
		return '"'
	default:
		l.bag.Add(pos, "unknown escape sequence \\%s", string(c))
		return rune(c)
	}
}

func (l *lexer) skipSpaceAndComments() {
	for !l.eof() {
		switch l.peek() {
		case ' ', '\t', '\r':
			l.advance()
		case '#':
			l.skipComment()
		case '\n':
			// Inside brackets a newline is pure whitespace.
			if l.depth > 0 {
				l.advance()
				continue
			}
			return
		default:
			return
		}
	}
}

func (l *lexer) skipComment() {
	pos := l.pos()
	start := l.off
	for !l.eof() && l.peek() != '\n' {
		l.advance()
	}
	text := strings.TrimRight(strings.TrimPrefix(l.src[start:l.off], "#"), " \t\r")
	l.comments = append(l.comments, token.Comment{Pos: pos, Text: text})
}

func (l *lexer) push(t token.Token) {
	l.toks = append(l.toks, t)
	switch t.Kind {
	case token.Newline, token.Indent, token.Dedent, token.EOF:
	default:
		l.emitted = true
		l.lastKind = t.Kind
		if t.Kind == token.Ward && l.depth == 0 {
			l.hasWard = true
		}
	}
}

func (l *lexer) pos() token.Pos { return token.Pos{Line: l.line, Col: l.col} }

func (l *lexer) eof() bool { return l.off >= len(l.src) }

func (l *lexer) peek() byte {
	if l.eof() {
		return 0
	}
	return l.src[l.off]
}

func (l *lexer) peekAt(n int) byte {
	if l.off+n >= len(l.src) {
		return 0
	}
	return l.src[l.off+n]
}

func (l *lexer) advance() {
	if l.eof() {
		return
	}
	if l.src[l.off] == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	l.off++
}

// advanceRune consumes a multi-byte rune as a single column.
func (l *lexer) advanceRune(r rune) {
	l.off += utf8.RuneLen(r)
	l.col++
}

func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentStart(c byte) bool { return c == '_' || c >= utf8.RuneSelf || isAlpha(c) }
func isIdentRune(c byte) bool  { return isIdentStart(c) || isDigit(c) }
func isAlpha(c byte) bool      { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
