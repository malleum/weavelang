package codegen

import (
	"fmt"

	"github.com/malleum/weave/internal/ast"
)

// Tail calls.
//
// Weave has no loops, so every repetition is recursion, and SPEC.md promises
// that a tail call costs what a loop costs. Nothing else provides that: clang
// does not reliably turn these into jumps even at -O3, and at -O0 it certainly
// does not, so a five-million-step count would exhaust the C stack.
//
// A definition that calls itself in tail position is compiled as
//
//	static Value f(Value *env, Value *args) {
//	  Value p0 = args[0], p1 = args[1];
//	  for (;;) {
//	    ...
//	    Value t0 = ..., t1 = ...;   // the new arguments, computed first
//	    p0 = t0; p1 = t1;
//	    continue;                   // instead of calling f again
//	  }
//	}
//
// The arguments are evaluated into temporaries before any parameter is
// reassigned, since they usually read the parameters they are about to
// replace: `count (sub n 1) (add acc n)` must see the old `n` in both.
//
// Mutual tail recursion gets the same treatment. A set of definitions that
// tail-call each other is one strongly connected component of the tail-call
// graph, and is compiled into a single C function whose loop switches on which
// member is running:
//
//	static Value loop(int which, Value *args) {
//	  Value s[2] = {args[0], args[1]};
//	  for (;;) switch (which) {
//	    case 0: ... which = 1; s[0] = ...; continue;   // f tail-calls g
//	    case 1: ... which = 0; s[0] = ...; continue;   // g tail-calls f
//	  }
//	}
//
// Each member keeps its own entry point, which calls the loop with its own
// index, so an ordinary call is unchanged. The alternative — a trampoline —
// would cost every call an allocation whether or not it was ever needed.

// tailInfo describes the function currently being compiled, so a tail call
// within its group can jump instead.
type tailInfo struct {
	name   string   // the Weave name of the enclosing definition
	params []string // the C variables holding its parameters
	// owned marks parameters the loop threads without ever duplicating, so an
	// update to one may write through. See inplace.go.
	owned []bool
	// ownedVars holds the C variables of those parameters, so that returning
	// one can be recognised however the clause happens to name it.
	ownedVars map[string]bool

	// The three fields below are set only for a merged group of mutually
	// tail-recursive definitions. `which` names the C variable selecting the
	// member, `member` maps a name to its index, and `slots` are the parameter
	// slots every member shares.
	which  string
	member map[string]int
	arity  map[string]int
	slots  []string
}

// jumpTarget reports the group member a tail call names, if any.
func (ti *tailInfo) jumpTarget(name string, argc int) (int, bool) {
	if ti.member == nil {
		if name == ti.name && argc == len(ti.params) {
			return 0, true
		}
		return 0, false
	}
	idx, ok := ti.member[name]
	return idx, ok && argc == ti.arity[name]
}

// exprTail emits e, which sits in tail position. The second result reports
// that control jumped, so the caller must not also emit a return.
func (g *gen) exprTail(b *body, e ast.Expr, sc *scope, ti *tailInfo) (string, bool) {
	if ti == nil {
		return g.expr(b, e, sc), false
	}

	switch e := e.(type) {
	case *ast.Var:
		// A grid the loop owns is escaping to the caller, who may keep it, so
		// it becomes shared again on the way out.
		if c, found := sc.lookup(e.Name); found && ti.ownedVars[c] {
			return fmt.Sprintf("w_disown(%s)", c), false
		}

	case *ast.Ward:
		return g.wardWith(b, e, sc, ti)

	case *ast.Let:
		inner := newScope(sc)
		for _, bind := range e.Binds {
			g.bind(b, bind, inner)
		}
		return g.exprTail(b, e.Body, inner, ti)

	case *ast.App:
		// A collection the loop owns escaping inside a constructor — `Held xs`
		// — is escaping exactly as much as a bare mention is, and the caller
		// may keep what it gets. The analysis allows it on that understanding;
		// this is the other half of the bargain.
		if c, isCtor := e.Fn.(*ast.Ctor); isCtor && c.Name != "Source" {
			args := g.args(b, e.Args, sc)
			for i, a := range e.Args {
				if x, isVar := a.(*ast.Var); isVar {
					if cname, found := sc.lookup(x.Name); found && ti.ownedVars[cname] {
						args[i] = fmt.Sprintf("w_disown(%s)", args[i])
					}
				}
			}
			return g.ctorValue(b, c.Name, args, c.P, sc), false
		}
		if v, ok := e.Fn.(*ast.Var); ok {
			if _, shadowed := sc.lookup(v.Name); !shadowed {
				if idx, ok := ti.jumpTarget(v.Name, len(e.Args)); ok {
					g.emitTailJump(b, e, sc, ti, idx)
					return "", true
				}
				// `pick` is the one builtin whose arguments are themselves in
				// tail position, since only the taken branch runs.
				if v.Name == "pick" && len(e.Args) == 3 {
					return g.pickTail(b, e, sc, ti)
				}
			}
		}
	}

	return g.expr(b, e, sc), false
}

// emitTailJump rebinds the parameters and loops, selecting the member being
// jumped to when this is a merged group.
func (g *gen) emitTailJump(b *body, e *ast.App, sc *scope, ti *tailInfo, member int) {
	// A parameter this loop owns is single-threaded across the jump, and the
	// arguments are where that is true of it: whatever they build from it goes
	// into the slot it came from and nothing else can reach the old value. That
	// makes a helper it is handed over to, or a chain of updates, as writable
	// here as a bare `set` — see yieldsOwned.
	for v := range ti.ownedVars {
		if g.owned[v] {
			continue
		}
		g.owned[v] = true
		defer delete(g.owned, v)
	}

	next := make([]string, len(e.Args))
	for i, arg := range e.Args {
		// A grid this loop owns is updated in place rather than copied.
		if update, ok := g.inPlaceUpdate(b, arg, i, sc, ti); ok {
			next[i] = g.hoist(b, update)
			continue
		}
		next[i] = g.hoist(b, g.expr(b, arg, sc))
	}

	// The arguments are all evaluated before any slot is written, since they
	// usually read the parameters they are about to replace.
	dest := ti.params
	if ti.which != "" {
		dest = ti.slots
	}
	for i := range next {
		b.line("%s = %s;", dest[i], next[i])
	}
	if ti.which != "" {
		b.line("%s = %d;", ti.which, member)
	}
	b.line("continue;")
}

// pickTail compiles a conditional whose branches are in tail position.
func (g *gen) pickTail(b *body, e *ast.App, sc *scope, ti *tailInfo) (string, bool) {
	cond := g.expr(b, e.Args[0], sc)
	res := b.tmp()

	b.open("if ((%s).spirit) {", cond)
	whenLight, lightJumped := g.exprTail(b, e.Args[1], sc, ti)
	if !lightJumped {
		b.line("%s = %s;", res, whenLight)
	}
	b.close("} else {")
	b.indent++
	whenShadow, shadowJumped := g.exprTail(b, e.Args[2], sc, ti)
	if !shadowJumped {
		b.line("%s = %s;", res, whenShadow)
	}
	b.close("}")

	return res, lightJumped && shadowJumped
}

// tailCallees collects, for one definition, every name it calls in tail
// position along with how many arguments it passed. This is the edge set of
// the tail-call graph.
func tailCallees(d *ast.Decl) map[string]int {
	out := map[string]int{}
	for _, cl := range d.Clauses {
		bound := map[string]bool{}
		for _, p := range cl.Params {
			ast.BindPatternVars(p, bound)
		}
		collectTailCalls(cl.Body, bound, out)
	}
	return out
}

// collectTailCalls walks the tail positions of e. It mirrors exprTail exactly:
// anywhere that pass would jump, this one records the call.
func collectTailCalls(e ast.Expr, shadowed map[string]bool, out map[string]int) {
	switch e := e.(type) {
	case *ast.Ward:
		for _, arm := range e.Arms {
			inner := copyNames(shadowed)
			ast.BindPatternVars(arm.Pat, inner)
			collectTailCalls(arm.Body, inner, out)
		}

	case *ast.Let:
		inner := copyNames(shadowed)
		for _, b := range e.Binds {
			inner[b.Name] = true
		}
		collectTailCalls(e.Body, inner, out)

	case *ast.App:
		v, ok := e.Fn.(*ast.Var)
		if !ok || shadowed[v.Name] {
			return
		}
		if v.Name == "pick" && len(e.Args) == 3 {
			// The one builtin whose arguments are themselves in tail position,
			// since only the taken branch runs.
			collectTailCalls(e.Args[1], shadowed, out)
			collectTailCalls(e.Args[2], shadowed, out)
			return
		}
		out[v.Name] = len(e.Args)
	}
}

func copyNames(s map[string]bool) map[string]bool {
	out := make(map[string]bool, len(s))
	for k := range s {
		out[k] = true
	}
	return out
}
