// Package parser builds a Weave syntax tree from a token stream.
//
// The grammar is small enough for straightforward recursive descent. There is
// no operator precedence table because Weave has no operators: the only infix
// forms are the pipeline `|` and the particles `where`, `as` and `through`,
// which all sit at the same level, below application.
//
//	Program   := { Decl | Sig | OutputExpr }
//	Decl      := lower { PatternAtom } "is" Body
//	Sig       := lower "::" Type
//	Body      := Expr NEWLINE | NEWLINE INDENT Block DEDENT
//	Block     := { "weave" Bind | "channel" Bind } Expr
//	Expr      := App { ("|" | "where" | "as" | "through") App }
//	App       := Atom { Atom }
//	Atom      := literal | lower | Upper | "(" ... ")" | "[" ... "]"
//	           | "{" ... "}" | Ward | InlineLet
package parser

import (
	"fmt"
	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/lexer"
	"github.com/malleum/weave/internal/token"
)

// Parse scans and parses src. Errors are recorded in bag; the returned file is
// always non-nil so later phases can still run over whatever parsed cleanly.
func Parse(src string, bag *diag.Bag) *ast.File {
	toks := lexer.Lex(src, bag)
	p := &parser{toks: toks, bag: bag, armsAt: -1}
	return p.parseFile()
}

// ParseTypeString parses a standalone type, as written after `::`. It lets the
// prelude declare its signatures in Weave's own notation rather than building
// them by hand:
//
//	bend :: (a -> b) -> Thread a -> Thread b
func ParseTypeString(src string) (*ast.TypeExpr, error) {
	bag := diag.New("<signature>", src)
	toks := lexer.Lex(src, bag)
	p := &parser{toks: toks, bag: bag, armsAt: -1}
	ty := p.parseType()
	if !p.at(token.Newline) && !p.at(token.EOF) {
		p.errf(p.cur().Pos, "unexpected %s after type", describe(p.cur()))
	}
	if err := bag.Err(); err != nil {
		return nil, err
	}
	return ty, nil
}

type parser struct {
	toks []token.Token
	pos  int
	bag  *diag.Bag

	// decls indexes declarations by name so that consecutive clauses of a
	// multi-clause definition collect into one Decl.
	decls map[string]*ast.Decl

	// armsAt is the token a ward's inline arms begin at while its subject is
	// being read, or -1 when there are none. See parseWard for how it is found:
	// it is one exact position rather than a rule about what an argument may
	// look like, which is what lets a lambda appear in a subject.
	armsAt int
}

// ------------------------------------------------------------------ plumbing

// holdArmStop is kept for the bracketed atoms that call it, but has nothing
// left to do: the arms are found by position now, and no position inside a
// bracketed group can be the one the arms start at.
func (p *parser) holdArmStop() func() { return func() {} }

func (p *parser) cur() token.Token     { return p.toks[p.pos] }
func (p *parser) kind() token.Kind     { return p.toks[p.pos].Kind }
func (p *parser) at(k token.Kind) bool { return p.kind() == k }

func (p *parser) peek(n int) token.Token {
	i := p.pos + n
	if i >= len(p.toks) {
		i = len(p.toks) - 1
	}
	return p.toks[i]
}

func (p *parser) next() token.Token {
	t := p.toks[p.pos]
	if p.pos < len(p.toks)-1 {
		p.pos++
	}
	return t
}

func (p *parser) accept(k token.Kind) bool {
	if p.at(k) {
		p.next()
		return true
	}
	return false
}

func (p *parser) expect(k token.Kind) token.Token {
	if p.at(k) {
		return p.next()
	}
	p.errf(p.cur().Pos, "expected %s, found %s", k, describe(p.cur()))
	return token.Token{Kind: k, Pos: p.cur().Pos}
}

func (p *parser) errf(pos token.Pos, format string, args ...any) {
	p.bag.Add(pos, format, args...)
}

func describe(t token.Token) string {
	switch t.Kind {
	case token.Lower, token.Upper:
		return "`" + t.Lit + "`"
	case token.EOF:
		return "end of file"
	default:
		return t.Kind.String()
	}
}

// skipNewlines consumes blank logical lines.
func (p *parser) skipNewlines() {
	for p.at(token.Newline) {
		p.next()
	}
}

// recover advances past the current logical line so parsing can continue after
// an error without cascading.
func (p *parser) recover() {
	depth := 0
	for {
		switch p.kind() {
		case token.EOF:
			return
		case token.Newline:
			if depth <= 0 {
				p.next()
				return
			}
		case token.Indent:
			depth++
		case token.Dedent:
			if depth <= 0 {
				return
			}
			depth--
		}
		p.next()
	}
}

// --------------------------------------------------------------- top level

func (p *parser) parseFile() *ast.File {
	f := &ast.File{}
	p.decls = map[string]*ast.Decl{}

	for {
		p.skipNewlines()
		if p.at(token.EOF) {
			break
		}
		// Stray layout tokens can appear after a recovery; drop them.
		if p.at(token.Indent) || p.at(token.Dedent) {
			p.next()
			continue
		}
		before := p.pos
		p.parseTopLevel(f)
		if p.pos == before { // no progress: force one, to guarantee termination
			p.next()
		}
	}
	p.reportStrayHoles(f)
	return f
}

// reportStrayHoles complains about every `_` that no bracket group or pipeline
// stage claimed. Doing it once over the finished tree rather than at each
// construction site means there is exactly one place that decides what an
// unclaimed `_` is, and one message.
func (p *parser) reportStrayHoles(f *ast.File) {
	var stray []*ast.Hole
	for _, d := range f.Decls {
		for _, cl := range d.Clauses {
			ast.FindHoles(cl.Body, &stray)
		}
	}
	for _, out := range f.Outputs {
		ast.FindHoles(out, &stray)
	}
	for _, h := range stray {
		switch {
		case h.Parts != 0:
			p.bag.AddHint(h.P,
				"`former` and `latter` are the halves of a Twine of two, and `fore`, `mid` and `aft` the parts of a Twine of three — of the value given to the brackets around them, or to the pipeline stage they are in",
				"`%s` has no Twine to be part of here",
				ast.HoleVarName(h.Slot, h.Parts, h.At))
		case h.Slot > 0:
			p.bag.AddHint(h.P,
				"`that` is the second argument of the brackets around it",
				"`that` has nothing to stand for here")
		default:
			p.bag.AddHint(h.P,
				"a `_` stands for the argument of the brackets around it, or of the pipeline stage it is in",
				"`_` has nothing to stand for here")
		}
	}
}

func (p *parser) parseTopLevel(f *ast.File) {
	// A top-level line is a declaration when it has `is` or `::` at the outer
	// level; otherwise it is the program's output expression.
	if p.at(token.Remember) {
		if p.peek(1).Kind != token.Lower {
			p.errf(p.cur().Pos, "`remember` marks a definition, so a name must follow it")
			p.recover()
			return
		}
		p.parseDecl(f)
		return
	}
	if p.at(token.Lower) {
		switch p.lineForm() {
		case formSig:
			p.parseSig()
			return
		case formDecl:
			p.parseDecl(f)
			return
		}
	}
	// `(width, height) is dimsOf Source` — a definition that takes its value
	// apart, the same way a `weave` binding may.
	if p.startsBindPattern() && p.lineForm() == formDecl {
		p.parsePatternDecl(f)
		return
	}
	if p.at(token.Upper) && p.startsTypeDecl() {
		p.parseTypeDecl(f)
		return
	}
	// A bare expression. Several are allowed: the program prints the last, and
	// `weave trace` reports them all, so several things under test can sit in
	// one file rather than taking turns behind a comment.
	e := p.parseExpr()
	p.endLine()
	f.Outputs = append(f.Outputs, e)
}

type lineForm int

const (
	formOutput lineForm = iota
	formDecl
	formSig
)

// lineForm looks ahead over the current logical line to classify it.
func (p *parser) lineForm() lineForm {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.LParen, token.LBracket, token.LBrace:
			depth++
		case token.RParen, token.RBracket, token.RBrace:
			depth--
		case token.Newline, token.EOF, token.Indent, token.Dedent:
			if depth <= 0 {
				return formOutput
			}
		case token.Is:
			if depth <= 0 {
				return formDecl
			}
		case token.ColonColon:
			if depth <= 0 {
				return formSig
			}
		}
	}
	return formOutput
}

// startsTypeDecl reports whether the line is `Name p1 p2 is ...`, the shape of
// a sum type declaration. Requiring nothing but type parameters before `is`
// keeps it apart from an output expression that happens to begin with a
// constructor, such as `Source | lines | sum`.
func (p *parser) startsTypeDecl() bool {
	for i := p.pos + 1; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.Lower:
			continue
		case token.Is:
			return true
		default:
			return false
		}
	}
	return false
}

// parseTypeDecl reads a sum type:
//
//	Direction is North | South | East | West
//	Tree a is Leaf | Node (Tree a) a (Tree a)
//
// The alternatives may also be laid out one per line under the `is`, with the
// `|` starting each continuation line.
func (p *parser) parseTypeDecl(f *ast.File) {
	name := p.next()
	d := &ast.TypeDecl{Name: name.Lit, NamePos: name.Pos}
	for p.at(token.Lower) {
		d.Params = append(d.Params, p.next().Lit)
	}
	p.expect(token.Is)

	indented := false
	if p.at(token.Newline) {
		p.next()
		if !p.at(token.Indent) {
			p.errf(p.cur().Pos, "expected the constructors of `%s` after `is`", d.Name)
			return
		}
		p.next()
		indented = true
		p.skipNewlines()
	}

	for {
		d.Ctors = append(d.Ctors, p.parseCtorDecl())
		if !p.accept(token.Pipe) {
			break
		}
	}
	if indented {
		p.endLine()
		p.skipNewlines()
		p.expect(token.Dedent)
	} else {
		p.endLine()
	}

	f.Types = append(f.Types, d)
}

func (p *parser) parseCtorDecl() *ast.CtorDecl {
	t := p.cur()
	if !p.at(token.Upper) {
		p.errf(t.Pos, "expected a constructor name, found %s", describe(t))
		p.next()
		return &ast.CtorDecl{Name: "?", NamePos: t.Pos}
	}
	p.next()
	ct := &ast.CtorDecl{Name: t.Lit, NamePos: t.Pos}
	// Fields are type atoms: an applied type such as `Tree a` needs brackets,
	// so that `Node (Tree a) a` reads as two fields rather than one.
	for p.at(token.Upper) || p.at(token.Lower) || p.at(token.LParen) {
		ct.Fields = append(ct.Fields, p.parseTypeAtom())
	}
	return ct
}

// parseSig reads `name :: Type`, attaching the annotation to the declaration.
func (p *parser) parseSig() {
	name := p.next()
	p.expect(token.ColonColon)
	ty := p.parseType()
	p.endLine()

	d := p.decls[name.Lit]
	if d == nil {
		d = &ast.Decl{Name: name.Lit, NamePos: name.Pos}
		p.decls[name.Lit] = d
		// The declaration is appended when its first clause is parsed; record
		// it now so the clause can find the signature.
		d.Sig = ty
		return
	}
	if d.Sig != nil {
		p.errf(name.Pos, "`%s` already has a signature", name.Lit)
	}
	d.Sig = ty
}

// parsePatternDecl reads a top-level definition that takes its value apart.
// The name it is given cannot be written in a program — a space is not a name
// character — so it can never collide with one, and nothing but the expansion
// in ast.ExpandPatterns ever mentions it.
func (p *parser) parsePatternDecl(f *ast.File) {
	pos := p.cur().Pos
	pat := p.parsePatternAtom()
	p.expect(token.Is)
	body := p.parseBody()
	f.Decls = append(f.Decls, &ast.Decl{
		Name:    fmt.Sprintf("whole %d", len(f.Decls)),
		NamePos: pos,
		Pat:     pat,
		Clauses: []*ast.Clause{{Body: body, ClauseP: pos}},
	})
}

func (p *parser) parseDecl(f *ast.File) {
	memo := p.accept(token.Remember)
	name := p.next()
	var params []ast.Pattern
	for p.startsPattern() {
		params = append(params, p.parsePatternAtom())
	}
	p.expect(token.Is)
	body := p.parseBody()

	clause := &ast.Clause{Params: params, Body: body, ClauseP: name.Pos}

	d, seen := p.decls[name.Lit]
	if !seen {
		d = &ast.Decl{Name: name.Lit, NamePos: name.Pos}
		p.decls[name.Lit] = d
	}
	// `remember` marks the definition, so writing it on any clause marks all of
	// them; the formatter puts it back on the first.
	d.Memo = d.Memo || memo
	if len(d.Clauses) == 0 {
		f.Decls = append(f.Decls, d)
	} else if len(d.Clauses[0].Params) != len(params) {
		p.errf(name.Pos, "clause of `%s` takes %d argument(s), but an earlier clause takes %d",
			name.Lit, len(params), len(d.Clauses[0].Params))
	}
	d.Clauses = append(d.Clauses, clause)
}

// parseBody reads the right-hand side of `is`: either an inline expression or
// an indented block.
func (p *parser) parseBody() ast.Expr {
	if p.at(token.Newline) {
		p.next()
		if !p.at(token.Indent) {
			p.errf(p.cur().Pos, "expected an indented body after `is`")
			return &ast.Bad{P: p.cur().Pos}
		}
		p.next()
		e := p.parseBlock()
		p.expect(token.Dedent)
		return e
	}
	e := p.parseExpr()
	p.endLine()
	return e
}

// endLine consumes the newline that terminates a logical line, tolerating the
// end of an enclosing block.
func (p *parser) endLine() {
	if p.at(token.Newline) {
		p.next()
		return
	}
	if p.at(token.Dedent) || p.at(token.EOF) {
		return
	}
	p.errf(p.cur().Pos, "unexpected %s at end of expression", describe(p.cur()))
	p.recover()
}

// parseBlock reads local bindings followed by the block's result expression.
func (p *parser) parseBlock() ast.Expr {
	var binds []*ast.Bind
	pos := p.cur().Pos

	for {
		p.skipNewlines()
		if (p.at(token.Weave) || p.at(token.Channel)) && !p.lineHasInto() {
			binds = append(binds, p.parseBind())
			continue
		}
		break
	}

	p.skipNewlines()
	if p.at(token.Dedent) || p.at(token.EOF) {
		p.errf(p.cur().Pos, "block ends with a binding; it needs a final expression to return")
		return &ast.Bad{P: pos}
	}

	body := p.parseExpr()
	p.endLine()
	p.skipNewlines()

	if len(binds) == 0 {
		return body
	}
	return &ast.Let{Binds: binds, Body: body, P: pos}
}

// lineHasInto reports whether the current logical line contains a top-level
// `into`, which marks an inline let-expression rather than a block binding.
func (p *parser) lineHasInto() bool {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.LParen, token.LBracket, token.LBrace:
			depth++
		case token.RParen, token.RBracket, token.RBrace:
			depth--
		case token.Into:
			if depth <= 0 {
				return true
			}
		case token.Newline, token.EOF, token.Indent, token.Dedent:
			if depth <= 0 {
				return false
			}
		}
	}
	return false
}

// parseBind reads `weave name is Body` or `channel name params is Body`.
func (p *parser) parseBind() *ast.Bind {
	isChannel := p.at(token.Channel)
	p.next()

	nameTok := p.cur()
	// A binding may take its value apart instead of naming it whole:
	// `weave (a, b) is p`. Only `weave` does — a `channel` is a function, and a
	// function has a name.
	if !isChannel && p.startsBindPattern() {
		pat := p.parsePatternAtom()
		p.expect(token.Is)
		return &ast.Bind{Pat: pat, NamePos: nameTok.Pos, Value: p.parseBody()}
	}
	if !p.at(token.Lower) {
		p.errf(nameTok.Pos, "expected a name after `%s`, found %s",
			map[bool]string{true: "channel", false: "weave"}[isChannel], describe(nameTok))
		p.recover()
		return &ast.Bind{Name: "_", NamePos: nameTok.Pos, Value: &ast.Bad{P: nameTok.Pos}}
	}
	p.next()

	var params []ast.Pattern
	for p.startsPattern() {
		params = append(params, p.parsePatternAtom())
	}
	if isChannel && len(params) == 0 {
		p.bag.AddHint(nameTok.Pos, "use `weave` for a value binding",
			"`channel %s` declares a function but has no parameters", nameTok.Lit)
	}
	if !isChannel && len(params) > 0 {
		p.bag.AddHint(nameTok.Pos, "use `channel` for a function binding",
			"`weave %s` binds a value but was given parameters", nameTok.Lit)
	}

	p.expect(token.Is)
	value := p.parseBody()
	return &ast.Bind{Name: nameTok.Lit, NamePos: nameTok.Pos, Params: params, Value: value}
}

// ------------------------------------------------------------- expressions

// parseExpr handles the pipeline level: `|` and the particles.
func (p *parser) parseExpr() ast.Expr {
	left := p.parseApp()
	for {
		switch p.kind() {
		case token.Pipe, token.Through:
			op := p.next()
			right := p.parseApp()
			if use := ast.Holes(right); use.Any {
				p.reportWidthClash(use)
				// `web | get _ "a"` puts the piped value where the `_` is
				// rather than at the end. A binding, not a lambda, so the
				// value is still evaluated once however many `_` there are.
				if use.Args > 1 {
					// A stage hands over one value, so there is no second
					// argument for `that` to be. Brackets are what supply one.
					p.bag.AddHint(op.Pos,
						"`that` is the second argument of the brackets around it, and a pipeline stage hands over one value; write the brackets",
						"a pipeline stage has no second argument")
				}
				if use.Parts > 0 {
					// `fore`/`mid`/`aft` ask for the Twine opened, and a match
					// is what opens one: `pair | add fore aft`.
					left = &ast.Ward{
						Subject: left,
						Arms:    []*ast.Arm{{Pat: partsPattern(op.Pos, use.Parts), Body: ast.FillHoles(right), P: op.Pos}},
						P:       op.Pos,
						Via:     op.Kind.String(),
					}
					continue
				}
				left = &ast.Let{
					Binds: []*ast.Bind{{Name: ast.HoleName, NamePos: op.Pos, Value: left}},
					Body:  ast.FillHoles(right),
					P:     op.Pos,
					Via:   op.Kind.String(),
				}
				continue
			}
			left = feed(right, left, op.Pos, op.Kind.String())
		case token.As:
			op := p.next()
			fn := p.parseApp()
			// `as` feeds the function, not the value, the same way `where`
			// does: `xs as mul _ 2` maps by `(x : mul x 2)`.
			fn = p.holeLambda(fn, op.Pos)
			// `xs as f` is `bend f xs`.
			left = &ast.App{
				Fn:   &ast.Var{Name: "bend", P: op.Pos},
				Args: []ast.Expr{fn, left},
				P:    op.Pos,
				Via:  "as",
			}
		case token.Else, token.Failing:
			// `h else d` is `otherwise d h`, and `w failing d` is `snag d w`:
			// the value that might not be there, then what to use instead.
			// Unlike `where` and `as` these feed a value rather than a
			// function, so a `_` in one is not a lambda and has nothing here
			// to claim it.
			op := p.next()
			verb, via := "otherwise", "else"
			if op.Kind == token.Failing {
				verb, via = "snag", "failing"
			}
			left = &ast.App{
				Fn:   &ast.Var{Name: verb, P: op.Pos},
				Args: []ast.Expr{p.parseApp(), left},
				P:    op.Pos,
				Via:  via,
			}
		case token.Where:
			op := p.next()
			pred := p.parseApp()
			// `where` feeds the predicate, not the value, so a `_` in it makes
			// a function: `xs where mod _ 2` filters by `(x : mod x 2)`.
			pred = p.holeLambda(pred, op.Pos)
			// `xs where p` is `sift p xs`.
			left = &ast.App{
				Fn:   &ast.Var{Name: "sift", P: op.Pos},
				Args: []ast.Expr{pred, left},
				P:    op.Pos,
				Via:  "where",
			}
		default:
			return left
		}
	}
}

// feed applies value as the final argument of fn, which is what makes the
// pipeline work with Weave's data-last standard library.
func feed(fn, value ast.Expr, pos token.Pos, via string) ast.Expr {
	if app, ok := fn.(*ast.App); ok {
		return &ast.App{
			Fn:   app.Fn,
			Args: append(append([]ast.Expr{}, app.Args...), value),
			P:    app.P,
			Via:  via,
		}
	}
	return &ast.App{Fn: fn, Args: []ast.Expr{value}, P: pos, Via: via}
}

// holeLambda turns an expression containing `_` into the function of one
// argument it stands for, and leaves anything else alone.
func (p *parser) holeLambda(e ast.Expr, pos token.Pos) ast.Expr {
	use := ast.Holes(e)
	if !use.Any {
		return e
	}
	p.reportWidthClash(use)
	// The first parameter is the value whole, or taken apart once a `fore`, a
	// `mid` or an `aft` has asked for that; a second is added as soon as a
	// `that` appears anywhere in the group.
	params := []ast.Pattern{&ast.PVar{Name: ast.HoleName, P: pos}}
	if use.Parts > 0 {
		params[0] = partsPattern(pos, use.Parts)
	}
	if use.Args > 1 {
		params = append(params, &ast.PVar{Name: ast.PartnerName, P: pos})
	}
	return &ast.Lambda{Params: params, Body: ast.FillHoles(e), P: pos}
}

// partsPattern is `(fore, aft)` or `(fore, mid, aft)`: the components those
// words ask for. `aft` is the last of however many there are, so the width is
// what a `mid` decided, and the pattern is what fixes it.
// reportWidthClash catches a group whose component words disagree about how
// wide the Twine is. Each word carries its own width, so `add former aft` says
// two and three in the same breath and cannot be given a meaning.
func (p *parser) reportWidthClash(use ast.HoleUse) {
	if !use.Clashed {
		return
	}
	p.bag.AddHint(use.ClashAt,
		"`former` and `latter` are the halves of a Twine of two; `fore`, `mid` and `aft` are the parts of one of three. One group cannot ask for both",
		"these words disagree about how wide the Twine is")
}

// partsPattern is `(former, latter)` or `(fore, mid, aft)`: the components the
// words of that width ask for. The width came from the words themselves, so
// there is nothing here to work out.
func partsPattern(pos token.Pos, n int) ast.Pattern {
	names := ast.PartNames(n)
	elems := make([]ast.Pattern, len(names))
	for i, name := range names {
		elems[i] = &ast.PVar{Name: name, P: pos}
	}
	return &ast.PTwine{Elems: elems, P: pos}
}

// parseApp reads a juxtaposition chain, `f a b`.
func (p *parser) parseApp() ast.Expr {
	head := p.parseAtom()
	var args []ast.Expr
	for p.startsAtom() {
		args = append(args, p.parseAtom())
	}
	if len(args) == 0 {
		return head
	}
	return &ast.App{Fn: head, Args: args, P: head.Pos()}
}

// startsAtom reports whether the current token can begin an atom, which is how
// the parser knows where a juxtaposition chain ends.
func (p *parser) startsAtom() bool {
	if p.pos == p.armsAt {
		// Reading a ward's subject, and this is where its arms begin.
		return false
	}
	switch p.kind() {
	case token.Int, token.Float, token.Char, token.Text,
		token.Lower, token.Upper, token.LParen, token.LBracket, token.LBrace,
		token.Ward, token.Weave,
		token.Underscore, token.Partner, token.Former, token.Latter,
		token.Fore, token.Mid, token.Aft:
		return true
	}
	return false
}

func (p *parser) parseAtom() ast.Expr {
	t := p.cur()
	switch t.Kind {
	case token.Int:
		p.next()
		return &ast.IntLit{Value: t.Int, P: t.Pos}
	case token.Float:
		p.next()
		return &ast.FloatLit{Value: t.Float, P: t.Pos}
	case token.Char:
		p.next()
		return &ast.CharLit{Value: t.Char, P: t.Pos}
	case token.Text:
		p.next()
		return &ast.TextLit{Value: t.Lit, P: t.Pos}
	case token.Lower:
		p.next()
		return &ast.Var{Name: t.Lit, P: t.Pos}
	case token.Upper:
		p.next()
		return &ast.Ctor{Name: t.Lit, P: t.Pos}
	case token.LParen:
		defer p.holdArmStop()()
		return p.parseParen()
	case token.LBracket:
		defer p.holdArmStop()()
		return p.parseThreadLit()
	case token.LBrace:
		defer p.holdArmStop()()
		return p.parseWebLit()
	case token.Ward:
		return p.parseWard()
	case token.Weave, token.Channel:
		defer p.holdArmStop()()
		return p.parseInlineLet()
	case token.Underscore:
		p.next()
		return &ast.Hole{P: t.Pos}
	case token.Partner:
		p.next()
		return &ast.Hole{P: t.Pos, Slot: 1}
	case token.Former:
		p.next()
		return &ast.Hole{P: t.Pos, Parts: 2, At: 0}
	case token.Latter:
		p.next()
		return &ast.Hole{P: t.Pos, Parts: 2, At: 1}
	case token.Fore:
		p.next()
		return &ast.Hole{P: t.Pos, Parts: 3, At: 0}
	case token.Mid:
		p.next()
		return &ast.Hole{P: t.Pos, Parts: 3, At: 1}
	case token.Aft:
		p.next()
		return &ast.Hole{P: t.Pos, Parts: 3, At: 2}
	default:
		p.errf(t.Pos, "expected an expression, found %s", describe(t))
		p.recover()
		return &ast.Bad{P: t.Pos}
	}
}

// parseParen reads a lambda, a tuple, or a grouped expression.
func (p *parser) parseParen() ast.Expr {
	open := p.expect(token.LParen)

	if p.parenIsLambda() {
		var params []ast.Pattern
		for !p.at(token.Colon) && !p.at(token.RParen) && !p.at(token.EOF) {
			params = append(params, p.parsePatternAtom())
		}
		p.expect(token.Colon)
		body := p.parseExpr()
		p.expect(token.RParen)
		return &ast.Lambda{Params: params, Body: body, P: open.Pos}
	}

	if p.accept(token.RParen) {
		p.errf(open.Pos, "empty parentheses are not a value")
		return &ast.Bad{P: open.Pos}
	}

	first := p.parseExpr()
	if !p.at(token.Comma) {
		p.expect(token.RParen)
		// Brackets are what bind a `_`: this group is the function it stands
		// for. Nesting resolves to the innermost group, so `(gt 0 (mod _ 2))`
		// makes a function of the inner brackets and is a type error, rather
		// than quietly meaning something else.
		return p.holeLambda(first, open.Pos)
	}
	elems := []ast.Expr{first}
	for p.accept(token.Comma) {
		elems = append(elems, p.parseExpr())
	}
	p.expect(token.RParen)
	return p.holeLambda(&ast.TwineLit{Elems: elems, P: open.Pos}, open.Pos)
}

// parenIsLambda looks ahead for a `:` directly inside the just-opened paren,
// which distinguishes `(x : body)` from `(expr)` and `(a, b)`.
func (p *parser) parenIsLambda() bool {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.LParen, token.LBracket, token.LBrace:
			depth++
		case token.RParen, token.RBracket, token.RBrace:
			if depth == 0 {
				return false
			}
			depth--
		case token.Colon:
			if depth == 0 {
				return true
			}
		case token.EOF:
			return false
		}
	}
	return false
}

// parseThreadLit reads `[1 2 3]` or `[Step North 3, Rest]`.
//
// A comma anywhere at the top level of the brackets makes the elements full
// expressions; without one they are atoms, so that `[1 2 3]` is three numbers
// rather than an application. The alternative — always taking atoms — reads
// `[Step North 3, Rest]` as four elements, which is a trap, and always taking
// applications loses the notation that makes a literal grid readable.
func (p *parser) parseThreadLit() ast.Expr {
	open := p.expect(token.LBracket)
	commas := p.bracketHasComma()
	var elems []ast.Expr
	for !p.at(token.RBracket) && !p.at(token.EOF) {
		if commas {
			elems = append(elems, p.parseExpr())
			if !p.at(token.RBracket) {
				p.expect(token.Comma)
			}
			continue
		}
		elems = append(elems, p.parseAtom())
	}
	p.expect(token.RBracket)
	return &ast.ThreadLit{Elems: elems, P: open.Pos}
}

// bracketHasComma reports whether the bracket just opened holds a comma at its
// own level.
func (p *parser) bracketHasComma() bool {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.LParen, token.LBracket, token.LBrace:
			depth++
		case token.RParen, token.RBrace:
			depth--
		case token.RBracket:
			if depth == 0 {
				return false
			}
			depth--
		case token.Comma:
			if depth == 0 {
				return true
			}
		case token.EOF:
			return false
		}
	}
	return false
}

func (p *parser) parseWebLit() ast.Expr {
	open := p.expect(token.LBrace)
	var pairs []ast.WebPair
	for !p.at(token.RBrace) && !p.at(token.EOF) {
		k := p.parseAtom()
		p.expect(token.Colon)
		v := p.parseAtom()
		pairs = append(pairs, ast.WebPair{Key: k, Val: v})
		p.accept(token.Comma)
	}
	p.expect(token.RBrace)
	return &ast.WebLit{Pairs: pairs, P: open.Pos}
}

// parseWard reads a pattern match and its arms, in either of the two shapes a
// ward has:
//
//	ward c              ward c (Light : 1) (Shadow : 0)
//	  Light : 1
//	  Shadow : 0
//
// What separates them is the line, not the brackets. If the ward's line is
// followed by an indented block then the arms are in that block and *all* of
// the line is the subject; otherwise the arms are the run of bracketed
// `pattern : body` groups the line ends with, and the subject is what comes
// before them.
//
// The earlier rule — the subject stops at the first bracketed group holding a
// `:` — could not tell an arm from a lambda, so `ward seekidx (r : test r) rows`
// read the lambda as an arm and the rest as nothing at all. Deciding on the
// block first, and then only on a run at the *end* of the line, leaves a lambda
// anywhere in a subject unambiguous: it has something after it.
//
// This is also the rule the tree-sitter grammar already used, so the two agree.
func (p *parser) parseWard() ast.Expr {
	kw := p.expect(token.Ward)

	saved := p.armsAt
	if p.blockArmsAhead() {
		p.armsAt = -1
	} else {
		p.armsAt = p.inlineArmsStart()
	}
	inline := p.armsAt
	subject := p.parseExpr()
	p.armsAt = saved

	if inline >= 0 && p.at(token.LParen) {
		return p.parseInlineArms(kw, subject)
	}

	if !p.at(token.Newline) {
		p.bag.AddHint(kw.Pos,
			"put the arms on following lines, indented, or bracket them on this one: `ward c (Light : 1) (_ : 0)`",
			"`ward` needs its arms")
		p.recover()
		return &ast.Bad{P: kw.Pos}
	}
	p.next()
	if !p.at(token.Indent) {
		p.errf(p.cur().Pos, "expected the ward's arms: an indented block, or bracketed on the `ward` line")
		return &ast.Bad{P: kw.Pos}
	}
	p.next()

	var arms []*ast.Arm
	for {
		p.skipNewlines()
		if p.at(token.Dedent) || p.at(token.EOF) {
			break
		}
		armPos := p.cur().Pos
		pat := p.parsePattern()
		p.expect(token.Colon)
		body := p.parseBody()
		arms = append(arms, &ast.Arm{Pat: pat, Body: body, P: armPos})
	}
	p.expect(token.Dedent)

	if len(arms) == 0 {
		p.errf(kw.Pos, "`ward` needs at least one arm")
		return &ast.Bad{P: kw.Pos}
	}
	return &ast.Ward{Subject: subject, Arms: arms, P: kw.Pos}
}

// matchingClose finds the bracket closing the group opening at at, or the end
// of the token stream when nothing does.
func (p *parser) matchingClose(at int) int {
	depth := 0
	for i := at; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.LParen, token.LBracket, token.LBrace:
			depth++
		case token.RParen, token.RBracket, token.RBrace:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(p.toks)
}

// parseInlineArms reads the one-line form, `ward c (Light : 1) (_ : 0)`.
//
// An arm and a lambda are written the same way, which is not a coincidence and
// not a problem: `(x : e)` means the same thing read either way. What decides
// it is position — inside a ward's head, a bracketed group holding a `:` is an
// arm, and everywhere else it is a lambda.
func (p *parser) parseInlineArms(kw token.Token, subject ast.Expr) ast.Expr {
	var arms []*ast.Arm
	for p.at(token.LParen) && p.groupHoldsColon(p.pos, p.matchingClose(p.pos)) {
		open := p.next()
		pat := p.parsePattern()
		if !p.at(token.Colon) {
			// More than one pattern before the `:`. A lambda may take several
			// arguments; an arm matches one value.
			p.errf(p.cur().Pos, "an arm matches one value, so it takes one pattern")
			p.recover()
			return &ast.Bad{P: kw.Pos}
		}
		p.expect(token.Colon)
		body := p.parseExpr()
		p.expect(token.RParen)
		arms = append(arms, &ast.Arm{Pat: pat, Body: body, P: open.Pos})
	}
	if len(arms) == 0 {
		p.errf(kw.Pos, "`ward` needs at least one arm")
		return &ast.Bad{P: kw.Pos}
	}
	return &ast.Ward{Subject: subject, Arms: arms, P: kw.Pos, Inline: true}
}

// lineEnd is where the logical line under the cursor finishes: its newline, or
// the bracket of whatever encloses it, whichever comes first.
func (p *parser) lineEnd() int {
	depth := 0
	for i := p.pos; i < len(p.toks); i++ {
		switch p.toks[i].Kind {
		case token.LParen, token.LBracket, token.LBrace:
			depth++
		case token.RParen, token.RBracket, token.RBrace:
			if depth == 0 {
				return i
			}
			depth--
		case token.Newline, token.EOF, token.Dedent:
			if depth == 0 {
				return i
			}
		}
	}
	return len(p.toks)
}

// blockArmsAhead reports whether an indented block opens after this line, which
// is what says a ward's arms are down there rather than on the line itself.
func (p *parser) blockArmsAhead() bool {
	end := p.lineEnd()
	return end < len(p.toks) && p.toks[end].Kind == token.Newline &&
		end+1 < len(p.toks) && p.toks[end+1].Kind == token.Indent
}

// inlineArmsStart finds where the run of bracketed `pattern : body` groups the
// line ends with begins, or -1 when the line does not end in one. Only a run
// reaching the very end of the line counts, which is what keeps a lambda in the
// middle of a subject from being mistaken for an arm.
func (p *parser) inlineArmsStart() int {
	end := p.lineEnd()
	start := end
	for {
		open := p.groupBefore(start)
		if open < 0 || !p.groupHoldsColon(open, start-1) {
			break
		}
		start = open
	}
	if start == end {
		return -1
	}
	return start
}

// groupBefore finds the `(` matching a `)` sitting just before at, or -1 when
// what is there is not a closed bracket group within this line.
func (p *parser) groupBefore(at int) int {
	if at-1 < p.pos || p.toks[at-1].Kind != token.RParen {
		return -1
	}
	depth := 0
	for i := at - 1; i >= p.pos; i-- {
		switch p.toks[i].Kind {
		case token.RParen, token.RBracket, token.RBrace:
			depth++
		case token.LParen, token.LBracket, token.LBrace:
			depth--
			if depth == 0 {
				if p.toks[i].Kind != token.LParen {
					return -1
				}
				return i
			}
		}
	}
	return -1
}

// groupHoldsColon reports whether the group from open to close has a `:` at its
// own level, which is what an arm has and a group of anything else does not.
func (p *parser) groupHoldsColon(open, close int) bool {
	depth := 0
	for i := open + 1; i < close; i++ {
		switch p.toks[i].Kind {
		case token.LParen, token.LBracket, token.LBrace:
			depth++
		case token.RParen, token.RBracket, token.RBrace:
			depth--
		case token.Colon:
			if depth == 0 {
				return true
			}
		}
	}
	return false
}

// parseInlineLet reads `weave a is 1, b is 2 into body`.
func (p *parser) parseInlineLet() ast.Expr {
	pos := p.cur().Pos
	p.next() // `weave` or `channel`

	var binds []*ast.Bind
	for {
		// After a comma the keyword may be repeated, but need not be.
		if !p.accept(token.Weave) {
			p.accept(token.Channel)
		}
		if p.startsBindPattern() {
			pos := p.cur().Pos
			pat := p.parsePatternAtom()
			p.expect(token.Is)
			binds = append(binds, &ast.Bind{Pat: pat, NamePos: pos, Value: p.parseExpr()})
			if p.accept(token.Comma) {
				continue
			}
			break
		}
		if !p.at(token.Lower) {
			p.errf(p.cur().Pos, "expected a binding name, found %s", describe(p.cur()))
			break
		}
		nameTok := p.next()
		var params []ast.Pattern
		for p.startsPattern() {
			params = append(params, p.parsePatternAtom())
		}
		p.expect(token.Is)
		value := p.parseExpr()
		binds = append(binds, &ast.Bind{
			Name: nameTok.Lit, NamePos: nameTok.Pos, Params: params, Value: value,
		})

		if p.accept(token.Comma) {
			continue
		}
		break
	}
	p.expect(token.Into)
	body := p.parseExpr()
	return &ast.Let{Binds: binds, Body: body, P: pos}
}

// ---------------------------------------------------------------- patterns

// startsBindPattern reports whether what follows `weave` takes the value apart
// rather than naming it.
//
// A bare name is a name, not a pattern — `weave x is 1` binds `x` and does not
// match against it — and a constructor with no brackets would be ambiguous with
// one. So the shapes that count are the bracketed ones and `_`, which is what
// anyone writing a destructuring binding reaches for.
func (p *parser) startsBindPattern() bool {
	switch p.kind() {
	case token.LParen, token.LBracket, token.Underscore:
		return true
	}
	return false
}

func (p *parser) startsPattern() bool {
	switch p.kind() {
	case token.Underscore, token.Lower, token.Upper,
		token.Int, token.Float, token.Char, token.Text, token.LParen, token.LBracket:
		return true
	}
	return false
}

// parsePattern reads a pattern in a position where constructor application
// needs no parentheses, such as a ward arm: `Held n : ...`.
func (p *parser) parsePattern() ast.Pattern {
	t := p.cur()
	switch t.Kind {
	case token.Upper:
		p.next()
		var args []ast.Pattern
		for p.startsPattern() {
			args = append(args, p.parsePatternAtom())
		}
		return &ast.PCtor{Name: t.Lit, Args: args, P: t.Pos}
	case token.Lower:
		// A bare lower-case name binds; applied to arguments it names a
		// built-in constructor such as `knot r c`.
		if p.startsPatternAt(1) {
			p.next()
			var args []ast.Pattern
			for p.startsPattern() {
				args = append(args, p.parsePatternAtom())
			}
			return &ast.PCtor{Name: t.Lit, Args: args, P: t.Pos}
		}
	}
	return p.parsePatternAtom()
}

func (p *parser) startsPatternAt(n int) bool {
	switch p.peek(n).Kind {
	case token.Underscore, token.Lower, token.Upper,
		token.Int, token.Float, token.Char, token.Text, token.LParen, token.LBracket:
		return true
	}
	return false
}

// parseThreadPattern reads `[p1 p2]` or `[p1 ..rest]`: a Thread taken apart by
// position, with an optional name for everything past the fixed elements.
//
// Commas are accepted but not required, matching the Thread literal, which
// reads `[1 2 3]` and `[Step North 3, Rest]` alike.
func (p *parser) parseThreadPattern() ast.Pattern {
	open := p.next() // `[`
	out := &ast.PThread{P: open.Pos}
	for !p.at(token.RBracket) && !p.at(token.EOF) {
		if p.at(token.DotDot) {
			dots := p.next()
			switch {
			case p.at(token.Lower):
				name := p.next()
				out.Rest = &ast.PVar{Name: name.Lit, P: name.Pos}
			case p.at(token.Underscore):
				u := p.next()
				out.Rest = &ast.PWild{P: u.Pos}
			default:
				// `[a ..]` is `[a .._]` — the rest is matched but not named.
				out.Rest = &ast.PWild{P: dots.Pos}
			}
			break
		}
		out.Elems = append(out.Elems, p.parsePatternAtom())
		p.accept(token.Comma)
	}
	p.expect(token.RBracket)
	return out
}

// parsePatternAtom reads a pattern that binds tighter than application, as
// required in parameter position.
func (p *parser) parsePatternAtom() ast.Pattern {
	t := p.cur()
	switch t.Kind {
	case token.Underscore:
		p.next()
		return &ast.PWild{P: t.Pos}
	case token.Lower:
		p.next()
		return &ast.PVar{Name: t.Lit, P: t.Pos}
	case token.Upper:
		p.next()
		return &ast.PCtor{Name: t.Lit, P: t.Pos}
	case token.Int:
		p.next()
		return &ast.PInt{Value: t.Int, P: t.Pos}
	case token.Float:
		p.next()
		return &ast.PFloat{Value: t.Float, P: t.Pos}
	case token.Char:
		p.next()
		return &ast.PChar{Value: t.Char, P: t.Pos}
	case token.Text:
		p.next()
		return &ast.PText{Value: t.Lit, P: t.Pos}
	case token.LParen:
		p.next()
		first := p.parsePattern()
		if !p.at(token.Comma) {
			p.expect(token.RParen)
			return first
		}
		elems := []ast.Pattern{first}
		for p.accept(token.Comma) {
			elems = append(elems, p.parsePattern())
		}
		p.expect(token.RParen)
		return &ast.PTwine{Elems: elems, P: t.Pos}
	case token.LBracket:
		return p.parseThreadPattern()
	default:
		p.errf(t.Pos, "expected a pattern, found %s", describe(t))
		p.next()
		return &ast.PBad{P: t.Pos}
	}
}

// ------------------------------------------------------------------- types

// parseType reads a type annotation: `Thread a -> Earth`. The arrow is
// right-associative and builds a node of its own, so a parenthesised function
// type such as `(a -> b) -> c` keeps its inner arrow intact.
func (p *parser) parseType() *ast.TypeExpr {
	head := p.parseTypeApp()
	if p.accept(token.Arrow) {
		return &ast.TypeExpr{
			Name: ast.FuncTypeName,
			Args: []*ast.TypeExpr{head, p.parseType()},
			P:    head.P,
		}
	}
	return head
}

func (p *parser) parseTypeApp() *ast.TypeExpr {
	head := p.parseTypeAtom()
	for p.at(token.Upper) || p.at(token.Lower) || p.at(token.LParen) {
		head.Args = append(head.Args, p.parseTypeAtom())
	}
	return head
}

func (p *parser) parseTypeAtom() *ast.TypeExpr {
	t := p.cur()
	switch t.Kind {
	case token.Upper, token.Lower:
		p.next()
		return &ast.TypeExpr{Name: t.Lit, P: t.Pos}
	case token.LParen:
		p.next()
		inner := p.parseType()
		if p.at(token.Comma) {
			elems := []*ast.TypeExpr{inner}
			for p.accept(token.Comma) {
				elems = append(elems, p.parseType())
			}
			p.expect(token.RParen)
			return &ast.TypeExpr{Name: ast.TwineTypeName, Args: elems, P: t.Pos}
		}
		p.expect(token.RParen)
		return inner
	default:
		p.errf(t.Pos, "expected a type, found %s", describe(t))
		p.next()
		return &ast.TypeExpr{Name: "?", P: t.Pos}
	}
}
