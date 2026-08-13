// Package ast defines the Weave syntax tree.
//
// The tree is deliberately small. Surface forms that are pure sugar are
// desugared by the parser, so later phases see fewer shapes:
//
//	x | f a        becomes  f a x        (the pipe feeds the last argument)
//	xs where p     becomes  sift p xs    (D-particle aliases)
//	weave a is 1
//	b              becomes  Let{a=1} in b
package ast

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/malleum/weave/internal/token"
)

// Node is any syntax tree node.
type Node interface {
	Pos() token.Pos
}

// ---------------------------------------------------------------- top level

// File is one parsed .weave source file.
type File struct {
	// Types are the sum types the file declares, in source order.
	Types []*TypeDecl
	Decls []*Decl
	// Outputs are the file's bare expressions, in source order. A file may
	// hold several so that things being tried out can sit side by side; the
	// program prints only the last, and `weave trace` reports every one, which
	// is what puts each of them beside its own line in the editor.
	Outputs []Expr
}

// Output is the expression the program prints: the last bare one, or nil for a
// file that only defines names.
func (f *File) Output() Expr {
	if len(f.Outputs) == 0 {
		return nil
	}
	return f.Outputs[len(f.Outputs)-1]
}

// TypeDecl declares a sum type and its constructors:
//
//	Direction is North | South | East | West
//	Tree a is Leaf | Node (Tree a) a (Tree a)
//
// A line beginning with an upper-case name is always a type declaration, which
// is what keeps this apart from an ordinary definition.
type TypeDecl struct {
	Name    string
	NamePos token.Pos
	// Params are the lower-case type parameters, in order.
	Params []string
	Ctors  []*CtorDecl
}

func (d *TypeDecl) Pos() token.Pos { return d.NamePos }

// CtorDecl is one alternative of a TypeDecl: a constructor name and the types
// of the values it carries.
type CtorDecl struct {
	Name    string
	NamePos token.Pos
	Fields  []*TypeExpr
}

func (d *CtorDecl) Pos() token.Pos { return d.NamePos }

// Decl is a top-level definition. A definition with several pattern-matching
// clauses collects them all here, in source order.
//
//	fib 0 is 0
//	fib 1 is 1
//	fib n is add (fib (sub n 1)) (fib (sub n 2))
type Decl struct {
	Name    string
	NamePos token.Pos
	Clauses []*Clause
	// Sig is the optional `name :: Type` annotation, if one was written.
	Sig *TypeExpr
	// Memo records that the definition was written `remember name ... is ...`:
	// its results are kept, keyed on the arguments, so a call that repeats
	// costs a lookup. It is a property of the definition, not of one clause.
	Memo bool
	// Pat is set when the definition takes its value apart rather than naming
	// it: `(width, height) is dimsOf Source`. Name is then a name no source can
	// spell, holding the value itself, and ExpandPatterns turns the definition
	// into that one plus a projection for each name the pattern binds. Only the
	// parser and the formatter see this; the checker expands it first.
	Pat Pattern
	// Hidden keeps a definition ExpandPatterns generated out of `weave trace`,
	// so that a line binding several names reports one value and not one per
	// name. Display is what to call the one that is reported.
	Hidden  bool
	Display string
}

func (d *Decl) Pos() token.Pos { return d.NamePos }

// Arity reports the number of parameters the declaration takes.
func (d *Decl) Arity() int {
	if len(d.Clauses) == 0 {
		return 0
	}
	return len(d.Clauses[0].Params)
}

// Clause is one equation of a declaration.
type Clause struct {
	Params  []Pattern
	Body    Expr
	ClauseP token.Pos
}

func (c *Clause) Pos() token.Pos { return c.ClauseP }

// --------------------------------------------------------------- expressions

// Expr is a Weave expression. Every construct in the language produces a
// value, so there are no statements.
type Expr interface {
	Node
	exprNode()
}

// IntLit is an Earth literal.
type IntLit struct {
	Value int64
	P     token.Pos
}

// FloatLit is a Water literal.
type FloatLit struct {
	Value float64
	P     token.Pos
}

// CharLit is a Fire literal.
type CharLit struct {
	Value rune
	P     token.Pos
}

// TextLit is an Air literal.
type TextLit struct {
	Value string
	P     token.Pos
}

// Var references a lower-case binding: a parameter, local, or top-level name.
type Var struct {
	Name string
	P    token.Pos
}

// Ctor references an upper-case constructor, such as Held or Light. It is an
// atom; applying it to arguments produces an App.
type Ctor struct {
	Name string
	P    token.Pos
}

// App applies Fn to Args. Application is curried, but the parser groups a
// juxtaposition chain into one node so codegen can emit direct calls.
type App struct {
	Fn   Expr
	Args []Expr
	P    token.Pos
	// Via records the surface form this call was written in: "" for ordinary
	// application, or "|", "where", "as" or "through" when the last argument
	// arrived through a pipeline. Only the formatter reads it; every other
	// phase sees the same desugared call either way.
	Via string
}

// Lambda is an anonymous function, written `(x : body)`.
type Lambda struct {
	Params []Pattern
	Body   Expr
	P      token.Pos
}

// Bind is a single local binding inside a Let. Params is non-empty when the
// binding was introduced with `channel`, i.e. it defines a local function.
type Bind struct {
	Name    string
	NamePos token.Pos
	Params  []Pattern
	Value   Expr
	// Pat is set instead of Name when the binding takes its value apart:
	// `weave (a, b) is p`. A binding that does that has no name of its own and
	// no parameters, so Name is empty and Params is nil. Everywhere that reads
	// Name has to ask about Pat first.
	Pat Pattern
}

func (b *Bind) Pos() token.Pos { return b.NamePos }

// Let introduces local bindings scoped over Body. Bindings are visible to
// later bindings in the same block and to Body.
type Let struct {
	Binds []*Bind
	Body  Expr
	P     token.Pos
	// Via is set when the Let came from a pipeline stage holding a `_`, and
	// records which particle wrote it. Only the formatter reads it.
	Via string
}

// Ward is a pattern match. The compiler checks that Arms are exhaustive.
type Ward struct {
	Subject Expr
	Arms    []*Arm
	P       token.Pos
	// Via is set when the Ward came from a pipeline stage holding a `that`,
	// and records which particle wrote it. Only the formatter reads it.
	Via string
	// Inline records that the arms were written bracketed on the subject's own
	// line. Only the formatter reads it, and only to decide whether the one
	// line is still short enough.
	Inline bool
	// Binding marks a ward ExpandPatterns generated for a definition that takes
	// its value apart. Nobody wrote it, so a diagnostic about it has to name
	// what was written instead.
	Binding bool
}

// Arm is one `pattern : expression` case of a Ward.
type Arm struct {
	Pat  Pattern
	Body Expr
	P    token.Pos
}

func (a *Arm) Pos() token.Pos { return a.P }

// ThreadLit is a sequence literal, `[1 2 3]`.
type ThreadLit struct {
	Elems []Expr
	P     token.Pos
}

// TwineLit is a tuple literal, `(1, "a")`.
type TwineLit struct {
	Elems []Expr
	P     token.Pos
}

// WebPair is one key/value entry of a Web literal.
type WebPair struct {
	Key, Val Expr
}

// WebLit is a map literal, `{"a" : 1  "b" : 2}`.
type WebLit struct {
	Pairs []WebPair
	P     token.Pos
}

// Hole is a word written where a value goes: one of the arguments the
// enclosing bracket group or pipeline stage stands ready to receive.
//
// There are three families, and they answer three different questions.
//
//	_  it  this   the first argument       xs where (mod _ 2 | eq 0)
//	that          the second argument      braid (add this that) 0
//	former        the first half of it     pairs as add former latter
//	latter        the second half of it
//
// `_`, `it` and `this` are the same token; the first is the symbol and the
// other two the words. `that` says the group takes two arguments, which is what
// lets a folding function be written without naming its parameters.
// `former` and `latter` say the first argument is a two-part Twine and ask for
// it opened, so the group binds both halves rather than the pair.
//
// The parser removes every Hole as it builds the tree — a bracket group holding
// one becomes a Lambda, and a pipeline stage holding one becomes a Let or a
// match over the piped value, so nothing downstream ever sees this node. One
// that survives had nothing to stand for, which the parser reports.
type Hole struct {
	P token.Pos
	// Slot is which argument: 0 for `_` and `this`, 1 for `that`.
	Slot int
	// Parts is how wide a Twine this word names a component of, and At is
	// which component. A word that names the argument whole has Parts 0.
	//
	// Both are fixed by the word itself and by nothing else, which is the whole
	// point. Two earlier spellings named a component *relative* to a width —
	// "the latter of two", "the last of however many" — and a relative word
	// cannot be desugared until the type is known, which is later than the
	// parser and, in some positions, later than it can be asked for at all. So
	// a word carries its width: `former` and `latter` are the halves of a
	// Twine of two, `fore`, `mid` and `aft` the three parts of a Twine of
	// three, and there is no word for a component of a wider one — that is a
	// pattern, and says so.
	Parts, At int
}

// The names a filled Hole is given. Patterns spell `_` as PWild and the rest
// are keywords, so none can collide with anything a program writes.
const (
	HoleName    = "_"
	PartnerName = "that"
	FormerName  = "former"
	LatterName  = "latter"
	ForeName    = "fore"
	MidName     = "mid"
	AftName     = "aft"
)

// partNames is what each width calls its components, in order.
var partNames = map[int][]string{
	2: {FormerName, LatterName},
	3: {ForeName, MidName, AftName},
}

// PartNames is what a Twine of this width calls its components, or nil when
// nothing does.
func PartNames(parts int) []string { return partNames[parts] }

// HoleVarName is the name the hole spelled by these is bound to.
func HoleVarName(slot, parts, at int) string {
	if names := partNames[parts]; at < len(names) {
		return names[at]
	}
	if slot == 1 {
		return PartnerName
	}
	return HoleName
}

// Bad marks a subtree the parser could not read. It lets parsing continue so
// one run can report several errors.
type Bad struct {
	P token.Pos
}

func (e *IntLit) Pos() token.Pos    { return e.P }
func (e *FloatLit) Pos() token.Pos  { return e.P }
func (e *CharLit) Pos() token.Pos   { return e.P }
func (e *TextLit) Pos() token.Pos   { return e.P }
func (e *Var) Pos() token.Pos       { return e.P }
func (e *Ctor) Pos() token.Pos      { return e.P }
func (e *App) Pos() token.Pos       { return e.P }
func (e *Lambda) Pos() token.Pos    { return e.P }
func (e *Let) Pos() token.Pos       { return e.P }
func (e *Ward) Pos() token.Pos      { return e.P }
func (e *ThreadLit) Pos() token.Pos { return e.P }
func (e *TwineLit) Pos() token.Pos  { return e.P }
func (e *WebLit) Pos() token.Pos    { return e.P }
func (e *Hole) Pos() token.Pos      { return e.P }
func (e *Bad) Pos() token.Pos       { return e.P }

func (*IntLit) exprNode()    {}
func (*FloatLit) exprNode()  {}
func (*CharLit) exprNode()   {}
func (*TextLit) exprNode()   {}
func (*Var) exprNode()       {}
func (*Ctor) exprNode()      {}
func (*App) exprNode()       {}
func (*Lambda) exprNode()    {}
func (*Let) exprNode()       {}
func (*Ward) exprNode()      {}
func (*ThreadLit) exprNode() {}
func (*TwineLit) exprNode()  {}
func (*WebLit) exprNode()    {}
func (*Hole) exprNode()      {}
func (*Bad) exprNode()       {}

// ------------------------------------------------------------------ patterns

// Pattern matches and destructures a value.
type Pattern interface {
	Node
	patternNode()
}

// PWild is `_`, matching anything without binding it.
type PWild struct{ P token.Pos }

// PVar binds whatever it matches to Name.
type PVar struct {
	Name string
	P    token.Pos
}

// PInt, PFloat, PChar and PText match a literal value.
type PInt struct {
	Value int64
	P     token.Pos
}
type PFloat struct {
	Value float64
	P     token.Pos
}
type PChar struct {
	Value rune
	P     token.Pos
}
type PText struct {
	Value string
	P     token.Pos
}

// PCtor matches a constructor and its fields, such as `Held n` or `knot r c`.
type PCtor struct {
	Name string
	Args []Pattern
	P    token.Pos
}

// PTwine destructures a tuple, `(x, y)`.
type PTwine struct {
	Elems []Pattern
	P     token.Pos
}

// PThread destructures a Thread by position, `[a b]`, with an optional rest
// pattern that takes everything past the fixed elements, `[first ..rest]`.
//
// Without a rest it matches a Thread of exactly that length; with one it
// matches a Thread of at least that length. Either way it can never be
// exhaustive on its own — a Thread's length is not drawn from a finite set —
// except for `[..rest]`, which fixes nothing and so matches anything.
type PThread struct {
	Elems []Pattern
	// Rest is the pattern the remainder is bound to, or nil when the length
	// is fixed. It is always a PVar or a PWild.
	Rest Pattern
	P    token.Pos
}

// PBad marks a pattern the parser could not read.
type PBad struct{ P token.Pos }

func (p *PWild) Pos() token.Pos   { return p.P }
func (p *PVar) Pos() token.Pos    { return p.P }
func (p *PInt) Pos() token.Pos    { return p.P }
func (p *PFloat) Pos() token.Pos  { return p.P }
func (p *PChar) Pos() token.Pos   { return p.P }
func (p *PText) Pos() token.Pos   { return p.P }
func (p *PCtor) Pos() token.Pos   { return p.P }
func (p *PTwine) Pos() token.Pos  { return p.P }
func (p *PThread) Pos() token.Pos { return p.P }
func (p *PBad) Pos() token.Pos    { return p.P }
func (*PWild) patternNode()       {}
func (*PVar) patternNode()        {}
func (*PInt) patternNode()        {}
func (*PFloat) patternNode()      {}
func (*PChar) patternNode()       {}
func (*PText) patternNode()       {}
func (*PCtor) patternNode()       {}
func (*PTwine) patternNode()      {}
func (*PThread) patternNode()     {}
func (*PBad) patternNode()        {}

// --------------------------------------------------------------------- types

// Reserved heads for type expressions that are not ordinary constructors.
const (
	// TwineTypeName heads a tuple type, `(a, b)`.
	TwineTypeName = "Twine"
	// FuncTypeName heads a function type, with Args holding {from, to}. It is
	// a node rather than a field so that a parenthesised function type keeps
	// its own arrow: `(a -> b) -> c` must not collapse into `a -> c`.
	FuncTypeName = "->"
)

// TypeExpr is a written type annotation, as in `size :: Thread a -> Earth`.
type TypeExpr struct {
	// Name is the head of the type: `Earth`, `Thread`, a lower-case variable
	// such as `a`, or one of the reserved heads above.
	Name string
	// Args are type arguments, e.g. `a` in `Thread a`.
	Args []*TypeExpr
	P    token.Pos
}

func (t *TypeExpr) Pos() token.Pos { return t.P }

// ---------------------------------------------------------------------- dump

// Dump renders a node as an indented s-expression. It is used by tests and by
// `weave parse`, and is not part of any stable interface.
func Dump(n Node) string {
	var sb strings.Builder
	dump(&sb, n, 0)
	return strings.TrimRight(sb.String(), "\n")
}

// DumpFile renders every declaration in f, followed by its output expression.
func DumpFile(f *File) string {
	var sb strings.Builder
	for _, t := range f.Types {
		dump(&sb, t, 0)
	}
	for _, d := range f.Decls {
		dump(&sb, d, 0)
	}
	if f.Output() != nil {
		sb.WriteString("(output\n")
		dump(&sb, f.Output(), 1)
		sb.WriteString(")\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func dump(sb *strings.Builder, n Node, depth int) {
	pad := strings.Repeat("  ", depth)
	line := func(format string, args ...any) {
		fmt.Fprintf(sb, "%s%s\n", pad, fmt.Sprintf(format, args...))
	}

	switch n := n.(type) {
	case *TypeDecl:
		head := n.Name
		for _, p := range n.Params {
			head += " " + p
		}
		line("(type %s", head)
		for _, ct := range n.Ctors {
			alt := ct.Name
			for _, f := range ct.Fields {
				s := typeString(f)
				if len(f.Args) > 0 && f.Name != TwineTypeName {
					s = "(" + s + ")"
				}
				alt += " " + s
			}
			line("  (ctor %s)", alt)
		}
		line(")")
	case *Decl:
		line("(decl %s", n.Name)
		if n.Memo {
			line("  (remember)")
		}
		if n.Sig != nil {
			line("  (sig %s)", typeString(n.Sig))
		}
		for _, c := range n.Clauses {
			dump(sb, c, depth+1)
		}
		line(")")
	case *Clause:
		if len(n.Params) == 0 {
			line("(clause")
		} else {
			line("(clause (params %s)", patternList(n.Params))
		}
		dump(sb, n.Body, depth+1)
		line(")")
	case *IntLit:
		line("%d", n.Value)
	case *FloatLit:
		line("%s", strconv.FormatFloat(n.Value, 'g', -1, 64))
	case *CharLit:
		line("'%c'", n.Value)
	case *TextLit:
		line("%q", n.Value)
	case *Var:
		line("%s", n.Name)
	case *Ctor:
		line("%s", n.Name)
	case *App:
		line("(app")
		dump(sb, n.Fn, depth+1)
		for _, a := range n.Args {
			dump(sb, a, depth+1)
		}
		line(")")
	case *Lambda:
		line("(lambda (%s)", patternList(n.Params))
		dump(sb, n.Body, depth+1)
		line(")")
	case *Let:
		line("(let")
		for _, b := range n.Binds {
			switch {
			case b.Pat != nil:
				line("  (weave-pattern %s", patternString(b.Pat))
			case len(b.Params) > 0:
				line("  (channel %s (%s)", b.Name, patternList(b.Params))
			default:
				line("  (weave %s", b.Name)
			}
			dump(sb, b.Value, depth+2)
			line("  )")
		}
		dump(sb, n.Body, depth+1)
		line(")")
	case *Ward:
		line("(ward")
		dump(sb, n.Subject, depth+1)
		for _, a := range n.Arms {
			line("  (arm %s", patternString(a.Pat))
			dump(sb, a.Body, depth+2)
			line("  )")
		}
		line(")")
	case *ThreadLit:
		line("(thread")
		for _, e := range n.Elems {
			dump(sb, e, depth+1)
		}
		line(")")
	case *TwineLit:
		line("(tuple")
		for _, e := range n.Elems {
			dump(sb, e, depth+1)
		}
		line(")")
	case *WebLit:
		line("(web")
		for _, p := range n.Pairs {
			line("  (pair")
			dump(sb, p.Key, depth+2)
			dump(sb, p.Val, depth+2)
			line("  )")
		}
		line(")")
	case *Bad:
		line("(bad)")
	default:
		line("(?%T)", n)
	}
}

func patternList(ps []Pattern) string {
	parts := make([]string, len(ps))
	for i, p := range ps {
		parts[i] = patternString(p)
	}
	return strings.Join(parts, " ")
}

func patternString(p Pattern) string {
	switch p := p.(type) {
	case *PWild:
		return "_"
	case *PVar:
		return p.Name
	case *PInt:
		return strconv.FormatInt(p.Value, 10)
	case *PFloat:
		return strconv.FormatFloat(p.Value, 'g', -1, 64)
	case *PChar:
		return fmt.Sprintf("'%c'", p.Value)
	case *PText:
		return strconv.Quote(p.Value)
	case *PCtor:
		if len(p.Args) == 0 {
			return p.Name
		}
		return "(" + p.Name + " " + patternList(p.Args) + ")"
	case *PTwine:
		parts := make([]string, len(p.Elems))
		for i, e := range p.Elems {
			parts[i] = patternString(e)
		}
		return "(, " + strings.Join(parts, " ") + ")"
	case *PThread:
		parts := make([]string, 0, len(p.Elems)+1)
		for _, e := range p.Elems {
			parts = append(parts, patternString(e))
		}
		if p.Rest != nil {
			parts = append(parts, ".."+patternString(p.Rest))
		}
		return "(thread " + strings.Join(parts, " ") + ")"
	case *PBad:
		return "(bad)"
	default:
		return fmt.Sprintf("(?%T)", p)
	}
}

func typeString(t *TypeExpr) string {
	if t == nil {
		return "?"
	}
	switch t.Name {
	case FuncTypeName:
		if len(t.Args) != 2 {
			return "?"
		}
		return "(" + typeString(t.Args[0]) + " -> " + typeString(t.Args[1]) + ")"
	case TwineTypeName:
		parts := make([]string, len(t.Args))
		for i, a := range t.Args {
			parts[i] = typeString(a)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	}
	s := t.Name
	for _, a := range t.Args {
		s += " " + typeString(a)
	}
	return s
}

// ------------------------------------------------------------- name analysis

// FreeVars collects into out every name e refers to without binding it itself.
// The compiler uses this twice: to order top-level definitions by dependency,
// and to decide what a lambda must capture.
func FreeVars(e Expr, bound map[string]bool, out map[string]bool) {
	switch e := e.(type) {
	case *Var:
		if !bound[e.Name] {
			out[e.Name] = true
		}

	case *App:
		FreeVars(e.Fn, bound, out)
		for _, a := range e.Args {
			FreeVars(a, bound, out)
		}

	case *Lambda:
		inner := copySet(bound)
		for _, p := range e.Params {
			BindPatternVars(p, inner)
		}
		FreeVars(e.Body, inner, out)

	case *Let:
		inner := copySet(bound)
		for _, b := range e.Binds {
			// A binding may refer to itself, so its own name and parameters are
			// in scope while its value is being read.
			valueScope := copySet(inner)
			valueScope[b.Name] = true
			for _, p := range b.Params {
				BindPatternVars(p, valueScope)
			}
			FreeVars(b.Value, valueScope, out)
			if b.Pat != nil {
				// A binding that takes its value apart cannot refer to itself,
				// and every name in the pattern is in scope from here on.
				BindPatternVars(b.Pat, inner)
				continue
			}
			inner[b.Name] = true
		}
		FreeVars(e.Body, inner, out)

	case *Ward:
		FreeVars(e.Subject, bound, out)
		for _, arm := range e.Arms {
			inner := copySet(bound)
			BindPatternVars(arm.Pat, inner)
			FreeVars(arm.Body, inner, out)
		}

	case *ThreadLit:
		for _, el := range e.Elems {
			FreeVars(el, bound, out)
		}

	case *TwineLit:
		for _, el := range e.Elems {
			FreeVars(el, bound, out)
		}

	case *WebLit:
		for _, p := range e.Pairs {
			FreeVars(p.Key, bound, out)
			FreeVars(p.Val, bound, out)
		}
	}
}

// ---------------------------------------------------------------- holes

// HasHole reports whether e contains a `_` that nothing has claimed yet.
//
// It stops at a Lambda: a group that already names its parameter has settled
// what its own `_` would have meant, so one written inside it belongs to
// nobody. FindHoles reports those.
func HasHole(e Expr) bool {
	return Holes(e).Any
}

// HoleUse summarises the unclaimed holes in an expression: what the group
// claiming them has to bind.
type HoleUse struct {
	Any bool
	// Args is how many arguments the group takes: 1 normally, 2 once a `that`
	// appears.
	Args int
	// Parts is how wide the first argument is taken apart: 0 to leave it whole,
	// and otherwise the width the words used name the components of.
	Parts int
	// Clashed says a second word asked for a different width, which is a group
	// that cannot mean anything — `add former aft` says two and three in the
	// same breath. ClashAt is the word that disagreed.
	Clashed bool
	ClashAt token.Pos
	// at is where the word that set Parts was.
	at token.Pos
}

func (u HoleUse) merge(v HoleUse) HoleUse {
	if v.Args > u.Args {
		u.Args = v.Args
	}
	u.Any = u.Any || v.Any
	if v.Clashed && !u.Clashed {
		u.Clashed, u.ClashAt = true, v.ClashAt
	}
	if v.Parts != 0 {
		if u.Parts == 0 {
			u.Parts, u.at = v.Parts, v.at
		} else if u.Parts != v.Parts && !u.Clashed {
			u.Clashed, u.ClashAt = true, v.at
		}
	}
	return u
}

// Holes reports what the unclaimed holes in e ask of whatever claims them.
//
// It stops at a Lambda: a group that already names its parameters has settled
// what its own holes would have meant, so one written inside it belongs to
// nobody. FindHoles reports those.
func Holes(e Expr) HoleUse {
	switch e := e.(type) {
	case *Hole:
		return HoleUse{Any: true, Args: e.Slot + 1, Parts: e.Parts, at: e.P}
	case *App:
		return holesIn(append([]Expr{e.Fn}, e.Args...))
	case *ThreadLit:
		return holesIn(e.Elems)
	case *TwineLit:
		return holesIn(e.Elems)
	case *WebLit:
		var es []Expr
		for _, p := range e.Pairs {
			es = append(es, p.Key, p.Val)
		}
		return holesIn(es)
	case *Let:
		es := make([]Expr, 0, len(e.Binds)+1)
		for _, b := range e.Binds {
			es = append(es, b.Value)
		}
		return holesIn(append(es, e.Body))
	case *Ward:
		es := []Expr{e.Subject}
		for _, arm := range e.Arms {
			es = append(es, arm.Body)
		}
		return holesIn(es)
	}
	return HoleUse{}
}

func holesIn(es []Expr) HoleUse {
	var u HoleUse
	for _, e := range es {
		u = u.merge(Holes(e))
	}
	return u
}

// FillHoles replaces every unclaimed `_` in e with a reference to HoleName,
// which whatever is claiming them binds. It walks exactly where HasHole does.
func FillHoles(e Expr) Expr {
	switch e := e.(type) {
	case *Hole:
		return &Var{Name: HoleVarName(e.Slot, e.Parts, e.At), P: e.P}
	case *App:
		e.Fn = FillHoles(e.Fn)
		fillAll(e.Args)
	case *ThreadLit:
		fillAll(e.Elems)
	case *TwineLit:
		fillAll(e.Elems)
	case *WebLit:
		for i := range e.Pairs {
			e.Pairs[i].Key = FillHoles(e.Pairs[i].Key)
			e.Pairs[i].Val = FillHoles(e.Pairs[i].Val)
		}
	case *Let:
		for _, b := range e.Binds {
			b.Value = FillHoles(b.Value)
		}
		e.Body = FillHoles(e.Body)
	case *Ward:
		e.Subject = FillHoles(e.Subject)
		for _, arm := range e.Arms {
			arm.Body = FillHoles(arm.Body)
		}
	}
	return e
}

func fillAll(es []Expr) {
	for i := range es {
		es[i] = FillHoles(es[i])
	}
}

// FindHoles collects every `_` still in the tree, which by then means every one
// with nothing to stand for. Unlike HasHole it does look inside lambdas.
func FindHoles(e Expr, out *[]*Hole) {
	switch e := e.(type) {
	case *Hole:
		*out = append(*out, e)
	case *App:
		FindHoles(e.Fn, out)
		for _, a := range e.Args {
			FindHoles(a, out)
		}
	case *Lambda:
		FindHoles(e.Body, out)
	case *ThreadLit:
		for _, el := range e.Elems {
			FindHoles(el, out)
		}
	case *TwineLit:
		for _, el := range e.Elems {
			FindHoles(el, out)
		}
	case *WebLit:
		for _, p := range e.Pairs {
			FindHoles(p.Key, out)
			FindHoles(p.Val, out)
		}
	case *Let:
		for _, b := range e.Binds {
			FindHoles(b.Value, out)
		}
		FindHoles(e.Body, out)
	case *Ward:
		FindHoles(e.Subject, out)
		for _, arm := range e.Arms {
			FindHoles(arm.Body, out)
		}
	}
}

// BindPatternVars adds every name a pattern binds to the set.
func BindPatternVars(p Pattern, into map[string]bool) {
	switch p := p.(type) {
	case *PVar:
		into[p.Name] = true
	case *PCtor:
		for _, a := range p.Args {
			BindPatternVars(a, into)
		}
	case *PTwine:
		for _, a := range p.Elems {
			BindPatternVars(a, into)
		}
	case *PThread:
		for _, a := range p.Elems {
			BindPatternVars(a, into)
		}
		if p.Rest != nil {
			BindPatternVars(p.Rest, into)
		}
	}
}

func copySet(s map[string]bool) map[string]bool {
	out := make(map[string]bool, len(s))
	for k := range s {
		out[k] = true
	}
	return out
}
