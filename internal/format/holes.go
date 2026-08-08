package format

import (
	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/parser"
	"github.com/malleum/weave/internal/token"
)

// The hole words are the shortest way to write most one-off functions, so the
// formatter reaches for them: `(x : mul x 2)` comes back as `(mul this 2)`.
//
// Whether it can is not a property of the lambda alone. A hole is claimed by
// the brackets closest to it, so an occurrence sitting inside a nested group
// or a pipeline stage would be claimed by that instead and the program would
// mean something else. Rather than enumerate the cases, the rewrite is checked
// by doing it: print the candidate, read it back, and keep it only if what
// comes back is the candidate. That check cannot be fooled by a case nobody
// thought of, which is the point.

// holeSpelling returns the text of e written with the hole words, or "" when
// it has no such spelling or the spelling would not read back.
func (p *printer) holeSpelling(e *ast.Lambda) string {
	// A pipeline is rendered once to measure it and again to break it, so a
	// lambda can be visited twice. The renaming happens in place, so the
	// decision is remembered: a second visit must not read a body the first
	// one has already rewritten and conclude something different.
	if text, seen := p.holeText[e]; seen {
		return text
	}
	text := p.freshHoleSpelling(e)
	if p.holeText == nil {
		p.holeText = map[*ast.Lambda]string{}
	}
	p.holeText[e] = text
	return text
}

func (p *printer) freshHoleSpelling(e *ast.Lambda) string {
	cand, undo := holeCandidate(e)
	if cand == nil {
		return ""
	}
	text := p.expr(cand.Body, precTop)
	if _, tuple := cand.Body.(*ast.TwineLit); !tuple {
		// A Twine literal brings its own brackets, and doubling them would
		// have the inner pair claim the holes.
		text = "(" + text + ")"
	}
	if !readsBackAs(text, cand) {
		// The lambda is printed the long way after all, so it has to be the
		// lambda it was: the renaming happened in place and is put back.
		undo()
		return ""
	}
	return text
}

// holeCandidate renames a lambda's parameters to the hole words, when its
// shape has a spelling in them. The renaming is done in place, so it also
// returns the way to put it back for a candidate that turns out not to read.
func holeCandidate(e *ast.Lambda) (*ast.Lambda, func()) {
	if len(e.Params) == 0 || len(e.Params) > 2 || isHoleLambda(e) {
		return nil, nil
	}
	// The first parameter is either one name or a pair of them; the second, if
	// there is one, has to be a plain name.
	names := map[string]string{}
	params := make([]ast.Pattern, len(e.Params))
	switch first := e.Params[0].(type) {
	case *ast.PVar:
		names[first.Name] = ast.HoleName
		params[0] = &ast.PVar{Name: ast.HoleName, P: first.P}
	case *ast.PTwine:
		if len(first.Elems) != 2 {
			return nil, nil
		}
		a, ok := first.Elems[0].(*ast.PVar)
		if !ok {
			return nil, nil
		}
		b, ok := first.Elems[1].(*ast.PVar)
		if !ok {
			return nil, nil
		}
		names[a.Name] = ast.FormerName
		names[b.Name] = ast.LatterName
		params[0] = pairPattern(first.P)
	default:
		return nil, nil
	}
	if len(e.Params) == 2 {
		second, ok := e.Params[1].(*ast.PVar)
		if !ok {
			return nil, nil
		}
		names[second.Name] = ast.PartnerName
		params[1] = &ast.PVar{Name: ast.PartnerName, P: second.P}
	}
	// A parameter already called one of the words, or a body that mentions one
	// for its own reasons, would have the renaming shadow it.
	back := map[string]string{}
	for from, to := range names {
		if from == to || isHoleWord(from) {
			return nil, nil
		}
		back[to] = from
	}
	// The words cannot already be in the body — nothing binds them and a
	// reference to one would have been claimed elsewhere — but checking is
	// what makes putting the renaming back exact.
	if mentionsHoleWord(e.Body) {
		return nil, nil
	}
	renameVars(e.Body, names)
	undo := func() { renameVars(e.Body, back) }
	return &ast.Lambda{Params: params, Body: e.Body, P: e.P}, undo
}

// pairPattern is the `(former, latter)` a lambda over a Twine becomes.
func pairPattern(pos token.Pos) ast.Pattern {
	return &ast.PTwine{
		Elems: []ast.Pattern{
			&ast.PVar{Name: ast.FormerName, P: pos},
			&ast.PVar{Name: ast.LatterName, P: pos},
		},
		P: pos,
	}
}

func isHoleWord(name string) bool {
	switch name {
	case ast.HoleName, ast.PartnerName, ast.FormerName, ast.LatterName:
		return true
	}
	return false
}

// readsBackAs parses text as an expression and reports whether it is the
// lambda the formatter meant to write.
func readsBackAs(text string, want *ast.Lambda) bool {
	src := "x is " + text + "\nx\n"
	bag := diag.New("fmt.weave", src)
	got := parser.Parse(src, bag)
	if !bag.Empty() || len(got.Decls) != 1 || len(got.Decls[0].Clauses) != 1 {
		return false
	}
	return ast.Dump(got.Decls[0].Clauses[0].Body) == ast.Dump(want)
}

// renameVars rewrites references, in place, wherever a name is being spelled
// as a hole word. It does not descend into a binder that shadows the name,
// since a reference there is to the shadowing binding and not to ours.
func renameVars(e ast.Expr, names map[string]string) {
	switch e := e.(type) {
	case *ast.Var:
		if to, ok := names[e.Name]; ok {
			e.Name = to
		}
	case *ast.App:
		renameVars(e.Fn, names)
		renameAll(e.Args, names)
	case *ast.Lambda:
		renameVars(e.Body, shadowed(names, boundBy(e.Params)))
	case *ast.ThreadLit:
		renameAll(e.Elems, names)
	case *ast.TwineLit:
		renameAll(e.Elems, names)
	case *ast.WebLit:
		for i := range e.Pairs {
			renameVars(e.Pairs[i].Key, names)
			renameVars(e.Pairs[i].Val, names)
		}
	case *ast.Let:
		inner := names
		for _, b := range e.Binds {
			renameVars(b.Value, shadowed(inner, boundBy(b.Params)))
			inner = shadowed(inner, map[string]bool{b.Name: true})
		}
		renameVars(e.Body, inner)
	case *ast.Ward:
		renameVars(e.Subject, names)
		for _, arm := range e.Arms {
			renameVars(arm.Body, shadowed(names, boundBy([]ast.Pattern{arm.Pat})))
		}
	}
}

func renameAll(es []ast.Expr, names map[string]string) {
	for _, e := range es {
		renameVars(e, names)
	}
}

// shadowed drops the names a binder has taken over.
func shadowed(names map[string]string, bound map[string]bool) map[string]string {
	var out map[string]string
	for from := range names {
		if bound[from] {
			if out == nil {
				out = copyNames(names)
			}
			delete(out, from)
		}
	}
	if out == nil {
		return names
	}
	return out
}

func copyNames(names map[string]string) map[string]string {
	out := make(map[string]string, len(names))
	for k, v := range names {
		out[k] = v
	}
	return out
}

func boundBy(pats []ast.Pattern) map[string]bool {
	out := map[string]bool{}
	for _, pat := range pats {
		patternNames(pat, out)
	}
	return out
}

func patternNames(pat ast.Pattern, out map[string]bool) {
	switch pat := pat.(type) {
	case *ast.PVar:
		out[pat.Name] = true
	case *ast.PCtor:
		for _, a := range pat.Args {
			patternNames(a, out)
		}
	case *ast.PTwine:
		for _, el := range pat.Elems {
			patternNames(el, out)
		}
	case *ast.PThread:
		for _, el := range pat.Elems {
			patternNames(el, out)
		}
		if pat.Rest != nil {
			patternNames(pat.Rest, out)
		}
	}
}

// mentionsHoleWord reports whether an expression already uses one of the
// words, either as a reference or as a binder, which the renaming would
// silently capture.
func mentionsHoleWord(e ast.Expr) bool {
	found := false
	walkNames(e, func(name string) {
		if isHoleWord(name) {
			found = true
		}
	})
	return found
}

func walkNames(e ast.Expr, see func(string)) {
	switch e := e.(type) {
	case *ast.Var:
		see(e.Name)
	case *ast.App:
		walkNames(e.Fn, see)
		for _, a := range e.Args {
			walkNames(a, see)
		}
	case *ast.Lambda:
		for name := range boundBy(e.Params) {
			see(name)
		}
		walkNames(e.Body, see)
	case *ast.ThreadLit:
		for _, el := range e.Elems {
			walkNames(el, see)
		}
	case *ast.TwineLit:
		for _, el := range e.Elems {
			walkNames(el, see)
		}
	case *ast.WebLit:
		for _, pair := range e.Pairs {
			walkNames(pair.Key, see)
			walkNames(pair.Val, see)
		}
	case *ast.Let:
		for _, b := range e.Binds {
			see(b.Name)
			for name := range boundBy(b.Params) {
				see(name)
			}
			walkNames(b.Value, see)
		}
		walkNames(e.Body, see)
	case *ast.Ward:
		walkNames(e.Subject, see)
		for _, arm := range e.Arms {
			for name := range boundBy([]ast.Pattern{arm.Pat}) {
				see(name)
			}
			walkNames(arm.Body, see)
		}
	}
}

// walkLambdas visits every lambda in a file, innermost last. It exists for the
// test that checks formatting preserves the program: the formatter renames a
// lambda's parameters when it writes one with the hole words, so the two trees
// are compared with that renaming applied to both.
func walkLambdas(f *ast.File, see func(*ast.Lambda)) {
	var expr func(ast.Expr)
	expr = func(e ast.Expr) {
		switch e := e.(type) {
		case *ast.Lambda:
			see(e)
			expr(e.Body)
		case *ast.App:
			expr(e.Fn)
			for _, a := range e.Args {
				expr(a)
			}
		case *ast.ThreadLit:
			for _, el := range e.Elems {
				expr(el)
			}
		case *ast.TwineLit:
			for _, el := range e.Elems {
				expr(el)
			}
		case *ast.WebLit:
			for _, pair := range e.Pairs {
				expr(pair.Key)
				expr(pair.Val)
			}
		case *ast.Let:
			for _, b := range e.Binds {
				expr(b.Value)
			}
			expr(e.Body)
		case *ast.Ward:
			expr(e.Subject)
			for _, arm := range e.Arms {
				expr(arm.Body)
			}
		}
	}
	for _, d := range f.Decls {
		for _, cl := range d.Clauses {
			expr(cl.Body)
		}
	}
	for _, out := range f.Outputs {
		expr(out)
	}
}
