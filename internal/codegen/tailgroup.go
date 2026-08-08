package codegen

import (
	"fmt"
	"strings"

	"github.com/malleum/weave/internal/ast"
)

// Grouping definitions that tail-call each other.
//
// The tail-call graph has an edge from f to g when f calls g in tail position
// with the right number of arguments. A strongly connected component of that
// graph is a set of definitions that can hand control to one another forever,
// so it is exactly the set that has to become one loop.
//
// Most components have one member, and one member with no self-edge needs no
// loop at all — which is the ordinary case, and stays exactly as it was.

// tailGroup is one component of the tail-call graph, in the order the members
// were declared.
type tailGroup struct {
	members []*ast.Decl
	// loops is false for the common case: a single definition that never calls
	// itself in tail position, and so needs no loop around its body.
	loops bool
}

// tailGroups partitions the file's functions into components, keeping
// declaration order both within a group and between groups.
func tailGroups(decls []*ast.Decl, arity map[string]int) []tailGroup {
	fns := make([]*ast.Decl, 0, len(decls))
	index := map[string]int{}
	for _, d := range decls {
		if d.Arity() == 0 {
			continue
		}
		index[d.Name] = len(fns)
		fns = append(fns, d)
	}

	// Edges, restricted to saturated calls of the file's own functions.
	edges := make([][]int, len(fns))
	selfEdge := make([]bool, len(fns))
	for i, d := range fns {
		for name, argc := range tailCallees(d) {
			j, ok := index[name]
			if !ok || arity[name] != argc {
				continue
			}
			edges[i] = append(edges[i], j)
			if j == i {
				selfEdge[i] = true
			}
		}
	}

	var groups []tailGroup
	for _, comp := range stronglyConnected(len(fns), edges) {
		g := tailGroup{loops: len(comp) > 1}
		for _, i := range comp {
			g.members = append(g.members, fns[i])
			if selfEdge[i] {
				g.loops = true
			}
		}
		groups = append(groups, g)
	}
	return groups
}

// stronglyConnected returns the components of a graph, each as a sorted list of
// vertices, with the components themselves ordered by their first vertex. Order
// is fixed so that the generated C does not move around between builds.
func stronglyConnected(n int, edges [][]int) [][]int {
	const unvisited = -1

	index := make([]int, n)
	low := make([]int, n)
	onStack := make([]bool, n)
	for i := range index {
		index[i] = unvisited
	}
	var stack []int
	next := 0
	var out [][]int

	var strongConnect func(v int)
	strongConnect = func(v int) {
		index[v], low[v] = next, next
		next++
		stack = append(stack, v)
		onStack[v] = true

		for _, w := range edges[v] {
			switch {
			case index[w] == unvisited:
				strongConnect(w)
				low[v] = min(low[v], low[w])
			case onStack[w]:
				low[v] = min(low[v], index[w])
			}
		}

		if low[v] != index[v] {
			return
		}
		var comp []int
		for {
			w := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			onStack[w] = false
			comp = append(comp, w)
			if w == v {
				break
			}
		}
		sortInts(comp)
		out = append(out, comp)
	}

	for v := 0; v < n; v++ {
		if index[v] == unvisited {
			strongConnect(v)
		}
	}

	sortComponents(out)
	return out
}

func sortInts(xs []int) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

func sortComponents(cs [][]int) {
	for i := 1; i < len(cs); i++ {
		for j := i; j > 0 && cs[j][0] < cs[j-1][0]; j-- {
			cs[j], cs[j-1] = cs[j-1], cs[j]
		}
	}
}

// ------------------------------------------------------------------ emission

// emitMergedGroup compiles a set of mutually tail-recursive definitions into
// one C function, so a tail call between them is a jump. Each member keeps its
// own entry point, which enters the loop at that member's index, so nothing
// about an ordinary call changes.
func (g *gen) emitMergedGroup(group tailGroup) {
	loop := fmt.Sprintf("wu__loop%d", g.fresh())

	member := map[string]int{}
	arity := map[string]int{}
	width := 0
	for i, d := range group.members {
		member[d.Name] = i
		arity[d.Name] = d.Arity()
		if d.Arity() > width {
			width = d.Arity()
		}
	}

	g.decls = append(g.decls, fmt.Sprintf("static Value %s(int which, Value *slots);", loop))

	// Each member's entry point fills the shared slots from its own arguments
	// and enters the loop at its own index. The copy happens here rather than
	// inside the loop because the members' arities differ: only the entering
	// one knows how many arguments its caller actually passed.
	for i, d := range group.members {
		cname := g.cnames[d.Name]
		if d.Memo {
			g.emitMemo(d, cname)
			cname += "_body"
		}
		g.decls = append(g.decls, fmt.Sprintf("static Value %s(Value *env, Value *args);", cname))

		var copies strings.Builder
		for j := 0; j < d.Arity(); j++ {
			fmt.Fprintf(&copies, "  slots[%d] = args[%d];\n", j, j)
		}
		g.defs = append(g.defs, fmt.Sprintf(
			"static Value %s(Value *env, Value *args) {\n  (void)env;\n"+
				"  Value slots[%d];\n%s  return %s(%d, slots);\n}\n",
			cname, width, copies.String(), loop, i))
	}

	slots := make([]string, width)
	for i := range slots {
		slots[i] = fmt.Sprintf("slots[%d]", i)
	}

	b := &body{g: g}
	b.open("static Value %s(int which, Value *slots) {", loop)
	b.open("for (;;) {")
	b.open("switch (which) {")
	for i, d := range group.members {
		b.open("case %d: {", i)
		ti := &tailInfo{
			name:   d.Name,
			params: slots[:d.Arity()],
			owned:  make([]bool, d.Arity()),
			// In-place updating is not attempted across a merged group: proving
			// a grid is threaded without duplication would have to hold for
			// every path through every member, not one loop.
			ownedVars: map[string]bool{},
			which:     "which",
			member:    member,
			arity:     arity,
			slots:     slots,
		}
		g.emitClauses(b, d, slots[:d.Arity()], ti)
		b.close("}")
	}
	b.close("}")
	b.close("}")
	// Unreachable: every case returns or continues.
	b.line("return w_earth(0);")
	b.close("}")
	g.defs = append(g.defs, b.sb.String())
}
