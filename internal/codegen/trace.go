package codegen

import (
	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/token"
)

// A pipeline written one stage to a line is read one stage at a time, so
// `weave trace` reports it that way: every line that ends a stage gets the
// value the chain holds at that point, not just the line the definition
// started on.
//
// Doing that means compiling the chain stage by stage instead of as one
// expression, which costs the fusion that would normally collapse it into a
// single loop. Trace mode is for editing, not for timing, so that is the
// right trade — but it is why staged compilation lives behind Options.Trace.

// pipeName is the scope entry that carries a hoisted pipeline prefix. It is
// spelled with a byte no source name can contain, so it cannot shadow one.
const pipeName = "\x00stage"

// link is one stage of a pipeline: the node that consumes the value flowing
// into it, and the slot inside that node where the value sits.
type link struct {
	node ast.Expr  // the App or Let the pipe built
	slot *ast.Expr // the incoming value's place in it
	line int       // the source line the stage was written on
}

// pipeStages peels a pipeline apart from the outside in, returning the head
// the chain starts from and the stages laid over it, outermost first.
//
// Every particle produces the same shape: `feed` appends the incoming value as
// the last argument, `where` builds `sift p value`, and a stage containing `_`
// becomes a Let binding the value. All three record the particle in Via, which
// is what separates a pipeline stage from an ordinary application.
func pipeStages(e ast.Expr) (ast.Expr, []link) {
	var stages []link
	for {
		switch n := e.(type) {
		case *ast.App:
			if n.Via == "" || len(n.Args) == 0 {
				return e, stages
			}
			slot := &n.Args[len(n.Args)-1]
			stages = append(stages, link{n, slot, n.P.Line})
			e = *slot
		case *ast.Let:
			if n.Via == "" || len(n.Binds) != 1 {
				return e, stages
			}
			slot := &n.Binds[0].Value
			stages = append(stages, link{n, slot, n.P.Line})
			e = *slot
		default:
			return e, stages
		}
	}
}

// traced compiles e and reports what it holds at the end of every source line
// it spans, returning the C expression for the finished value. The name is
// attached to the last report, since that is the one the definition means.
func (g *gen) traced(b *body, e ast.Expr, sc *scope, name string, nameLine int) string {
	head, stages := pipeStages(e)

	if g.endlessChain(head, stages, sc) {
		// An endless producer is never built: the loop that consumes it holds
		// one element at a time, and only something downstream ends it. So the
		// chain cannot be taken apart — a prefix of it would run for ever, and
		// the compiler is right to refuse. It is compiled and reported whole.
		out := g.expr(b, e, sc)
		tmp := b.tmp()
		line := nameLine
		if len(stages) > 0 && stages[0].line > line {
			line = stages[0].line
		}
		b.line("%s = %s;", tmp, out)
		b.line("w_trace(%d, \"%s\", %s);", line, escape(name), tmp)
		return tmp
	}

	// Innermost first, so the values come out in the order they are computed.
	lines := []int{head.Pos().Line}
	for i := len(stages) - 1; i >= 0; i-- {
		lines = append(lines, stages[i].line)
	}
	single := true
	for _, l := range lines {
		if l != lines[0] {
			single = false
		}
	}

	cur := g.expr(b, head, sc)
	// Each step is held in a local: the stage after it may read it more than
	// once, and the C expression behind it can be a call.
	report := func(step int, value string) string {
		tmp := b.tmp()
		b.line("%s = %s;", tmp, value)
		if step+1 < len(lines) && lines[step+1] == lines[step] {
			// More of this line to come; report it when it ends.
			return tmp
		}
		label, line := "", lines[step]
		if step == len(lines)-1 {
			label = name
			if single {
				line = nameLine
			}
		}
		b.line("w_trace(%d, \"%s\", %s);", line, escape(label), tmp)
		return tmp
	}
	cur = report(0, cur)

	for i, step := len(stages)-1, 1; i >= 0; i, step = i-1, step+1 {
		st := stages[i]
		// Standing the prefix in as a variable keeps the stage's own node — and
		// so its inferred type, and any specialisation that depends on it —
		// exactly as the checker left it.
		held := *st.slot
		v := &ast.Var{Name: pipeName, P: token.Pos{Line: st.line}}
		if t, ok := g.info.Types[held]; ok {
			g.info.Types[v] = t
		}
		*st.slot = v

		inner := newScope(sc)
		inner.bind(pipeName, cur)
		cur = g.expr(b, st.node, inner)

		*st.slot = held
		cur = report(step, cur)
	}
	return cur
}

// endlessChain reports whether any part of a chain is an endless producer.
//
// `flow` and `cycle` exist only as fused loops — there is no value for one to
// have — so a chain holding either is compiled as a whole or not at all.
func (g *gen) endlessChain(head ast.Expr, stages []link, sc *scope) bool {
	if isEndless(g, head, sc) {
		return true
	}
	for _, st := range stages {
		if isEndless(g, st.node, sc) {
			return true
		}
	}
	return false
}

func isEndless(g *gen, e ast.Expr, sc *scope) bool {
	app, ok := e.(*ast.App)
	if !ok {
		return false
	}
	name, ok := g.verbCallee(app, sc)
	return ok && (name == "flow" || name == "cycle")
}
