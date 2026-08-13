// Package codegen emits C for a checked Weave program.
//
// Every Weave value becomes a runtime `Value`. Functions are lifted to
// top-level C functions taking (env, args): lambdas and local `channel`
// bindings capture their free variables into env, while top-level definitions
// capture nothing. A call whose callee is known and saturated compiles to a
// direct C call; anything else goes through the closure protocol, so partial
// application still works.
//
// Top-level values are compiled to memoised accessors, which makes them
// independent of declaration order and free when unused.
package codegen

import (
	"fmt"
	"sort"
	"strings"

	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/check"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/token"
	"github.com/malleum/weave/internal/types"
)

// builtin describes a prelude verb that the runtime implements.
type builtin struct {
	cname string
	arity int
}

// builtins maps prelude names to their C implementations. A prelude name that
// is absent is declared in internal/prelude but not yet backed by the runtime;
// using one is a clean compile error rather than a link failure.
var builtins = map[string]builtin{
	"add": {"wp_add", 2}, "sub": {"wp_sub", 2}, "mul": {"wp_mul", 2},
	"div": {"wp_div", 2}, "mod": {"wp_mod", 2}, "abs": {"wp_abs", 1},
	"inc": {"wp_inc", 1}, "dec": {"wp_dec", 1},
	"neg": {"wp_neg", 1}, "min": {"wp_min", 2}, "max": {"wp_max", 2},
	"even": {"wp_even", 1}, "odd": {"wp_odd", 1}, "divBy": {"wp_divBy", 2},

	"eq": {"wp_eq", 2}, "neq": {"wp_neq", 2}, "lt": {"wp_lt", 2},
	"lte": {"wp_lte", 2}, "gt": {"wp_gt", 2}, "gte": {"wp_gte", 2},
	"and": {"wp_and", 2}, "or": {"wp_or", 2}, "not": {"wp_not", 1},

	"isDigit": {"wp_isDigit", 1}, "isAlpha": {"wp_isAlpha", 1},
	"isSpace": {"wp_isSpace", 1},

	"lines": {"wp_lines", 1}, "words": {"wp_words", 1}, "fires": {"wp_fires", 1},
	"split": {"wp_split", 2}, "strip": {"wp_strip", 1}, "join": {"wp_join", 2},
	"air": {"wp_air", 1}, "earth": {"wp_earth", 1},
	"water": {"wp_water", 1}, "fire": {"wp_fire", 1},

	"otherwise": {"wp_otherwise", 2}, "holds": {"wp_holds", 1},
	"rescue": {"wp_rescue", 2},

	"bend": {"wp_bend", 2}, "sift": {"wp_sift", 2}, "braid": {"wp_braid", 3},
	"seek": {"wp_seek", 2}, "span": {"wp_span", 2}, "len": {"wp_len", 1},
	"count": {"wp_count", 2}, "sum": {"wp_sum", 1}, "prod": {"wp_prod", 1},
	"take": {"wp_take", 2}, "drop": {"wp_drop", 2}, "zip": {"wp_zip", 2},
	"sort": {"wp_sort", 1}, "all": {"wp_all", 2}, "any": {"wp_any", 2},
	"zipwith": {"wp_zipwith", 3}, "thread": {"wp_thread", 1}, "weld": {"wp_weld", 2}, "mend": {"wp_mend", 3},
	"sever": {"wp_sever", 2}, "strands": {"wp_strands", 2},
	"turn": {"wp_turn", 2}, "wrap": {"wp_wrap", 2},
	"plait": {"wp_plait", 2}, "cull": {"wp_cull", 2}, "bendr": {"wp_bendr", 2}, "siftr": {"wp_siftr", 2},
	"zipr": {"wp_zipr", 3}, "sums": {"wp_sums", 1}, "prods": {"wp_prods", 1},
	"cellwise": {"wp_cellwise", 2},
	"first":    {"wp_first", 1}, "last": {"wp_last", 1}, "rev": {"wp_rev", 1},
	"flat": {"wp_flat", 1}, "uniq": {"wp_uniq", 1},

	"web": {"wp_web", 1}, "get": {"wp_get", 2}, "put": {"wp_put", 3},
	"known": {"wp_known", 2}, "forget": {"wp_forget", 2}, "keys": {"wp_keys", 1},
	"vals": {"wp_vals", 1}, "items": {"wp_items", 1}, "merge": {"wp_merge", 2},
	"freq": {"wp_freq", 1}, "most": {"wp_most", 1},

	"circle": {"wp_circle", 1}, "member": {"wp_member", 2}, "insert": {"wp_insert", 2},
	"remove": {"wp_remove", 2}, "members": {"wp_members", 1},

	"taveren": {"wp_taveren", 1}, "push": {"wp_push", 2}, "pop": {"wp_pop", 1},
	"dijkstra": {"wp_dijkstra", 2}, "reach": {"wp_reach", 2},
	"route": {"wp_route", 3}, "toposort": {"wp_toposort", 2},
	"clumps": {"wp_clumps", 2}, "settle": {"wp_settle", 2},
	"couples": {"wp_couples", 1}, "index": {"wp_index", 1},
	"squeeze": {"wp_squeeze", 1}, "mesh": {"wp_mesh", 1},
	"carve": {"wp_carve", 2},
	"link":  {"wp_link", 1}, "bind": {"wp_bind", 3},
	"bound": {"wp_bound", 3}, "clumped": {"wp_clumped", 1},
	"tallies": {"wp_tallies", 1}, "tallied": {"wp_tallied", 3},

	"earths": {"wp_earths", 1}, "spans": {"wp_spans", 1}, "waters": {"wp_waters", 1}, "contains": {"wp_contains", 2},
	"chunk": {"wp_chunk", 2}, "windows": {"wp_windows", 2},
	"pivot": {"wp_pivot", 1}, "gcd": {"wp_gcd", 2}, "lcm": {"wp_lcm", 2},
	"solve":  {"wp_solve", 1},
	"sortby": {"wp_sortby", 2}, "group": {"wp_group", 2},
	"idx": {"wp_idx", 2}, "nth": {"wp_nth", 2}, "has": {"wp_has", 2},
	"glean": {"wp_glean", 2}, "harvest": {"wp_harvest", 2},

	"second": {"wp_second", 1},
	"none":   {"wp_none", 2},
	"enum":   {"wp_enum", 1},
	"scan":   {"wp_scan", 3}, "priors": {"wp_priors", 3}, "gentle": {"wp_gentle", 3}, "snag": {"wp_snag", 2},
	"high": {"wp_high", 1}, "low": {"wp_low", 1},
	"highidx": {"wp_highidx", 1}, "lowidx": {"wp_lowidx", 1},
	"seekidx": {"wp_seekidx", 2}, "twist": {"wp_twist", 3},
	"wind":    {"wp_wind", 3},
	"siftidx": {"wp_siftidx", 2}, "idxs": {"wp_idxs", 2},
	"overlaps": {"wp_overlaps", 2}, "overlapping": {"wp_overlapping", 2},
	"within": {"wp_within", 2}, "spanning": {"wp_spanning", 2},
	"holding": {"wp_holding", 2}, "width": {"wp_width", 1}, "dupe": {"wp_dupe", 1},
	"top":       {"wp_top", 2},
	"bot":       {"wp_bot", 2},
	"pairs":     {"wp_pairs", 1},
	"cross":     {"wp_cross", 2},
	"combos":    {"wp_combos", 2},
	"perms":     {"wp_perms", 1},
	"compact":   {"wp_compact", 1},
	"takewhile": {"wp_takewhile", 2},
	"dropwhile": {"wp_dropwhile", 2},
	"mapcat":    {"wp_mapcat", 2},
	"maxby":     {"wp_maxby", 2},
	"minby":     {"wp_minby", 2},
	"blocks":    {"wp_blocks", 1},
	"upper":     {"wp_upper", 1},
	"lower":     {"wp_lower", 1},
	"padl":      {"wp_padl", 3},
	"padr":      {"wp_padr", 3},
	"starts":    {"wp_starts", 2},
	"ends":      {"wp_ends", 2},
	"cutstart":  {"wp_cutstart", 2},
	"cutend":    {"wp_cutend", 2},
	"replace":   {"wp_replace", 3}, "delve": {"wp_delve", 2},
	"ord":    {"wp_ord", 1},
	"spark":  {"wp_spark", 1},
	"digit":  {"wp_digit", 1},
	"repeat": {"wp_repeat", 2},
	"sign":   {"wp_sign", 1},
	"sqrt":   {"wp_sqrt", 1},
	"cbrt":   {"wp_cbrt", 1},
	"ceil":   {"wp_ceil", 1},
	"floor":  {"wp_floor", 1},
	"round":  {"wp_round", 1},
	"clamp":  {"wp_clamp", 3},
	"pow":    {"wp_pow", 2},
	"bor":    {"wp_bor", 2},
	"band":   {"wp_band", 2},
	"bxor":   {"wp_bxor", 2},
	"bnot":   {"wp_bnot", 1},
	"shl":    {"wp_shl", 2},
	"shr":    {"wp_shr", 2},
	"base":   {"wp_base", 2}, "unbase": {"wp_unbase", 2},
	"mdist":   {"wp_mdist", 2},
	"pi":      {"wp_pi", 0},
	"e":       {"wp_e", 0},
	"inf":     {"wp_inf", 0},
	"inb":     {"wp_inb", 2},
	"shape":   {"wp_shape", 1},
	"dirs4":   {"wp_dirs4", 0},
	"dirs8":   {"wp_dirs8", 0},
	"around4": {"wp_around4", 2},
	"around8": {"wp_around8", 2},
	"mapvals": {"wp_mapvals", 2},
	"union":   {"wp_union", 2},
	"inter":   {"wp_inter", 2},
	"diff":    {"wp_diff", 2},

	"pattern": {"wp_pattern", 1}, "weft": {"wp_weft", 2},
	"spin": {"wp_spin", 1}, "flip": {"wp_flip", 1}, "cell": {"wp_cell", 2}, "set": {"wp_set", 3},
	"knots": {"wp_knots", 1}, "cells": {"wp_cells", 1},
	"sited": {"wp_sited", 2}, "sites": {"wp_sites", 2},
	"under": {"wp_under", 1}, "copies": {"wp_copies", 2},
	"woven": {"wp_woven", 1}, "covers": {"wp_covers", 2},
	"warp": {"wp_warp", 3},
	"nb4":  {"wp_nb4", 2}, "nb8": {"wp_nb8", 2},
	"rows": {"wp_rows", 1}, "cols": {"wp_cols", 1},
	"knot": {"wp_knot", 2}, "row": {"wp_row", 1}, "col": {"wp_col", 1},
}

// ctorArity gives the number of fields each built-in data constructor carries.
var ctorArity = map[string]int{
	"Light": 0, "Shadow": 0, "Held": 1, "Stilled": 0, "knot": 2,
	"Woven": 1, "Gentled": 1,
}

// weavingIndex gives the constructor index of Weaving's two cases. They are
// compiled exactly as a declared sum type would be, on the same runtime
// representation — the only thing built in about them is the name.
var weavingIndex = map[string]int{"Woven": 0, "Gentled": 1}

// userCtor describes a constructor of a sum type the program declares. Index is
// its position in the declaration, which is what a pattern tests and what
// orders the type.
type userCtor struct {
	Index int
	Arity int
}

// Options control code generation.
type Options struct {
	// DisableFusion compiles Thread chains as a call per stage. It exists so
	// tests can run the same program both ways and compare, which is the only
	// cheap way to be confident an optimisation preserves meaning.
	DisableFusion bool
	// DisableSpecialize keeps the general, tag-dispatching prelude verbs
	// instead of the typed helpers, for the same reason.
	DisableSpecialize bool
	// DisableRegions keeps every turn of a fused loop's storage instead of
	// handing it back when the turn ends. See regions.go.
	DisableRegions bool
	// DisableInPlace makes every grid update copy, for the same reason.
	DisableInPlace bool
	// DisableRelease keeps every Thread a function builds, instead of handing
	// the dead ones back at the return. Same reason again.
	DisableRelease bool
	// Watch names one function whose calls are recorded: what each of its
	// names held, on each call, rather than one value for the line. It is the
	// answer to "ghost text does not work inside a recursion", and it is opt-in
	// per function because recording per call costs the fusion inside the body.
	// See w_watch in the runtime.
	Watch string

	// Trace makes the program report every top-level definition's value rather
	// than only the output expression, which is what lets an editor show each
	// line's result beside it. See `weave trace`.
	Trace bool
}

// Generate compiles a checked file to C source. Problems are reported into bag.
func Generate(f *ast.File, info *check.Info, bag *diag.Bag, opts Options) string {
	g := &gen{
		bag:         bag,
		info:        info,
		opts:        opts,
		topFns:      map[string]int{},
		memoed:      map[string]bool{},
		foldOwnedAt: -1,
		byName:      map[string]*ast.Decl{},
		topVals:     map[string]bool{},
		cnames:      map[string]string{},
		wrappers:    map[string]string{},
		closures:    map[string]string{},
		owned:       map[string]bool{},
		consumed:    map[string][]bool{},
		userCtors:   map[string]userCtor{},
	}
	for _, td := range f.Types {
		for i, ct := range td.Ctors {
			g.userCtors[ct.Name] = userCtor{Index: i, Arity: len(ct.Fields)}
		}
	}
	return g.file(f)
}

type gen struct {
	bag  *diag.Bag
	info *check.Info
	// escaping holds the clause bindings whose Thread storage is released
	// before the function returns, and released the C locals they became, in
	// the order they were bound. See escape.go.
	// closures caches the file-scope closure made for each capture-free
	// function, so a program builds one per function rather than one per call,
	// and closureDecls holds their definitions.
	closures     map[string]string
	closureDecls []string

	// watching is set while the body of Options.Watch's definition is being
	// emitted, and is what turns g.watch from nothing into a record.
	// watchLine is the line the watched definition starts on, which is where
	// the record for what a call answered goes: the answer belongs to the
	// function, not to whichever line the last expression happened to end on.
	watching  bool
	watchLine int
	watchVar  string
	// inFunc is set while a function's body is being emitted, which is where a
	// `weave` binding reports the first value it ever holds. A top-level value
	// is reported line by line already; a function's body is not, because it
	// holds a different thing on every call. See firstValue.
	inFunc bool
	// lifted holds the lambdas made out of a named definition's body, so that
	// inlining one still counts as being inside that definition.
	lifted map[*ast.Lambda]bool
	// weaving names the variables a fused `gentle` step assigns instead of
	// building the Weaving the loop would take straight back apart. See
	// weaving.go.
	weaving weavingSplit

	escaping map[*ast.Bind]bool
	released []string

	opts Options

	topFns map[string]int  // top-level function name -> arity
	memoed map[string]bool // top-level names declared with `remember`

	// foldOwnedName is the pattern variable a fold's step binds to the half of
	// a Twine accumulator that is a collection, and foldOwnedAt is which half.
	// The inliner marks that variable owned as it binds it; nothing else knows
	// the C name it lands in. See inplace.go.
	foldOwnedName string
	foldOwnedAt   int

	byName  map[string]*ast.Decl // top-level definitions, for inlining a named step
	topVals map[string]bool      // top-level value name (arity 0)
	cnames  map[string]string

	wrappers  map[string]string // builtin name -> closure wrapper C name
	userCtors map[string]userCtor

	// owned holds the C variables that hold a collection nothing else can see,
	// so an update to one may write through. A fused fold puts its accumulator
	// here for as long as its body is being emitted. See inplace.go.
	owned map[string]bool

	// consumed names, for each top-level function that has one, the parameters
	// a caller may hand over outright rather than lend. Those functions get a
	// second entry point that keeps the ownership. See consume.go.
	consumed map[string][]bool

	decls []string // forward declarations
	defs  []string // function definitions
	next  int
}

// ------------------------------------------------------------------ scoping

// scope maps Weave names to the C expression that reads them.
type scope struct {
	parent *scope
	names  map[string]string
}

func newScope(parent *scope) *scope {
	return &scope{parent: parent, names: map[string]string{}}
}

func (s *scope) bind(name, expr string) { s.names[name] = expr }

func (s *scope) lookup(name string) (string, bool) {
	for e := s; e != nil; e = e.parent {
		if v, ok := e.names[name]; ok {
			return v, true
		}
	}
	return "", false
}

// ------------------------------------------------------------------- output

// body accumulates the statements of one C function.
type body struct {
	g      *gen
	sb     strings.Builder
	indent int
	tmps   int
}

func (b *body) line(format string, args ...any) {
	b.sb.WriteString(strings.Repeat("  ", b.indent))
	fmt.Fprintf(&b.sb, format, args...)
	b.sb.WriteByte('\n')
}

func (b *body) open(format string, args ...any) {
	b.line(format, args...)
	b.indent++
}

func (b *body) close(format string, args ...any) {
	b.indent--
	b.line(format, args...)
}

// tmp declares a fresh local and returns its name.
func (b *body) tmp() string {
	b.tmps++
	name := fmt.Sprintf("t%d", b.tmps)
	b.line("Value %s;", name)
	return name
}

// --------------------------------------------------------------------- file

func (g *gen) file(f *ast.File) string {
	for _, d := range f.Decls {
		g.byName[d.Name] = d
		if d.Arity() > 0 {
			g.topFns[d.Name] = d.Arity()
			if d.Memo {
				g.memoed[d.Name] = true
			}
		} else {
			g.topVals[d.Name] = true
		}
		// `wu_` rather than `w_`: the runtime owns the `w_` namespace, and a
		// definition called `apply` or `equal` must not collide with it.
		g.cnames[d.Name] = "wu_" + sanitize(d.Name)
	}

	// The grouping has to be known before the consumed-parameter fixpoint runs:
	// a set of mutually tail-recursive definitions is compiled into one C
	// function with a shared slot array, and a second entry point per member is
	// not a shape that fits. Those members simply do not consume.
	groups := tailGroups(f.Decls, g.topFns)
	merged := map[string]bool{}
	for _, group := range groups {
		if len(group.members) > 1 {
			for _, d := range group.members {
				merged[d.Name] = true
			}
		}
	}
	g.computeConsumed(f, merged)

	// Values first, in declaration order; then the functions, grouped so that a
	// set which tail-calls itself round becomes one loop.
	for _, d := range f.Decls {
		if d.Arity() == 0 {
			g.emitTopValue(d)
		}
	}
	for _, group := range groups {
		if len(group.members) > 1 {
			g.emitMergedGroup(group)
			continue
		}
		g.emitTopFunc(group.members[0], group.loops)
	}

	main := &body{g: g}
	main.open("int main(void) {")
	main.line("w_init();")
	if g.opts.Trace {
		g.emitTrace(main, f)
		if g.opts.Watch != "" {
			// The head of the ring has been written as it went; this is the
			// count and the tail. A run cut short by a limit never gets here,
			// and the head is the half that matters most.
			main.line("w_watch_flush();")
		}
	} else {
		// Every bare expression is an answer, printed in the order it was
		// written. A chain bound to a name stays quiet; a chain left bare is
		// something the program was asked for. An Advent of Code file is then
		// one binding for the input and two bare chains for the two parts.
		for _, e := range f.Outputs {
			out := g.expr(main, e, newScope(nil))
			main.line("w_print_result(%s);", out)
		}
	}
	main.line("return 0;")
	main.close("}")

	var sb strings.Builder
	sb.WriteString("// Generated by the Weave compiler. Do not edit.\n")
	sb.WriteString("#include \"weave.h\"\n\n")
	sb.WriteString("static Value g_source;\nstatic bool g_source_ready;\n")
	sb.WriteString("static Value w_get_source(void) {\n")
	sb.WriteString("  if (!g_source_ready) { g_source = w_source(); g_source_ready = true; }\n")
	sb.WriteString("  return g_source;\n}\n\n")
	for _, d := range g.decls {
		sb.WriteString(d)
		sb.WriteByte('\n')
	}
	for _, d := range g.closureDecls {
		sb.WriteString(d)
		sb.WriteByte('\n')
	}
	sb.WriteByte('\n')
	for _, d := range g.defs {
		sb.WriteString(d)
		sb.WriteByte('\n')
	}
	sb.WriteString(main.sb.String())
	return sb.String()
}

// emitTrace reports every top-level definition in source order, which is what
// `weave trace` and the editor plugin built on it consume.
//
// A definition with no arguments has a value, so it is evaluated and rendered.
// One with arguments does not, and its inferred type is the useful thing to
// show instead — that is known at compile time, so it goes in as a literal.
//
// A value that spans several lines reports one value per line rather than one
// for the whole definition; see trace.go.
func (g *gen) emitTrace(b *body, f *ast.File) {
	// Definitions and bare expressions are forced in one source-ordered pass,
	// so `weave trace` reads down the file the way the file does. A definition
	// another one needs still reports early, since that is when it is built.
	decls, outs := f.Decls, f.Outputs
	for len(decls) > 0 || len(outs) > 0 {
		if len(decls) > 0 && (len(outs) == 0 || decls[0].NamePos.Line <= outs[0].Pos().Line) {
			d := decls[0]
			decls = decls[1:]
			if d.Arity() > 0 {
				text := "a function"
				if sch, ok := g.info.Decls[d.Name]; ok {
					text = types.SchemeString(sch)
				}
				b.line("w_trace_text(%d, \"%s\", \"%s\");", d.NamePos.Line, escape(d.Name), escape(text))
				continue
			}
			// The accessor reports itself, line by line; forcing it is enough.
			// A definition ExpandPatterns generated reports nothing of its own —
			// the line already reported the whole value it was taken out of —
			// but it still has to be forced, or the names it binds never run.
			b.line("(void)%s();", g.cnames[d.Name])
			continue
		}
		e := outs[0]
		outs = outs[1:]
		g.traced(b, e, newScope(nil), "", e.Pos().Line)
	}
}

// emitTopValue compiles a nullary definition into a memoised accessor, so that
// declaration order does not matter and an unused definition costs nothing.
func (g *gen) emitTopValue(d *ast.Decl) {
	cname := g.cnames[d.Name]
	g.decls = append(g.decls, fmt.Sprintf("static Value %s(void);", cname))

	b := &body{g: g}
	b.line("static Value %s_v;", cname)
	b.line("static bool %s_ready;", cname)
	b.open("static Value %s(void) {", cname)
	b.open("if (!%s_ready) {", cname)
	if len(d.Clauses) == 0 {
		b.line("%s_v = w_earth(0);", cname)
	} else if g.opts.Trace && !d.Hidden {
		// Reporting happens where the value is built, so that a definition
		// spanning several lines reports one value per line. emitTrace only
		// has to make sure the accessor runs.
		//
		// A definition that took its value apart reports under the pattern that
		// was written, holding the Twine or Thread whole: `(width, height)` on
		// one line rather than `width` and `height` on the same one. The
		// projections it expanded into are Hidden and report nothing.
		label := d.Name
		if d.Display != "" {
			label = d.Display
		}
		v := g.traced(b, d.Clauses[0].Body, newScope(nil), label, d.NamePos.Line)
		b.line("%s_v = %s;", cname, v)
	} else {
		v := g.expr(b, d.Clauses[0].Body, newScope(nil))
		b.line("%s_v = %s;", cname, v)
	}
	b.line("%s_ready = true;", cname)
	b.close("}")
	b.line("return %s_v;", cname)
	b.close("}")
	g.defs = append(g.defs, b.sb.String())
}

// emitTopFunc compiles a definition with parameters, including its clauses.
//
// Parameters are copied into locals so that a tail call can rebind them, and
// the body is wrapped in a loop when the definition calls itself in tail
// position. That is what makes recursion cost what a loop costs, rather than
// growing the C stack until it runs out.
func (g *gen) emitTopFunc(d *ast.Decl, loops bool) {
	cname := g.cnames[d.Name]
	if d.Memo {
		// The name every call site uses becomes the lookup, and the body moves
		// aside. Recursive calls go through it too, which is the whole point:
		// `fib` calling itself hits the table.
		g.emitMemo(d, cname)
		cname += "_body"
	}

	// A function with a consumed parameter is written twice over: the body,
	// which keeps the ownership it was handed, and the name everyone else
	// calls, which gives it back. See consume.go.
	consumed := g.consumed[d.Name]
	if len(consumed) > 0 {
		g.decls = append(g.decls, fmt.Sprintf("static Value %s(Value *env, Value *args);", cname))
		g.defs = append(g.defs, fmt.Sprintf(
			"static Value %s(Value *env, Value *args) {\n  return w_disown(%s_move(env, args));\n}\n",
			cname, cname))
		cname += "_move"
	}

	g.decls = append(g.decls, fmt.Sprintf("static Value %s(Value *env, Value *args);", cname))

	b := &body{g: g}
	b.open("static Value %s(Value *env, Value *args) {", cname)
	b.line("(void)env; (void)args;")

	params := make([]string, d.Arity())
	for i := range params {
		params[i] = fmt.Sprintf("p%d", g.fresh())
		b.line("Value %s = args[%d];", params[i], i)
	}

	// The consumed parameters are ours for the length of the body, so an update
	// to one writes through and a fold seeded with one inherits the ownership
	// rather than disowning what it hands back.
	for i, takes := range consumed {
		if takes {
			g.owned[params[i]] = true
			defer delete(g.owned, params[i])
		}
	}

	var ti *tailInfo
	// A memoised definition keeps its self-calls as calls: turning one into a
	// jump would step around the very lookup the marker asked for.
	if loops && !d.Memo {
		owned := g.ownedParams(d)
		vars := map[string]bool{}
		for i, ok := range owned {
			// A consumed parameter is not disowned on the way out: the caller
			// gave up its reference, so what leaves is still ours to write.
			if ok && !(i < len(consumed) && consumed[i]) {
				vars[params[i]] = true
			}
		}
		ti = &tailInfo{name: d.Name, params: params, owned: owned, ownedVars: vars}
		b.open("for (;;) {")
	}

	if g.opts.Trace {
		g.inFunc = true
		defer func() { g.inFunc = false }()
	}

	if g.opts.Watch != "" && d.Name == g.opts.Watch {
		// Everything emitted from here reports per call. This sits inside the
		// loop when there is one, because a self tail call is a jump and each
		// turn of that loop is a call — a counter bumped outside would file the
		// whole recursion as one.
		//
		// It also sits before the parameters are matched, so that every record
		// a call makes carries the same number, including the ones a pattern in
		// the parameter list binds.
		g.watchVar = fmt.Sprintf("c%d", g.fresh())
		b.line("int64_t %s = w_watch_enter();", g.watchVar)
		g.watching, g.watchLine = true, d.NamePos.Line
		defer func() { g.watching = false }()
	}

	g.emitClauses(b, d, params, ti)

	if ti != nil {
		b.close("}")
		// Unreachable: every path through the loop returns or continues.
		b.line("return w_earth(0);")
	}
	b.close("}")
	g.defs = append(g.defs, b.sb.String())
}

// emitMemo writes the lookup half of a `remember`ed definition: a table from
// the arguments to the result, consulted before the body runs and extended
// after.
//
// The table hashes and compares with the operations every other value already
// uses, so a remembered function can be keyed on a Knot, a Twine or a declared
// sum type without another line of runtime. It is never pruned, which is
// exactly the cost the marker asks you to accept.
//
// The arguments are copied to the stack first, because the body is free to
// overwrite them: a self tail call is a jump that assigns the parameters.
func (g *gen) emitMemo(d *ast.Decl, cname string) {
	g.decls = append(g.decls, fmt.Sprintf("static Value %s(Value *env, Value *args);", cname))
	g.decls = append(g.decls, fmt.Sprintf("static Value %s_body(Value *env, Value *args);", cname))

	n := d.Arity()
	b := &body{g: g}
	b.line("static WMemo *%s_memo;", cname)
	b.open("static Value %s(Value *env, Value *args) {", cname)
	b.line("if (%s_memo == NULL) %s_memo = w_memo_new(%d);", cname, cname, n)
	b.line("Value key[%d];", n)
	for i := 0; i < n; i++ {
		b.line("key[%d] = args[%d];", i, i)
	}
	b.line("Value seen;")
	b.line("if (w_memo_get(%s_memo, key, &seen)) return seen;", cname)
	b.line("Value result = %s_body(env, args);", cname)
	b.line("w_memo_put(%s_memo, key, result);", cname)
	b.line("return result;")
	b.close("}")
	g.defs = append(g.defs, b.sb.String())
}

// emitClauses lays out a multi-clause definition as ordered pattern tests over
// the parameter variables.
func (g *gen) emitClauses(b *body, d *ast.Decl, params []string, ti *tailInfo) {
	for _, cl := range d.Clauses {
		sc := newScope(nil)
		var conds []string
		var binds []binding
		for i, p := range cl.Params {
			c, bs := g.match(p, params[i])
			if c != "" {
				conds = append(conds, c)
			}
			binds = append(binds, bs...)
		}

		g.escaping, g.released = g.releasable(cl), nil

		if len(conds) == 0 {
			g.emitBound(b, binds, sc)
			v, jumped := g.exprTail(b, cl.Body, sc, ti)
			if !jumped {
				g.emitReturn(b, v)
			}
			g.escaping, g.released = nil, nil
			return // an unconditional clause ends the chain
		}
		b.open("if (%s) {", strings.Join(conds, " && "))
		g.emitBound(b, binds, sc)
		v, jumped := g.exprTail(b, cl.Body, sc, ti)
		if !jumped {
			g.emitReturn(b, v)
		}
		b.close("}")
		g.escaping, g.released = nil, nil
	}
	b.line("w_fail(\"no clause of `%s` matched\");", escape(d.Name))
	b.line("return w_earth(0);")
}

// emitReturn writes the return, handing back first whatever Threads this clause
// built and nothing else can reach. The result is taken into a local before the
// frees run, since it is usually read out of one of them.
func (g *gen) emitReturn(b *body, v string) {
	if g.watching {
		// What the call answered. A tail call does not come through here — it
		// is a jump, and the next call's records are what follows it — which is
		// right: an iteration's answer is the next iteration.
		out := fmt.Sprintf("z%d", g.fresh())
		b.line("Value %s = %s;", out, v)
		b.line("w_watch(%s, %d, \"\", %s);", g.watchVar, g.watchLine, out)
		v = out
	}
	if len(g.released) == 0 {
		b.line("return %s;", v)
		return
	}
	out := fmt.Sprintf("z%d", g.fresh())
	b.line("Value %s = %s;", out, v)
	for _, name := range g.released {
		b.line("w_thread_release(%s);", name)
	}
	b.line("return %s;", out)
}

// binding is one variable a pattern introduces, with the C expression that
// reads it out of the matched value.
type binding struct {
	name string
	expr string
	pos  token.Pos // where the name was written, for `weave trace -watch`
}

func (g *gen) emitBound(b *body, binds []binding, sc *scope) {
	for _, bd := range binds {
		// A pattern that binds a whole variable needs no copy: naming the same
		// C variable is both cheaper and keeps the identity, which is what lets
		// the ownership analysis recognise a parameter it is threading.
		if isCIdent(bd.expr) {
			sc.bind(bd.name, bd.expr)
			g.watch(b, bd.pos, bd.name, bd.expr)
			continue
		}
		v := fmt.Sprintf("b%d", g.fresh())
		b.line("Value %s = %s;", v, bd.expr)
		sc.bind(bd.name, v)
		g.watch(b, bd.pos, bd.name, v)
	}
}

// watch records what a name held, when the function being emitted is the one
// `weave trace -watch` was pointed at. It is off everywhere else, which is the
// whole design: recording per call defeats the fusion inside a body the way
// staged compilation defeats it across lines, so it is paid for one function at
// a time and only when it is asked for.
func (g *gen) watch(b *body, pos token.Pos, name, val string) {
	if !g.watching {
		return
	}
	b.line("w_watch(%s, %d, \"%s\", %s);", g.watchVar, pos.Line, escape(name), val)
}

// firstValue reports the first value a binding inside a function body ever
// holds, as an ordinary by-line record — so the ghost text on that line says
// what the name was the first time through.
//
// It is one value where there are many, and it is the first rather than the
// last because the first is the one you can reason about: it is the call you
// would have made by hand. The rest are what `-watch` is for.
//
// The flag is per binding site rather than per function, which costs one
// predictable branch and is right for every shape at once — a lambda inside the
// body, a binding under a `ward`, a self tail call that comes round again. It
// records the first time this line is reached, whatever reached it.
func (g *gen) firstValue(b *body, pos token.Pos, name, val string) {
	if !g.opts.Trace || !g.inFunc || pos.Line == 0 {
		return
	}
	seen := fmt.Sprintf("seen%d", g.fresh())
	b.line("static bool %s;", seen)
	b.open("if (!%s) {", seen)
	b.line("%s = true;", seen)
	b.line("w_trace(%d, \"%s\", %s);", pos.Line, escape(name), val)
	b.close("}")
}

// isCIdent reports whether an emitted expression is just a variable name.
func isCIdent(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
			(i > 0 && c >= '0' && c <= '9')
		if !ok {
			return false
		}
	}
	return true
}

func (g *gen) fresh() int {
	g.next++
	return g.next
}

// match compiles a pattern into a test on subject plus the variables it binds.
// An empty condition means the pattern always matches.
func (g *gen) match(p ast.Pattern, subject string) (string, []binding) {
	switch p := p.(type) {
	case *ast.PWild, *ast.PBad:
		return "", nil

	case *ast.PVar:
		return "", []binding{{p.Name, subject, p.P}}

	case *ast.PInt:
		return fmt.Sprintf("(%s).earth == %dLL", subject, p.Value), nil
	case *ast.PFloat:
		return fmt.Sprintf("(%s).water == %v", subject, p.Value), nil
	case *ast.PChar:
		return fmt.Sprintf("(%s).fire == %d", subject, p.Value), nil
	case *ast.PText:
		return fmt.Sprintf("w_equal(%s, w_air_cstr(\"%s\"))", subject, escape(p.Value)), nil

	case *ast.PTwine:
		var conds []string
		var binds []binding
		for i, sub := range p.Elems {
			c, bs := g.match(sub, fmt.Sprintf("w_twine_at(%s, %d)", subject, i))
			if c != "" {
				conds = append(conds, c)
			}
			binds = append(binds, bs...)
		}
		return strings.Join(conds, " && "), binds

	case *ast.PThread:
		// A length test, then the elements by position. The rest, when there
		// is one, is a slice sharing the same storage — Threads are arrays, so
		// that costs a header and no copying.
		op := "=="
		if p.Rest != nil {
			op = ">="
		}
		conds := []string{fmt.Sprintf("w_thread_len(%s) %s %d", subject, op, len(p.Elems))}
		var binds []binding
		for i, sub := range p.Elems {
			c, bs := g.match(sub, fmt.Sprintf("w_thread_at(%s, %d)", subject, i))
			if c != "" {
				conds = append(conds, c)
			}
			binds = append(binds, bs...)
		}
		if v, ok := p.Rest.(*ast.PVar); ok {
			binds = append(binds, binding{v.Name,
				fmt.Sprintf("wp_drop(w_earth(%d), %s)", len(p.Elems), subject), v.P})
		}
		return strings.Join(conds, " && "), binds

	case *ast.PCtor:
		return g.matchCtor(p, subject)
	}
	return "", nil
}

func (g *gen) matchCtor(p *ast.PCtor, subject string) (string, []binding) {
	switch p.Name {
	case "Light":
		return fmt.Sprintf("(%s).spirit", subject), nil
	case "Shadow":
		return fmt.Sprintf("!(%s).spirit", subject), nil
	case "Stilled":
		return fmt.Sprintf("!w_is_held(%s)", subject), nil

	case "Held":
		cond := fmt.Sprintf("w_is_held(%s)", subject)
		if len(p.Args) != 1 {
			return cond, nil
		}
		inner := fmt.Sprintf("w_hold_inner(%s)", subject)
		sub, binds := g.match(p.Args[0], inner)
		if sub != "" {
			cond += " && " + sub
		}
		return cond, binds

	case "knot":
		var binds []binding
		var conds []string
		fields := []string{
			fmt.Sprintf("w_earth((%s).knot.row)", subject),
			fmt.Sprintf("w_earth((%s).knot.col)", subject),
		}
		for i, sub := range p.Args {
			if i >= len(fields) {
				break
			}
			c, bs := g.match(sub, fields[i])
			if c != "" {
				conds = append(conds, c)
			}
			binds = append(binds, bs...)
		}
		return strings.Join(conds, " && "), binds
	}

	if idx, ok := weavingIndex[p.Name]; ok {
		cond := fmt.Sprintf("w_data_index(%s) == %d", subject, idx)
		if len(p.Args) != 1 {
			return cond, nil
		}
		sub, binds := g.match(p.Args[0], fmt.Sprintf("w_data_field(%s, 0)", subject))
		if sub != "" {
			cond += " && " + sub
		}
		return cond, binds
	}

	if uc, ok := g.userCtors[p.Name]; ok {
		conds := []string{fmt.Sprintf("w_data_index(%s) == %d", subject, uc.Index)}
		var binds []binding
		for i, sub := range p.Args {
			c, bs := g.match(sub, fmt.Sprintf("w_data_field(%s, %d)", subject, i))
			if c != "" {
				conds = append(conds, c)
			}
			binds = append(binds, bs...)
		}
		return strings.Join(conds, " && "), binds
	}

	g.bag.Add(p.P, "`%s` is not supported by the backend yet", p.Name)
	return "0", nil
}

// -------------------------------------------------------------- expressions

// expr emits any statements the expression needs and returns a C expression
// holding its value.
func (g *gen) expr(b *body, e ast.Expr, sc *scope) string {
	switch e := e.(type) {
	case *ast.IntLit:
		// A number literal settles on Earth unless the definition it sits in
		// made it a Water — `sub n 1` where n is a Water is a Water 1.
		if g.primitiveType(e) == types.Water {
			return fmt.Sprintf("w_water(%d)", e.Value)
		}
		return fmt.Sprintf("w_earth(%dLL)", e.Value)
	case *ast.FloatLit:
		return fmt.Sprintf("w_water(%v)", e.Value)
	case *ast.CharLit:
		return fmt.Sprintf("w_fire(%d)", e.Value)
	case *ast.TextLit:
		return fmt.Sprintf("w_air(\"%s\", %d)", escape(e.Value), len(e.Value))

	case *ast.Var:
		return g.variable(b, e, sc)

	case *ast.Ctor:
		// `Source` is capitalised but is a value, not a constructor.
		if e.Name == "Source" {
			return "w_get_source()"
		}
		return g.ctorValue(b, e.Name, nil, e.P, sc)

	case *ast.App:
		return g.app(b, e, sc)

	case *ast.Lambda:
		return g.lambda(b, e, sc)

	case *ast.Let:
		inner := newScope(sc)
		for _, bind := range e.Binds {
			g.bind(b, bind, inner)
		}
		return g.expr(b, e.Body, inner)

	case *ast.Ward:
		return g.ward(b, e, sc)

	case *ast.ThreadLit:
		if len(e.Elems) == 0 {
			return "w_thread_empty()"
		}
		items := make([]string, len(e.Elems))
		for i, el := range e.Elems {
			items[i] = g.expr(b, el, sc)
		}
		arr := fmt.Sprintf("a%d", g.fresh())
		b.line("Value %s[] = {%s};", arr, strings.Join(items, ", "))
		return fmt.Sprintf("w_thread_copy(%s, %d)", arr, len(items))

	case *ast.TwineLit:
		items := make([]string, len(e.Elems))
		for i, el := range e.Elems {
			items[i] = g.expr(b, el, sc)
		}
		arr := fmt.Sprintf("a%d", g.fresh())
		b.line("Value %s[] = {%s};", arr, strings.Join(items, ", "))
		return fmt.Sprintf("w_twine_copy(%s, %d)", arr, len(items))

	case *ast.WebLit:
		// A literal builds up from the empty Web, one entry at a time.
		acc := fmt.Sprintf("m%d", g.fresh())
		b.line("Value %s = w_web_empty();", acc)
		for _, pair := range e.Pairs {
			k := g.expr(b, pair.Key, sc)
			v := g.expr(b, pair.Val, sc)
			b.line("%s = w_web_put(%s, %s, %s);", acc, acc, k, v)
		}
		return acc
	}
	return "w_earth(0)"
}

func (g *gen) variable(b *body, e *ast.Var, sc *scope) string {
	if c, ok := sc.lookup(e.Name); ok {
		return c
	}
	if e.Name == "Source" {
		return "w_get_source()"
	}
	if g.topVals[e.Name] {
		return g.cnames[e.Name] + "()"
	}
	if arity, ok := g.topFns[e.Name]; ok {
		return fmt.Sprintf("w_closure_value(&%s)", g.constClosure(g.cnames[e.Name], arity))
	}
	if bi, ok := builtins[e.Name]; ok {
		// A builtin taking no arguments is a constant, so naming it calls it.
		if bi.arity == 0 {
			return fmt.Sprintf("%s()", bi.cname)
		}
		return fmt.Sprintf("w_closure_value(&%s)", g.constClosure(g.wrapper(e.Name, bi), bi.arity))
	}
	g.unsupported(e.P, e.Name)
	return "w_earth(0)"
}

// unsupported reports a prelude name the runtime does not implement yet.
func (g *gen) unsupported(pos token.Pos, name string) {
	if name == "flow" || name == "cycle" {
		// Neither is ever built, only run: the loop that consumes one holds a
		// single element at a time. So it has to be created and consumed in
		// the same pipeline, where the compiler can see both ends.
		example := "`flow (add 1) 1 | take 10`"
		if name == "cycle" {
			example = "`cycle [1 2 3] | take 10`"
		}
		g.bag.AddHint(pos,
			"consume it in the same pipeline that creates it, as in "+example,
			"a `%s` is endless, so it cannot be bound to a name or passed around", name)
		return
	}
	g.bag.AddHint(pos, "the runtime does not implement it yet",
		"`%s` cannot be compiled", name)
}

// wrapper emits, once per builtin, a closure-shaped adapter so the verb can be
// passed around as a value.
func (g *gen) wrapper(name string, bi builtin) string {
	if w, ok := g.wrappers[name]; ok {
		return w
	}
	w := "wrap_" + sanitize(name)
	g.wrappers[name] = w

	args := make([]string, bi.arity)
	for i := range args {
		args[i] = fmt.Sprintf("args[%d]", i)
	}
	g.decls = append(g.decls, fmt.Sprintf("static Value %s(Value *env, Value *args);", w))
	g.defs = append(g.defs, fmt.Sprintf(
		"static Value %s(Value *env, Value *args) {\n  (void)env;\n  return %s(%s);\n}\n",
		w, bi.cname, strings.Join(args, ", ")))
	return w
}

// ctorValue builds a constructor application, or a closure over the
// constructor when it is used as a plain value, as in `bend Held xs`.
func (g *gen) ctorValue(b *body, name string, args []string, pos token.Pos, sc *scope) string {
	arity, known := ctorArity[name]
	if uc, isUser := g.userCtors[name]; isUser {
		arity, known = uc.Arity, true
	}
	if !known {
		g.unsupported(pos, name)
		return "w_earth(0)"
	}

	// A constructor that carries nothing is already a value.
	switch name {
	case "Light":
		return "W_LIGHT"
	case "Shadow":
		return "W_SHADOW"
	case "Stilled":
		return "w_stilled()"
	}
	if uc, isUser := g.userCtors[name]; isUser && uc.Arity == 0 {
		return g.nullaryCtor(name, uc.Index) + "()"
	}

	if len(args) < arity {
		closure := fmt.Sprintf("w_closure_value(&%s)", g.constClosure(g.ctorWrapper(name, arity), arity))
		if len(args) == 0 {
			return closure
		}
		// Partially applied, as in `bend (Step North) ns`: hand the arguments
		// so far to the closure, which keeps them until the rest arrive.
		arr := fmt.Sprintf("a%d", g.fresh())
		b.line("Value %s[] = {%s};", arr, strings.Join(args, ", "))
		return fmt.Sprintf("w_call(%s, %s, %d)", closure, arr, len(args))
	}
	return g.ctorBuild(name, args)
}

// nullaryCtor emits an accessor for a constructor that carries nothing, so the
// one object it needs is built once rather than on every mention.
func (g *gen) nullaryCtor(name string, index int) string {
	key := "nullary:" + name
	if w, ok := g.wrappers[key]; ok {
		return w
	}
	fn := "wd_" + sanitize(name)
	g.wrappers[key] = fn
	g.decls = append(g.decls, fmt.Sprintf("static Value %s(void);", fn))
	g.defs = append(g.defs, fmt.Sprintf(`static Value %s(void) {
  static Value v;
  static bool built;
  if (!built) {
    v = w_data("%s", %d, NULL, 0);
    built = true;
  }
  return v;
}
`, fn, name, index))
	return fn
}

// ctorBuild is the C expression constructing name from its fields.
func (g *gen) ctorBuild(name string, args []string) string {
	switch name {
	case "Held":
		return fmt.Sprintf("w_held(%s)", args[0])
	case "knot":
		return fmt.Sprintf("w_knot_make((%s).earth, (%s).earth)", args[0], args[1])
	}
	if idx, ok := weavingIndex[name]; ok {
		return fmt.Sprintf("w_data(\"%s\", %d, (Value[]){%s}, 1)", name, idx, args[0])
	}
	if uc, ok := g.userCtors[name]; ok {
		return fmt.Sprintf("w_data(\"%s\", %d, (Value[]){%s}, %d)",
			name, uc.Index, strings.Join(args, ", "), len(args))
	}
	return "w_earth(0)"
}

// ctorWrapper emits, once per constructor, a closure-shaped adapter so the
// constructor can be passed around like any other function.
func (g *gen) ctorWrapper(name string, arity int) string {
	key := "ctor:" + name
	if w, ok := g.wrappers[key]; ok {
		return w
	}
	w := "wrap_ctor_" + sanitize(name)
	g.wrappers[key] = w

	fields := make([]string, arity)
	for i := range fields {
		fields[i] = fmt.Sprintf("args[%d]", i)
	}
	g.decls = append(g.decls, fmt.Sprintf("static Value %s(Value *env, Value *args);", w))
	g.defs = append(g.defs, fmt.Sprintf(
		"static Value %s(Value *env, Value *args) {\n  (void)env;\n  return %s;\n}\n",
		w, g.ctorBuild(name, fields)))
	return w
}

// arrayOf lays out arguments as the C array a compiled function takes, and
// names it.
func (g *gen) arrayOf(b *body, args []string) string {
	arr := fmt.Sprintf("a%d", g.fresh())
	b.line("Value %s[] = {%s};", arr, strings.Join(args, ", "))
	return arr
}

// app compiles a call, using a direct C call whenever the callee is known and
// saturated.
func (g *gen) app(b *body, e *ast.App, sc *scope) string {
	// A chain of Thread verbs becomes one loop rather than a call per stage.
	if out, fused := g.tryFuse(b, e, sc); fused {
		return out
	}

	// `pick` must not evaluate the branch it does not take.
	if v, ok := e.Fn.(*ast.Var); ok && v.Name == "pick" && len(e.Args) == 3 {
		if _, shadowed := sc.lookup(v.Name); !shadowed {
			return g.pick(b, e, sc)
		}
	}

	if v, ok := e.Fn.(*ast.Ctor); ok && v.Name != "Source" {
		args := g.args(b, e.Args, sc)
		return g.ctorValue(b, v.Name, args, v.P, sc)
	}

	// An update to a collection the compiler is holding owned — a fused fold's
	// accumulator — writes through instead of copying.
	if out, ok := g.ownedUpdate(b, e, sc); ok {
		return out
	}

	// A helper handed a collection the generator holds owned takes it outright,
	// so what it hands back is still ours to write to.
	if out, ok := g.movedCall(b, e, sc); ok {
		return out
	}

	if v, ok := e.Fn.(*ast.Var); ok {
		if _, shadowed := sc.lookup(v.Name); !shadowed {
			if arity, isTop := g.topFns[v.Name]; isTop && len(e.Args) == arity {
				args := g.args(b, e.Args, sc)
				return fmt.Sprintf("%s(NULL, %s)", g.cnames[v.Name], g.arrayOf(b, args))
			}
			if bi, ok := builtins[v.Name]; ok && len(e.Args) == bi.arity {
				args := g.args(b, e.Args, sc)
				// With the operand type known, call the typed helper rather
				// than the verb that has to dispatch on tags.
				if typed, ok := g.specialiseCall(v.Name, e.Args); ok {
					return fmt.Sprintf("%s(%s)", typed, strings.Join(args, ", "))
				}
				return fmt.Sprintf("%s(%s)", bi.cname, strings.Join(args, ", "))
			}
		}
	}

	// General case: build the callee, then apply the arguments one at a time.
	fn := g.expr(b, e.Fn, sc)
	args := g.args(b, e.Args, sc)
	arr := fmt.Sprintf("a%d", g.fresh())
	b.line("Value %s[] = {%s};", arr, strings.Join(args, ", "))
	return fmt.Sprintf("w_call(%s, %s, %d)", fn, arr, len(args))
}

func (g *gen) args(b *body, exprs []ast.Expr, sc *scope) []string {
	out := make([]string, len(exprs))
	for i, a := range exprs {
		out[i] = g.expr(b, a, sc)
	}
	return out
}

// pick compiles the lazy conditional to a C statement so only the taken branch
// runs.
func (g *gen) pick(b *body, e *ast.App, sc *scope) string {
	cond := g.expr(b, e.Args[0], sc)
	res := b.tmp()
	b.open("if ((%s).spirit) {", cond)
	v := g.expr(b, e.Args[1], sc)
	b.line("%s = %s;", res, v)
	b.close("} else {")
	b.indent++
	w := g.expr(b, e.Args[2], sc)
	b.line("%s = %s;", res, w)
	b.close("}")
	return res
}

// lambda lifts an anonymous function to the top level, capturing whatever it
// refers to from the enclosing scope.
func (g *gen) lambda(b *body, e *ast.Lambda, sc *scope) string {
	cname, captured := g.lambdaFn(e, sc)
	if len(captured) == 0 {
		return fmt.Sprintf("w_closure_value(&%s)", g.constClosure(cname, len(e.Params)))
	}
	arr := g.captureArray(b, captured, sc)
	return fmt.Sprintf("w_closure(%s, %d, %s, %d)", cname, len(e.Params), arr, len(captured))
}

// captureArray emits the array of values a closure carries.
// constClosure names a file-scope closure for a function that captures nothing.
//
// Such a closure is a constant: `w_apply` copies before it writes, so nothing
// can ever mutate one, and every `w_closure(f, n, NULL, 0)` in a program builds
// the identical object. Building it per call cost two allocations each time —
// on Advent of Code 2024 day 2, which makes two of them per report and then
// again per damped retry, that was nine thousand allocations of the same two
// values.
func (g *gen) constClosure(cname string, arity int) string {
	if name, seen := g.closures[cname]; seen {
		return name
	}
	name := fmt.Sprintf("cl%d", g.fresh())
	slots := arity
	if slots == 0 {
		slots = 1
	}
	// These go after every forward declaration rather than among them: the
	// static names the function, and the function may not have been declared
	// at the point its first closure was wanted.
	g.closureDecls = append(g.closureDecls,
		fmt.Sprintf("static Value %s_slots[%d];", name, slots),
		fmt.Sprintf("static WClosure %s = {{W_SHARED, W_CLOSURE}, %s, %d, 0, 0, %s_slots};",
			name, cname, arity, name))
	g.closures[cname] = name
	return name
}

func (g *gen) captureArray(b *body, captured []string, sc *scope) string {
	vals := make([]string, len(captured))
	for i, name := range captured {
		c, _ := sc.lookup(name)
		vals[i] = c
	}
	arr := fmt.Sprintf("c%d", g.fresh())
	b.line("Value %s[] = {%s};", arr, strings.Join(vals, ", "))
	return arr
}

// lambdaFn lifts a lambda's body to a top-level C function and reports the
// names it captures, so the caller can decide how to build the closure. A
// local function that calls itself has to be built in two steps, which is why
// this is separate from lambda.
func (g *gen) lambdaFn(e *ast.Lambda, sc *scope) (string, []string) {
	bound := map[string]bool{}
	for _, p := range e.Params {
		ast.BindPatternVars(p, bound)
	}
	free := map[string]bool{}
	ast.FreeVars(e.Body, bound, free)

	// Only names that live in an enclosing scope need capturing; globals and
	// builtins are reachable directly.
	var captured []string
	for name := range free {
		if _, ok := sc.lookup(name); ok {
			captured = append(captured, name)
		}
	}
	sort.Strings(captured)

	cname := fmt.Sprintf("w_lam%d", g.fresh())
	g.decls = append(g.decls, fmt.Sprintf("static Value %s(Value *env, Value *args);", cname))

	inner := newScope(nil)
	for i, name := range captured {
		inner.bind(name, fmt.Sprintf("env[%d]", i))
	}

	lb := &body{g: g}
	lb.open("static Value %s(Value *env, Value *args) {", cname)
	lb.line("(void)env; (void)args;")
	var binds []binding
	for i, p := range e.Params {
		cond, bs := g.match(p, fmt.Sprintf("args[%d]", i))
		if cond != "" {
			// A refutable lambda parameter cannot fail in a checked program,
			// but keep the test honest rather than silently ignoring it.
			lb.line("if (!(%s)) w_fail(\"lambda argument did not match\");", cond)
		}
		binds = append(binds, bs...)
	}
	g.emitBound(lb, binds, inner)
	v := g.expr(lb, e.Body, inner)
	lb.line("return %s;", v)
	lb.close("}")
	g.defs = append(g.defs, lb.sb.String())
	return cname, captured
}

// bind compiles one local binding. A binding with parameters becomes a lambda,
// which keeps local functions and anonymous ones on the same path.
func (g *gen) bind(b *body, bind *ast.Bind, sc *scope) {
	// A binding that takes its value apart is the value in a local, then the
	// same match a parameter gets. A pattern that can fail is a soft
	// diagnostic in the checker, so the trap here is the case that got past it.
	if bind.Pat != nil {
		v := g.expr(b, bind.Value, sc)
		name := fmt.Sprintf("l%d", g.fresh())
		b.line("Value %s = %s;", name, v)
		cond, binds := g.match(bind.Pat, name)
		if cond != "" {
			b.line("if (!(%s)) w_fail(\"the binding did not match\");", cond)
		}
		g.emitBound(b, binds, sc)
		return
	}
	if len(bind.Params) > 0 {
		lam := &ast.Lambda{Params: bind.Params, Body: bind.Value, P: bind.Pos()}
		if g.mentions(bind) {
			g.recursiveBind(b, bind, lam, sc)
			return
		}
		v := g.lambda(b, lam, sc)
		name := fmt.Sprintf("l%d", g.fresh())
		b.line("Value %s = %s;", name, v)
		sc.bind(bind.Name, name)
		return
	}
	v := g.expr(b, bind.Value, sc)
	name := fmt.Sprintf("l%d", g.fresh())
	b.line("Value %s = %s;", name, v)
	sc.bind(bind.Name, name)
	g.watch(b, bind.NamePos, bind.Name, name)
	g.firstValue(b, bind.NamePos, bind.Name, name)
	if g.escaping[bind] {
		g.released = append(g.released, name)
	}
}

// mentions reports whether a local function's body refers to itself.
func (g *gen) mentions(bind *ast.Bind) bool {
	bound := map[string]bool{}
	for _, p := range bind.Params {
		ast.BindPatternVars(p, bound)
	}
	if bound[bind.Name] {
		return false // the parameter shadows the name
	}
	free := map[string]bool{}
	ast.FreeVars(bind.Value, bound, free)
	return free[bind.Name]
}

// recursiveBind compiles a local function that calls itself.
//
// The closure has to exist before its own environment can mention it, so it is
// made empty, its name is bound, and the captured values — which now include
// the closure itself — are filled in afterwards. `channel` was the one binder
// whose obvious use did not work: the checker had always allowed a local
// function to recurse, and only the code generator could not say it.
func (g *gen) recursiveBind(b *body, bind *ast.Bind, lam *ast.Lambda, sc *scope) {
	name := fmt.Sprintf("l%d", g.fresh())
	sc.bind(bind.Name, name)

	cname, captured := g.lambdaFn(lam, sc)
	b.line("Value %s = w_closure(%s, %d, NULL, 0);", name, cname, len(lam.Params))
	if len(captured) == 0 {
		return
	}
	arr := g.captureArray(b, captured, sc)
	b.line("w_closure_env(%s, %s, %d);", name, arr, len(captured))
}

// ward compiles a pattern match into ordered tests over a temporary.
func (g *gen) ward(b *body, e *ast.Ward, sc *scope) string {
	v, _ := g.wardWith(b, e, sc, nil)
	return v
}

// wardWith emits a ward as a chain of nested if/else. Nesting rather than
// breaking out of a block matters for tail calls: a `continue` in an arm has to
// belong to the enclosing function loop, not to a construct wrapped around the
// match.
//
// When ti is non-nil the arms are in tail position, and the second result says
// whether every one of them jumped rather than producing a value.
func (g *gen) wardWith(b *body, e *ast.Ward, sc *scope, ti *tailInfo) (string, bool) {
	subject := g.expr(b, e.Subject, sc)
	sv := fmt.Sprintf("s%d", g.fresh())
	b.line("Value %s = %s;", sv, subject)

	res := b.tmp()
	allJumped := true
	depth := 0

	closeAll := func() {
		for ; depth > 0; depth-- {
			b.close("}")
		}
	}

	for _, arm := range e.Arms {
		cond, binds := g.match(arm.Pat, sv)
		inner := newScope(sc)

		if cond == "" {
			// An irrefutable arm ends the chain.
			g.emitBound(b, binds, inner)
			v, jumped := g.exprTail(b, arm.Body, inner, ti)
			if !jumped {
				b.line("%s = %s;", res, v)
				allJumped = false
			}
			closeAll()
			return res, allJumped
		}

		b.open("if (%s) {", cond)
		g.emitBound(b, binds, inner)
		v, jumped := g.exprTail(b, arm.Body, inner, ti)
		if !jumped {
			b.line("%s = %s;", res, v)
			allJumped = false
		}
		b.close("} else {")
		b.indent++
		depth++
	}

	// Exhaustiveness is checked before this point, so reaching here means the
	// compiler is wrong rather than the program.
	b.line("w_fail(\"no ward arm matched\");")
	b.line("%s = w_earth(0);", res)
	closeAll()
	return res, allJumped
}

// ------------------------------------------------------------------- naming

// sanitize turns a Weave name into a C identifier. Weave identifiers are
// already C-safe, so this only guards against colliding with the runtime.
func sanitize(name string) string {
	var sb strings.Builder
	for _, r := range name {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' {
			sb.WriteRune(r)
		} else {
			sb.WriteByte('_')
		}
	}
	return sb.String()
}

// escape renders a Weave string as a C string literal body.
func escape(s string) string {
	var sb strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case '"':
			sb.WriteString("\\\"")
		case '\\':
			sb.WriteString("\\\\")
		case '\n':
			sb.WriteString("\\n")
		case '\t':
			sb.WriteString("\\t")
		case '\r':
			sb.WriteString("\\r")
		default:
			if c < 32 || c >= 127 {
				fmt.Fprintf(&sb, "\\%03o", c)
			} else {
				sb.WriteByte(c)
			}
		}
	}
	return sb.String()
}
