// Package format renders a Weave program in canonical form.
//
// The formatter is deliberately opinionated: every program with the same
// meaning comes out looking the same. Rather than tidying the text it was
// given, it throws the layout away and prints the syntax tree, so redundant
// parentheses, stray blank lines and inconsistent indentation all disappear by
// construction — there is no code path that could reproduce them.
//
// It has one setting, and it is about spelling rather than layout. Weave gives
// its punctuation words — `is` for `=`, `gives` for `:`, `through` for `|`,
// `this` for `_` — and the words are the language while the symbols are the
// shorthand, so the words are what it prints. `weave fmt -terse` prints the
// symbols instead. The two are the same tokens to the lexer, so either can be
// read back.
//
// The setting reaches further than spelling one token as another. The
// formatter *chooses* between forms rather than keeping the one that was
// written, wherever the choice is not a matter of meaning:
//
//   - `sift p` prints as `where p` and `bend f` as `as f` in the wordy style,
//     and back into verbs in the terse one. A particle desugars to its verb by
//     name, so the two resolve to the same thing even where the name has been
//     shadowed — see stage() below.
//   - A lambda prints as a hole group wherever that reads back as the same
//     function, so `(x : add x 1)` becomes `(add this 1)`. See holes.go, and
//     in particular why the check is done by doing it.
//   - A ward keeps whichever of its two forms was written, for as long as it
//     fits, because there the choice *is* a matter of taste.
//
// Two things survive that a tree alone would lose. Comments are collected by
// the lexer and reattached by line, and the parser records whether a call was
// written as a pipeline, so `xs | sift even` does not come back as
// `sift even xs`.
package format

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/lexer"
	"github.com/malleum/weave/internal/parser"
	"github.com/malleum/weave/internal/token"
)

// Width is the column the formatter tries to stay inside. A pipeline longer
// than this is broken one stage per line.
const Width = 80

// Indent is one level of indentation.
const Indent = "  "

// Source formats a Weave program. It reports an error if the program does not
// parse, since formatting something the compiler cannot read would be
// guesswork.
func Source(src string, file string) (string, error) {
	return SourceStyle(src, file, Wordy)
}

// Style is how the punctuation is spelled.
type Style int

const (
	// Wordy prints `is`, `gives` and `through`.
	Wordy Style = iota
	// Terse prints `=`, `:` and `|`.
	Terse
)

// SourceStyle formats a program with the punctuation spelled one way or the
// other. The syntax tree is the same either way; only the words change.
func SourceStyle(src string, file string, style Style) (string, error) {
	bag := diag.New(file, src)
	_, comments := lexer.LexAll(src, bag)
	// The parser reports into the same bag, so a program it could not read is
	// refused rather than printed. It recovers by leaving the unreadable part
	// out of the tree, and printing that tree would quietly delete the line
	// the mistake was on — the one thing a formatter must never do.
	tree := parser.Parse(src, bag)
	if err := bag.Err(); err != nil {
		return "", err
	}

	p := &printer{comments: comments, style: style}
	p.file(tree)
	return p.finish(), nil
}

type printer struct {
	sb       strings.Builder
	comments []token.Comment
	used     int // comments before this index have been emitted
	depth    int
	style    Style
	// holeText remembers whether each lambda could be written with the hole
	// words, since a pipeline is rendered more than once. See holes.go.
	holeText map[*ast.Lambda]string
}

// A ward arm's separator is the same `:` a lambda's is — `gives` spells both —
// so it follows the same style rather than staying a symbol while everything
// around it becomes a word.
//
// binder is `is` or `=`, gives is `gives` or `:`, and pipe is what a `|` in the
// source is printed as. The other particles — `where`, `as`, `into` — have no
// symbol, so they are the same in both.
func (p *printer) binder() string {
	if p.style == Terse {
		return "="
	}
	return "is"
}

func (p *printer) gives() string {
	if p.style == Terse {
		return ":"
	}
	return "gives"
}

// hole is `this` or `_`. Its partner `that` has no symbol, so it prints as
// itself in both styles.
func (p *printer) hole() string {
	if p.style == Terse {
		return "_"
	}
	return "this"
}

// via prints a pipeline particle. Only `|` and `through` are the same thing
// spelled two ways; anything else is passed through as it was written.
func (p *printer) via(written string) string {
	switch written {
	case "|", "through":
		if p.style == Terse {
			return "|"
		}
		return "through"
	}
	return written
}

// ------------------------------------------------------------------- output

func (p *printer) line(text string) {
	if text == "" {
		p.sb.WriteByte('\n')
		return
	}
	p.sb.WriteString(strings.Repeat(Indent, p.depth))
	p.sb.WriteString(text)
	p.sb.WriteByte('\n')
}

func (p *printer) blank() {
	s := p.sb.String()
	if s == "" || strings.HasSuffix(s, "\n\n") {
		return
	}
	p.sb.WriteByte('\n')
}

func (p *printer) finish() string {
	// Anything left is a trailing comment at the end of the file.
	p.flushBefore(1 << 30)
	out := strings.TrimRight(p.sb.String(), "\n")
	if out == "" {
		return ""
	}
	return out + "\n"
}

// flushBefore emits every comment that starts above the given line.
func (p *printer) flushBefore(line int) {
	for p.used < len(p.comments) && p.comments[p.used].Pos.Line < line {
		c := p.comments[p.used]
		p.used++
		if c.Text == "" {
			p.line("#")
			continue
		}
		p.line("# " + strings.TrimSpace(c.Text))
	}
}

// trailingOn returns the comment written at the end of a given line, if any.
func (p *printer) trailingOn(line int) string {
	for i := p.used; i < len(p.comments); i++ {
		if p.comments[i].Pos.Line == line {
			c := p.comments[i]
			// Consume it along with anything before it.
			p.comments = append(p.comments[:i], p.comments[i+1:]...)
			return "  # " + strings.TrimSpace(c.Text)
		}
		if p.comments[i].Pos.Line > line {
			break
		}
	}
	return ""
}

// ---------------------------------------------------------------- top level

func (p *printer) file(f *ast.File) {
	sort.SliceStable(p.comments, func(i, j int) bool {
		return p.comments[i].Pos.Line < p.comments[j].Pos.Line
	})

	// Type declarations and definitions keep the order they were written in,
	// so the formatter never rearranges a file's narrative.
	tops := make([]ast.Node, 0, len(f.Types)+len(f.Decls))
	for _, t := range f.Types {
		tops = append(tops, t)
	}
	for _, d := range f.Decls {
		tops = append(tops, d)
	}
	sort.SliceStable(tops, func(i, j int) bool {
		return tops[i].Pos().Line < tops[j].Pos().Line
	})

	for i, n := range tops {
		// The separating blank line comes before the comments, not after: a
		// comment on its own line introduces what follows it, so it has to
		// stay with the declaration and not drift up to the one above.
		if i > 0 {
			p.blank()
		}
		p.flushBefore(n.Pos().Line)
		switch n := n.(type) {
		case *ast.TypeDecl:
			p.typeDecl(n)
		case *ast.Decl:
			p.decl(n)
		}
	}

	for _, out := range f.Outputs {
		p.blank()
		p.flushBefore(out.Pos().Line)
		p.body(out, "")
	}
}

// typeDecl prints a sum type on one line, or one alternative per line when
// that would run past the margin.
func (p *printer) typeDecl(d *ast.TypeDecl) {
	head := d.Name
	for _, param := range d.Params {
		head += " " + param
	}
	head += " " + p.binder()

	alts := make([]string, len(d.Ctors))
	for i, ct := range d.Ctors {
		alt := ct.Name
		for _, f := range ct.Fields {
			s := typeString(f)
			if len(f.Args) > 0 || f.Name == ast.FuncTypeName {
				s = "(" + s + ")"
			}
			alt += " " + s
		}
		alts[i] = alt
	}

	one := head + " " + strings.Join(alts, " | ")
	if len(strings.Repeat(Indent, p.depth))+len(one) <= Width {
		p.line(one + p.trailingOn(d.NamePos.Line))
		return
	}
	p.line(head)
	p.depth++
	p.line(alts[0])
	for _, alt := range alts[1:] {
		p.line("| " + alt)
	}
	p.depth--
}

func (p *printer) decl(d *ast.Decl) {
	// A definition that takes its value apart has a name no source can spell,
	// so what goes before the `is` is the pattern.
	if d.Pat != nil {
		for _, cl := range d.Clauses {
			p.body(cl.Body, patternString(d.Pat, true)+" "+p.binder())
		}
		return
	}
	if d.Sig != nil {
		p.line(d.Name + " :: " + typeString(d.Sig))
	}
	for i, cl := range d.Clauses {
		head := d.Name
		// `remember` marks the definition, not one equation, so it goes once on
		// the first clause however many clauses said it.
		if d.Memo && i == 0 {
			head = "remember " + head
		}
		for _, param := range cl.Params {
			head += " " + patternString(param, true)
		}
		p.body(cl.Body, head+" "+p.binder())
	}
}

// ------------------------------------------------------------------- bodies

// body prints an expression in a position that may span lines. head, when set,
// is written before it — `name args is` — and the body either follows on the
// same line or opens a block.
func (p *printer) body(e ast.Expr, head string) {
	switch e := e.(type) {
	case *ast.Ward:
		if _, _, hole := openedPipe(e); hole {
			break // it prints as the pipeline it was written as
		}
		if _, fits := p.inlineWard(e); fits && head != "" {
			break // it prints on the one line it was written on
		}
		if head != "" {
			p.line(head)
			p.depth++
			defer func() { p.depth-- }()
		}
		p.ward(e)
		return

	case *ast.Let:
		if _, _, hole := holePipe(e); hole {
			break // it prints as the pipeline it was written as
		}
		if head != "" {
			p.line(head)
			p.depth++
			defer func() { p.depth-- }()
		}
		p.let(e)
		return
	}

	one := p.expr(e, precTop)
	if head == "" {
		p.emitWrapped(one, e)
		return
	}
	if len(strings.Repeat(Indent, p.depth))+len(head)+1+len(one) <= Width {
		p.line(head + " " + one + p.trailingOn(e.Pos().Line))
		return
	}
	p.line(head)
	p.depth++
	p.emitWrapped(one, e)
	p.depth--
}

// emitWrapped writes an expression, breaking a long pipeline one stage per
// line rather than letting it run past the margin.
func (p *printer) emitWrapped(one string, e ast.Expr) {
	if len(strings.Repeat(Indent, p.depth))+len(one) <= Width {
		p.line(one + p.trailingOn(e.Pos().Line))
		return
	}
	if lit, ok := e.(*ast.ThreadLit); ok && len(lit.Elems) > 1 && !allAtoms(lit.Elems) {
		p.thread(lit)
		return
	}
	stages := p.pipeline(e)
	if len(stages) < 2 {
		if app, ok := e.(*ast.App); ok && app.Via == "" && len(app.Args) > 0 {
			p.emitCall(app, "", "")
			return
		}
		p.line(one)
		return
	}
	p.emitPipe(stages, "", "")
}

// emitStage writes one stage of a broken pipeline, breaking inside it when the
// stage alone still runs past the margin.
//
// The arguments go one to a line at a further indent, which is what keeps a
// continuation from being read as the next stage: a stage begins with a
// particle and these do not.
func (p *printer) emitStage(s stageLine, close string) {
	if s.app == nil || p.fits(s.text+close) {
		p.line(s.text + close)
		return
	}
	verb, args := p.stageParts(s.app)
	if len(args) == 0 {
		p.line(s.text + close)
		return
	}
	p.line(verb)
	p.depth++
	for i, a := range args {
		tail := ""
		if i == len(args)-1 {
			tail = close
		}
		p.emitArg(s.app.Args[i], a, tail)
	}
	p.depth--
}

// emitCall writes an application that will not fit on one line, at the same
// seam a pipeline stage is broken at: its arguments.
//
// As many arguments as the line will hold stay on it, and the rest go one to a
// line at a further indent. A deeper line that opens no block continues the one
// above it, which is exactly what an argument on its own line is, so what comes
// back reads as the call it was — and reparses as it.
//
// `open` and `close` are the brackets an argument being broken inside sits in;
// at the top level they are empty. They are carried rather than printed by the
// caller so that the closing bracket lands on the last line written, however
// deep the breaking went.
func (p *printer) emitCall(app *ast.App, open, close string) {
	head := open + p.expr(app.Fn, precArg)
	texts := make([]string, len(app.Args))
	for i, a := range app.Args {
		texts[i] = p.expr(a, precArg)
	}

	line, used := head, 0
	for used < len(texts) {
		trial := line + " " + texts[used]
		if used == len(texts)-1 {
			trial += close
		}
		if !p.fits(trial) {
			break
		}
		line = line + " " + texts[used]
		used++
	}
	if used == len(texts) {
		p.line(line + close)
		return
	}

	p.line(line)
	p.depth++
	for i := used; i < len(texts); i++ {
		tail := ""
		if i == len(texts)-1 {
			tail = close
		}
		p.emitArg(app.Args[i], texts[i], tail)
	}
	p.depth--
}

// emitArg writes one argument of a broken call, breaking inside it when the
// argument is itself too long. `p.expr` has already bracketed whatever needed
// it, so the brackets are handed on rather than printed, and the closing one
// lands on the last line however deep the breaking went.
//
// Five things can be broken into: an ordinary call, a pipeline, a lambda whose
// body is either, and the two bracketed literals — a Twine and a Thread, which
// break one element to a line with the comma leading, so that a line which
// opens no block continues the one above it. Anything else — a ward, a name — is
// printed as it stands and may run past the margin, which is the honest outcome
// when there is no seam.
func (p *printer) emitArg(e ast.Expr, text, close string) {
	if p.fits(text + close) {
		p.line(text + close)
		return
	}
	switch e := e.(type) {
	case *ast.Lambda:
		// A lambda a hole word produced — or one the formatter has just
		// rewritten into that spelling, which renames the body in place — has
		// no head of its own. It prints as its body inside brackets, and a
		// Twine literal body brings its own, so the seam is the body's either
		// way.
		if isHoleLambda(e) || p.holeSpelling(e) != "" {
			if p.emitBracketed(e.Body, "(", ")"+close) {
				return
			}
			break
		}
		if head, ok := p.lambdaHead(e); ok {
			p.line(head)
			p.depth++
			p.emitArg(e.Body, p.expr(e.Body, precTop), ")"+close)
			p.depth--
			return
		}

	case *ast.ThreadLit:
		// A Thread of atoms is written space-separated and has no comma to
		// lead with, so there is nothing to break at.
		if len(e.Elems) > 1 && !allAtoms(e.Elems) {
			p.elems(e.Elems, "[", "]"+close)
			return
		}

	default:
		if p.emitBracketed(e, "(", ")"+close) {
			return
		}
	}
	p.line(text + close)
}

// emitBracketed writes an expression that will not fit, at whatever seam it
// has, inside the brackets `p.expr` already put round it. It reports whether it
// found one.
func (p *printer) emitBracketed(e ast.Expr, open, close string) bool {
	if stages := p.pipeline(e); len(stages) >= 2 {
		p.emitPipe(stages, open, close)
		return true
	}
	switch e := e.(type) {
	case *ast.App:
		if e.Via == "" && len(e.Args) > 0 {
			p.emitCall(e, open, close)
			return true
		}
	case *ast.TwineLit:
		p.elems(e.Elems, open, close)
		return true
	}
	return false
}

// lambdaHead is the `(a b gives` a lambda opens with when its body has to go
// on lines of its own, and whether this lambda has one at all: the ones the
// hole words spell have no parameters to write.
func (p *printer) lambdaHead(e *ast.Lambda) (string, bool) {
	if isHoleLambda(e) || p.holeSpelling(e) != "" {
		return "", false
	}
	params := make([]string, len(e.Params))
	for i, param := range e.Params {
		params[i] = patternString(param, true)
	}
	return "(" + strings.Join(params, " ") + " " + p.gives(), true
}

// emitPipe writes a pipeline one stage per line, with the brackets it sits in
// if it sits in any.
func (p *printer) emitPipe(stages []stageLine, open, close string) {
	p.line(open + stages[0].text)
	p.depth++
	for i, s := range stages[1:] {
		tail := ""
		if i == len(stages)-2 {
			tail = close
		}
		p.emitStage(s, tail)
	}
	p.depth--
}

func (p *printer) fits(text string) bool {
	return p.depth*len(Indent)+len(text) <= Width
}

// stageParts splits a stage into what leads it and the arguments that follow,
// which is the seam a too-long stage is broken at.
func (p *printer) stageParts(app *ast.App) (string, []string) {
	if verb, fn, ok := particleStage(app); ok {
		if p.style == Terse {
			return "| " + verb, []string{p.expr(fn, precArg)}
		}
		return particles[verb], []string{p.expr(fn, precPipe)}
	}
	var args []string
	for _, a := range app.Args[:len(app.Args)-1] {
		args = append(args, p.expr(a, precArg))
	}
	return p.via(app.Via) + " " + p.expr(app.Fn, precArg), args
}

// thread writes a Thread literal that will not fit on one line. It is `elems`
// with nothing round it, since a literal in this position sits at the top of a
// definition rather than inside a call.
func (p *printer) thread(lit *ast.ThreadLit) {
	p.elems(lit.Elems, "[", "]")
}

// elems writes a bracketed literal one element to a line, the comma leading so
// that every line after the first opens no block and continues the one above.
// `close` carries whatever brackets the literal itself sits inside, so they
// land on the last line written.
//
// An element that will not fit either is broken at the seams a call and a
// pipeline already have, with the comma standing in for the bracket they would
// otherwise open with.
func (p *printer) elems(es []ast.Expr, open, close string) {
	for i, el := range es {
		lead := ", "
		if i == 0 {
			lead = open + " "
		}
		text := lead + p.expr(el, precTop)
		stages := p.pipeline(el)
		app, isCall := el.(*ast.App)
		switch {
		case p.fits(text):
			p.line(text)
		case len(stages) >= 2:
			p.emitPipe(stages, lead, "")
		case isCall && app.Via == "" && len(app.Args) > 0:
			p.emitCall(app, lead, "")
		default:
			p.line(text)
		}
	}
	p.line(close)
}

// stageLine is one line of a broken pipeline: what to print, and the call it
// came from, which is what lets a stage be broken further.
type stageLine struct {
	text string
	app  *ast.App // nil for the value the chain starts from
}

// pipeline flattens a chain into the head plus one `| stage` per element.
//
// Two of the stages are not calls at all. A piped hole word desugars in the
// *parser* — `xs | latter` is a match on the pair it opens, `xs | _` is a
// binding — so a chain ending in one is a Ward or a Let with a chain inside it,
// and reading those back as stages is what lets such a chain be broken. Without
// it a single `| latter` at the end of a line stopped the whole pipeline from
// breaking, which is two of the three over-long lines in this repository.
func (p *printer) pipeline(e ast.Expr) []stageLine {
	switch e := e.(type) {
	case *ast.App:
		if e.Via == "" || len(e.Args) == 0 {
			return nil
		}
		return p.chain(e.Args[len(e.Args)-1], stageLine{p.stage(e), e})

	case *ast.Ward:
		if via, value, ok := openedPipe(e); ok {
			return p.chain(value, p.holeStage(via, e.Arms[0].Body))
		}

	case *ast.Let:
		if via, value, ok := holePipe(e); ok {
			return p.chain(value, p.holeStage(via, e.Body))
		}
	}
	return nil
}

// holeStage renders the stage a piped hole word became, which is the particle
// it was written with and whatever was said with the word.
func (p *printer) holeStage(via string, body ast.Expr) stageLine {
	return stageLine{text: p.via(via) + " " + p.expr(body, precPipe)}
}

// chain puts a stage after whatever fed it, flattening that in turn.
func (p *printer) chain(value ast.Expr, stage stageLine) []stageLine {
	if inner := p.pipeline(value); inner != nil {
		return append(inner, stage)
	}
	return []stageLine{{text: p.expr(value, precPipe)}, stage}
}

// particles are the verbs that have a D-particle spelling. The first two feed
// a function, the last two a value.
var particles = map[string]string{
	"sift":      "where",
	"bend":      "as",
	"otherwise": "else",
	"snag":      "failing",
}

// particleStage recognises a stage that has one: `sift p` is `where p`,
// `bend f` is `as f`, `otherwise d` is `else d` and `snag d` is `failing d`.
//
// Which of the two a program was written with is a matter of spelling, not of
// meaning — the particles desugar to these verbs *by name*, so both forms
// resolve to the same thing even where the name has been shadowed. So the
// style chooses, and not the author.
func particleStage(app *ast.App) (verb string, fn ast.Expr, ok bool) {
	if len(app.Args) != 2 {
		return "", nil, false
	}
	v, isVar := app.Fn.(*ast.Var)
	if !isVar {
		return "", nil, false
	}
	if _, has := particles[v.Name]; !has {
		return "", nil, false
	}
	return v.Name, app.Args[0], true
}

// stage renders a pipeline stage — the particle and what follows it — without
// the value piped in.
func (p *printer) stage(app *ast.App) string {
	if verb, fn, ok := particleStage(app); ok {
		if p.style == Terse {
			return "| " + verb + " " + p.expr(fn, precArg)
		}
		// A particle's stage is a whole application, so `as split ""` needs no
		// brackets: it ends at the next particle.
		return particles[verb] + " " + p.expr(fn, precPipe)
	}
	parts := []string{p.expr(app.Fn, precArg)}
	for _, a := range app.Args[:len(app.Args)-1] {
		parts = append(parts, p.expr(a, precArg))
	}
	return p.via(app.Via) + " " + strings.Join(parts, " ")
}

// bindHead is what a binding is written as before its `is`: a name, or the
// pattern it takes its value apart with.
func bindHead(b *ast.Bind) string {
	if b.Pat != nil {
		return patternString(b.Pat, true)
	}
	return b.Name
}

func (p *printer) let(e *ast.Let) {
	for _, b := range e.Binds {
		p.flushBefore(b.NamePos.Line)
		head := "weave " + bindHead(b)
		if len(b.Params) > 0 {
			head = "channel " + b.Name
			for _, param := range b.Params {
				head += " " + patternString(param, true)
			}
		}
		p.body(b.Value, head+" "+p.binder())
	}
	p.flushBefore(e.Body.Pos().Line)
	p.body(e.Body, "")
}

func (p *printer) ward(e *ast.Ward) {
	// A ward written on one line stays on one line for as long as it fits.
	// The block form is not more canonical than the bracketed one; they are
	// the same ward, and which reads better is a question of length.
	if text, ok := p.inlineWard(e); ok {
		p.line(text + p.trailingOn(e.P.Line))
		return
	}
	p.line("ward " + p.expr(e.Subject, precPipe) + p.trailingOn(e.P.Line))
	p.depth++
	for _, arm := range e.Arms {
		p.flushBefore(arm.P.Line)
		p.body(arm.Body, patternString(arm.Pat, false)+" "+p.gives())
	}
	p.depth--
}

// inlineWard renders the bracketed one-line form, and reports whether it is
// worth using: the ward has to have been written that way, and it has to fit.
//
// A ward that was written as a block stays one, however short it is. The
// formatter has an opinion about spelling and none about how much of a
// decision deserves its own lines.
func (p *printer) inlineWard(e *ast.Ward) (string, bool) {
	if !e.Inline {
		return "", false
	}
	for _, arm := range e.Arms {
		if !oneLineBody(arm.Body) {
			return "", false
		}
	}
	text := p.wardArms(e)
	if p.depth*len(Indent)+len(text) > Width {
		return "", false
	}
	return text, true
}

// wardArms renders the bracketed form whether or not it fits.
func (p *printer) wardArms(e *ast.Ward) string {
	text := "ward " + p.expr(e.Subject, precArg)
	for _, arm := range e.Arms {
		text += " (" + patternString(arm.Pat, false) + " " + p.gives() + " " +
			p.expr(arm.Body, precTop) + ")"
	}
	return text
}

// oneLineBody reports whether an arm's body prints without a block of its own.
func oneLineBody(e ast.Expr) bool {
	switch e := e.(type) {
	case *ast.Ward:
		if _, _, ok := openedPipe(e); ok {
			return true
		}
		_, fits := (&printer{}).inlineWard(e)
		return fits
	case *ast.Let:
		_, _, ok := holePipe(e)
		return ok
	}
	return true
}

// -------------------------------------------------------------- expressions

// Parenthesisation levels. An expression is wrapped when it binds looser than
// the position it appears in, and never otherwise — which is what removes
// redundant parentheses without a rule for each case.
const (
	precTop = iota
	precPipe
	precArg
)

func (p *printer) expr(e ast.Expr, prec int) string {
	switch e := e.(type) {
	case *ast.IntLit:
		return strconv.FormatInt(e.Value, 10)
	case *ast.FloatLit:
		return formatFloat(e.Value)
	case *ast.CharLit:
		return "'" + escapeRune(e.Value) + "'"
	case *ast.TextLit:
		return strconv.Quote(e.Value)
	case *ast.Var:
		if e.Name == ast.HoleName {
			return p.hole()
		}
		return e.Name
	case *ast.Ctor:
		return e.Name

	case *ast.App:
		return p.app(e, prec)

	case *ast.Lambda:
		// A lambda a `_` produced goes back to the brackets it came from,
		// since the body still refers to the parameter as `_`.
		if isHoleLambda(e) {
			if _, tuple := e.Body.(*ast.TwineLit); tuple {
				return p.expr(e.Body, precTop)
			}
			return "(" + p.expr(e.Body, precTop) + ")"
		}
		// A lambda that names its parameters is written more shortly with the
		// hole words, when that reads back as the same function. See holes.go.
		if text := p.holeSpelling(e); text != "" {
			return text
		}
		params := make([]string, len(e.Params))
		for i, param := range e.Params {
			params[i] = patternString(param, true)
		}
		return "(" + strings.Join(params, " ") + " " + p.gives() + " " +
			p.expr(e.Body, precTop) + ")"

	case *ast.ThreadLit:
		// Elements that are single atoms are separated by spaces, which is what
		// makes `[1 2 3]` and a literal grid readable. Anything larger needs
		// commas, since without them `[Step North 3, Rest]` would be four
		// elements rather than two — and that is exactly how the parser reads
		// it back.
		//
		// One element is the exception, because the comma that does the
		// separating never gets written: `[(inc x)]` under the comma rule
		// would come out `[inc x]`, which reads back as two elements. A Thread
		// of one is always written the space-separated way, so whatever is
		// inside keeps the brackets that make it one thing.
		parts := make([]string, len(e.Elems))
		sep := ", "
		if len(e.Elems) < 2 || allAtoms(e.Elems) {
			sep = " "
		}
		for i, el := range e.Elems {
			if sep == " " {
				parts[i] = p.expr(el, precArg)
			} else {
				parts[i] = p.expr(el, precTop)
			}
		}
		return "[" + strings.Join(parts, sep) + "]"

	case *ast.TwineLit:
		parts := make([]string, len(e.Elems))
		for i, el := range e.Elems {
			parts[i] = p.expr(el, precTop)
		}
		return "(" + strings.Join(parts, ", ") + ")"

	case *ast.WebLit:
		parts := make([]string, len(e.Pairs))
		for i, pair := range e.Pairs {
			parts[i] = p.expr(pair.Key, precArg) + " : " + p.expr(pair.Val, precArg)
		}
		return "{" + strings.Join(parts, "  ") + "}"

	case *ast.Let:
		// A binding a piped `_` produced goes back to being that pipeline.
		if via, value, ok := holePipe(e); ok {
			out := p.expr(value, precPipe) + " " + p.via(via) + " " + p.expr(e.Body, precPipe)
			return wrap(out, prec > precPipe)
		}
		// Nested in an expression, a let has to use its inline form.
		binds := make([]string, len(e.Binds))
		for i, b := range e.Binds {
			head := bindHead(b)
			for _, param := range b.Params {
				head += " " + patternString(param, true)
			}
			binds[i] = head + " " + p.binder() + " " + p.expr(b.Value, precPipe)
		}
		out := "weave " + strings.Join(binds, ", ") + " into " + p.expr(e.Body, precTop)
		return wrap(out, prec > precTop)

	case *ast.Ward:
		// A match a piped `that` produced goes back to being that pipeline.
		if via, value, ok := openedPipe(e); ok {
			out := p.expr(value, precPipe) + " " + p.via(via) + " " + p.expr(e.Arms[0].Body, precPipe)
			return wrap(out, prec > precPipe)
		}
		// Every other ward in expression position got there by being written
		// with bracketed arms, since the block form cannot be. So it prints
		// that way whether or not it fits: a formatter that shortened one by
		// dropping arms would be worse than one that ran past the margin.
		return wrap(p.wardArms(e), prec >= precArg)
	}
	return "?"
}

func (p *printer) app(e *ast.App, prec int) string {
	if e.Via != "" && len(e.Args) > 0 {
		value := e.Args[len(e.Args)-1]
		out := p.expr(value, precPipe) + " " + p.stage(e)
		return wrap(out, prec > precPipe)
	}
	parts := []string{p.expr(e.Fn, precArg)}
	for _, a := range e.Args {
		parts = append(parts, p.expr(a, precArg))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return wrap(strings.Join(parts, " "), prec >= precArg)
}

// isHoleLambda reports whether a lambda is one the hole words produced: a
// first parameter bound whole or taken apart into the components its width's
// words asked for, and a second only when a `that` asked for one.
func isHoleLambda(e *ast.Lambda) bool {
	if len(e.Params) == 0 || len(e.Params) > 2 {
		return false
	}
	if len(e.Params) == 2 {
		v, ok := e.Params[1].(*ast.PVar)
		if !ok || v.Name != ast.PartnerName {
			return false
		}
	}
	if v, ok := e.Params[0].(*ast.PVar); ok {
		return v.Name == ast.HoleName
	}
	return isPartsPattern(e.Params[0])
}

// isPartsPattern recognises the `(former, latter)` or `(fore, mid, aft)` the
// parser writes for a group holding one of those words.
func isPartsPattern(pat ast.Pattern) bool {
	tw, ok := pat.(*ast.PTwine)
	if !ok {
		return false
	}
	want := ast.PartNames(len(tw.Elems))
	if want == nil {
		return false
	}
	for i, el := range tw.Elems {
		v, ok := el.(*ast.PVar)
		if !ok || v.Name != want[i] {
			return false
		}
	}
	return true
}

// openedPipe recognises the match a piped component word produced, and returns
// the particle it was written with and the value it was given.
func openedPipe(e *ast.Ward) (via string, value ast.Expr, ok bool) {
	if e.Via == "" || len(e.Arms) != 1 || !isPartsPattern(e.Arms[0].Pat) {
		return "", nil, false
	}
	return e.Via, e.Subject, true
}

// holePipe recognises the binding a piped `_` produced, and returns the
// particle it was written with and the value it was given.
func holePipe(e *ast.Let) (via string, value ast.Expr, ok bool) {
	if e.Via == "" || len(e.Binds) != 1 || e.Binds[0].Name != ast.HoleName {
		return "", nil, false
	}
	return e.Via, e.Binds[0].Value, true
}

// allAtoms reports whether every element of a Thread literal is a single atom,
// which is what lets it be written space-separated.
func allAtoms(elems []ast.Expr) bool {
	for _, el := range elems {
		if !isAtomExpr(el) {
			return false
		}
	}
	return true
}

// isAtomExpr reports whether an expression prints without needing brackets in
// argument position.
func isAtomExpr(e ast.Expr) bool {
	switch e.(type) {
	case *ast.IntLit, *ast.FloatLit, *ast.CharLit, *ast.TextLit,
		*ast.Var, *ast.Ctor, *ast.Lambda, *ast.ThreadLit, *ast.TwineLit, *ast.WebLit:
		return true
	}
	return false
}

func wrap(s string, need bool) string {
	if need {
		return "(" + s + ")"
	}
	return s
}

// ------------------------------------------------------------------ pieces

func patternString(pat ast.Pattern, atom bool) string {
	switch pat := pat.(type) {
	case *ast.PWild:
		return "_"
	case *ast.PVar:
		return pat.Name
	case *ast.PInt:
		return strconv.FormatInt(pat.Value, 10)
	case *ast.PFloat:
		return formatFloat(pat.Value)
	case *ast.PChar:
		return "'" + escapeRune(pat.Value) + "'"
	case *ast.PText:
		return strconv.Quote(pat.Value)
	case *ast.PCtor:
		if len(pat.Args) == 0 {
			return pat.Name
		}
		parts := []string{pat.Name}
		for _, a := range pat.Args {
			parts = append(parts, patternString(a, true))
		}
		return wrap(strings.Join(parts, " "), atom)
	case *ast.PTwine:
		parts := make([]string, len(pat.Elems))
		for i, el := range pat.Elems {
			parts[i] = patternString(el, false)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *ast.PThread:
		parts := make([]string, 0, len(pat.Elems)+1)
		for _, el := range pat.Elems {
			parts = append(parts, patternString(el, true))
		}
		switch rest := pat.Rest.(type) {
		case *ast.PVar:
			parts = append(parts, ".."+rest.Name)
		case *ast.PWild:
			parts = append(parts, "..")
		}
		return "[" + strings.Join(parts, " ") + "]"
	}
	return "_"
}

func typeString(t *ast.TypeExpr) string {
	switch t.Name {
	case ast.FuncTypeName:
		if len(t.Args) != 2 {
			return "?"
		}
		left := typeString(t.Args[0])
		if t.Args[0].Name == ast.FuncTypeName {
			left = "(" + left + ")"
		}
		return left + " -> " + typeString(t.Args[1])
	case ast.TwineTypeName:
		parts := make([]string, len(t.Args))
		for i, a := range t.Args {
			parts[i] = typeString(a)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	}
	out := t.Name
	for _, a := range t.Args {
		s := typeString(a)
		if len(a.Args) > 0 || a.Name == ast.FuncTypeName {
			s = "(" + s + ")"
		}
		out += " " + s
	}
	return out
}

func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}

func escapeRune(r rune) string {
	switch r {
	case '\n':
		return "\\n"
	case '\t':
		return "\\t"
	case '\r':
		return "\\r"
	case '\\':
		return "\\\\"
	case '\'':
		return "\\'"
	}
	if r < 32 {
		return fmt.Sprintf("\\%03o", r)
	}
	return string(r)
}
