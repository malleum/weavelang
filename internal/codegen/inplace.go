package codegen

import (
	"strings"

	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/types"
)

// In-place collection updates (SPEC.md section 13).
//
// `set g k v` returns a new grid, and the runtime honours that by copying the
// whole cell array. In a loop that is quadratic: twenty thousand single-cell
// updates to a 200x200 grid copy eight hundred million cells. `put w k v` and
// `insert c x` have the same shape: they path-copy the trie, and a map built
// over a million steps keeps a million paths, since the arena never frees.
//
// The copy is only needed when someone else can still see the old collection.
// This pass finds the case where nobody can — a grid, Web or Circle threaded
// through a tail-recursive loop, updated on each turn — and lets the update
// write through instead.
//
// Proving it takes two halves, because neither alone is enough:
//
//   - Statically, that the loop never duplicates the grid. Every mention of the
//     parameter must be either the grid being updated in the tail call, or an
//     argument to a verb that reads without keeping a reference. If the grid is
//     bound to another name, stored in a structure, captured by a lambda, or
//     passed anywhere else, the analysis gives up.
//
//   - Dynamically, that the grid did not arrive already shared. The first call
//     usually passes a grid the caller still holds — `step sheet 2000` hands
//     over the memoised `sheet` — so the runtime marks a grid shared when it is
//     built and owned only when it is the copy that `set` itself just made. The
//     first update in the loop copies; every later one writes through.
//
// The result is that a loop of updates costs one copy rather than one per turn,
// without the compiler having to reason about anything outside the function.

// ownable describes a collection type that can be updated in place: the verb
// that updates it, and the verbs that read it without keeping a reference.
//
// A verb that hands back something sharing the collection's storage is
// deliberately absent — `cells`, `keys`, `vals`, `items`, `members` — since the
// Thread it returns outlives the call.
type ownable struct {
	// update is the verb whose result may write through, and owned is the C
	// function that does it.
	update, owned string
	// arity is how many arguments the update verb takes.
	arity int
	// dataLast marks a collection whose verbs take it as their *last*
	// argument rather than their first. That is the sequence convention, and
	// what lets `mend i x xs` sit in a pipeline: `set g k v` names the grid
	// first, `mend` names the Thread last.
	dataLast bool
	// also are further verbs that update the collection the same way, each
	// with its own owned form. `twist` is `mend` with the new value worked
	// out from the old one, and is as safe to write through.
	also []ownableVerb
	// readOnly are the verbs that may name the collection without sharing it.
	readOnly map[string]bool
}

// ownableVerb is one more way to update a collection: the verb, the C function
// that writes through, and how many arguments it takes.
type ownableVerb struct {
	verb, owned string
	arity       int
}

// updates lists every verb that may write through this collection, the main
// one first.
func (o ownable) updates() []ownableVerb {
	out := []ownableVerb{{o.update, o.owned, o.arity}}
	return append(out, o.also...)
}

// at reports which argument of an n-argument call holds the collection.
func (o ownable) at(n int) int {
	if o.dataLast {
		return n - 1
	}
	return 0
}

var ownables = map[string]ownable{
	types.PatternCon: {
		update: "set", owned: "wp_set_owned", arity: 3,
		readOnly: map[string]bool{
			"cell": true, "rows": true, "cols": true, "shape": true,
			"knots": true, "nb4": true, "nb8": true,
			"around4": true, "around8": true, "inb": true,
		},
	},
	types.WebCon: {
		update: "put", owned: "wp_put_owned", arity: 3,
		also: []ownableVerb{{"forget", "wp_forget_owned", 2}},
		readOnly: map[string]bool{
			"get": true, "known": true, "len": true,
		},
	},
	types.CircleCon: {
		update: "insert", owned: "wp_insert_owned", arity: 2,
		also: []ownableVerb{{"remove", "wp_remove_owned", 2}},
		readOnly: map[string]bool{
			"member": true, "len": true,
		},
	},
	// A Thread's buffer is one block, like a Pattern's, so it takes the same
	// one bit and the same proof. What is different is that a Thread's storage
	// can be shared without a second buffer: `take`, `drop`, `sever`,
	// `strands`, `tail` and the Thread patterns all hand back a window on it,
	// so none of them appears here. Every verb that does returns an element or
	// a number, never a view.
	types.ThreadCon: {
		update: "mend", owned: "wp_mend_owned", arity: 3, dataLast: true,
		also: []ownableVerb{{"twist", "wp_twist_owned", 3}},
		readOnly: map[string]bool{
			"len": true, "nth": true, "has": true, "idx": true,
			"first": true, "second": true, "last": true, "head": true,
			"seek": true, "dupe": true, "maxby": true, "minby": true,
			"sum": true, "prod": true, "count": true, "all": true, "any": true,
			"freq": true,
		},
	},
}

// ownedParams reports, for each parameter of a self-tail-recursive definition,
// whether an update to it in the tail call may write through.
func (g *gen) ownedParams(d *ast.Decl) []bool {
	kinds := g.ownableParams(d)
	out := make([]bool, d.Arity())
	for i, kind := range kinds {
		out[i] = kind != "" && paramIsSingleThreaded(d, i, ownables[kind])
	}
	return out
}

// ownableParams names, for each parameter, the collection type it holds, or the
// empty string when it is not one that can be updated in place. w_disown must
// never be applied to anything else.
func (g *gen) ownableParams(d *ast.Decl) []string {
	out := make([]string, d.Arity())
	sch, ok := g.info.Decls[d.Name]
	if !ok {
		return out
	}
	t := sch.Body
	for i := 0; i < d.Arity(); i++ {
		fn, ok := types.Resolve(t).(*types.Fn)
		if !ok {
			return out
		}
		if con, ok := types.Resolve(fn.From).(*types.Con); ok {
			if _, isOwnable := ownables[con.Name]; isOwnable {
				out[i] = con.Name
			}
		}
		t = fn.To
	}
	return out
}

// paramIsSingleThreaded checks every clause for a use of parameter i that could
// leave a second reference behind.
func paramIsSingleThreaded(d *ast.Decl, i int, kind ownable) bool {
	updated := false

	for _, cl := range d.Clauses {
		if i >= len(cl.Params) {
			return false
		}
		p, ok := cl.Params[i].(*ast.PVar)
		if !ok {
			// The parameter is destructured or ignored, so there is no name to
			// thread through.
			continue
		}
		v := &visitor{name: p.Name, self: d.Name, index: i, arity: d.Arity(), kind: kind}
		if !v.walk(cl.Body, true) {
			return false
		}
		updated = updated || v.updated
	}

	// Only worth anything if some clause actually updates the collection in its
	// tail call; otherwise nothing would use the owned form.
	return updated
}

// visitor walks a clause body checking that `name` is used only in ways that
// cannot leave another reference to it.
type visitor struct {
	name    string // the parameter being traced
	self    string // the enclosing definition
	index   int    // which parameter it is
	arity   int
	kind    ownable // what sort of collection it is
	updated bool    // a tail call updates it in place
	// fold marks the accumulator of a braid rather than a parameter of a tail
	// recursion. The difference is only where the update may appear: a loop
	// updates it in the tail call's argument, a fold in the lambda's result.
	fold bool
}

// walk reports whether every use of the parameter in e is safe. tail says
// whether e is in tail position.
func (v *visitor) walk(e ast.Expr, tail bool) bool {
	switch e := e.(type) {
	case *ast.Var:
		if e.Name != v.name {
			return true
		}
		// Returning the grid is allowed: it escapes to the caller, and codegen
		// marks it shared on the way out. Anywhere else a bare mention copies
		// the reference, which is what must not happen.
		return tail

	case *ast.IntLit, *ast.FloatLit, *ast.CharLit, *ast.TextLit, *ast.Ctor, *ast.Bad:
		return true

	case *ast.App:
		return v.walkApp(e, tail)

	case *ast.Lambda:
		// A lambda that mentions the parameter captures it, and the closure can
		// outlive the turn.
		free := map[string]bool{}
		bound := map[string]bool{}
		for _, p := range e.Params {
			ast.BindPatternVars(p, bound)
		}
		ast.FreeVars(e.Body, bound, free)
		return !free[v.name]

	case *ast.Let:
		for _, b := range e.Binds {
			// Binding the grid to another name gives it a second owner.
			if !v.walk(b.Value, false) {
				return false
			}
			if b.Name == v.name {
				// The parameter is shadowed from here on, so later code cannot
				// reach it.
				return true
			}
		}
		return v.walk(e.Body, tail)

	case *ast.Ward:
		if !v.walk(e.Subject, false) {
			return false
		}
		for _, arm := range e.Arms {
			bound := map[string]bool{}
			ast.BindPatternVars(arm.Pat, bound)
			if bound[v.name] {
				continue // shadowed in this arm
			}
			if !v.walk(arm.Body, tail) {
				return false
			}
		}
		return true

	case *ast.ThreadLit:
		return v.allSafe(e.Elems)
	case *ast.TwineLit:
		return v.allSafe(e.Elems)
	case *ast.WebLit:
		for _, p := range e.Pairs {
			if !v.walk(p.Key, false) || !v.walk(p.Val, false) {
				return false
			}
		}
		return true
	}
	return true
}

func (v *visitor) allSafe(es []ast.Expr) bool {
	return v.allSafeExcept(es, -1)
}

// allSafeExcept is allSafe over every argument but one, which the caller has
// already accounted for as the collection itself.
func (v *visitor) allSafeExcept(es []ast.Expr, skip int) bool {
	for i, e := range es {
		if i == skip {
			continue
		}
		if !v.walk(e, false) {
			return false
		}
	}
	return true
}

// walkApp handles the two shapes where mentioning the parameter is allowed: a
// read-only verb, and the update inside the loop's own tail call.
func (v *visitor) walkApp(e *ast.App, tail bool) bool {
	callee, isVar := e.Fn.(*ast.Var)

	// A fold's update is its result: the lambda hands back the collection it
	// just wrote to, and nothing else sees the old one.
	if v.fold && tail && v.isUpdateOf(e) {
		v.updated = true
		return true
	}

	// So is a fold *over* it. `braid (t o : items o | braid (u p : put u …) t)`
	// is two loops writing to one map, and the outer accumulator is as
	// single-threaded as the inner one — without this the inner fold's seed
	// arrives shared on every turn of the outer, and copies the whole map.
	if v.fold && tail && v.isFoldOf(e) {
		v.updated = true
		return true
	}

	// `pick` is the one builtin whose arguments are themselves in tail
	// position, since only the taken branch runs — the same rule tailcall.go
	// follows. Without this, a loop written `pick done acc (go (set …) …)`
	// looks to the analysis like an update buried in an ordinary call, and
	// copies on every turn while the `ward` spelling of the same loop does
	// not.
	if isVar && callee.Name == "pick" && len(e.Args) == 3 {
		return v.walk(e.Args[0], false) &&
			v.walk(e.Args[1], tail) && v.walk(e.Args[2], tail)
	}

	// A verb that only reads the collection may name it directly.
	if isVar && v.kind.readOnly[callee.Name] && len(e.Args) > 0 {
		at := v.kind.at(len(e.Args))
		if held, ok := e.Args[at].(*ast.Var); ok && held.Name == v.name {
			return v.allSafeExcept(e.Args, at)
		}
	}

	// The self tail call may update the parameter in place, so long as this is
	// the only mention of it in the whole call.
	if tail && isVar && callee.Name == v.self && len(e.Args) == v.arity {
		for i, arg := range e.Args {
			if i == v.index && v.isUpdateOf(arg) {
				v.updated = true
				continue
			}
			if !v.walk(arg, false) {
				return false
			}
		}
		return true
	}

	if !v.walk(e.Fn, false) {
		return false
	}
	return v.allSafe(e.Args)
}

// isUpdateOf reports whether e is this collection's update verb applied to the
// traced parameter, with the other arguments not mentioning it.
// isUpdateVerb reports whether a name is one of this collection's updating
// verbs, and how many arguments that one takes. They do not all take the same
// number — `put w k v` against `forget w k` — so the arity has to come from
// the verb rather than from the collection.
func (v *visitor) isUpdateVerb(name string) (int, bool) {
	for _, u := range v.kind.updates() {
		if u.verb == name {
			return u.arity, true
		}
	}
	return 0, false
}

func (v *visitor) isUpdateOf(e ast.Expr) bool {
	app, ok := e.(*ast.App)
	if !ok {
		return false
	}
	callee, ok := app.Fn.(*ast.Var)
	if !ok {
		return false
	}
	arity, isUpdate := v.isUpdateVerb(callee.Name)
	if !isUpdate || len(app.Args) != arity {
		return false
	}
	at := v.kind.at(len(app.Args))
	target, ok := app.Args[at].(*ast.Var)
	if !ok || target.Name != v.name {
		return false
	}
	return v.allSafeExcept(app.Args, at)
}

// isFoldOf reports whether e is a fold seeded with the traced collection whose
// own accumulator is single-threaded in turn.
func (v *visitor) isFoldOf(e ast.Expr) bool {
	app, ok := e.(*ast.App)
	if !ok || len(app.Args) != 3 {
		return false
	}
	callee, ok := app.Fn.(*ast.Var)
	if !ok || callee.Name != "braid" {
		return false
	}
	seed, ok := app.Args[1].(*ast.Var)
	if !ok || seed.Name != v.name {
		return false
	}
	// Neither the step function nor the Thread may mention it again.
	free := map[string]bool{}
	ast.FreeVars(app.Args[0], map[string]bool{}, free)
	ast.FreeVars(app.Args[2], map[string]bool{}, free)
	if free[v.name] {
		return false
	}
	return foldBodyIsSingleThreaded(app.Args[0], v.kind)
}

// foldBodyIsSingleThreaded runs the same proof over a braid's step function.
func foldBodyIsSingleThreaded(fn ast.Expr, kind ownable) bool {
	lam, ok := fn.(*ast.Lambda)
	if !ok || len(lam.Params) != 2 {
		return false
	}
	acc, ok := lam.Params[0].(*ast.PVar)
	if !ok {
		return false
	}
	bound := map[string]bool{}
	ast.BindPatternVars(lam.Params[1], bound)
	if bound[acc.Name] {
		return false
	}
	inner := &visitor{name: acc.Name, kind: kind, fold: true}
	return inner.walk(lam.Body, true) && inner.updated
}

// inPlaceUpdate returns the C call for an update that may write through, when
// this argument of a tail call is one.
func (g *gen) inPlaceUpdate(b *body, arg ast.Expr, index int, sc *scope, ti *tailInfo) (string, bool) {
	if g.opts.DisableInPlace || ti == nil || index >= len(ti.owned) || !ti.owned[index] {
		return "", false
	}
	app, ok := arg.(*ast.App)
	if !ok {
		return "", false
	}
	callee, ok := app.Fn.(*ast.Var)
	if !ok {
		return "", false
	}
	site, isUpdate := updateVerbs[callee.Name]
	if !isUpdate || len(app.Args) != site.verb.arity {
		return "", false
	}
	if _, shadowed := sc.lookup(callee.Name); shadowed {
		return "", false
	}
	if _, isTop := g.topFns[callee.Name]; isTop {
		return "", false
	}
	args := g.args(b, app.Args, sc)
	return site.verb.owned + "(" + strings.Join(args, ", ") + ")", true
}

// updateVerbs indexes every updating verb by name, alongside the collection it
// updates and the C function that writes through.
var updateVerbs = func() map[string]updateSite {
	out := map[string]updateSite{}
	for _, o := range ownables {
		for _, u := range o.updates() {
			out[u.verb] = updateSite{kind: o, verb: u}
		}
	}
	return out
}()

// updateSite pairs an updating verb with the collection it updates, since the
// collection decides which argument holds it and the verb decides what to call.
type updateSite struct {
	kind ownable
	verb ownableVerb
}

// ------------------------------------------------- in place inside a fold

// The analysis above recognises a collection threaded through a tail
// recursion. A fold is the same loop written differently:
//
//	ks | braid (w k : put w k 1) (web [])
//
// and it used to path-copy on every step, because the accumulator lived inside
// the runtime's `braid` where no analysis could see it. It does not any more:
// a fused chain ending in `braid` emits the loop itself and inlines the
// lambda, so the accumulator is an ordinary C variable and the same proof
// applies to it — the body may read it, and must otherwise only update it and
// hand the result back.
//
// The dynamic half is unchanged and is what makes it safe: the seed arrives
// shared, since the caller may still hold it, so the first update copies and
// marks the copy owned. What comes out of the loop escapes, so it is disowned.

// foldsIntoMap reports whether a fold's seed is a Web or a Circle, which are
// the accumulators that can be given their final size up front.
func (g *gen) foldsIntoMap(seed ast.Expr) bool {
	t, ok := g.info.Types[seed]
	if !ok {
		return false
	}
	con, isCon := types.Resolve(t).(*types.Con)
	return isCon && (con.Name == types.WebCon || con.Name == types.CircleCon)
}

// foldOwned reports the accumulator parameter of a braid lambda whose updates
// may write through, and the kind of collection it is.
func (g *gen) foldOwned(fn ast.Expr, seed ast.Expr) (string, bool) {
	if g.opts.DisableInPlace {
		return "", false
	}
	lam, ok := fn.(*ast.Lambda)
	if !ok || len(lam.Params) != 2 {
		return "", false
	}
	acc, ok := lam.Params[0].(*ast.PVar)
	if !ok {
		return "", false
	}
	// The seed's type says which collection this is, and whether it is one
	// that can be updated in place at all.
	t, ok := g.info.Types[seed]
	if !ok {
		return "", false
	}
	con, ok := types.Resolve(t).(*types.Con)
	if !ok {
		return "", false
	}
	kind, isOwnable := ownables[con.Name]
	if !isOwnable {
		return "", false
	}

	if !foldBodyIsSingleThreaded(lam, kind) {
		return "", false
	}
	return acc.Name, true
}

// ownedUpdate compiles `put w k v` as a write-through when w is a variable the
// generator is currently holding owned.
//
// The tail-recursion case has its own path, because there the update sits in
// the argument of a jump and has to be recognised there. This one is for the
// update wherever it appears in an expression, which is what a fold's lambda
// body needs.
func (g *gen) ownedUpdate(b *body, e *ast.App, sc *scope) (string, bool) {
	if g.opts.DisableInPlace || len(g.owned) == 0 {
		return "", false
	}
	callee, ok := e.Fn.(*ast.Var)
	if !ok {
		return "", false
	}
	site, isUpdate := updateVerbs[callee.Name]
	if !isUpdate || len(e.Args) != site.verb.arity {
		return "", false
	}
	if _, shadowed := sc.lookup(callee.Name); shadowed {
		return "", false
	}
	if _, isTop := g.topFns[callee.Name]; isTop {
		return "", false
	}
	target, ok := e.Args[site.kind.at(len(e.Args))].(*ast.Var)
	if !ok {
		return "", false
	}
	cname, bound := sc.lookup(target.Name)
	if !bound || !g.owned[cname] {
		return "", false
	}
	args := g.args(b, e.Args, sc)
	return site.verb.owned + "(" + strings.Join(args, ", ") + ")", true
}
