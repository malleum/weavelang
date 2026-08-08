package check

import "github.com/malleum/weave/internal/ast"

// sccGroups orders top-level definitions for inference: each group is a set of
// mutually recursive definitions, and groups come out dependency-first, so a
// definition is generalised before anything that uses it is checked.
//
// This is Tarjan's algorithm, which emits a strongly connected component only
// after every component reachable from it, giving the order we want for free.
func sccGroups(decls []*ast.Decl, byName map[string]*ast.Decl) [][]*ast.Decl {
	t := &tarjan{
		byName: byName,
		index:  map[string]int{},
		low:    map[string]int{},
		onPath: map[string]bool{},
		edges:  map[string][]string{},
	}
	for _, d := range decls {
		t.edges[d.Name] = declRefs(d, byName)
	}
	for _, d := range decls {
		if _, seen := t.index[d.Name]; !seen {
			t.visit(d.Name)
		}
	}
	return t.groups
}

type tarjan struct {
	byName map[string]*ast.Decl
	edges  map[string][]string

	next   int
	index  map[string]int
	low    map[string]int
	stack  []string
	onPath map[string]bool
	groups [][]*ast.Decl
}

func (t *tarjan) visit(name string) {
	t.index[name] = t.next
	t.low[name] = t.next
	t.next++
	t.stack = append(t.stack, name)
	t.onPath[name] = true

	for _, dep := range t.edges[name] {
		if _, seen := t.index[dep]; !seen {
			t.visit(dep)
			t.low[name] = min(t.low[name], t.low[dep])
		} else if t.onPath[dep] {
			t.low[name] = min(t.low[name], t.index[dep])
		}
	}

	if t.low[name] != t.index[name] {
		return
	}
	var group []*ast.Decl
	for {
		top := t.stack[len(t.stack)-1]
		t.stack = t.stack[:len(t.stack)-1]
		t.onPath[top] = false
		if d, ok := t.byName[top]; ok {
			group = append(group, d)
		}
		if top == name {
			break
		}
	}
	if len(group) > 0 {
		t.groups = append(t.groups, group)
	}
}

// declRefs lists the top-level names a definition depends on.
func declRefs(d *ast.Decl, byName map[string]*ast.Decl) []string {
	free := map[string]bool{}
	for _, cl := range d.Clauses {
		bound := map[string]bool{}
		for _, p := range cl.Params {
			ast.BindPatternVars(p, bound)
		}
		ast.FreeVars(cl.Body, bound, free)
	}

	var out []string
	seen := map[string]bool{}
	for name := range free {
		if _, isTop := byName[name]; isTop && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}
