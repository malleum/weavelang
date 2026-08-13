package codegen

import (
	"github.com/malleum/weave/internal/ast"
)

// A `gentle` loop builds a Weaving on every turn and takes it apart on the same
// turn, and nothing else ever sees it.
//
//	res = <the step>;                   // w_data("Woven", 0, {acc}, 1)
//	if (w_data_index(res) != 0) break;  // reads the tag straight back
//	res_acc = w_data_field(res, 0);     // and the field straight back
//
// That is two allocations a turn — the WData and its field array — for an
// object whose whole life is three lines. On the trampoline walk of Advent of
// Code 2017 day 5 it was 1.3 GB of the program's 2.9, across 56 million
// objects, and every one of them was dead before the next turn began.
//
// So the step is compiled in *statement* position instead: `Woven x` assigns
// the accumulator and clears a flag, `Gentled y` assigns it and sets the flag,
// and the Weaving is built once when the loop ends, because the value `gentle`
// answers with is a Weaving and something downstream still wants one.
//
// Not every step can be written that way, so this is all-or-nothing: the shape
// is checked before a line is emitted, and a step that does not fit is compiled
// exactly as it was.

// splittableWeaving reports whether e ends in Weaving constructors that can be
// assigned rather than built.
//
// The shapes are the ones a step is actually written in: a constructor outright,
// a `pick` between two of them, and a `weave` in front of either. A `ward` is
// not among them yet — it decides its arms through machinery that hands back an
// expression, and reaching into that is a bigger change than this one.
func (g *gen) splittableWeaving(e ast.Expr, sc *scope) bool {
	switch n := e.(type) {
	case *ast.App:
		if c, ok := n.Fn.(*ast.Ctor); ok && len(n.Args) == 1 {
			_, isWeaving := weavingIndex[c.Name]
			return isWeaving
		}
		if v, ok := n.Fn.(*ast.Var); ok && v.Name == "pick" && len(n.Args) == 3 {
			if _, shadowed := sc.lookup(v.Name); shadowed {
				return false
			}
			return g.splittableWeaving(n.Args[1], sc) && g.splittableWeaving(n.Args[2], sc)
		}
		return false

	case *ast.Let:
		// The bindings are ordinary expressions wherever they are; only the
		// body is in tail position.
		return g.splittableWeaving(n.Body, sc)
	}
	return false
}

// emitWeaving compiles a step that splittableWeaving has passed, assigning the
// accumulator and whether the loop is finished.
//
// The accumulator and the answer are separate variables, because they are
// separate things: everything after the loop that reads the accumulator — the
// disown that ends its ownership, above all — expects the last thing a `Woven`
// handed on, and a `Gentled` answering with an Earth must not overwrite it.
func (g *gen) emitWeaving(b *body, e ast.Expr, sc *scope, w weavingSplit) {
	switch n := e.(type) {
	case *ast.App:
		if c, ok := n.Fn.(*ast.Ctor); ok && len(n.Args) == 1 {
			done := weavingIndex[c.Name] != 0
			if !done && w.parts != nil {
				// The accumulator is carried a component at a time, so what a
				// `Woven` was given is written straight into them.
				//
				// Every component is worked out before any is assigned: the
				// second half of a state Twine routinely reads the first, and
				// writing the first would be reading the next turn's.
				lit := n.Args[0].(*ast.TwineLit)
				next := make([]string, len(lit.Elems))
				for i, el := range lit.Elems {
					v := g.expr(b, el, sc)
					next[i] = b.tmp()
					b.line("%s = %s;", next[i], v)
				}
				for i, name := range w.parts {
					b.line("%s = %s;", name, next[i])
				}
				b.line("%s = false;", w.done)
				return
			}
			v := g.expr(b, n.Args[0], sc)
			if done {
				b.line("%s = %s;", w.out, v)
			} else {
				b.line("%s = %s;", w.acc, v)
			}
			b.line("%s = %s;", w.done, boolText(done))
			return
		}
		// `pick`, which must not evaluate the branch it does not take — the
		// same reason it is not an ordinary call anywhere else.
		cond := g.expr(b, n.Args[0], sc)
		b.open("if ((%s).spirit) {", cond)
		g.emitWeaving(b, n.Args[1], sc, w)
		b.close("} else {")
		b.indent++
		g.emitWeaving(b, n.Args[2], sc, w)
		b.close("}")

	case *ast.Let:
		inner := newScope(sc)
		for _, bind := range n.Binds {
			g.bind(b, bind, inner)
		}
		g.emitWeaving(b, n.Body, inner, w)
	}
}

func boolText(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// weavingSplit is the state a fused `gentle` sets while its step is compiled,
// naming the two variables the step assigns instead of the Weaving it would
// otherwise build. It is empty everywhere else, which is what keeps every other
// call site compiling exactly as it did.
type weavingSplit struct {
	acc  string // what a `Woven` hands to the next turn
	out  string // what a `Gentled` finished with
	done string // which of the two the last step was
	// parts holds the accumulator one component to a variable, when it is a
	// Twine of state the step takes apart on the way in and builds again on
	// the way out. Then it never exists either: the loop carries the
	// components, and the Twine is put together once at the end.
	parts []string
}

// splitStep compiles the body of an inlined step in statement position when the
// enclosing loop asked for it, and reports whether it did. A false answer means
// nothing has been emitted and the caller should compile the body as an
// ordinary expression.
// splitAcc reports how many components the accumulator can be carried in,
// which needs the step to take it apart on the way in and to build it again out
// of a literal on the way out. Zero means it stays whole.
//
// It is the other half of the same allocation: the trampoline walk built a
// Weaving *and* a two-Twine on every turn, took both apart on the same turn,
// and neither was ever seen again.
func (g *gen) splitAcc(lam *ast.Lambda, sc *scope) int {
	if len(lam.Params) == 0 {
		return 0
	}
	tw, ok := lam.Params[0].(*ast.PTwine)
	if !ok || len(tw.Elems) < 2 {
		return 0
	}
	if !g.wovenTwines(lam.Body, sc, len(tw.Elems)) {
		return 0
	}
	return len(tw.Elems)
}

// wovenTwines reports whether every `Woven` in tail position hands on a Twine
// written out on the spot, of exactly the width the step took apart. Anything
// else — a name, a call answering a Twine — is a Twine the loop would have to
// build anyway.
func (g *gen) wovenTwines(e ast.Expr, sc *scope, n int) bool {
	switch t := e.(type) {
	case *ast.App:
		if c, ok := t.Fn.(*ast.Ctor); ok && len(t.Args) == 1 {
			if c.Name != "Woven" {
				return true // a `Gentled` hands on nothing
			}
			lit, isLit := t.Args[0].(*ast.TwineLit)
			return isLit && len(lit.Elems) == n
		}
		if v, ok := t.Fn.(*ast.Var); ok && v.Name == "pick" && len(t.Args) == 3 {
			return g.wovenTwines(t.Args[1], sc, n) && g.wovenTwines(t.Args[2], sc, n)
		}
	case *ast.Let:
		return g.wovenTwines(t.Body, sc, n)
	}
	return false
}

func (g *gen) splitStep(b *body, body ast.Expr, sc *scope) bool {
	if g.weaving.acc == "" {
		return false
	}
	// The step's own body may hold another fused chain, and that chain's step
	// is not this one's. The naming is put back when this body is done.
	split := g.weaving
	g.weaving = weavingSplit{}
	defer func() { g.weaving = split }()

	if !g.splittableWeaving(body, sc) {
		return false
	}
	g.emitWeaving(b, body, sc, split)
	return true
}
