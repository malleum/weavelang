// Package check performs name resolution, type inference and exhaustiveness
// checking over a parsed Weave file.
//
// Inference is Hindley–Milner with level-based generalisation. Top-level
// definitions are grouped into strongly connected components and inferred
// dependency-first, so that a definition is generalised before its users are
// checked — that is what lets `bend` be used at two different element types in
// one program while still catching real mistakes.
package check

import (
	"fmt"
	"sort"
	"strings"

	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/parser"
	"github.com/malleum/weave/internal/prelude"
	"github.com/malleum/weave/internal/token"
	"github.com/malleum/weave/internal/types"
)

// Info is what checking produces for later phases.
type Info struct {
	// Types records the inferred type of every expression, for the backend.
	Types map[ast.Expr]types.Type
	// Decls holds the generalised type of each top-level definition.
	Decls map[string]*types.Scheme
	// Output is the type of the program's result expression, if it has one.
	Output types.Type
}

// ctorInfo is a data constructor's arity and type.
type ctorInfo struct {
	prelude.Ctor
	Scheme *types.Scheme
}

// checker carries the state of one compilation.
type checker struct {
	bag   *diag.Bag
	alloc *types.Alloc
	level int

	global *scope
	ctors  map[string]*ctorInfo
	info   *Info

	// typeArity is builtinTypeArity extended with the file's own sum types,
	// and typeCtors is prelude.TypeCtors likewise, so exhaustiveness sees
	// declared types on the same footing as Hold and Weaving.
	typeArity map[string]int
	typeCtors map[string][]string
	// derived holds each declared type's inherited Talents, so that every Con
	// built for it — in a signature as much as in a constructor — agrees.
	derived map[string]types.TalentSet
	// numbers are the variables standing in for number literals that have not
	// been decided yet. See settleNumbers.
	numbers []*types.Var
}

// File type-checks f, reporting problems into bag.
func File(f *ast.File, bag *diag.Bag) *Info {
	c := &checker{
		bag:   bag,
		alloc: &types.Alloc{},
		ctors: map[string]*ctorInfo{},
		info: &Info{
			Types: map[ast.Expr]types.Type{},
			Decls: map[string]*types.Scheme{},
		},
	}
	c.typeArity = make(map[string]int, len(builtinTypeArity))
	for n, a := range builtinTypeArity {
		c.typeArity[n] = a
	}
	c.typeCtors = make(map[string][]string, len(prelude.TypeCtors))
	for n, cs := range prelude.TypeCtors {
		c.typeCtors[n] = cs
	}
	c.global = newScope(nil)
	c.loadPrelude()
	c.loadTypes(f.Types)
	c.inferTopLevel(f)
	return c.info
}

// ------------------------------------------------------------------- scopes

type scope struct {
	parent *scope
	names  map[string]*types.Scheme
}

func newScope(parent *scope) *scope {
	return &scope{parent: parent, names: map[string]*types.Scheme{}}
}

func (s *scope) child() *scope { return newScope(s) }

func (s *scope) bind(name string, sch *types.Scheme) { s.names[name] = sch }

func (s *scope) lookup(name string) (*types.Scheme, bool) {
	for e := s; e != nil; e = e.parent {
		if sch, ok := e.names[name]; ok {
			return sch, true
		}
	}
	return nil, false
}

// visible collects every name in scope, for "did you mean" suggestions.
func (s *scope) visible() []string {
	seen := map[string]bool{}
	var out []string
	for e := s; e != nil; e = e.parent {
		for n := range e.names {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
			}
		}
	}
	sort.Strings(out)
	return out
}

// ------------------------------------------------------------------ prelude

func (c *checker) loadPrelude() {
	for _, e := range prelude.Values {
		sch, err := c.schemeFromSig(e.Sig, e.Where)
		if err != nil {
			panic(fmt.Sprintf("prelude entry %q: %v", e.Name, err))
		}
		c.global.bind(e.Name, sch)
	}
	for _, ct := range prelude.Ctors {
		sch, err := c.schemeFromSig(ct.Sig, "")
		if err != nil {
			panic(fmt.Sprintf("prelude constructor %q: %v", ct.Name, err))
		}
		c.ctors[ct.Name] = &ctorInfo{Ctor: ct, Scheme: sch}
		// Constructors are also ordinary values: `Held` on its own is a
		// function, and `bend Held xs` must work.
		c.global.bind(ct.Name, sch)
	}
}

// schemeFromSig parses a prelude signature and generalises it.
func (c *checker) schemeFromSig(sig, where string) (*types.Scheme, error) {
	te, err := parser.ParseTypeString(sig)
	if err != nil {
		return nil, err
	}
	vars := map[string]*types.Var{}
	body := c.typeFromAST(te, vars, 1, nil)
	if err := applyConstraints(where, vars); err != nil {
		return nil, err
	}
	return types.Generalize(0, body), nil
}

// applyConstraints attaches Talent constraints such as "Ord a" to the named
// type variables of a signature.
func applyConstraints(where string, vars map[string]*types.Var) error {
	for _, clause := range strings.Split(where, ",") {
		clause = strings.TrimSpace(clause)
		if clause == "" {
			continue
		}
		fields := strings.Fields(clause)
		if len(fields) != 2 {
			return fmt.Errorf("malformed constraint %q", clause)
		}
		set, ok := talentByName(fields[0])
		if !ok {
			return fmt.Errorf("unknown Talent %q", fields[0])
		}
		v, ok := vars[fields[1]]
		if !ok {
			return fmt.Errorf("constraint names unknown type variable %q", fields[1])
		}
		v.Talents |= set
	}
	return nil
}

func talentByName(name string) (types.TalentSet, bool) {
	switch name {
	case "Eq":
		return types.Eq, true
	case "Ord":
		// Ordering implies equality.
		return types.Ord | types.Eq, true
	case "Show":
		return types.Show, true
	case "Reckon":
		return types.Reckon, true
	case "Bulk":
		return types.Bulk, true
	}
	return 0, false
}

// ------------------------------------------------------------- type syntax

// builtinTypeArity records how many arguments each built-in type constructor
// takes. A file's own sum types are added to the checker's copy.
var builtinTypeArity = map[string]int{
	types.Earth: 0, types.Water: 0, types.Fire: 0, types.Air: 0,
	types.Spirit: 0, types.KnotCon: 0,
	types.ThreadCon: 1, types.PatternCon: 1, types.CircleCon: 1,
	types.TaverenCon: 1, types.HoldCon: 1,
	types.WebCon: 2, types.WeavingCon: 2,
}

// typeFromAST converts written type syntax into a type. Lower-case names
// become type variables, shared through vars. When bag is non-nil, unknown
// constructors and arity mistakes are reported.
func (c *checker) typeFromAST(te *ast.TypeExpr, vars map[string]*types.Var, level int, bag *diag.Bag) types.Type {
	if te == nil {
		return c.alloc.Fresh(level)
	}
	var head types.Type

	switch {
	case te.Name == ast.FuncTypeName:
		if len(te.Args) != 2 {
			return c.alloc.Fresh(level)
		}
		return &types.Fn{
			From: c.typeFromAST(te.Args[0], vars, level, bag),
			To:   c.typeFromAST(te.Args[1], vars, level, bag),
		}

	case te.Name == ast.TwineTypeName:
		args := make([]types.Type, len(te.Args))
		for i, a := range te.Args {
			args[i] = c.typeFromAST(a, vars, level, bag)
		}
		head = &types.Con{Name: types.TwineCon, Args: args}

	case isTypeVarName(te.Name):
		v, ok := vars[te.Name]
		if !ok {
			v = c.alloc.Fresh(level)
			vars[te.Name] = v
		}
		if len(te.Args) > 0 && bag != nil {
			bag.Add(te.P, "type variable `%s` cannot take arguments", te.Name)
		}
		head = v

	default:
		args := make([]types.Type, len(te.Args))
		for i, a := range te.Args {
			args[i] = c.typeFromAST(a, vars, level, bag)
		}
		if want, known := c.typeArity[te.Name]; !known {
			if bag != nil {
				bag.AddHint(te.P, suggest(te.Name, c.knownTypeNames()),
					"unknown type `%s`", te.Name)
			}
			head = c.alloc.Fresh(level)
		} else if len(args) != want {
			if bag != nil {
				bag.Add(te.P, "`%s` takes %d type argument(s), but was given %d",
					te.Name, want, len(args))
			}
			head = c.alloc.Fresh(level)
		} else {
			head = &types.Con{Name: te.Name, Args: args, Derived: c.derived[te.Name]}
		}
	}

	return head
}

func isTypeVarName(name string) bool {
	return name != "" && name[0] >= 'a' && name[0] <= 'z'
}

func (c *checker) knownTypeNames() []string {
	out := make([]string, 0, len(c.typeArity))
	for n := range c.typeArity {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------- top level

func (c *checker) inferTopLevel(f *ast.File) {
	byName := map[string]*ast.Decl{}
	for _, d := range f.Decls {
		if prev, dup := byName[d.Name]; dup && prev != d {
			c.bag.Add(d.NamePos, "`%s` is defined twice", d.Name)
			continue
		}
		byName[d.Name] = d
	}

	for _, group := range sccGroups(f.Decls, byName) {
		c.inferGroup(group, byName)
	}

	// Every bare expression is checked and has to be showable, since `weave
	// trace` renders each of them. Only the last is the program's output.
	for _, out := range f.Outputs {
		mark := len(c.numbers)
		t := c.infer(out, c.global)
		c.settleNumbers(mark)
		c.require(out.Pos(), t, types.Show, "a program's output")
		c.info.Output = t
	}
}

// settleNumbers commits the number literals recorded since mark.
//
// `1` starts life as a variable so that it can be a Water where the rest of
// the definition is Water — `fact n is mul n (fact (sub n 1))` over Waters
// used to fail at the `1`, two lines from anything the reader wrote wrong.
// Anything unification did not decide is an Earth, which is both what a reader
// expects and what the language did before literals moved at all.
//
// It runs per definition rather than at the end so that nothing is generalised
// while still undecided: a value of type `a where Reckon a` has no
// representation to compile.
func (c *checker) settleNumbers(mark int) {
	for _, v := range c.numbers[mark:] {
		if _, stillOpen := types.Resolve(v).(*types.Var); stillOpen {
			_ = types.Unify(v, types.TEarth)
		}
	}
	c.numbers = c.numbers[:mark]
}

// inferGroup infers one mutually recursive set of definitions together, then
// generalises them as a unit.
func (c *checker) inferGroup(group []*ast.Decl, byName map[string]*ast.Decl) {
	c.level++
	mark := len(c.numbers)

	// Assume a monomorphic type for each member while inferring the group.
	assumed := map[string]types.Type{}
	for _, d := range group {
		var t types.Type
		if d.Sig != nil {
			vars := map[string]*types.Var{}
			t = c.typeFromAST(d.Sig, vars, c.level, c.bag)
		} else {
			t = c.alloc.Fresh(c.level)
		}
		assumed[d.Name] = t
		c.global.bind(d.Name, types.Mono(t))
	}

	for _, d := range group {
		c.inferDecl(d, assumed[d.Name])
	}

	// Before generalising: a literal that nothing pinned down is an Earth.
	c.settleNumbers(mark)

	c.level--

	for _, d := range group {
		sch := types.Generalize(c.level, assumed[d.Name])
		c.global.bind(d.Name, sch)
		c.info.Decls[d.Name] = sch
		c.checkMemo(d, assumed[d.Name])
	}
}

// checkMemo validates a `remember` marker. A memo table is a Web keyed on the
// arguments, so every argument must have Eq; and a definition without
// arguments is computed once already, which makes the marker a mistake worth
// pointing at rather than ignoring.
func (c *checker) checkMemo(d *ast.Decl, t types.Type) {
	if !d.Memo {
		return
	}
	if d.Arity() == 0 {
		c.bag.AddHint(d.NamePos,
			"a definition with no arguments is already computed once, the first time it is used",
			"`remember` needs something to remember by: `%s` takes no arguments", d.Name)
		return
	}
	for i := 0; i < d.Arity(); i++ {
		fn, ok := types.Resolve(t).(*types.Fn)
		if !ok {
			return
		}
		if err := types.Require(fn.From, types.Eq); err != nil {
			c.bag.AddHint(d.NamePos,
				"a remembered call is looked up by its arguments, so each of them has to be comparable",
				"`%s` cannot be remembered: %s", d.Name, err)
			return
		}
		t = fn.To
	}
}

// inferDecl checks every clause of a definition against its assumed type.
func (c *checker) inferDecl(d *ast.Decl, self types.Type) {
	for _, cl := range d.Clauses {
		inner := c.global.child()
		params := make([]types.Type, len(cl.Params))
		for i, p := range cl.Params {
			params[i] = c.alloc.Fresh(c.level)
			c.checkPattern(p, params[i], inner)
		}
		body := c.infer(cl.Body, inner)
		c.unify(cl.Pos(), self, types.Func(body, params...),
			fmt.Sprintf("this clause of `%s`", d.Name))
	}

	if len(d.Clauses) > 0 && d.Arity() > 0 {
		c.checkClauseCoverage(d, self)
	}
}

// --------------------------------------------------------------- expressions

func (c *checker) infer(e ast.Expr, sc *scope) types.Type {
	t := c.inferRaw(e, sc)
	c.info.Types[e] = t
	return t
}

func (c *checker) inferRaw(e ast.Expr, sc *scope) types.Type {
	switch e := e.(type) {
	case *ast.IntLit:
		// A number literal is not committed to a Power yet: `1` in a Water
		// function is a Water. It is a fresh variable with the Reckon Talent —
		// which only Earth and Water have — and it settles on Earth if nothing
		// in the definition decides otherwise. See settleNumbers.
		v := c.alloc.Fresh(c.level)
		if err := types.Require(v, types.Reckon); err != nil {
			c.bag.Add(e.Pos(), "%s", err)
		}
		c.numbers = append(c.numbers, v)
		return v
	case *ast.FloatLit:
		return types.TWater
	case *ast.CharLit:
		return types.TFire
	case *ast.TextLit:
		return types.TAir

	case *ast.Var:
		sch, ok := sc.lookup(e.Name)
		if !ok {
			c.bag.AddHint(e.P, suggest(e.Name, sc.visible()), "cannot find `%s`", e.Name)
			return c.alloc.Fresh(c.level)
		}
		return c.alloc.Instantiate(sch, c.level)

	case *ast.Ctor:
		sch, ok := sc.lookup(e.Name)
		if !ok {
			c.bag.AddHint(e.P, suggest(e.Name, c.ctorNames()), "cannot find constructor `%s`", e.Name)
			return c.alloc.Fresh(c.level)
		}
		return c.alloc.Instantiate(sch, c.level)

	case *ast.App:
		return c.inferApp(e, sc)

	case *ast.Lambda:
		inner := sc.child()
		params := make([]types.Type, len(e.Params))
		for i, p := range e.Params {
			params[i] = c.alloc.Fresh(c.level)
			c.checkPattern(p, params[i], inner)
		}
		return types.Func(c.infer(e.Body, inner), params...)

	case *ast.Let:
		inner := sc
		for _, b := range e.Binds {
			inner = c.inferBind(b, inner)
		}
		return c.infer(e.Body, inner)

	case *ast.Ward:
		return c.inferWard(e, sc)

	case *ast.ThreadLit:
		elem := c.alloc.Fresh(c.level)
		for _, el := range e.Elems {
			c.unify(el.Pos(), elem, c.infer(el, sc), "this Thread element")
		}
		return types.Thread(elem)

	case *ast.TwineLit:
		elems := make([]types.Type, len(e.Elems))
		for i, el := range e.Elems {
			elems[i] = c.infer(el, sc)
		}
		return types.Twine(elems...)

	case *ast.WebLit:
		k := c.alloc.Fresh(c.level)
		v := c.alloc.Fresh(c.level)
		for _, pair := range e.Pairs {
			c.unify(pair.Key.Pos(), k, c.infer(pair.Key, sc), "this Web key")
			c.unify(pair.Val.Pos(), v, c.infer(pair.Val, sc), "this Web value")
		}
		c.require(e.P, k, types.Eq, "a Web key")
		return &types.Con{Name: types.WebCon, Args: []types.Type{k, v}}

	case *ast.Bad:
		return c.alloc.Fresh(c.level)
	}

	return c.alloc.Fresh(c.level)
}

// inferBind handles one `weave` or `channel` binding, generalising it so that
// later code can use it at several types.
func (c *checker) inferBind(b *ast.Bind, sc *scope) *scope {
	inner := sc.child()
	c.level++

	self := c.alloc.Fresh(c.level)
	// Bind the name before inferring the value so a `channel` can recurse.
	inner.bind(b.Name, types.Mono(self))

	var t types.Type
	if len(b.Params) > 0 {
		body := inner.child()
		params := make([]types.Type, len(b.Params))
		for i, p := range b.Params {
			params[i] = c.alloc.Fresh(c.level)
			c.checkPattern(p, params[i], body)
		}
		t = types.Func(c.infer(b.Value, body), params...)
	} else {
		t = c.infer(b.Value, inner)
	}
	c.unify(b.Pos(), self, t, fmt.Sprintf("the binding `%s`", b.Name))

	c.level--

	out := sc.child()
	out.bind(b.Name, types.Generalize(c.level, self))
	return out
}

// inferApp types a call: the callee, then one argument at a time.
func (c *checker) inferApp(e *ast.App, sc *scope) types.Type {
	fnT := c.infer(e.Fn, sc)
	for _, arg := range e.Args {
		argT := c.infer(arg, sc)
		res := c.alloc.Fresh(c.level)
		if err := types.Unify(fnT, &types.Fn{From: argT, To: res}); err != nil {
			c.reportApply(e, arg, fnT, argT, err)
			return c.alloc.Fresh(c.level)
		}
		fnT = res
	}
	return fnT
}

// reportApply turns a failed application into a diagnostic that names the
// callee where it can.
func (c *checker) reportApply(app *ast.App, arg ast.Expr, fnT, argT types.Type, err error) {
	callee := calleeName(app.Fn)
	at := blame(app, arg)

	// A Talent violation carries its own explanation, which is far more useful
	// than the shapes that failed to line up.
	var mismatch *types.MismatchError
	if asMismatch(err, &mismatch) && mismatch.Detail != "" {
		// Unless the thing without the Talent is a function, which means the
		// callee was never one — applying a number, say. Saying so is the
		// point; which Talent a function lacks is not.
		if !strings.HasPrefix(mismatch.Detail, "a function has no") {
			c.bag.Add(at, "%s: %s", callee, mismatch.Detail)
			return
		}
		c.bag.AddHint(at, "only a function can be applied to an argument",
			"%s is not a function", callee)
		return
	}

	if fn, ok := types.Resolve(fnT).(*types.Fn); ok {
		// A `_` binds to the brackets nearest it, so a function turning up
		// where a value belongs usually means the brackets were the inner
		// ones. Saying so is worth more than the two types.
		if isHoleLambda(arg) {
			c.bag.AddHint(at,
				"a `_` stands for the argument of the brackets closest to it, so nesting one call inside another splits them up: pipe instead — `(mod _ 3 | eq 0)` — or name the argument with `(x : ...)`",
				"%s expects %s here, but the `_` made these brackets a function",
				callee, types.String(fn.From))
			return
		}
		c.bag.Add(at, "%s expects %s here, but found %s",
			callee, types.String(fn.From), types.String(argT))
		return
	}
	if _, isVar := types.Resolve(fnT).(*types.Var); !isVar {
		c.bag.AddHint(at, fmt.Sprintf("%s is not a function", callee),
			"cannot apply %s to an argument", types.String(fnT))
		return
	}
	c.bag.Add(at, "%s", err)
}

// blame picks where to point a failed application.
//
// Normally that is the offending argument. In a pipeline it is not: the
// argument is everything to the left, so `Source | fires | pairs | sift p`
// blamed `Source` — a caret under the first stage for a mistake in the last.
// The stage that could not accept what it was handed is the interesting end,
// so a piped call is blamed on its own position, which the parser records as
// the verb when the stage has arguments and as the `|` or `through` before it
// when the stage is a bare name.
func blame(app *ast.App, arg ast.Expr) token.Pos {
	if app.Via != "" && len(app.Args) > 0 && app.Args[len(app.Args)-1] == arg {
		return app.P
	}
	return arg.Pos()
}

// isHoleLambda reports whether an expression is the function a `_` produced,
// rather than one the program wrote out.
func isHoleLambda(e ast.Expr) bool {
	lam, ok := e.(*ast.Lambda)
	if !ok || len(lam.Params) != 1 {
		return false
	}
	v, ok := lam.Params[0].(*ast.PVar)
	return ok && v.Name == ast.HoleName
}

func calleeName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.Var:
		return "`" + e.Name + "`"
	case *ast.Ctor:
		return "`" + e.Name + "`"
	default:
		return "this call"
	}
}

// inferWard types a pattern match and checks that it is exhaustive.
func (c *checker) inferWard(e *ast.Ward, sc *scope) types.Type {
	subject := c.infer(e.Subject, sc)
	result := c.alloc.Fresh(c.level)

	for _, arm := range e.Arms {
		inner := sc.child()
		c.checkPattern(arm.Pat, subject, inner)
		body := c.infer(arm.Body, inner)
		c.unify(arm.Body.Pos(), result, body, "this ward arm")
	}

	c.checkWardCoverage(e, subject)
	return result
}

// ------------------------------------------------------------------ patterns

// checkPattern checks p against want and binds any variables it introduces.
func (c *checker) checkPattern(p ast.Pattern, want types.Type, sc *scope) {
	switch p := p.(type) {
	case *ast.PWild, *ast.PBad:
		return

	case *ast.PVar:
		if _, exists := sc.names[p.Name]; exists {
			c.bag.Add(p.P, "`%s` is bound twice in the same pattern", p.Name)
		}
		sc.bind(p.Name, types.Mono(want))

	case *ast.PInt:
		c.unify(p.P, want, types.TEarth, "this pattern")
	case *ast.PFloat:
		c.unify(p.P, want, types.TWater, "this pattern")
	case *ast.PChar:
		c.unify(p.P, want, types.TFire, "this pattern")
	case *ast.PText:
		c.unify(p.P, want, types.TAir, "this pattern")

	case *ast.PTwine:
		elems := make([]types.Type, len(p.Elems))
		for i := range p.Elems {
			elems[i] = c.alloc.Fresh(c.level)
		}
		c.unify(p.P, want, types.Twine(elems...), "this Twine pattern")
		for i, sub := range p.Elems {
			c.checkPattern(sub, elems[i], sc)
		}

	case *ast.PThread:
		// Every element has the same type, and the rest is a Thread of it.
		elem := c.alloc.Fresh(c.level)
		c.unify(p.P, want, types.Thread(elem), "this Thread pattern")
		for _, sub := range p.Elems {
			c.checkPattern(sub, elem, sc)
		}
		if p.Rest != nil {
			c.checkPattern(p.Rest, types.Thread(elem), sc)
		}

	case *ast.PCtor:
		info, ok := c.ctors[p.Name]
		if !ok {
			c.bag.AddHint(p.P, suggest(p.Name, c.ctorNames()),
				"cannot find constructor `%s`", p.Name)
			return
		}
		if len(p.Args) != info.Arity {
			c.bag.Add(p.P, "`%s` carries %d value(s), but this pattern binds %d",
				p.Name, info.Arity, len(p.Args))
			return
		}
		// Peel the constructor's type apart to learn its field types.
		t := c.alloc.Instantiate(info.Scheme, c.level)
		fields := make([]types.Type, 0, info.Arity)
		for range p.Args {
			fn, ok := types.Resolve(t).(*types.Fn)
			if !ok {
				break
			}
			fields = append(fields, fn.From)
			t = fn.To
		}
		c.unify(p.P, want, t, fmt.Sprintf("the pattern `%s`", p.Name))
		for i, sub := range p.Args {
			if i < len(fields) {
				c.checkPattern(sub, fields[i], sc)
			}
		}
	}
}

func (c *checker) ctorNames() []string {
	out := make([]string, 0, len(c.ctors))
	for n := range c.ctors {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// -------------------------------------------------------------- diagnostics

func (c *checker) unify(pos token.Pos, want, got types.Type, what string) {
	if err := types.Unify(want, got); err != nil {
		var mism *types.MismatchError
		if ok := asMismatch(err, &mism); ok && mism.Detail != "" {
			c.bag.Add(pos, "%s: %s", what, mism.Detail)
			return
		}
		c.bag.Add(pos, "%s is %s, but %s was expected",
			what, types.String(got), types.String(want))
	}
}

func asMismatch(err error, out **types.MismatchError) bool {
	if m, ok := err.(*types.MismatchError); ok {
		*out = m
		return true
	}
	return false
}

func (c *checker) require(pos token.Pos, t types.Type, want types.TalentSet, what string) {
	if err := types.Require(t, want); err != nil {
		c.bag.AddHint(pos, fmt.Sprintf("%s needs the %s Talent", what, want), "%s", err)
	}
}

// suggest returns a "did you mean" hint for an unknown name.
func suggest(name string, candidates []string) string {
	best, bestDist := "", 3 // only suggest genuinely close names
	for _, cand := range candidates {
		if d := editDistance(name, cand); d < bestDist {
			best, bestDist = cand, d
		}
	}
	if best == "" {
		return ""
	}
	return fmt.Sprintf("did you mean `%s`?", best)
}

func editDistance(a, b string) int {
	if len(a) > len(b) {
		a, b = b, a
	}
	prev := make([]int, len(a)+1)
	cur := make([]int, len(a)+1)
	for i := range prev {
		prev[i] = i
	}
	for j := 1; j <= len(b); j++ {
		cur[0] = j
		for i := 1; i <= len(a); i++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[i] = min3(cur[i-1]+1, prev[i]+1, prev[i-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(a)]
}

func min3(a, b, c int) int {
	if b < a {
		a = b
	}
	if c < a {
		a = c
	}
	return a
}
