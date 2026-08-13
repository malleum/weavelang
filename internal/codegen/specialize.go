package codegen

import (
	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/types"
)

// Primitive specialisation.
//
// The prelude's arithmetic and comparison verbs work on any type: `add`
// branches on whether either side is Water, and `eq` calls w_compare, which
// compares tags and then switches on them. That generality costs a branch per
// arithmetic operation and an out-of-line call per comparison, on every element
// of every loop.
//
// None of it is needed once the types are known, and they always are — the
// checker records the type of every expression. When both operands of `add`
// are Earth this pass emits w_add_e, which is an integer addition; when the
// operands of `gt` are Earth it emits w_gt_e, which is a compare instruction
// rather than a call into the runtime.
//
// Specialisation is skipped wherever the type is not a concrete primitive, so
// a polymorphic definition keeps the general verb and behaves exactly as
// before. This is what makes the pass safe: it never has to prove anything the
// type checker has not already proved.

// specialisations maps a verb and its operand type to the typed C helper that
// implements it. A missing entry simply means the general verb is used.
var specialisations = map[string]map[string]string{
	types.Earth: {
		"add": "w_add_e", "sub": "w_sub_e", "mul": "w_mul_e",
		"div": "w_div_e", "mod": "w_mod_e",
		"neg": "w_neg_e", "abs": "w_abs_e",
		"min": "w_min_e", "max": "w_max_e",
		"even": "w_even_e", "odd": "w_odd_e", "divBy": "w_divby_e",
		"eq": "w_eq_e", "neq": "w_neq_e",
		"lt": "w_lt_e", "lte": "w_lte_e", "gt": "w_gt_e", "gte": "w_gte_e",
	},
	types.Water: {
		"add": "w_add_w", "sub": "w_sub_w", "mul": "w_mul_w", "div": "w_div_w",
		"neg": "w_neg_w", "abs": "w_abs_w",
		"min": "w_min_w", "max": "w_max_w",
		"eq": "w_eq_w", "neq": "w_neq_w",
		"lt": "w_lt_w", "lte": "w_lte_w", "gt": "w_gt_w", "gte": "w_gte_w",
	},
	types.Fire: {
		"eq": "w_eq_f", "neq": "w_neq_f",
		"lt": "w_lt_f", "lte": "w_lte_f", "gt": "w_gt_f", "gte": "w_gte_f",
	},
	types.Spirit: {
		"eq": "w_eq_s", "neq": "w_neq_s",
		"and": "w_and_s", "or": "w_or_s", "not": "w_not_s",
	},
}

// specialCname returns the typed helper for a verb at a given operand type.
func (g *gen) specialCname(verb, operand string) (string, bool) {
	if g.opts.DisableSpecialize || operand == "" {
		return "", false
	}
	byType, ok := specialisations[operand]
	if !ok {
		return "", false
	}
	cname, ok := byType[verb]
	return cname, ok
}

// primitiveType names the primitive an expression has, or "" when its type is
// not a primitive or was never inferred. Every verb this pass touches takes
// operands of one type, so any operand's type identifies the whole call.
func (g *gen) primitiveType(e ast.Expr) string {
	t, ok := g.info.Types[e]
	if !ok {
		return ""
	}
	con, ok := types.Resolve(t).(*types.Con)
	if !ok || len(con.Args) != 0 {
		return ""
	}
	switch con.Name {
	case types.Earth, types.Water, types.Fire, types.Spirit:
		return con.Name
	}
	return ""
}

// packedElem names the C tag every element of e will carry, when e is a Thread
// whose elements are all one Power that lives in the Value itself. A loop
// building such a Thread can write payloads alone and put the tag in the
// header — eight bytes to the element rather than sixteen. See the layout note
// in weave.h.
//
// Earth alone for now, which is what a parsed input and everything counted out
// of one is made of. Water and Fire would work the same way and are waiting on
// a benchmark that wants them.
func (g *gen) packedElem(e ast.Expr) string {
	t, ok := g.info.Types[e]
	if !ok {
		return ""
	}
	con, ok := types.Resolve(t).(*types.Con)
	if !ok || con.Name != types.ThreadCon || len(con.Args) != 1 {
		return ""
	}
	inner, ok := types.Resolve(con.Args[0]).(*types.Con)
	if !ok || len(inner.Args) != 0 || inner.Name != types.Earth {
		return ""
	}
	return "W_EARTH"
}

// operandType finds the type shared by a call's operands, taking the first one
// that is known. A partially applied verb still identifies its type this way:
// in `gt 4` the supplied bound is an Earth, so the comparison is at Earth.
func (g *gen) operandType(args []ast.Expr) string {
	for _, a := range args {
		if name := g.primitiveType(a); name != "" {
			return name
		}
	}
	return ""
}

// specialiseCall returns the typed helper for a saturated builtin call.
func (g *gen) specialiseCall(verb string, args []ast.Expr) (string, bool) {
	return g.specialCname(verb, g.operandType(args))
}

// argumentType names the primitive a function expression takes, which is how a
// verb standing on its own as a pipeline stage gets specialised. In
// `xs | sift even` there is no operand to read a type off — `even` is passed,
// not applied — but the checker recorded its type at this use as
// `Earth -> Spirit`, and the argument side of that is what the loop feeds it.
func (g *gen) argumentType(fn ast.Expr) string {
	t, ok := g.info.Types[fn]
	if !ok {
		return ""
	}
	f, ok := types.Resolve(t).(*types.Fn)
	if !ok {
		return ""
	}
	con, ok := types.Resolve(f.From).(*types.Con)
	if !ok || len(con.Args) != 0 {
		return ""
	}
	switch con.Name {
	case types.Earth, types.Water, types.Fire, types.Spirit:
		return con.Name
	}
	return ""
}
