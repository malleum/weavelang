package check

import (
	"strings"

	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/prelude"
	"github.com/malleum/weave/internal/types"
)

// loadTypes registers the file's sum type declarations: their arities, their
// constructors' schemes, and the set of constructors each type has, which is
// what exhaustiveness checking reads.
//
// It runs in three passes because a declaration may mention any other — types
// are mutually recursive by default, and `Tree a is Leaf | Node (Tree a) a
// (Tree a)` mentions itself. The passes are: names and arities, then field
// types, then the Talents the types derive.
func (c *checker) loadTypes(decls []*ast.TypeDecl) {
	c.derived = map[string]types.TalentSet{}
	if len(decls) == 0 {
		return
	}

	// Pass 1: names and arities, so that pass 2 can resolve forward and
	// recursive references.
	kept := make([]*ast.TypeDecl, 0, len(decls))
	for _, d := range decls {
		if _, taken := c.typeArity[d.Name]; taken {
			c.bag.Add(d.NamePos, "`%s` is already a type", d.Name)
			continue
		}
		if !c.checkParams(d) {
			continue
		}
		c.typeArity[d.Name] = len(d.Params)
		// Optimistic until pass 3 proves otherwise: a recursive type's own
		// fields must not talk it out of the Talents it is being tested for.
		c.derived[d.Name] = derivable
		kept = append(kept, d)
	}

	// Pass 2: field types. Every Con naming a declared type is recorded so
	// pass 3 can update it in place once the fixed point is known.
	type built struct {
		decl   *ast.TypeDecl
		result []types.Type // one per constructor: the type it produces
		fields [][]types.Type
	}
	all := make([]built, 0, len(kept))
	var cons []*types.Con
	for _, d := range kept {
		b := built{decl: d}
		for _, ct := range d.Ctors {
			// Each constructor gets its own copy of the parameters, since each
			// becomes an independently generalised function.
			vars := map[string]*types.Var{}
			args := make([]types.Type, len(d.Params))
			for i, p := range d.Params {
				v := c.alloc.Fresh(1)
				vars[p] = v
				args[i] = v
			}
			result := &types.Con{Name: d.Name, Args: args, Derived: derivable}
			cons = append(cons, result)

			fields := make([]types.Type, len(ct.Fields))
			for i, fe := range ct.Fields {
				c.checkFieldVars(fe, d)
				fields[i] = c.typeFromAST(fe, vars, 1, c.bag)
				cons = append(cons, collectCons(fields[i], c.typeArity, c.derived)...)
			}
			b.result = append(b.result, result)
			b.fields = append(b.fields, fields)
		}
		all = append(all, b)
	}

	// Pass 3: the greatest fixed point of "T derives K if every field of every
	// constructor does". Starting from everything and removing what fails is
	// what makes a recursive type derive Show rather than nothing.
	for changed := true; changed; {
		changed = false
		for _, b := range all {
			have := c.derived[b.decl.Name]
			for _, fields := range b.fields {
				for _, f := range fields {
					if missing, ok := types.Supports(f, have); !ok {
						have &^= missing
					}
				}
			}
			if have != c.derived[b.decl.Name] {
				c.derived[b.decl.Name] = have
				changed = true
			}
		}
	}
	for _, con := range cons {
		con.Derived = c.derived[con.Name]
	}

	// Finally, publish the constructors.
	for _, b := range all {
		names := make([]string, 0, len(b.decl.Ctors))
		for i, ct := range b.decl.Ctors {
			if !c.checkCtorName(ct) {
				continue
			}
			sch := types.Generalize(0, types.Func(b.result[i], b.fields[i]...))
			c.ctors[ct.Name] = &ctorInfo{
				Ctor: prelude.Ctor{
					Name:  ct.Name,
					Sig:   types.String(types.Func(b.result[i], b.fields[i]...)),
					Owner: b.decl.Name,
					Arity: len(ct.Fields),
					Doc:   "a " + b.decl.Name,
				},
				Scheme: sch,
			}
			// Constructors are ordinary values too, so `bend Node xs` works.
			c.global.bind(ct.Name, sch)
			names = append(names, ct.Name)
		}
		c.typeCtors[b.decl.Name] = names
	}
}

// derivable is the set of Talents a sum type can get for free from its fields.
// Reckon and Bulk are not among them: adding two Directions is meaningless, and
// a Direction has no size.
const derivable = types.Eq | types.Ord | types.Show

// checkParams validates a declaration's type parameters.
func (c *checker) checkParams(d *ast.TypeDecl) bool {
	seen := map[string]bool{}
	for _, p := range d.Params {
		if seen[p] {
			c.bag.Add(d.NamePos, "`%s` lists the type parameter `%s` twice", d.Name, p)
			return false
		}
		seen[p] = true
	}
	if len(d.Ctors) == 0 {
		c.bag.Add(d.NamePos, "`%s` declares no constructors", d.Name)
		return false
	}
	return true
}

// checkCtorName rejects a constructor that would shadow an existing one.
func (c *checker) checkCtorName(ct *ast.CtorDecl) bool {
	if ct.Name == "Source" {
		c.bag.Add(ct.NamePos, "`Source` is the program's input, so it cannot name a constructor")
		return false
	}
	if _, taken := c.ctors[ct.Name]; taken {
		c.bag.Add(ct.NamePos, "`%s` is already a constructor", ct.Name)
		return false
	}
	return true
}

// checkFieldVars reports any lower-case name in a field type that the
// declaration does not bind. Accepting one would make the constructor's scheme
// quantify a variable the pattern match cannot recover.
func (c *checker) checkFieldVars(te *ast.TypeExpr, d *ast.TypeDecl) {
	if te == nil {
		return
	}
	if isTypeVarName(te.Name) && !contains(d.Params, te.Name) {
		hint := ""
		if len(d.Params) == 0 {
			hint = "write `" + d.Name + " " + te.Name + " is ...` to make it a parameter"
		} else {
			hint = "`" + d.Name + "` takes " + strings.Join(d.Params, ", ")
		}
		c.bag.AddHint(te.P, hint,
			"`%s` does not take a type parameter named `%s`", d.Name, te.Name)
	}
	for _, a := range te.Args {
		c.checkFieldVars(a, d)
	}
}

func contains(ss []string, s string) bool {
	for _, v := range ss {
		if v == s {
			return true
		}
	}
	return false
}

// collectCons finds every Con in t that names a declared sum type, so its
// Derived set can be filled in once the fixed point settles.
func collectCons(t types.Type, arity map[string]int, user map[string]types.TalentSet) []*types.Con {
	var out []*types.Con
	var walk func(types.Type)
	walk = func(t types.Type) {
		switch t := types.Resolve(t).(type) {
		case *types.Con:
			if _, ok := user[t.Name]; ok {
				out = append(out, t)
			}
			for _, a := range t.Args {
				walk(a)
			}
		case *types.Fn:
			walk(t.From)
			walk(t.To)
		}
	}
	walk(t)
	return out
}
