package codegen

import (
	"github.com/malleum/weave/internal/ast"
)

// A backtracking search is the one shape no ownership analysis will ever help
// with. It uses the collection it was handed once *per option*, not once — the
// old value has to survive for the next branch — so every branch it tries
// copies, and every copy is dead the moment that branch is abandoned. Advent of
// Code 2025 day 10 held 296 MB at exit and had freed none of it: 1.1 million
// copied rows, one per node of a search whose answer is a single number.
//
// What a search wants is not to free those one at a time but to forget them all
// at once, and the arena can do exactly that: mark where the bump pointer has
// got to, let a turn of the loop allocate whatever it likes, put the pointer
// back. See w_mark and w_release.
//
// The whole difficulty is the one condition that makes it sound:
//
//	nothing allocated during a turn may be reachable after that turn
//
// which is a question about the loop, not about the arena. What follows is how
// that question is answered, and it is answered conservatively: a loop that
// cannot be shown to satisfy it is compiled exactly as it was.

// turnReleasable reports whether each turn of this fused loop may hand its storage
// back at the end of it.
//
// Four things have to hold, and each of them is one way for something allocated
// inside a turn to still be reachable outside it.
//
//   - The accumulator is what a turn is *for*, and it is the one value that
//     deliberately outlives the turn. So it has to be a Power that lives in the
//     Value itself — an Earth, a Water, a Fire, a Spirit — carrying no pointer
//     into the region at all.
//   - The elements have to be immediate for the same reason, since a stage or
//     the step may keep one. A span counts; a Thread being walked does not,
//     because its elements are pointers into storage older than the region, and
//     a loop over one is not the shape this is for.
//   - No stage may hold state across turns: `scan`, `dupe` and the rest keep a
//     collection built one turn and read the next, which is precisely what a
//     release would take away.
//   - Nothing the step calls may store into a global, because a global outlives
//     every region. There are two of those in a Weave program: a `remember`
//     table, and the memoised accessor a top-level value is compiled into. The
//     first rules the loop out; the second is dealt with by forcing those values
//     before the loop starts, where the storage is outside every region.
func (g *gen) turnReleasable(p pipeline, braidPlan callPlan, sc *scope) bool {
	// Only the folds, because only they have an accumulator whose type says
	// what survives a turn. A `collect` builds its Thread out of the very
	// storage a release would take back.
	if g.opts.DisableRegions {
		return false
	}
	if p.consumer != conBraid || len(p.stages) > 0 || !p.producer.isSpan() {
		return false
	}
	if p.elem == "" || g.primitiveType(p.braidSeed) == "" {
		return false
	}
	// The step has to be one the compiler can see into. A closure reached
	// through a pointer could be anything.
	if braidPlan.lambda == nil {
		return false
	}
	return g.storesNothingGlobal(braidPlan.lambda.Body, sc, map[string]bool{})
}

// reachable collects every name e can mention, following into the definitions
// it names, so that a question about a loop body is really a question about
// everything that body runs.
func (g *gen) reachable(e ast.Expr, sc *scope, seen map[string]bool) []string {
	free := map[string]bool{}
	ast.FreeVars(e, map[string]bool{}, free)

	var out []string
	for name := range free {
		if seen[name] {
			continue
		}
		if _, shadowed := sc.lookup(name); shadowed {
			continue
		}
		seen[name] = true
		out = append(out, name)
		d := g.byName[name]
		if d == nil {
			continue // a builtin, whose body is the runtime's
		}
		for _, cl := range d.Clauses {
			bound := map[string]bool{}
			for _, param := range cl.Params {
				ast.BindPatternVars(param, bound)
			}
			inner := map[string]bool{}
			ast.FreeVars(cl.Body, bound, inner)
			for n := range inner {
				if !seen[n] {
					out = append(out, g.reachable(&ast.Var{Name: n}, newScope(nil), seen)...)
				}
			}
		}
	}
	return out
}

// storesNothingGlobal reports whether the loop body, and everything it can
// call, leaves no value anywhere that outlives a turn.
//
// A `remember` table is the one that cannot be worked around: it is a global
// that keeps whatever it is given, and an entry made inside a turn would point
// at storage the turn is about to take back.
func (g *gen) storesNothingGlobal(e ast.Expr, sc *scope, seen map[string]bool) bool {
	for _, name := range g.reachable(e, sc, seen) {
		if g.memoed[name] {
			return false
		}
	}
	return true
}

// topValuesReached lists the top-level values the loop body can force, so that
// they can be built before the region opens.
//
// A top-level value is a memoised accessor: the first mention builds it and
// keeps it in a global. Built inside a region, that global would point at
// storage the turn is about to forget — and would go on pointing there for the
// rest of the program. Forcing them first puts them outside every region, which
// is where they were always going to end up.
func (g *gen) topValuesReached(e ast.Expr, sc *scope) []string {
	var out []string
	for _, name := range g.reachable(e, sc, map[string]bool{}) {
		if g.topVals[name] {
			out = append(out, name)
		}
	}
	return out
}
