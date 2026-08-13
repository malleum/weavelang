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
}

// sharesStorage names the prelude verbs whose result can still reach the
// collection they were given, so that naming an owned collection there would
// leave a second way to see it.
//
// There are two ways for that to happen and both are here. Nine verbs hand back
// a *window* on the argument's array — `w_thread(t->items + k, …)` — so writing
// through the original would be seen through the window. Three more hand the
// argument itself straight back when there is nothing to do: `mend` and `twist`
// at a position that is not there, `set` at a knot off the grid.
//
// This replaced a per-type whitelist of about seventeen verbs. The whitelist
// was the wrong shape: every verb it did not mention was refused, which is
// nearly the whole prelude, and adding a verb silently narrowed the analysis
// further. A blacklist plus the type rule below turns that round — a verb has
// to *earn* its way out, and a test fails if a new prelude verb is not
// classified either way.
var sharesStorage = map[string]bool{
	// A window on the argument's own array.
	"take": true, "drop": true, "sever": true, "strands": true,
	"takewhile": true, "dropwhile": true, "chunk": true, "windows": true,
	"cells": true,
	// The argument itself, when the update had nowhere to go.
	"mend": true, "twist": true, "set": true, "wind": true,
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
	},
	types.WebCon: {
		update: "put", owned: "wp_put_owned", arity: 3,
		also: []ownableVerb{{"forget", "wp_forget_owned", 2}},
	},
	// A Link's `slots` and `nodes` never change, so binding copies only the two
	// int32 arrays — eight bytes a node. That makes the shared path cheap
	// enough that failing to thread a Link single-threadedly is slow rather
	// than hopeless, which is not true of the others here.
	types.LinkCon: {
		update: "bind", owned: "wp_bind_owned", arity: 3,
	},
	types.CircleCon: {
		update: "insert", owned: "wp_insert_owned", arity: 2,
		also: []ownableVerb{{"remove", "wp_remove_owned", 2}},
	},
	// A Thread's buffer is one block, like a Pattern's, so it takes the same
	// one bit and the same proof. What is different is that a Thread's storage
	// can be shared without a second buffer: `take`, `drop`, `sever`,
	// `strands` and the Thread patterns all hand back a window on it,
	// so none of them appears here. Every verb that does returns an element or
	// a number, never a view.
	types.ThreadCon: {
		update: "mend", owned: "wp_mend_owned", arity: 3, dataLast: true,
		also: []ownableVerb{
			{"twist", "wp_twist_owned", 3},
			{"wind", "wp_wind_owned", 3},
		},
	},
}

// ownedParams reports, for each parameter of a self-tail-recursive definition,
// whether an update to it in the tail call may write through.
func (g *gen) ownedParams(d *ast.Decl) []bool {
	kinds := g.ownableParams(d)
	out := make([]bool, d.Arity())
	for i, kind := range kinds {
		out[i] = kind != "" && g.paramIsSingleThreaded(d, i, kind)
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
func (g *gen) paramIsSingleThreaded(d *ast.Decl, i int, con string) bool {
	kind := ownables[con]
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
		v := &visitor{g: g, con: con, name: p.Name, self: d.Name, index: i, arity: d.Arity(), kind: kind, twineAt: -1}
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
	g       *gen   // for the recorded types, and for which definitions are memoised
	con     string // the collection's type constructor, e.g. types.ThreadCon
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
	// twineAt is the slot the collection occupies when the accumulator is a
	// Twine of state, or -1 when the accumulator is the collection itself.
	twineAt int
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
		// A lambda that mentions the collection captures it, and a closure can
		// outlive the turn that made it — unless it was handed straight to a
		// prelude verb, which cannot keep it past the call. `span a b as (j : …
		// nth j xs …)` reads the Thread through a lambda that is gone before
		// the next statement, and refusing that cost the whole loop.
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
			if b.Pat != nil {
				// A binding that takes its value apart may shadow the traced
				// name; it can never *be* it, since its value has just been
				// walked and a bare mention there would already have failed.
				shadowed := map[string]bool{}
				ast.BindPatternVars(b.Pat, shadowed)
				if shadowed[v.name] {
					return true
				}
				continue
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
		// The step of a fold whose accumulator is a Twine of state rebuilds it
		// here, and the slot the collection lives in is where its update has to
		// be. The other slots are evaluated after that one, so they may not
		// mention it: they would see a write that has not happened.
		if v.fold && tail && v.twineAt >= 0 && v.twineAt < len(e.Elems) {
			if v.isUpdateOf(e.Elems[v.twineAt]) {
				for i, el := range e.Elems {
					if i == v.twineAt {
						continue
					}
					free := map[string]bool{}
					ast.FreeVars(el, map[string]bool{}, free)
					if free[v.name] {
						return false
					}
				}
				v.updated = true
				return true
			}
		}
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

// readsAreSafe walks the arguments of a call that has already been cleared as a
// read, skipping the one holding the collection.
//
// A lambda written out *here* is different from a lambda anywhere else. It is
// handed straight to a verb that cannot keep it, so the closure dies with the
// call and a mention of the collection inside it is a read like any other —
// `span a b as (j : nth j xs)` reads the Thread through a closure that is gone
// before the next statement. Everywhere else a lambda may be stored, and a
// capture is refused.
//
// The body is then walked with that permission spent: a bare mention still
// fails, since the lambda would be handing the collection out.
func (v *visitor) readsAreSafe(args []ast.Expr, skip int) bool {
	for i, a := range args {
		if i == skip {
			continue
		}
		if lam, isLambda := a.(*ast.Lambda); isLambda {
			if !v.walk(lam.Body, false) {
				return false
			}
			continue
		}
		if !v.walk(a, false) {
			return false
		}
	}
	return true
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

	// A `gentle` step answers `Woven acc` to carry on and `Gentled x` to stop.
	// The Woven payload is the accumulator the next turn will see, so it is
	// this fold's tail position. The Gentled payload is the *answer* leaving
	// the fold, and a collection handed out there would escape still marked
	// owned, so it is read like any other argument.
	if v.fold && tail && len(e.Args) == 1 {
		if c, isCtor := e.Fn.(*ast.Ctor); isCtor {
			switch c.Name {
			case "Woven":
				return v.walk(e.Args[0], true)
			case "Gentled":
				return v.walk(e.Args[0], false)
			}
		}
	}

	// A constructor in tail position hands the collection to the caller inside
	// something — `Held xs` is the shape a search answers with. That is the same
	// escape a bare mention in tail position is, and codegen disowns it in the
	// same place, so it is allowed on the same terms. Naming it twice is fine
	// here where it is not in a tail call's arguments: both references are to
	// the one object and disowning it once covers them.
	//
	// Advent of Code 2025 day 10's back-substitution ends `Held xs`, and
	// refusing that cost it a copy of the whole solution vector per row.
	if tail && !v.fold {
		if _, isCtor := e.Fn.(*ast.Ctor); isCtor {
			for _, arg := range e.Args {
				if !v.walk(arg, true) {
					return false
				}
			}
			return true
		}
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

	// The self tail call may update the parameter in place, so long as this is
	// the only mention of it in the whole call.
	//
	// "The only mention" is not a tidiness rule, it is the whole correctness of
	// the update. The arguments of a tail call are evaluated in order into the
	// loop's slots, so an update at one of them writes through *before* a later
	// one is evaluated — and a later argument that reads the collection would
	// see a write that, in the language, has not happened. That is why a read
	// anywhere else in the body is fine and a read in a sibling argument is
	// not: everything else runs first.
	if tail && isVar && callee.Name == v.self && len(e.Args) == v.arity {
		updating := v.index < len(e.Args) && v.isUpdateOf(e.Args[v.index])
		for i, arg := range e.Args {
			if i == v.index {
				if v.isUpdateOf(arg) {
					v.updated = true
					continue
				}
				// Handed straight back into its own slot, unchanged. The next
				// turn holds exactly the reference this one did, so this is as
				// single-threaded as updating it — and it is what every loop
				// with a branch that skips the update looks like, which is most
				// of them: "this pair is already joined, go on to the next".
				//
				// Without this the whole loop copies on every turn, including
				// the turns that do update.
				if held, ok := arg.(*ast.Var); ok && held.Name == v.name {
					continue
				}
			}
			if updating {
				// This argument is evaluated after the write, so it may not
				// look at the collection at all.
				free := map[string]bool{}
				ast.FreeVars(arg, map[string]bool{}, free)
				if free[v.name] {
					return false
				}
				continue
			}
			if !v.walk(arg, false) {
				return false
			}
		}
		return true
	}

	// A call that only *reads* the collection may name it, wherever in the
	// argument list it sits.
	//
	// Two things have to hold, and between them they replace the old whitelist
	// of blessed verbs. The callee must not be one of the twelve whose result
	// can still reach the collection's storage (see sharesStorage). And the
	// collection's type must not occur anywhere in the call's *result* type —
	// which is what stops `copies 3 g`, `weld [g] xs` or `put w k g` from
	// quietly keeping it, since a verb that hands the collection back inside
	// something has that something's type say so.
	//
	// A user-defined function is read the same way, unless it is `remember`ed:
	// a memo table keeps its arguments for the rest of the program, and no
	// type can say that.
	//
	// The result-type rule is only about the collection being *handed over*. If
	// it is never an argument — only read inside a lambda the call is holding —
	// then the call cannot have been given it to put anywhere, and the walk of
	// the lambda's body has already refused every way the closure could hand it
	// out. All that is left to rule out is the closure itself surviving the
	// call, which it can only do inside a function the call returns. So that
	// case asks a weaker question: does the result carry a function?
	//
	// That distinction is the whole of `span (add c 1) w as (j : mul (nth j pr)
	// (nth j xs)) | sum` — a read of `xs` through a lambda, inside a `bend`
	// whose result is a Thread. The old rule saw a Thread in the result and gave
	// up, which cost Advent of Code 2025 day 10 a copy of the whole solution
	// vector per back-substitution: 277 MB of `mend`.
	if isVar && len(e.Args) > 0 && v.calleeCannotRetain(callee.Name) {
		at := -1
		for i, a := range e.Args {
			if held, isVar := a.(*ast.Var); isVar && held.Name == v.name {
				if at >= 0 {
					return false // named twice in one call
				}
				at = i
			}
		}
		if v.resultIsClear(e, at >= 0) && v.readsAreSafe(e.Args, at) {
			return true
		}
		// Otherwise this was not a read after all — the collection is in there
		// somewhere the rule cannot account for — so let the branches below
		// have their turn.
	}

	if !v.walk(e.Fn, false) {
		return false
	}
	return v.allSafe(e.Args)
}

// calleeCannotRetain reports whether a call is one that cannot keep what it is
// given past the call itself.
//
// A prelude verb qualifies, bar the twelve whose result can still reach the
// collection's storage (see sharesStorage). So does the program's own function,
// unless it is `remember`ed: a memo table keeps its arguments for the rest of
// the program, and no type can say that.
func (v *visitor) calleeCannotRetain(callee string) bool {
	if sharesStorage[callee] {
		return false
	}
	if v.g == nil {
		return false // no recorded types, so nothing can be proved
	}
	if _, isTop := v.g.topFns[callee]; isTop {
		return !v.g.memoed[callee]
	}
	_, isBuiltin := builtins[callee]
	return isBuiltin
}

// resultIsClear reports whether what a call hands back can be trusted not to
// carry the collection away.
//
// `handedOver` says the collection is an argument of this call. Then the
// question is whether the result could contain it — `copies 3 g`, `weld [g] xs`
// and `put w k g` all have the collection's own type constructor in their
// result, which is what says so.
//
// Otherwise the collection is only mentioned inside a lambda this call is
// holding, and the walk of that lambda's body has already refused every way it
// could hand the collection out. What is left is the closure outliving the
// call, which needs the result to be, or to contain, a function.
func (v *visitor) resultIsClear(e *ast.App, handedOver bool) bool {
	t, known := v.g.info.Types[e]
	if !known {
		return false
	}
	if handedOver {
		return !mentionsCon(t, v.con)
	}
	return !carriesFn(t)
}

// carriesFn reports whether a function could be hiding anywhere in a type. It
// answers yes for anything it cannot see into, since an unresolved variable
// could be instantiated with one.
func carriesFn(t types.Type) bool {
	con, ok := types.Resolve(t).(*types.Con)
	if !ok {
		return true
	}
	for _, a := range con.Args {
		if carriesFn(a) {
			return true
		}
	}
	return false
}

// mentionsCon reports whether a type constructor occurs anywhere in a type. It
// answers yes for anything it cannot see into — an unresolved variable could be
// instantiated with the collection itself, and a function could hand it back.
func mentionsCon(t types.Type, name string) bool {
	con, ok := types.Resolve(t).(*types.Con)
	if !ok {
		return true
	}
	if con.Name == name {
		return true
	}
	for _, a := range con.Args {
		if mentionsCon(a, name) {
			return true
		}
	}
	return false
}

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

// isUpdateOf reports whether e hands the traced collection on: an update verb
// applied to it, or a call to a function that consumes it (see consume.go).
// Either way exactly one reference goes in and one comes out.
func (v *visitor) isUpdateOf(e ast.Expr) bool {
	return v.isUpdateVerbOf(e) || v.isMoveOf(e)
}

func (v *visitor) isUpdateVerbOf(e ast.Expr) bool {
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
	// The target is the traced collection, or another update of it. A chain —
	// `put (put w a 1) b 2` — is as single-threaded as one update: each link
	// hands its result straight to the next and nothing else sees it.
	if target, isVar := app.Args[at].(*ast.Var); !isVar || target.Name != v.name {
		if !v.isUpdateOf(app.Args[at]) {
			return false
		}
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
	return foldBodyIsSingleThreaded(v.g, v.con, app.Args[0], v.kind)
}

// foldBodyIsSingleThreaded runs the same proof over a braid's step function.
//
// The step has to be written out on the spot. A named one is proved the same
// way — the code for that is straightforward — but proving it would not help:
// the update then happens inside the callee, where this function's ownership
// bookkeeping does not reach. Marking a parameter of an ordinary function owned
// is the generalisation that would cover it, and is not built. See TODO.md.
func foldBodyIsSingleThreaded(g *gen, con string, fn ast.Expr, kind ownable) bool {
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
	inner := &visitor{g: g, con: con, name: acc.Name, kind: kind, fold: true, twineAt: -1}
	return inner.walk(lam.Body, true) && inner.updated
}

// inPlaceUpdate returns the C call for an update that may write through, when
// this argument of a tail call is one.
func (g *gen) inPlaceUpdate(b *body, arg ast.Expr, index int, sc *scope, ti *tailInfo) (string, bool) {
	if g.opts.DisableInPlace || ti == nil || index >= len(ti.owned) || !ti.owned[index] {
		return "", false
	}
	// A helper that consumes the collection and hands it back is as good as an
	// update here: it is given the loop's own reference and its result goes
	// straight into the slot that reference came from.
	if out, ok := g.movedCall(b, arg, sc); ok {
		return out, true
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

// foldOwned reports the accumulator of a fold whose updates may write through:
// the name to mark owned, and whether it was found.
//
// The accumulator is usually the collection itself. It may also be a *Twine* of
// state with the collection as one half — `(board, position)` threaded through
// a walk — which is the natural way to carry two things and used to be the
// worst shape in the language, since it lost the analysis entirely and copied
// the whole collection once per element. A Twine the step takes apart on entry
// and rebuilds on exit has exactly one reference to each half, so the half that
// is a collection is as single-threaded as a bare accumulator would be.
func (g *gen) foldOwned(fn ast.Expr, seed ast.Expr) (string, int, bool) {
	if g.opts.DisableInPlace {
		return "", -1, false
	}
	lam, ok := fn.(*ast.Lambda)
	if !ok || len(lam.Params) != 2 {
		return "", -1, false
	}
	t, ok := g.info.Types[seed]
	if !ok {
		return "", -1, false
	}
	con, ok := types.Resolve(t).(*types.Con)
	if !ok {
		return "", -1, false
	}

	// The accumulator itself.
	if kind, isOwnable := ownables[con.Name]; isOwnable {
		acc, isVar := lam.Params[0].(*ast.PVar)
		if !isVar {
			return "", -1, false
		}
		if !foldBodyIsSingleThreaded(g, con.Name, lam, kind) {
			return "", -1, false
		}
		return acc.Name, -1, true
	}

	// One half of a Twine of state.
	if con.Name != types.TwineCon {
		return "", -1, false
	}
	tw, isTwine := lam.Params[0].(*ast.PTwine)
	if !isTwine || len(tw.Elems) != len(con.Args) {
		return "", -1, false
	}
	at, name := -1, ""
	for i, arg := range con.Args {
		c, isCon := types.Resolve(arg).(*types.Con)
		if !isCon {
			continue
		}
		k, isOwnable := ownables[c.Name]
		if !isOwnable {
			continue
		}
		if at >= 0 {
			return "", -1, false // two of them: which is threaded is not clear
		}
		pv, isVar := tw.Elems[i].(*ast.PVar)
		if !isVar {
			return "", -1, false
		}
		if !foldComponentIsSingleThreaded(g, c.Name, k, pv.Name, lam, i) {
			return "", -1, false
		}
		at, name = i, pv.Name
	}
	if at < 0 {
		return "", -1, false
	}
	return name, at, true
}

// foldComponentIsSingleThreaded runs the proof over a step whose accumulator is
// a Twine, tracing the half at index `at`.
//
// The one extra rule is where the update has to be: the step's result must be a
// Twine literal whose slot `at` is the update, and no other slot may mention the
// collection. The slots are evaluated in order, so a later one that read it
// would see a write that has not happened — the same hazard a sibling argument
// of a tail call has.
func foldComponentIsSingleThreaded(g *gen, con string, kind ownable, name string, lam *ast.Lambda, at int) bool {
	bound := map[string]bool{}
	ast.BindPatternVars(lam.Params[1], bound)
	if bound[name] {
		return false
	}
	v := &visitor{g: g, con: con, name: name, kind: kind, fold: true, twineAt: at}
	return v.walk(lam.Body, true) && v.updated
}

// ownedUpdate compiles `put w k v` as a write-through when w is a variable the
// generator is currently holding owned.
//
// The tail-recursion case has its own path, because there the update sits in
// the argument of a jump and has to be recognised there. This one is for the
// update wherever it appears in an expression, which is what a fold's lambda
// body needs.
func (g *gen) ownedUpdate(b *body, e ast.Expr, sc *scope) (string, bool) {
	app, site, ok := g.updatesOwned(e, sc)
	if !ok {
		return "", false
	}
	args := g.args(b, app.Args, sc)
	return site.verb.owned + "(" + strings.Join(args, ", ") + ")", true
}

// updatesOwned reports whether e is an update applied to a collection this
// function owns, and which verb writes it through.
func (g *gen) updatesOwned(e ast.Expr, sc *scope) (*ast.App, updateSite, bool) {
	var none updateSite
	if g.opts.DisableInPlace {
		return nil, none, false
	}
	app, isApp := e.(*ast.App)
	if !isApp {
		return nil, none, false
	}
	callee, isVar := app.Fn.(*ast.Var)
	if !isVar {
		return nil, none, false
	}
	site, isUpdate := updateVerbs[callee.Name]
	if !isUpdate || len(app.Args) != site.verb.arity {
		return nil, none, false
	}
	if _, shadowed := sc.lookup(callee.Name); shadowed {
		return nil, none, false
	}
	if _, isTop := g.topFns[callee.Name]; isTop {
		return nil, none, false
	}
	if !g.yieldsOwned(app.Args[site.kind.at(len(app.Args))], sc) {
		return nil, none, false
	}
	return app, site, true
}

// yieldsOwned reports whether this expression hands back a collection nothing
// else can see, so an update applied to it may write through.
//
// Three ways to be owned, and they compose: a variable the generator is holding
// — a fold's accumulator, a consumed parameter, a loop's own parameter across a
// tail call — an update that already writes through, and a call to a function
// the collection was handed over to. That composition is what makes a chain
// cost one copy rather than one per link: `mend 1 2 (one t)` writes through the
// Thread `one` gave back rather than copying it again.
func (g *gen) yieldsOwned(e ast.Expr, sc *scope) bool {
	switch e := e.(type) {
	case *ast.Var:
		cname, bound := sc.lookup(e.Name)
		return bound && g.owned[cname]
	case *ast.App:
		if _, _, ok := g.updatesOwned(e, sc); ok {
			return true
		}
		_, ok := g.movesOwned(e, sc)
		return ok
	}
	return false
}
