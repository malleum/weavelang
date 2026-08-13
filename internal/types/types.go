// Package types implements Weave's type representation and unification.
//
// Inference is Hindley–Milner with level-based generalisation (Rémy's
// algorithm): every unification variable records the binding depth at which it
// was created, so generalising a `weave` binding only has to look at the type
// being generalised rather than scan the whole environment.
//
// Talents (Weave's minimal type classes) ride along as constraints attached to
// unification variables. When a constrained variable is unified with a real
// type, that type is checked for the required Talents; when two variables meet,
// their constraint sets merge. Because the backend monomorphises, no dictionary
// passing is needed — the constraints exist only to reject bad programs.
package types

import (
	"fmt"
	"strings"
)

// Type is a Weave type.
type Type interface{ isType() }

// Var is a unification variable. An unbound Var stands for an unknown type; a
// bound one forwards to Ref.
type Var struct {
	ID      int
	Level   int
	Ref     Type // nil while unbound
	Talents TalentSet
}

// Con is the application of a type constructor, such as `Earth`, `Thread a`,
// or `Web k v`. Tuples use the reserved name TwineCon.
type Con struct {
	Name string
	Args []Type
	// Derived carries the Talents a user-declared sum type inherits from its
	// arguments, worked out from its constructors' field types when the
	// declaration is read. It is zero for the built-ins, which are described by
	// talentRules instead. Keeping it on the type rather than in a registry
	// means Talent checking has no compilation-wide state to keep in step.
	Derived TalentSet
}

// Fn is a function type. Functions are curried, so `a -> b -> c` nests.
type Fn struct {
	From Type
	To   Type
}

func (*Var) isType() {}
func (*Con) isType() {}
func (*Fn) isType()  {}

// Names of the built-in type constructors.
const (
	Earth  = "Earth"  // Int
	Water  = "Water"  // Float
	Fire   = "Fire"   // Char
	Air    = "Air"    // Text
	Spirit = "Spirit" // Bool

	ThreadCon  = "Thread"
	PatternCon = "Pattern"
	WebCon     = "Web"
	CircleCon  = "Circle"
	TaverenCon = "Taveren"
	// LinkCon is who is joined to whom: disjoint sets over the nodes it was
	// built from, threaded through a fold as the joining happens.
	LinkCon = "Link"
	KnotCon = "Knot"

	// HoldCon is Weave's Option: `Held a | Stilled`.
	HoldCon = "Hold"
	// WeavingCon is Weave's Result: `Woven a | Gentled e`.
	WeavingCon = "Weaving"

	TwineCon = "Twine"
)

// Frequently used ground types.
var (
	TEarth  = &Con{Name: Earth}
	TWater  = &Con{Name: Water}
	TFire   = &Con{Name: Fire}
	TAir    = &Con{Name: Air}
	TSpirit = &Con{Name: Spirit}
	TKnot   = &Con{Name: KnotCon}
)

// Thread returns the type `Thread elem`.
func Thread(elem Type) Type { return &Con{Name: ThreadCon, Args: []Type{elem}} }

// Hold returns the type `Hold a`, Weave's Option.
func Hold(a Type) Type { return &Con{Name: HoldCon, Args: []Type{a}} }

// Twine returns the Twine type of elems.
func Twine(elems ...Type) Type { return &Con{Name: TwineCon, Args: elems} }

// Func builds a curried function type from params to result.
func Func(result Type, params ...Type) Type {
	t := result
	for i := len(params) - 1; i >= 0; i-- {
		t = &Fn{From: params[i], To: t}
	}
	return t
}

// ------------------------------------------------------------------ talents

// Talent is one of Weave's minimal type classes.
type Talent uint8

// TalentSet is a set of Talents.
type TalentSet uint8

// The built-in Talents.
const (
	Eq     TalentSet = 1 << iota // eq, neq
	Ord                          // lt, gt, sort, Taveren ordering
	Show                         // rendering for output
	Reckon                       // arithmetic: Earth and Water
	Bulk                         // has a size: the collections
	// Ply is the narrower thing Bulk is not: elements lying in a known order,
	// so that a run of them can be taken, dropped or turned round. `Thread` and
	// `Air` have it; a `Web` has a size but no order, which is why `len` asks
	// for Bulk and `take` asks for this.
	Ply
)

var talentNames = []struct {
	bit  TalentSet
	name string
}{{Eq, "Eq"}, {Ord, "Ord"}, {Show, "Show"}, {Reckon, "Reckon"}, {Bulk, "Bulk"}, {Ply, "Ply"}}

func (s TalentSet) String() string {
	var names []string
	for _, t := range talentNames {
		if s&t.bit != 0 {
			names = append(names, t.name)
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}

// talentRule describes which Talents a type constructor supports: Base holds
// unconditionally, while Inherited holds only when every argument supports it.
type talentRule struct {
	Base      TalentSet
	Inherited TalentSet
}

var talentRules = map[string]talentRule{
	Earth:   {Base: Eq | Ord | Show | Reckon},
	Water:   {Base: Eq | Ord | Show | Reckon},
	Fire:    {Base: Eq | Ord | Show},
	Air:     {Base: Eq | Ord | Show | Bulk | Ply},
	Spirit:  {Base: Eq | Show},
	KnotCon: {Base: Eq | Ord | Show},

	ThreadCon:  {Base: Bulk | Ply, Inherited: Eq | Ord | Show},
	CircleCon:  {Base: Bulk, Inherited: Eq | Show},
	PatternCon: {Base: Bulk, Inherited: Eq | Show},
	TaverenCon: {Base: Bulk, Inherited: Show},
	LinkCon:    {Inherited: Show},
	WebCon:     {Base: Bulk, Inherited: Eq | Show},
	HoldCon:    {Inherited: Eq | Ord | Show},
	WeavingCon: {Inherited: Eq | Ord | Show},
	TwineCon:   {Inherited: Eq | Ord | Show},
}

// ruleFor gives the Talent rule for a type constructor: the fixed table for the
// built-ins, and the constructors' own derivation for a user-declared type.
func ruleFor(t *Con) talentRule {
	if rule, ok := talentRules[t.Name]; ok {
		return rule
	}
	return talentRule{Inherited: t.Derived}
}

// Supports reports whether t satisfies every Talent in want, and names the
// first Talent it lacks otherwise.
func Supports(t Type, want TalentSet) (TalentSet, bool) {
	t = Resolve(t)
	switch t := t.(type) {
	case *Var:
		// An unbound variable can still acquire the constraint.
		return 0, true
	case *Fn:
		if want == 0 {
			return 0, true
		}
		return want, false
	case *Con:
		rule := ruleFor(t)
		have := rule.Base
		if rule.Inherited != 0 {
			inherited := rule.Inherited
			for _, arg := range t.Args {
				if missing, ok := Supports(arg, inherited); !ok {
					inherited &^= missing
				}
			}
			have |= inherited
		}
		if missing := want &^ have; missing != 0 {
			return missing, false
		}
		return 0, true
	}
	return want, false
}

// Require constrains t to satisfy want, recording the constraint on unbound
// variables and checking it against concrete types.
func Require(t Type, want TalentSet) error {
	t = Resolve(t)
	switch t := t.(type) {
	case *Var:
		t.Talents |= want
		return nil
	case *Con:
		rule := ruleFor(t)
		// Propagate inherited Talents into the arguments so that, for example,
		// `Eq (Thread a)` constrains `a` rather than being silently accepted.
		if pass := want & rule.Inherited; pass != 0 {
			for _, arg := range t.Args {
				if err := Require(arg, pass); err != nil {
					return err
				}
			}
		}
		if missing, ok := Supports(t, want&^rule.Inherited); !ok {
			return fmt.Errorf("%s has no %s Talent", String(t), missing)
		}
		return nil
	case *Fn:
		if want == 0 {
			return nil
		}
		return fmt.Errorf("a function has no %s Talent", want)
	}
	return nil
}

// ------------------------------------------------------------- unification

// Resolve follows bound variables to the type they stand for, compressing the
// chain as it goes.
func Resolve(t Type) Type {
	v, ok := t.(*Var)
	if !ok || v.Ref == nil {
		return t
	}
	r := Resolve(v.Ref)
	v.Ref = r
	return r
}

// MismatchError reports two types that could not be unified.
type MismatchError struct {
	Want, Got Type
	// Detail names a nested reason, such as a Talent that is not satisfied.
	Detail string
}

func (e *MismatchError) Error() string {
	if e.Detail != "" {
		return e.Detail
	}
	return fmt.Sprintf("expected %s, found %s", String(e.Want), String(e.Got))
}

// Unify makes a and b equal, or reports why it cannot.
func Unify(a, b Type) error {
	a, b = Resolve(a), Resolve(b)
	if a == b {
		return nil
	}

	if av, ok := a.(*Var); ok {
		return bindVar(av, b)
	}
	if bv, ok := b.(*Var); ok {
		return bindVar(bv, a)
	}

	switch at := a.(type) {
	case *Fn:
		bt, ok := b.(*Fn)
		if !ok {
			return &MismatchError{Want: a, Got: b}
		}
		if err := Unify(at.From, bt.From); err != nil {
			return err
		}
		return Unify(at.To, bt.To)

	case *Con:
		bt, ok := b.(*Con)
		if !ok || at.Name != bt.Name || len(at.Args) != len(bt.Args) {
			return &MismatchError{Want: a, Got: b}
		}
		for i := range at.Args {
			if err := Unify(at.Args[i], bt.Args[i]); err != nil {
				return err
			}
		}
		return nil
	}
	return &MismatchError{Want: a, Got: b}
}

// bindVar points v at t after the occurs check and Talent check.
func bindVar(v *Var, t Type) error {
	if occurs(v, t) {
		return &MismatchError{Want: v, Got: t,
			Detail: fmt.Sprintf("this type would contain itself (%s occurs in %s)", String(v), String(t))}
	}
	if other, ok := Resolve(t).(*Var); ok {
		// Two variables meet: merge their constraints.
		other.Talents |= v.Talents
	} else if v.Talents != 0 {
		if err := Require(t, v.Talents); err != nil {
			return &MismatchError{Want: v, Got: t, Detail: err.Error()}
		}
	}
	adjustLevel(v.Level, t)
	v.Ref = t
	return nil
}

// occurs reports whether v appears inside t, which would make the type
// infinite.
func occurs(v *Var, t Type) bool {
	switch t := Resolve(t).(type) {
	case *Var:
		return t == v
	case *Fn:
		return occurs(v, t.From) || occurs(v, t.To)
	case *Con:
		for _, a := range t.Args {
			if occurs(v, a) {
				return true
			}
		}
	}
	return false
}

// adjustLevel lowers the level of every variable in t to at most level, so
// that generalisation does not quantify a variable that escapes into an outer
// scope.
func adjustLevel(level int, t Type) {
	switch t := Resolve(t).(type) {
	case *Var:
		if t.Level > level {
			t.Level = level
		}
	case *Fn:
		adjustLevel(level, t.From)
		adjustLevel(level, t.To)
	case *Con:
		for _, a := range t.Args {
			adjustLevel(level, a)
		}
	}
}

// ------------------------------------------------------------------ schemes

// Scheme is a polymorphic type: Body, universally quantified over Vars.
type Scheme struct {
	Vars []*Var
	Body Type
	// Strands pairs a container variable with its element variable, for the
	// verbs that read an element out of something a Talent cannot describe.
	// `nth :: Earth -> c -> Hold e where Strand c e` says c is a Thread of e,
	// or is Air and e is Fire — a relation between two types rather than a
	// property of one, which is the whole reason it cannot be a Talent. See
	// settleStrands in internal/check.
	Strands [][2]*Var
}

// Mono returns a scheme with nothing quantified.
func Mono(t Type) *Scheme { return &Scheme{Body: t} }

// Alloc hands out unification variables. Each compilation owns one, which
// keeps variable identity out of package-level state.
type Alloc struct{ next int }

// Fresh returns a new unbound variable at the given binding level.
func (a *Alloc) Fresh(level int) *Var {
	a.next++
	return &Var{ID: a.next, Level: level}
}

// Instantiate replaces a scheme's quantified variables with fresh ones.
func (a *Alloc) Instantiate(s *Scheme, level int) Type {
	t, _ := a.InstantiateStrands(s, level)
	return t
}

// InstantiateStrands is Instantiate, also handing back the scheme's Strand
// pairs over the fresh variables, for the caller to settle once the container
// is known.
func (a *Alloc) InstantiateStrands(s *Scheme, level int) (Type, [][2]*Var) {
	if len(s.Vars) == 0 {
		return s.Body, nil
	}
	sub := make(map[*Var]*Var, len(s.Vars))
	for _, v := range s.Vars {
		fresh := a.Fresh(level)
		fresh.Talents = v.Talents
		sub[v] = fresh
	}
	var strands [][2]*Var
	for _, pair := range s.Strands {
		strands = append(strands, [2]*Var{sub[pair[0]], sub[pair[1]]})
	}
	return substitute(s.Body, sub), strands
}

func substitute(t Type, sub map[*Var]*Var) Type {
	switch t := Resolve(t).(type) {
	case *Var:
		if fresh, ok := sub[t]; ok {
			return fresh
		}
		return t
	case *Fn:
		return &Fn{From: substitute(t.From, sub), To: substitute(t.To, sub)}
	case *Con:
		if len(t.Args) == 0 {
			return t
		}
		args := make([]Type, len(t.Args))
		for i, a := range t.Args {
			args[i] = substitute(a, sub)
		}
		return &Con{Name: t.Name, Args: args, Derived: t.Derived}
	}
	return t
}

// Generalize quantifies every variable in t that was created deeper than
// level, which is exactly the set that does not escape into an outer scope.
func Generalize(level int, t Type) *Scheme {
	var vars []*Var
	seen := map[*Var]bool{}
	collectFree(t, level, seen, &vars)
	return &Scheme{Vars: vars, Body: t}
}

func collectFree(t Type, level int, seen map[*Var]bool, out *[]*Var) {
	switch t := Resolve(t).(type) {
	case *Var:
		if t.Level > level && !seen[t] {
			seen[t] = true
			*out = append(*out, t)
		}
	case *Fn:
		collectFree(t.From, level, seen, out)
		collectFree(t.To, level, seen, out)
	case *Con:
		for _, a := range t.Args {
			collectFree(a, level, seen, out)
		}
	}
}

// ------------------------------------------------------------------ display

// String renders a type using short, stable variable names.
func String(t Type) string {
	names := map[*Var]string{}
	var sb strings.Builder
	write(&sb, t, names, precTop)
	return sb.String()
}

// SchemeString renders a scheme, including any Talent constraints.
func SchemeString(s *Scheme) string {
	names := map[*Var]string{}
	var sb strings.Builder
	write(&sb, s.Body, names, precTop)
	var constraints []string
	for _, v := range s.Vars {
		if v.Talents != 0 {
			constraints = append(constraints, fmt.Sprintf("%s %s", v.Talents, varName(v, names)))
		}
	}
	if len(constraints) > 0 {
		return sb.String() + "  where " + strings.Join(constraints, ", ")
	}
	return sb.String()
}

func varName(v *Var, names map[*Var]string) string {
	if n, ok := names[v]; ok {
		return n
	}
	n := string(rune('a' + len(names)%26))
	if len(names) >= 26 {
		n = fmt.Sprintf("%s%d", n, len(names)/26)
	}
	names[v] = n
	return n
}

// Parenthesisation contexts for write.
const (
	precTop = iota // nothing binds tighter around us
	precFn         // left of an arrow: a function type needs parens
	precArg        // a type argument: functions and applications need parens
)

func write(sb *strings.Builder, t Type, names map[*Var]string, prec int) {
	switch t := Resolve(t).(type) {
	case *Var:
		sb.WriteString(varName(t, names))

	case *Fn:
		wrap := prec >= precFn
		if wrap {
			sb.WriteByte('(')
		}
		write(sb, t.From, names, precFn)
		sb.WriteString(" -> ")
		write(sb, t.To, names, precTop)
		if wrap {
			sb.WriteByte(')')
		}

	case *Con:
		if t.Name == TwineCon {
			sb.WriteByte('(')
			for i, a := range t.Args {
				if i > 0 {
					sb.WriteString(", ")
				}
				write(sb, a, names, precTop)
			}
			sb.WriteByte(')')
			return
		}
		if len(t.Args) == 0 {
			sb.WriteString(t.Name)
			return
		}
		wrap := prec >= precArg
		if wrap {
			sb.WriteByte('(')
		}
		sb.WriteString(t.Name)
		for _, a := range t.Args {
			sb.WriteByte(' ')
			write(sb, a, names, precArg)
		}
		if wrap {
			sb.WriteByte(')')
		}
	}
}
