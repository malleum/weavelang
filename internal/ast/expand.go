package ast

import (
	"fmt"
	"strconv"
	"strings"
)

// Expanding a top-level definition that takes its value apart.
//
//	(width, height) is dimsOf Source
//
// binds two names from one expression, and every phase after the parser is
// built around a definition being one name. So it becomes three:
//
//	<whole 0> is dimsOf Source
//	width     is ward <whole 0> ((width, height) : width)
//	height    is ward <whole 0> ((width, height) : height)
//
// which needs nothing new anywhere. A top-level value is a memoised accessor,
// so the expression runs once however many names read it; the dependency order
// falls out of the free variables, since each projection mentions the hidden
// name and nothing else; and the ward carries the exhaustiveness check, so a
// pattern that could fail is reported where a one-armed ward would be.
//
// This runs between parsing and checking rather than in the parser, because the
// formatter reads the parse tree and has to print what was written.

// ExpandPatterns rewrites every definition that takes its value apart. It is
// idempotent: a file with none is returned untouched, and an expanded
// definition has no Pat left to expand.
func ExpandPatterns(f *File) {
	expanded := make([]*Decl, 0, len(f.Decls))
	for _, d := range f.Decls {
		if d.Pat == nil {
			expanded = append(expanded, d)
			continue
		}
		expanded = append(expanded, projections(d)...)
	}
	f.Decls = expanded
}

// projections is the hidden definition holding the value, then one definition
// per name the pattern binds.
func projections(d *Decl) []*Decl {
	whole := &Decl{
		Name:    d.Name,
		NamePos: d.NamePos,
		Clauses: d.Clauses,
		// The value is what `weave trace` reports for the line, under the
		// pattern that was written — one record showing the whole Twine or
		// Thread rather than one per name pulled out of it.
		Display: PatternText(d.Pat),
	}
	out := []*Decl{whole}

	bound := map[string]bool{}
	BindPatternVars(d.Pat, bound)
	for _, name := range boundInOrder(d.Pat) {
		if !bound[name] {
			continue // a name the pattern mentions twice
		}
		delete(bound, name)
		out = append(out, &Decl{
			Name:    name,
			NamePos: d.NamePos,
			Hidden:  true,
			Clauses: []*Clause{{
				Body: &Ward{
					Subject: &Var{Name: whole.Name, P: d.NamePos},
					Arms:    []*Arm{{Pat: d.Pat, Body: &Var{Name: name, P: d.NamePos}, P: d.NamePos}},
					P:       d.NamePos,
					Binding: true,
				},
				ClauseP: d.NamePos,
			}},
		})
	}
	return out
}

// boundInOrder lists the names a pattern binds, left to right, so that the
// definitions come out in the order they were written.
func boundInOrder(p Pattern) []string {
	var out []string
	var walk func(Pattern)
	walk = func(p Pattern) {
		switch p := p.(type) {
		case *PVar:
			out = append(out, p.Name)
		case *PTwine:
			for _, e := range p.Elems {
				walk(e)
			}
		case *PThread:
			for _, e := range p.Elems {
				walk(e)
			}
			if p.Rest != nil {
				walk(p.Rest)
			}
		case *PCtor:
			for _, e := range p.Args {
				walk(e)
			}
		}
	}
	walk(p)
	return out
}

// PatternText renders a pattern the way it was written, for a diagnostic or a
// trace label. It is deliberately not the dump form patternString produces —
// that is for reading a tree, and spells a Twine `(, a b)`.
//
// The formatter has its own renderer, because it also has to choose between the
// two spellings of the hole words and cannot depend on this package's opinion.
// This one is the plain source form and nothing else.
func PatternText(p Pattern) string {
	switch p := p.(type) {
	case *PWild:
		return "_"
	case *PVar:
		return p.Name
	case *PInt:
		return strconv.FormatInt(p.Value, 10)
	case *PFloat:
		return strconv.FormatFloat(p.Value, 'g', -1, 64)
	case *PChar:
		return fmt.Sprintf("'%c'", p.Value)
	case *PText:
		return strconv.Quote(p.Value)
	case *PCtor:
		if len(p.Args) == 0 {
			return p.Name
		}
		parts := make([]string, len(p.Args))
		for i, a := range p.Args {
			parts[i] = PatternText(a)
		}
		return "(" + p.Name + " " + strings.Join(parts, " ") + ")"
	case *PTwine:
		parts := make([]string, len(p.Elems))
		for i, e := range p.Elems {
			parts[i] = PatternText(e)
		}
		return "(" + strings.Join(parts, ", ") + ")"
	case *PThread:
		parts := make([]string, 0, len(p.Elems)+1)
		for _, e := range p.Elems {
			parts = append(parts, PatternText(e))
		}
		if p.Rest != nil {
			parts = append(parts, ".."+PatternText(p.Rest))
		}
		return "[" + strings.Join(parts, " ") + "]"
	}
	return "?"
}
