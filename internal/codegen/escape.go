package codegen

import (
	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/types"
)

// Releasing the Threads a call leaves behind.
//
// The arena's free lists (weave.c) take back blocks whose death is obvious to
// the runtime: the array a buffer has outgrown, the node an owned insert has
// replaced. What they cannot see is the far larger case — a function that
// builds half a dozen intermediate Threads, uses them, and returns something
// that has nothing to do with any of them. Advent of Code 2024 day 22 does that
// two thousand times over, and it was 694 MB of Thread arrays that nothing ever
// asked for again.
//
// Deciding that is an escape question, and the answer here comes mostly from
// the types. If a function's result cannot *contain* a Thread — `Web Earth
// Earth` cannot, `Thread Air` obviously can, a type variable might — then no
// Thread the function built can be reached through what it returns. Weave has
// no assignment and no mutable globals, so being returned is the only way a
// value outlives the call that made it; the one exception is a `remember`ed
// function, whose table keeps its arguments, and that is checked separately.
//
// What is released, then, is a `weave` binding that
//
//   - is a Thread,
//   - was bound to a verb that allocates a fresh array nobody else can see —
//     never a slicing verb like `drop`, whose result shares its source's
//     storage,
//   - is only ever handed to prelude verbs, which cannot retain it, and
//   - sits in a function whose result type cannot carry a Thread.
//
// The frees go in just before the return, after the result is in hand. `weave
// build -no-release` turns the whole thing off, and the differential tests
// compile every program both ways and require the answers to agree.

// carriesThread reports whether a value of this type could hold a Thread
// anywhere inside it. It answers yes whenever it cannot be sure: an unresolved
// variable could be instantiated with anything, a function could have captured
// anything, and a declared sum type's field types are not recorded here.
func carriesThread(t types.Type) bool {
	con, ok := types.Resolve(t).(*types.Con)
	if !ok {
		return true // a type variable, or a function that captured who knows what
	}
	switch con.Name {
	case types.ThreadCon:
		return true
	case types.Earth, types.Water, types.Fire, types.Spirit, types.Air, types.KnotCon:
		return false
	case types.HoldCon, types.WeavingCon, types.TwineCon,
		types.WebCon, types.CircleCon, types.PatternCon, types.TaverenCon:
		for _, a := range con.Args {
			if carriesThread(a) {
				return true
			}
		}
		return false
	}
	return true // a sum type the program declared: assume the worst
}

// freshThread names the verbs whose result is a Thread over an array they have
// just allocated, with no other owner. The slicing verbs — `drop`, `take`,
// `dropwhile`, `takewhile`, `sever`, `strands` — are deliberately absent: their
// result shares the source's storage, so freeing one would free the other. So
// is `mend`, which hands back the Thread it was given when the position is not
// there.
var freshThread = map[string]bool{
	"bend": true, "sift": true, "cull": true, "zip": true, "zipwith": true,
	"rev": true, "weld": true, "plait": true,
	"sort": true, "sortby": true, "flat": true, "uniq": true,
	"lines": true, "words": true, "fires": true, "split": true, "splitOn": true,
	"earths": true, "spans": true, "waters": true, "sparks": true, "digits": true,
	"keys": true, "vals": true, "items": true, "members": true,
	"chunks": true, "windows": true, "perms": true, "pairs": true,
	"clumps": true, "couples": true, "squeeze": true, "mesh": true,
	"carve": true, "clumped": true, "sites": true,
	"under": true, "copies": true,
	"cells": true, "rows": true, "cols": true, "knots": true,
	"nb4": true, "nb8": true, "around4": true, "around8": true,
	"top": true, "bot": true, "cellwise": true, "mapvals": true,
}

// releasable picks out the bindings of one clause whose Thread storage may be
// handed back before the function returns.
//
// Only the clause body's own chain of `weave` bindings is considered. A binding
// inside a `ward` arm lives in a C block that has already closed by the time
// the return is written, so there would be nothing left to name.
func (g *gen) releasable(cl *ast.Clause) map[*ast.Bind]bool {
	if g.opts.DisableRelease {
		return nil
	}
	// Whatever the function hands back must not be able to carry a Thread.
	t, ok := g.info.Types[cl.Body]
	if !ok || carriesThread(t) {
		return nil
	}

	binds, body := letSpine(cl.Body)
	if len(binds) == 0 {
		return nil
	}

	out := map[*ast.Bind]bool{}
	for i, bind := range binds {
		if len(bind.Params) > 0 || bind.Pat != nil {
			// A local function, or a binding that takes its value apart and so
			// has several names rather than one to trace.
			continue
		}
		bt, known := g.info.Types[bind.Value]
		if !known {
			continue
		}
		if con, isCon := types.Resolve(bt).(*types.Con); !isCon || con.Name != types.ThreadCon {
			continue
		}
		if !g.freshArray(bind.Value) {
			continue
		}
		// Every later mention of the name has to be somewhere that cannot keep
		// it, which is checked over the rest of the body.
		if !g.usesAreSafe(bind.Name, binds[i+1:], body) {
			continue
		}
		out[bind] = true
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// letSpine unrolls the chain of `weave` bindings a clause body opens with,
// returning them in order along with what follows.
func letSpine(e ast.Expr) ([]*ast.Bind, ast.Expr) {
	var binds []*ast.Bind
	for {
		let, ok := e.(*ast.Let)
		if !ok {
			return binds, e
		}
		binds = append(binds, let.Binds...)
		e = let.Body
	}
}

// freshArray reports whether an expression allocates the Thread it returns.
func (g *gen) freshArray(e ast.Expr) bool {
	app, ok := e.(*ast.App)
	if !ok {
		return false
	}
	v, isVar := app.Fn.(*ast.Var)
	if !isVar || g.defined(v.Name) {
		return false
	}
	return freshThread[v.Name]
}

// usesAreSafe reports whether every mention of name in what follows is an
// argument to a prelude verb.
//
// A prelude verb cannot outlive the call, so whatever it does with the Thread —
// read it, slice it, put it in a Web — the result is still something this
// function either drops or returns, and the return has already been ruled out
// by its type. A *user* function is refused because it might be `remember`ed,
// and a memo table keeps its arguments for the rest of the program.
func (g *gen) usesAreSafe(name string, rest []*ast.Bind, body ast.Expr) bool {
	safe := true
	var walk func(e ast.Expr, argOfVerb bool)
	walk = func(e ast.Expr, argOfVerb bool) {
		if !safe || e == nil {
			return
		}
		switch e := e.(type) {
		case *ast.Var:
			if e.Name == name && !argOfVerb {
				safe = false
			}
		case *ast.App:
			verb := false
			if v, ok := e.Fn.(*ast.Var); ok {
				_, isBuiltin := builtins[v.Name]
				verb = isBuiltin && !g.defined(v.Name)
			} else {
				walk(e.Fn, false)
			}
			for _, a := range e.Args {
				walk(a, verb)
			}
		case *ast.Lambda:
			// A lambda may be stored in a closure that outlives the call.
			walk(e.Body, false)
		case *ast.Let:
			for _, bind := range e.Binds {
				walk(bind.Value, false)
			}
			walk(e.Body, false)
		case *ast.Ward:
			walk(e.Subject, false)
			for _, arm := range e.Arms {
				walk(arm.Body, false)
			}
		case *ast.ThreadLit:
			for _, el := range e.Elems {
				walk(el, false)
			}
		case *ast.TwineLit:
			for _, el := range e.Elems {
				walk(el, false)
			}
		case *ast.WebLit:
			for _, pair := range e.Pairs {
				walk(pair.Key, false)
				walk(pair.Val, false)
			}
		}
	}
	for _, bind := range rest {
		walk(bind.Value, false)
	}
	walk(body, false)
	return safe
}
