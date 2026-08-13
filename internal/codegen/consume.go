package codegen

import (
	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/types"
)

// Consumed parameters: handing a collection over rather than lending it.
//
// The analysis in inplace.go proves a collection single-threaded *within one
// function*. That stops at the call boundary, and the boundary is where most
// programs put the interesting half:
//
//	fill board ks is ks | braid (b i : mend i 1 b) board
//	search board pos is search (fill board (options pos)) (add pos 1)
//
// `fill`'s own fold writes through — the accumulator is single-threaded inside
// `fill` — but the Thread it hands back escapes to a caller who may keep it, so
// it is disowned on the way out and `search`'s next call copies the whole board.
// One copy per turn is exactly what the pass exists to remove.
//
// A parameter is *consumed* when the caller can give up its reference outright:
// the parameter is a collection, the function's result is that same collection,
// and inside the body the parameter is single-threaded and flows to the result.
// Then "lend me the board and I will hand it back" becomes "take the board",
// and the ownership the caller had passes through the call untouched.
//
// Two entry points carry that:
//
//   - `wu_f_move` holds the body. Its consumed parameters are marked owned, so
//     updates to them write through, and the result leaves *still owned*.
//   - `wu_f` is the ordinary one every other call site uses:
//     `w_disown(wu_f_move(...))`. A closure over `f`, a call from a context the
//     analysis has not cleared, a call from the top level — all of those keep
//     exactly the behaviour they had.
//
// Only a call site that has itself been proved single-threaded uses `_move`,
// and that is what makes the escape safe: the caller has promised the result
// goes straight into a slot nothing else can see.
//
// The dynamic bit still backs all of it. `_move` on a collection that arrived
// shared copies at the first update and hands back the copy, so calling it is
// never *wrong* — the static proof only decides where it is worth doing.

// computeConsumed fills g.consumed, mapping each top-level function to which of
// its parameters may be handed over.
//
// Recursion makes this a fixpoint, and it has to be the optimistic one. A loop
// written as a recursive function reaches its own consumed slot —
//
//	wipe [] p is p
//	wipe [k ..rest] p is wipe rest (set p k '.')
//
// — so asking "does `wipe` consume p?" needs the answer to itself. Starting
// every candidate at yes and striking out the ones that fail lands on the
// largest consistent set; starting at no would strike out every recursive
// function on the first pass and never put one back.
func (g *gen) computeConsumed(f *ast.File, skip map[string]bool) {
	if g.opts.DisableInPlace {
		return
	}
	kinds := map[string][]string{}
	for _, d := range f.Decls {
		// A `remember`ed function's table keeps its arguments for the rest of
		// the program, so nothing handed to one is ever single-threaded again.
		if d.Arity() == 0 || d.Memo || skip[d.Name] {
			continue
		}
		res := g.resultCon(d)
		if res == "" {
			continue
		}
		slots, any := make([]bool, d.Arity()), false
		for i, k := range g.ownableParams(d) {
			if k == res && namedInEveryClause(d, i) {
				slots[i], any = true, true
			}
		}
		if any {
			g.consumed[d.Name], kinds[d.Name] = slots, g.ownableParams(d)
		}
	}

	for changed := true; changed; {
		changed = false
		for name, slots := range g.consumed {
			for i, ok := range slots {
				if ok && !g.paramIsConsumed(g.byName[name], i, kinds[name][i]) {
					slots[i], changed = false, true
				}
			}
		}
	}

	for name, slots := range g.consumed {
		if !anyOf(slots) {
			delete(g.consumed, name)
		}
	}
}

func anyOf(bs []bool) bool {
	for _, b := range bs {
		if b {
			return true
		}
	}
	return false
}

// namedInEveryClause reports whether parameter i is bound to a plain name, or
// ignored, in every clause. A destructuring pattern is refused: `f [x ..rest]`
// binds `rest` to a window on the argument's own array, and handing that back
// would be a second way to see storage the caller has given up.
func namedInEveryClause(d *ast.Decl, i int) bool {
	for _, cl := range d.Clauses {
		if i >= len(cl.Params) {
			return false
		}
		switch cl.Params[i].(type) {
		case *ast.PVar, *ast.PWild:
		default:
			return false
		}
	}
	return true
}

// resultCon names the collection a definition hands back, or "" when its result
// is not one that can be updated in place. A consumed parameter has to leave by
// the same door it came in.
func (g *gen) resultCon(d *ast.Decl) string {
	sch, ok := g.info.Decls[d.Name]
	if !ok {
		return ""
	}
	t := sch.Body
	for i := 0; i < d.Arity(); i++ {
		fn, isFn := types.Resolve(t).(*types.Fn)
		if !isFn {
			return ""
		}
		t = fn.To
	}
	con, isCon := types.Resolve(t).(*types.Con)
	if !isCon {
		return ""
	}
	if _, isOwnable := ownables[con.Name]; !isOwnable {
		return ""
	}
	return con.Name
}

// paramIsConsumed runs the single-threading proof over a parameter, with the
// function's result standing in for the tail call's slot.
//
// It is the fold's proof, not the loop's: a fold's accumulator is single
// threaded when the step's *result* is the update, and a consumed parameter is
// single-threaded when the function's result is. The visitor's `fold` mode says
// exactly that, and it already accepts an update, a chain of updates, a fold
// seeded with the collection, and — through isMoveOf — a call that consumes it,
// which is what carries the recursive case.
func (g *gen) paramIsConsumed(d *ast.Decl, i int, con string) bool {
	kind := ownables[con]
	updated := false
	for _, cl := range d.Clauses {
		p, isVar := cl.Params[i].(*ast.PVar)
		if !isVar {
			continue // `_`: the clause cannot reach it at all
		}
		v := &visitor{g: g, con: con, name: p.Name, kind: kind, fold: true, twineAt: -1}
		if !v.walk(cl.Body, true) {
			return false
		}
		updated = updated || v.updated
	}
	// Worth nothing unless some clause actually writes: a function that only
	// reads its argument gains nothing from being handed it, and marking it
	// consumed would emit a second entry point for no reason.
	return updated
}

// isMoveOf reports whether e is a call that consumes the traced collection and
// hands back a collection of the same type — the call-boundary counterpart of
// `set g k v`.
func (v *visitor) isMoveOf(e ast.Expr) bool {
	if v.g == nil {
		return false
	}
	app, isApp := e.(*ast.App)
	if !isApp {
		return false
	}
	callee, isVar := app.Fn.(*ast.Var)
	if !isVar {
		return false
	}
	slots, consumes := v.g.consumed[callee.Name]
	if !consumes || len(app.Args) != len(slots) {
		return false
	}
	if v.g.resultCon(v.g.byName[callee.Name]) != v.con {
		return false
	}
	at := -1
	for i, takes := range slots {
		if !takes || !v.reaches(app.Args[i]) {
			continue
		}
		if at >= 0 {
			return false // handed to two slots at once
		}
		at = i
	}
	// Every other argument is evaluated before the call, so it may read the
	// collection but not keep it.
	return at >= 0 && v.allSafeExcept(app.Args, at)
}

// reaches reports whether the traced collection arrives at this argument: as
// itself, or through updates that hand it on.
func (v *visitor) reaches(e ast.Expr) bool {
	if held, isVar := e.(*ast.Var); isVar {
		return held.Name == v.name
	}
	return v.isUpdateOf(e)
}

// movesOwned reports whether e is a call to a consuming function that is being
// handed a collection this function owns, so its result comes back owned too.
//
// Naming the same variable in two argument slots is refused rather than
// trusted. The proof that cleared this site never allows it, so a second
// mention means something has been read wrong, and the copying call is the
// answer that is always right.
func (g *gen) movesOwned(e ast.Expr, sc *scope) (*ast.App, bool) {
	if g.opts.DisableInPlace {
		return nil, false
	}
	app, isApp := e.(*ast.App)
	if !isApp {
		return nil, false
	}
	callee, isVar := app.Fn.(*ast.Var)
	if !isVar {
		return nil, false
	}
	if _, shadowed := sc.lookup(callee.Name); shadowed {
		return nil, false
	}
	slots, consumes := g.consumed[callee.Name]
	if !consumes || len(app.Args) != len(slots) {
		return nil, false
	}
	at, seen := -1, map[string]bool{}
	for i, arg := range app.Args {
		if name, isVar := arg.(*ast.Var); isVar {
			if cname, bound := sc.lookup(name.Name); bound {
				if seen[cname] {
					return nil, false
				}
				seen[cname] = true
			}
		}
		if slots[i] && g.yieldsOwned(arg, sc) {
			if at >= 0 {
				return nil, false
			}
			at = i
		}
	}
	if at < 0 {
		return nil, false
	}
	return app, true
}

// movedCall compiles a call to a consuming function as the `_move` entry point,
// when the collection being handed over is one the generator is holding owned.
func (g *gen) movedCall(b *body, e ast.Expr, sc *scope) (string, bool) {
	app, ok := g.movesOwned(e, sc)
	if !ok {
		return "", false
	}
	callee := app.Fn.(*ast.Var)
	arr := g.arrayOf(b, g.args(b, app.Args, sc))
	return g.cnames[callee.Name] + "_move(NULL, " + arr + ")", true
}
