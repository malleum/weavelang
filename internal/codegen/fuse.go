package codegen

import (
	"fmt"
	"strings"

	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/types"
)

// Thread fusion.
//
// The parser turns a pipeline into nested applications, so
//
//	xs | bend f | sift p | seek q
//
// arrives here as `seek q (sift p (bend f xs))`. Compiled call by call that
// allocates a Thread per stage. This pass recognises the nest and emits one
// loop over the source instead, with no intermediate Thread at all:
//
//	for each x in xs:  x = f x;  if !p x: skip;  if q x: answer = Held x, stop
//
// Two further wins fall out of doing it here rather than in the runtime. A
// stage whose function is a lambda is inlined into the loop body, so
// `bend (x : mul x x)` becomes a multiplication with no closure and no
// indirect call; and a short-circuiting consumer stops the whole pipeline
// early, which is exactly the behaviour SPEC.md describes for a lazy Thread.

// stageKind is the sort of transformation a pipeline stage performs.
type stageKind int

const (
	stageBend  stageKind = iota // map
	stageSift                   // filter
	stageCull                   // filter, keeping what the test turns down
	stageTake                   // stop after n have passed
	stageWhile                  // stop when the test first fails
	stageDrop                   // let nothing through until n have gone by
	stageSkip                   // let nothing through until the test first fails
	stageScan                   // map with a memory: the running total so far
)

// bounds reports whether a stage can end the loop on its own, which is what
// makes an endless `flow` safe to consume.
func (k stageKind) bounds() bool { return k == stageTake || k == stageWhile }

type stage struct {
	kind stageKind
	fn   ast.Expr
	// seed is the value a scan starts from. Nothing else uses it.
	seed ast.Expr
}

// consumerKind is what finally eats the Thread.
type consumerKind int

const (
	conCollect consumerKind = iota // no terminal verb: build a Thread
	conSum
	conProduct
	conSize
	conCount
	conSeek
	conAny
	conAll
	conFirst
	conBraid
	conDupe
	conGentle
	conSeekIdx // seekidx p: where the first match is
	conNone    // none p: the negation of any
	conIdx     // idx x: where a value first occurs
	conHas     // has x: whether it occurs at all
	conNth     // nth i, and `second`, which is nth 1
)

// shortCircuits reports whether the consumer can stop before the end.
//
// This is the list an endless producer is measured against: a `flow` is only
// allowed if something downstream can end the loop. Every verb that answers
// from one element and stops belongs here, and leaving one out is not a missed
// optimisation but a program the compiler refuses to build — see acceptEndless.
func (c consumerKind) shortCircuits() bool {
	switch c {
	case conSeek, conAny, conAll, conFirst, conDupe, conGentle,
		conSeekIdx, conNone, conIdx, conHas, conNth:
		return true
	}
	return false
}

// source is where a fused loop's elements come from. A `span` is generated in
// the loop header rather than built first, so a range pipeline allocates
// nothing at all.
type source struct {
	expr   ast.Expr // a Thread to iterate, when nothing below is set
	lo, hi ast.Expr // a span's inclusive bounds
	// under is `under n`, which is the span 0 to n-1 written the way a program
	// that wants "the places of n things" writes it. It is kept apart from lo
	// and hi only because the bounds it stands for are arithmetic rather than
	// expressions the source wrote; everything downstream treats it as a span.
	under ast.Expr
	// flowFn and flowSeed belong to `flow f seed`, the endless Thread
	// seed, f seed, f (f seed), ... It is never built: the loop holds one
	// element at a time and something downstream has to stop it.
	flowFn, flowSeed ast.Expr
	// cycle belongs to `cycle xs`, the same Thread over and over. Endless for
	// the same reason a flow is, and bounded the same way.
	cycle ast.Expr
	// zipA/zipB belong to `zip a b` and items to `items w`. Both yield pairs,
	// and a pair a fold immediately takes apart never has to be built: the loop
	// keeps the two halves in separate C variables and binds the destructuring
	// pattern straight from them. See pairKept below.
	//
	// zipFn belongs to `zipwith f a b`, which is the same two sources with the
	// combining done on the spot. Its function is planned like a stage's, so a
	// lambda is inlined into the loop and the closure and the call per element
	// both go away — which is the whole reason to fuse it, since the result
	// array has to be written either way.
	zipA, zipB, zipFn ast.Expr
	items             ast.Expr
	// enumed belongs to `enum xs`, each element with where it lies. It is the
	// most-written pair producer of the three: the Twine exists only to be
	// taken apart by the very next stage.
	enumed ast.Expr
	// couples belongs to `couples xs`, every element with every element after
	// it. It yields pairs for the same reason `zip` does, and for a caller that
	// takes the pair apart — which is every caller there has ever been — the
	// Twine is never built. There are n(n-1)/2 of them, so on the five hundred
	// junction boxes of Advent of Code 2025 day 8 that is a quarter of a
	// million Twines allocated and thrown away.
	couples ast.Expr
	// grid belongs to the verbs that walk a Pattern: `knots`, `cells`, and the
	// four neighbour verbs. Each of them allocates a Thread whose only purpose
	// is to be walked once — and a neighbour verb allocates one per cell, which
	// on a grid program is the whole cost. gridPat is the Pattern; gridKnot is
	// the cell being asked about, for the neighbour verbs.
	grid              gridKind
	gridPat, gridKnot ast.Expr
}

// gridKind names the shape of a Pattern walk. The four neighbour verbs differ
// only in how many of the direction table they use and whether they yield the
// cell or the knot, so one loop serves all four.
type gridKind int

const (
	gridNone gridKind = iota
	gridKnots
	gridNb4
	gridNb8
	gridAround4
	gridAround8
)

// gridSources maps the verb to its walk and how many arguments it takes.
var gridSources = map[string]struct {
	kind  gridKind
	arity int
}{
	"knots":   {gridKnots, 1},
	"nb4":     {gridNb4, 2},
	"nb8":     {gridNb8, 2},
	"around4": {gridAround4, 2},
	"around8": {gridAround8, 2},
}

// dirs is how much of the direction table this walk uses; 0 means it walks the
// grid itself rather than a cell's neighbours.
func (k gridKind) dirs() int {
	switch k {
	case gridNb4, gridAround4:
		return 4
	case gridNb8, gridAround8:
		return 8
	}
	return 0
}

// yieldsKnot reports whether the walk hands back the coordinate rather than
// what is written there.
func (k gridKind) yieldsKnot() bool {
	return k == gridKnots || k == gridAround4 || k == gridAround8
}

func (s source) isSpan() bool    { return s.lo != nil || s.under != nil }
func (s source) isFlow() bool    { return s.flowFn != nil }
func (s source) isCycle() bool   { return s.cycle != nil }
func (s source) isZip() bool     { return s.zipA != nil }
func (s source) isItems() bool   { return s.items != nil }
func (s source) isCouples() bool { return s.couples != nil }
func (s source) isEnum() bool    { return s.enumed != nil }
func (s source) isGrid() bool    { return s.grid != gridNone }

// generated reports whether the loop makes its elements rather than reading
// them out of a Thread somebody had to build. When it does, fusing saves an
// array even with no stages at all, which is why these are worth fusing where
// a plain Thread source is not.
func (s source) generated() bool { return s.isSpan() || s.isGrid() }

// pairs reports whether the producer yields two halves rather than one value.
// A `zipwith` reads two Threads but yields what its function made of them.
func (s source) pairs() bool {
	return (s.isZip() && s.zipFn == nil) || s.isItems() || s.isCouples() || s.isEnum()
}

// endless reports whether the producer will keep yielding for ever, so that
// something downstream has to stop the chain.
func (s source) endless() bool { return s.isFlow() || s.isCycle() }

// pipeline is a recognised chain, in execution order.
type pipeline struct {
	producer source
	stages   []stage
	consumer consumerKind
	// elem names the primitive the chain yields, when it is one, so that a
	// summing consumer can fold with the typed operation.
	elem string
	// pred is the predicate of count, seek, any, all, seekidx and none.
	pred ast.Expr
	// match is the value `idx` and `has` are looking for, and at is the
	// position `nth` wants. Both stop the loop the way a predicate does, with
	// no function to call.
	match, at ast.Expr
	// braidFn and braidSeed belong to conBraid.
	braidFn, braidSeed ast.Expr
	// pack names the Value tag every element of a collected Thread will carry,
	// when the checker says they all carry the same one and it lives in the
	// Value itself. Then the loop writes payloads rather than Values: eight
	// bytes to the element rather than sixteen, and the tag written once into
	// the header. Empty means the ordinary layout. See the note in weave.h.
	pack string
}

// consumers maps a terminal verb to its kind and how many arguments it takes,
// with the Thread always last.
var consumers = map[string]struct {
	kind  consumerKind
	arity int
}{
	"sum":    {conSum, 1},
	"prod":   {conProduct, 1},
	"len":    {conSize, 1},
	"count":  {conCount, 2},
	"seek":   {conSeek, 2},
	"any":    {conAny, 2},
	"all":    {conAll, 2},
	"first":  {conFirst, 1},
	"braid":  {conBraid, 3},
	"dupe":   {conDupe, 1},
	"gentle": {conGentle, 3},
	// The rest of the verbs that answer without reading to the end. They are
	// here for the same reason `seek` is: fused they stop the loop, and only a
	// consumer the loop knows about can stop an endless one.
	"seekidx": {conSeekIdx, 2},
	"none":    {conNone, 2},
	"idx":     {conIdx, 2},
	"has":     {conHas, 2},
	"nth":     {conNth, 2},
	"second":  {conNth, 1},
}

// tryFuse compiles e as a fused loop, reporting false when it is not a chain
// worth fusing.
func (g *gen) tryFuse(b *body, e *ast.App, sc *scope) (string, bool) {
	p, ok := g.recognise(e, sc)
	// `-no-fuse` turns off an optimisation, not a feature: a flow has no
	// unfused form to fall back to, so it is compiled either way.
	if g.opts.DisableFusion && !p.producer.endless() {
		return "", false
	}
	if !ok {
		if p.producer.endless() {
			// recognise has already said why this one cannot be compiled;
			// claiming it keeps the general path from reporting it again.
			return "w_thread_empty()", true
		}
		return "", false
	}
	return g.emitFused(b, p, sc), true
}

// recognise matches the application nest against a pipeline shape.
func (g *gen) recognise(e *ast.App, sc *scope) (pipeline, bool) {
	var p pipeline

	// A terminal verb, if there is one.
	if name, ok := g.builtinCallee(e, sc); ok {
		if c, isConsumer := consumers[name]; isConsumer && len(e.Args) == c.arity &&
			(c.kind != conNth || g.isThread(e.Args[c.arity-1])) {
			// `nth` reads text as readily as a Thread — it rides `Strand`, not
			// the Thread type — and the loop below only knows how to walk a
			// Thread. The rest of the consumers name Thread in their own
			// signatures, so only this one has to be asked.
			p.consumer = c.kind
			switch c.kind {
			case conCount, conSeek, conAny, conAll, conSeekIdx, conNone:
				p.pred = e.Args[0]
			case conIdx, conHas:
				p.match = e.Args[0]
			case conNth:
				// `second` is `nth 1` with the position left out, which is
				// what the nil stands for.
				if c.arity == 2 {
					p.at = e.Args[0]
				}
			case conBraid, conGentle:
				p.braidFn, p.braidSeed = e.Args[0], e.Args[1]
			}
			// For sum and product the call's own type is the element type,
			// which is what the fold operates on.
			p.elem = g.primitiveType(e)
			rest, stages := g.peelStages(e.Args[c.arity-1], sc)
			p.stages = stages
			p.producer = g.recogniseSource(rest, p.consumer, sc)
			// One stage is enough to save an intermediate Thread — and a flow
			// has to be fused however few stages there are, since there is no
			// other way to run it. A fold is fused whatever it is over: the
			// loop replaces a closure call per element, and it is the only
			// shape in which the accumulator can be updated in place.
			// A generated producer is worth fusing with no stages at all: the
			// array it would have built is the whole saving, and for the
			// neighbour verbs that array is allocated once per cell.
			//
			// So is a consumer whose function is a lambda, for the reason
			// `braid` has always been fused whatever it is over: the loop
			// inlines the lambda, and what goes away is a heap closure built
			// every time the enclosing function runs plus an indirect call per
			// element. On a backtracking search that closure is most of the
			// memory — Advent of Code 2025 day 12 built nine million of them.
			return p, g.acceptEndless(e, p) &&
				(len(p.stages) >= 1 || p.producer.endless() ||
					p.producer.generated() || isLambda(p.pred) ||
					p.consumer == conBraid || p.consumer == conGentle ||
					p.consumer == conDupe || p.producer.zipFn != nil)
		}
	}

	// Otherwise the chain's result is itself a Thread.
	p.consumer = conCollect
	p.pack = g.packedElem(e)
	rest, stages := g.peelStages(e, sc)
	p.stages = stages
	p.producer = g.recogniseSource(rest, p.consumer, sc)
	if p.producer.endless() {
		return p, g.acceptEndless(e, p)
	}
	// With a single stage this would just be the runtime verb rewritten, so
	// only fuse once there is an intermediate to remove — unless the producer
	// yields pairs, where one stage already removes a Twine per element, or is
	// a `zipwith`, which is worth fusing on its own: the array gets written
	// either way, but the closure and the call per element do not.
	if p.producer.zipFn != nil {
		return p, true
	}
	if p.producer.pairs() || p.producer.generated() {
		return p, len(p.stages) >= 1
	}
	// One stage would just be the runtime verb rewritten — unless its function
	// is a lambda, which the loop inlines. The array is written either way, but
	// the closure built once per call and the indirect call per element are
	// not, and that is the same trade `zipwith` is fused for.
	if len(p.stages) == 1 && isLambda(p.stages[0].fn) {
		return p, true
	}
	return p, len(p.stages) >= 2
}

// isLambda reports whether a consumer's function is written out on the spot,
// so that fusing would inline it rather than call through a closure.
func isLambda(e ast.Expr) bool {
	_, ok := e.(*ast.Lambda)
	return ok
}

// acceptEndless checks that an endless producer is consumed by something that
// stops. Nothing else in the compiler can catch this, and the failure it
// prevents is a program that runs forever.
func (g *gen) acceptEndless(e *ast.App, p pipeline) bool {
	if !p.producer.endless() || p.bounded() {
		return true
	}
	verb := "flow"
	if p.producer.isCycle() {
		verb = "cycle"
	}
	g.bag.AddHint(e.Pos(),
		"end the chain with something that stops: `take n` or `takewhile p`, "+
			"or one of `seek`, `seekidx`, `first`, `second`, `nth`, `idx`, `has`, "+
			"`any`, `all`, `none`, `dupe`, `gentle`",
		"this pipeline would never finish: `%s` is endless", verb)
	return false
}

// recogniseSource spots a producer that can be generated rather than built.
func (g *gen) recogniseSource(e ast.Expr, con consumerKind, sc *scope) source {
	if app, ok := e.(*ast.App); ok {
		if name, isVerb := g.verbCallee(app, sc); isVerb {
			switch {
			case name == "span" && len(app.Args) == 2:
				return source{lo: app.Args[0], hi: app.Args[1]}
			case name == "under" && len(app.Args) == 1:
				return source{under: app.Args[0]}
			case name == "flow" && len(app.Args) == 2:
				return source{flowFn: app.Args[0], flowSeed: app.Args[1]}
			case name == "cycle" && len(app.Args) == 1:
				return source{cycle: app.Args[0]}
			case name == "zip" && len(app.Args) == 2:
				return source{zipA: app.Args[0], zipB: app.Args[1]}
			case name == "zipwith" && len(app.Args) == 3:
				return source{zipFn: app.Args[0], zipA: app.Args[1], zipB: app.Args[2]}
			case name == "items" && len(app.Args) == 1:
				return source{items: app.Args[0]}
			case name == "couples" && len(app.Args) == 1:
				return source{couples: app.Args[0]}
			case name == "enum" && len(app.Args) == 1:
				return source{enumed: app.Args[0]}
			}
			if gs, isGrid := gridSources[name]; isGrid && len(app.Args) == gs.arity &&
				(gs.kind.yieldsKnot() || (con != conBraid && con != conGentle)) {
				src := source{grid: gs.kind, gridPat: app.Args[0]}
				if gs.arity == 2 {
					src.gridKnot = app.Args[1]
				}
				return src
			}
		}
	}
	return source{expr: e}
}

// thins reports whether any stage can drop an element, so that the number the
// loop produces is not the number the producer offers.
func (p pipeline) thins() bool {
	for _, st := range p.stages {
		if st.kind != stageBend && st.kind != stageScan {
			return true
		}
	}
	return false
}

// bounded reports whether a pipeline can be relied on to stop. An endless flow
// needs this; a Thread or a span stops on its own.
func (p pipeline) bounded() bool {
	if p.consumer.shortCircuits() {
		return true
	}
	for _, st := range p.stages {
		if st.kind.bounds() {
			return true
		}
	}
	return false
}

// peelStages strips bend and sift applications off the outside of e, returning
// what remains and the stages in execution order.
func (g *gen) peelStages(e ast.Expr, sc *scope) (ast.Expr, []stage) {
	var reversed []stage
	cur := e
	for {
		app, ok := cur.(*ast.App)
		if !ok || len(app.Args) < 2 || len(app.Args) > 3 {
			break
		}
		name, ok := g.builtinCallee(app, sc)
		if !ok {
			break
		}
		// Every stage takes its Thread last; `scan` is the only one with two
		// arguments in front of it.
		one, two := len(app.Args) == 2, len(app.Args) == 3
		switch {
		case name == "bend" && one:
			reversed = append(reversed, stage{kind: stageBend, fn: app.Args[0]})
		case name == "sift" && one:
			reversed = append(reversed, stage{kind: stageSift, fn: app.Args[0]})
		case name == "cull" && one:
			reversed = append(reversed, stage{kind: stageCull, fn: app.Args[0]})
		case name == "take" && one && g.isThread(app.Args[1]):
			reversed = append(reversed, stage{kind: stageTake, fn: app.Args[0]})
		case name == "takewhile" && one:
			reversed = append(reversed, stage{kind: stageWhile, fn: app.Args[0]})
		case name == "drop" && one && g.isThread(app.Args[1]):
			reversed = append(reversed, stage{kind: stageDrop, fn: app.Args[0]})
		case name == "dropwhile" && one:
			reversed = append(reversed, stage{kind: stageSkip, fn: app.Args[0]})
		case name == "scan" && two:
			reversed = append(reversed, stage{stageScan, app.Args[0], app.Args[1]})
		default:
			return cur, reverseStages(reversed)
		}
		cur = app.Args[len(app.Args)-1]
	}
	return cur, reverseStages(reversed)
}

func reverseStages(s []stage) []stage {
	out := make([]stage, len(s))
	for i, v := range s {
		out[len(s)-1-i] = v
	}
	return out
}

// verbCallee names the prelude verb an application calls, provided the name has
// not been shadowed. It does not require the runtime to have a function for it:
// `flow` has none, since it exists only as a fused loop.
func (g *gen) verbCallee(e *ast.App, sc *scope) (string, bool) {
	v, ok := e.Fn.(*ast.Var)
	if !ok {
		return "", false
	}
	if _, shadowed := sc.lookup(v.Name); shadowed {
		return "", false
	}
	if g.defined(v.Name) {
		return "", false
	}
	return v.Name, true
}

// builtinCallee is verbCallee restricted to the verbs the runtime implements.
// isThread reports whether an expression is a Thread. `take`, `drop`, `sever`
// and `rev` carry the Ply Talent and so accept text as readily, and text is not
// something the loop fuser knows how to walk — `Source through take 5` is a
// substring, not a stage.
func (g *gen) isThread(e ast.Expr) bool {
	t, ok := g.info.Types[e]
	if !ok {
		return false
	}
	con, isCon := types.Resolve(t).(*types.Con)
	return isCon && con.Name == types.ThreadCon
}

func (g *gen) builtinCallee(e *ast.App, sc *scope) (string, bool) {
	name, ok := g.verbCallee(e, sc)
	if !ok {
		return "", false
	}
	if _, isBuiltin := builtins[name]; !isBuiltin {
		return "", false
	}
	return name, true
}

// defined reports whether the program declares this name at the top level, in
// which case it hides the prelude verb of the same name.
func (g *gen) defined(name string) bool {
	if _, isFn := g.topFns[name]; isFn {
		return true
	}
	_, isVal := g.topVals[name]
	return isVal
}

// ---------------------------------------------------------------- call plans

// callPlan says how to apply a stage's function to the loop variable. Deciding
// this once, before the loop, is what lets the body avoid both closure
// allocation and indirect calls in the common cases.
type callPlan struct {
	lambda *ast.Lambda // inline the body, binding the parameter
	cname  string      // call this C function directly
	fixed  []string    // already-hoisted leading arguments for cname
	value  string      // hoisted function value, called through w_callN
	// userFn marks cname as a compiled Weave definition, which takes
	// (env, args) rather than plain arguments. It is recorded rather than
	// inferred from the name, since the typed primitive helpers are also
	// called w_something.
	userFn bool
}

// planCall prepares to call fn with argc arguments, hoisting whatever must be
// evaluated once out of the loop.
func (g *gen) planCall(b *body, fn ast.Expr, argc int, sc *scope) callPlan {
	// A lambda of the right shape is inlined outright.
	if lam, ok := fn.(*ast.Lambda); ok && len(lam.Params) == argc {
		return callPlan{lambda: lam}
	}
	// So is a definition that *is* a lambda under another name: one clause,
	// patterns that cannot fail to match, and no mention of itself. Lifting a
	// fold's step out to a name is what you do the moment it grows past a line,
	// and it used to cost the whole loop — the closure, the call per element,
	// and, worst, the ownership analysis, since the update then happened inside
	// a function this one's bookkeeping does not reach. Inlining it puts the
	// body back where all of that already works.
	if lam, ok := g.lambdaForm(fn, argc, sc); ok {
		return callPlan{lambda: lam}
	}

	// A bare builtin of exactly the right arity becomes a direct C call. The
	// program's own definitions are looked at first: a top-level `sign` hides
	// the prelude's, and calling the prelude's would be a miscompilation.
	if v, ok := fn.(*ast.Var); ok {
		if _, shadowed := sc.lookup(v.Name); !shadowed {
			if arity, isTop := g.topFns[v.Name]; isTop && arity == argc {
				return callPlan{cname: g.cnames[v.Name], userFn: true}
			}
			if bi, isBuiltin := builtins[v.Name]; isBuiltin && bi.arity == argc && !g.defined(v.Name) {
				// A verb standing on its own as a stage — `sift even` — has no
				// operand to read a type off, but the checker recorded the type
				// of this very mention, and its argument side is what the loop
				// will feed it. Without this the loop calls the general,
				// tag-dispatching verb once per element, which on a twenty
				// million element chain is most of the running time.
				if typed, ok := g.specialCname(v.Name, g.argumentType(v)); ok {
					return callPlan{cname: typed}
				}
				return callPlan{cname: bi.cname}
			}
		}
	}

	// A partially applied builtin, such as `gt 10`: hoist the supplied
	// arguments and call the C function with them plus the loop variable.
	if app, ok := fn.(*ast.App); ok {
		if v, isVar := app.Fn.(*ast.Var); isVar {
			if _, shadowed := sc.lookup(v.Name); !shadowed && !g.defined(v.Name) {
				if bi, isBuiltin := builtins[v.Name]; isBuiltin && len(app.Args)+argc == bi.arity {
					fixed := make([]string, len(app.Args))
					for i, a := range app.Args {
						fixed[i] = g.hoist(b, g.expr(b, a, sc))
					}
					cname := bi.cname
					// `sift (gt 4)` compares at the type of the bound, so the
					// loop can use the typed comparison.
					if typed, ok := g.specialiseCall(v.Name, app.Args); ok {
						cname = typed
					}
					return callPlan{cname: cname, fixed: fixed}
				}
			}
		}
	}

	// Anything else: evaluate the function once, then call it each iteration.
	return callPlan{value: g.hoist(b, g.expr(b, fn, sc))}
}

// hoist binds a C expression to a temporary so it is evaluated once.
func (g *gen) hoist(b *body, expr string) string {
	name := fmt.Sprintf("h%d", g.fresh())
	b.line("Value %s = %s;", name, expr)
	return name
}

// emitCall applies a plan to arguments already held in C variables.
func (g *gen) emitCall(b *body, plan callPlan, args []string, sc *scope) string {
	if plan.lambda != nil {
		return g.inlineLambda(b, plan.lambda, args, sc)
	}
	if plan.cname != "" {
		all := append(append([]string{}, plan.fixed...), args...)
		if plan.userFn {
			arr := fmt.Sprintf("a%d", g.fresh())
			b.line("Value %s[] = {%s};", arr, strings.Join(all, ", "))
			return fmt.Sprintf("%s(NULL, %s)", plan.cname, arr)
		}
		return fmt.Sprintf("%s(%s)", plan.cname, strings.Join(all, ", "))
	}
	switch len(args) {
	case 1:
		return fmt.Sprintf("w_call1(%s, %s)", plan.value, args[0])
	case 2:
		return fmt.Sprintf("w_call2(%s, %s, %s)", plan.value, args[0], args[1])
	default:
		arr := fmt.Sprintf("a%d", g.fresh())
		b.line("Value %s[] = {%s};", arr, strings.Join(args, ", "))
		return fmt.Sprintf("w_call(%s, %s, %d)", plan.value, arr, len(args))
	}
}

// inlineLambda emits a lambda's body directly into the loop, with its
// parameters bound to the given variables.
func (g *gen) inlineLambda(b *body, lam *ast.Lambda, args []string, sc *scope) string {
	if g.lifted[lam] && !g.inFunc {
		g.inFunc = true
		defer func() { g.inFunc = false }()
	}
	inner := newScope(sc)
	var binds []binding
	for i, p := range lam.Params {
		// An accumulator carried a component at a time is bound from the
		// components; the Twine it would have been taken out of does not
		// exist, which is the point. See weaving.go.
		if i == 0 && g.weaving.parts != nil {
			if tw, ok := p.(*ast.PTwine); ok && len(tw.Elems) == len(g.weaving.parts) {
				for k, sub := range tw.Elems {
					cond, bs := g.match(sub, g.weaving.parts[k])
					if cond != "" {
						b.line("if (!(%s)) w_fail(\"argument did not match\");", cond)
					}
					binds = append(binds, bs...)
				}
				continue
			}
		}
		cond, bs := g.match(p, args[i])
		if cond != "" {
			b.line("if (!(%s)) w_fail(\"argument did not match\");", cond)
		}
		binds = append(binds, bs...)
	}
	g.emitBound(b, binds, inner)
	defer g.markFoldComponent(inner)()
	if g.splitStep(b, lam.Body, inner) {
		return ""
	}
	return g.expr(b, lam.Body, inner)
}

// markFoldComponent marks the half of a Twine accumulator that a fold's step
// has just bound, so that an update to it writes through. The C name is only
// known here, after the pattern has been taken apart, and it lasts exactly as
// long as the step's body.
func (g *gen) markFoldComponent(sc *scope) func() {
	if g.foldOwnedName == "" {
		return func() {}
	}
	cname, bound := sc.lookup(g.foldOwnedName)
	if !bound {
		return func() {}
	}
	g.owned[cname] = true
	return func() { delete(g.owned, cname) }
}

// lambdaForm reads a named definition as the lambda it already is.
//
// Only the shapes where that is exactly true: one clause, so there is no
// choice to make; every parameter an irrefutable pattern, so no match can
// fail; not memoised, since the point of `remember` is the lookup; and not
// recursive, since inlining a call to itself would not terminate. A top-level
// body sees only its parameters and other top-level names, both of which are
// in scope wherever it is called, so the substitution is sound.
func (g *gen) lambdaForm(fn ast.Expr, argc int, sc *scope) (*ast.Lambda, bool) {
	v, isVar := fn.(*ast.Var)
	if !isVar {
		return nil, false
	}
	if _, shadowed := sc.lookup(v.Name); shadowed {
		return nil, false
	}
	if v.Name == g.opts.Watch {
		// A watched function has to be called for its calls to be counted.
		// Inlining it here would put its body in the loop, where it is exactly
		// as invisible as it was before anybody asked to watch it — and a
		// `gentle` step lifted out to a name is the commonest thing to want to
		// watch.
		return nil, false
	}
	if arity, isTop := g.topFns[v.Name]; !isTop || arity != argc || g.memoed[v.Name] {
		return nil, false
	}
	d := g.byName[v.Name]
	if d == nil || len(d.Clauses) != 1 || len(d.Clauses[0].Params) != argc {
		return nil, false
	}
	for _, p := range d.Clauses[0].Params {
		if !irrefutable(p) {
			return nil, false
		}
	}
	free := map[string]bool{}
	ast.FreeVars(d.Clauses[0].Body, map[string]bool{}, free)
	if free[v.Name] {
		return nil, false
	}
	lam := &ast.Lambda{Params: d.Clauses[0].Params, Body: d.Clauses[0].Body}
	if g.opts.Trace {
		// It is a definition's body wherever it ends up, so a binding in it
		// still reports the first value it holds — which is the whole point of
		// that record, and the loop this is about to disappear into is exactly
		// where a body is hardest to see. Recorded here rather than inferred,
		// because a lambda written out on the spot is a different case: its
		// lines are the call's lines, and the chain already reports those.
		if g.lifted == nil {
			g.lifted = map[*ast.Lambda]bool{}
		}
		g.lifted[lam] = true
	}
	return lam, true
}

// irrefutable reports whether a pattern always matches, so that inlining it
// needs no test and can fail in no way the call could not.
func irrefutable(p ast.Pattern) bool {
	switch p := p.(type) {
	case *ast.PVar, *ast.PWild:
		return true
	case *ast.PTwine:
		for _, el := range p.Elems {
			if !irrefutable(el) {
				return false
			}
		}
		return true
	}
	return false
}

// pairParam reports the two-element Twine pattern a plan's last parameter takes
// apart, when the plan is a lambda of the right arity. That is the shape which
// lets a pair be consumed without being built.
func pairParam(plan callPlan, argc int) (*ast.PTwine, bool) {
	if plan.lambda == nil || len(plan.lambda.Params) != argc {
		return nil, false
	}
	tw, ok := plan.lambda.Params[argc-1].(*ast.PTwine)
	if !ok || len(tw.Elems) != 2 {
		return nil, false
	}
	return tw, true
}

// inlineLambdaPair inlines a lambda whose last parameter destructures a pair,
// binding the halves straight from the loop's two variables. Everything the
// body sees is identical to the ordinary path; the Twine simply never exists.
func (g *gen) inlineLambdaPair(b *body, lam *ast.Lambda, lead []string, left, right string, sc *scope) string {
	if g.lifted[lam] && !g.inFunc {
		g.inFunc = true
		defer func() { g.inFunc = false }()
	}
	inner := newScope(sc)
	var binds []binding
	bind := func(p ast.Pattern, subject string) {
		cond, bs := g.match(p, subject)
		if cond != "" {
			b.line("if (!(%s)) w_fail(\"argument did not match\");", cond)
		}
		binds = append(binds, bs...)
	}
	for i, p := range lam.Params[:len(lam.Params)-1] {
		// An accumulator carried a component at a time is bound from the
		// components. The Twine it would have been taken out of does not
		// exist, which is the point.
		if i == 0 && g.weaving.parts != nil {
			if tw, ok := p.(*ast.PTwine); ok && len(tw.Elems) == len(g.weaving.parts) {
				for k, sub := range tw.Elems {
					bind(sub, g.weaving.parts[k])
				}
				continue
			}
		}
		bind(p, lead[i])
	}
	tw := lam.Params[len(lam.Params)-1].(*ast.PTwine)
	bind(tw.Elems[0], left)
	bind(tw.Elems[1], right)
	g.emitBound(b, binds, inner)
	defer g.markFoldComponent(inner)()
	if g.splitStep(b, lam.Body, inner) {
		return ""
	}
	return g.expr(b, lam.Body, inner)
}

// pairKept reports whether everything in the chain that sees a pair takes it
// apart, so the loop need never build one. The pair survives only as far as the
// first `bend`, which replaces the element with whatever it returns.
func pairKept(p pipeline, plans, whilePlans []callPlan, braidPlan callPlan) bool {
	for i, st := range p.stages {
		switch st.kind {
		case stageTake:
			// counts, does not look at the element
		case stageWhile:
			if _, ok := pairParam(whilePlans[i], 1); !ok {
				return false
			}
		case stageSift, stageCull:
			if _, ok := pairParam(plans[i], 1); !ok {
				return false
			}
		case stageBend:
			_, ok := pairParam(plans[i], 1)
			return ok
		}
	}
	// No bend, so the consumer is what sees the pair.
	switch p.consumer {
	case conBraid:
		_, ok := pairParam(braidPlan, 2)
		return ok
	case conSize:
		return true // counts without reading
	}
	return false
}

// ------------------------------------------------------------------ emission

// emitFused writes the loop and returns the C expression holding its result.
func (g *gen) emitFused(b *body, p pipeline, sc *scope) string {
	// Everything that must happen once goes before the loop, in this order:
	// the stage functions, the consumer's function, then the source itself.
	plans := make([]callPlan, len(p.stages))
	for i, st := range p.stages {
		if st.kind == stageScan {
			plans[i] = g.planCall(b, st.fn, 2, sc)
			continue
		}
		plans[i] = g.planCall(b, st.fn, 1, sc)
	}
	var predPlan, braidPlan, zipPlan callPlan
	switch p.consumer {
	case conCount, conSeek, conAny, conAll, conSeekIdx, conNone:
		predPlan = g.planCall(b, p.pred, 1, sc)
	case conBraid, conGentle:
		braidPlan = g.planCall(b, p.braidFn, 2, sc)
	}
	if p.producer.zipFn != nil {
		zipPlan = g.planCall(b, p.producer.zipFn, 2, sc)
	}

	// Each bounding stage needs a counter or a test prepared before the loop,
	// and each scan an accumulator to carry across turns.
	takeLimits := make([]string, len(p.stages))
	whilePlans := make([]callPlan, len(p.stages))
	scanAccs := make([]string, len(p.stages))
	for i, st := range p.stages {
		switch st.kind {
		case stageTake, stageDrop:
			takeLimits[i] = g.hoist(b, g.expr(b, st.fn, sc))
		case stageWhile, stageSkip:
			whilePlans[i] = g.planCall(b, st.fn, 1, sc)
		case stageScan:
			scanAccs[i] = fmt.Sprintf("a%d", g.fresh())
			b.line("Value %s = %s;", scanAccs[i], g.expr(b, st.seed, sc))
		}
	}

	// A span becomes loop bounds and a flow a running state; anything else is
	// evaluated once and indexed.
	var src, n, lo, hi, seed, stepFn string
	var srcB, keys, vals string
	var gridPat, gridRow, gridCol, gridCols string
	var ci, cj string
	var flowPlan callPlan
	switch {
	case p.producer.isGrid():
		gridPat = g.hoist(b, g.expr(b, p.producer.gridPat, sc))
		n = fmt.Sprintf("n%d", g.fresh())
		if d := p.producer.grid.dirs(); d > 0 {
			// A neighbour walk is a fixed four or eight tries, each of which
			// may fall off the grid; the row and column it is around are read
			// once.
			k := g.hoist(b, g.expr(b, p.producer.gridKnot, sc))
			gridRow = fmt.Sprintf("gr%d", g.fresh())
			gridCol = fmt.Sprintf("gc%d", g.fresh())
			b.line("int64_t %s = (%s).knot.row;", gridRow, k)
			b.line("int64_t %s = (%s).knot.col;", gridCol, k)
			b.line("size_t %s = %d;", n, d)
		} else {
			rows := fmt.Sprintf("gw%d", g.fresh())
			gridCols = fmt.Sprintf("gh%d", g.fresh())
			b.line("size_t %s, %s;", rows, gridCols)
			b.line("size_t %s = w_pattern_shape(%s, &%s, &%s);", n, gridPat, rows, gridCols)
		}
	case p.producer.isZip():
		src = g.hoist(b, g.expr(b, p.producer.zipA, sc))
		srcB = g.hoist(b, g.expr(b, p.producer.zipB, sc))
		n = fmt.Sprintf("n%d", g.fresh())
		other := fmt.Sprintf("n%d", g.fresh())
		b.line("size_t %s = w_thread_len(%s);", n, src)
		b.line("size_t %s = w_thread_len(%s);", other, srcB)
		b.line("if (%s < %s) %s = %s;", other, n, n, other)
	case p.producer.isEnum():
		src = g.hoist(b, g.expr(b, p.producer.enumed, sc))
		n = fmt.Sprintf("n%d", g.fresh())
		b.line("size_t %s = w_thread_len(%s);", n, src)
	case p.producer.isCouples():
		src = g.hoist(b, g.expr(b, p.producer.couples, sc))
		n = fmt.Sprintf("n%d", g.fresh())
		cn := fmt.Sprintf("cn%d", g.fresh())
		b.line("size_t %s = w_thread_len(%s);", cn, src)
		b.line("size_t %s = %s < 2 ? 0 : %s * (%s - 1) / 2;", n, cn, cn, cn)
		ci = fmt.Sprintf("ci%d", g.fresh())
		cj = fmt.Sprintf("cj%d", g.fresh())
		b.line("size_t %s = 0, %s = 1;", ci, cj)
	case p.producer.isItems():
		web := g.hoist(b, g.expr(b, p.producer.items, sc))
		keys = fmt.Sprintf("ka%d", g.fresh())
		vals = fmt.Sprintf("va%d", g.fresh())
		n = fmt.Sprintf("n%d", g.fresh())
		b.line("Value *%s;", keys)
		b.line("Value *%s;", vals)
		b.line("size_t %s = w_web_entries(%s, &%s, &%s);", n, web, keys, vals)
	case p.producer.under != nil:
		// `under n` counts 0 to n-1, and none of them when n is not positive —
		// which the emptiness the bounds give is exactly, since 0 > -1.
		lo = g.hoist(b, "w_earth(0)")
		hi = g.hoist(b, fmt.Sprintf("w_sub_e(%s, w_earth(1))",
			g.expr(b, p.producer.under, sc)))
	case p.producer.isSpan():
		lo = g.hoist(b, g.expr(b, p.producer.lo, sc))
		hi = g.hoist(b, g.expr(b, p.producer.hi, sc))
	case p.producer.isFlow():
		flowPlan = g.planCall(b, p.producer.flowFn, 1, sc)
		seed = fmt.Sprintf("s%d", g.fresh())
		b.line("Value %s = %s;", seed, g.expr(b, p.producer.flowSeed, sc))
		stepFn = fmt.Sprintf("step%d", g.fresh())
		b.line("bool %s = false;", stepFn)
	case p.producer.isCycle():
		src = g.hoist(b, g.expr(b, p.producer.cycle, sc))
		n = fmt.Sprintf("n%d", g.fresh())
		b.line("size_t %s = w_thread_len(%s);", n, src)
	default:
		src = g.hoist(b, g.expr(b, p.producer.expr, sc))
		n = fmt.Sprintf("n%d", g.fresh())
		b.line("size_t %s = w_thread_len(%s);", n, src)
	}

	res := fmt.Sprintf("r%d", g.fresh())
	var out, k string

	cap, room := "", ""
	// wanted is the value `idx` and `has` are looking for, or the position
	// `nth` is counting to: whichever of the two the consumer stops on, worked
	// out once before the loop.
	wanted := ""
	// The accumulator's components, when a `gentle` carries them one to a
	// variable rather than as a Twine. See weaving.go.
	var gentleParts []string

	switch p.consumer {
	case conCollect:
		out = fmt.Sprintf("o%d", g.fresh())
		k = fmt.Sprintf("k%d", g.fresh())
		// A packed Thread is an array of payloads, so the buffer is one too;
		// everything else about the loop is the same.
		cell := "Value"
		if p.pack != "" {
			cell = "int64_t"
		}
		if p.producer.endless() {
			// An endless producer has no length to size against, so the
			// buffer grows.
			cap = fmt.Sprintf("cap%d", g.fresh())
			b.line("size_t %s = 16;", cap)
			b.line("%s *%s = (%s *)w_alloc(sizeof(%s) * %s);", cell, out, cell, cell, cap)
			room = cap
		} else {
			// A fused chain can only shrink, so one allocation of the source's
			// length is always enough and never needs to grow.
			count := n
			if p.producer.isSpan() {
				count = fmt.Sprintf("c%d", g.fresh())
				b.line("size_t %s = (%s).earth >= (%s).earth ? (size_t)((%s).earth - (%s).earth + 1) : 0;",
					count, hi, lo, hi, lo)
			}
			b.line("%s *%s = (%s *)w_alloc(sizeof(%s) * (%s ? %s : 1));", cell, out, cell, cell, count, count)
			room = fmt.Sprintf("(%s ? %s : 1)", count, count)
		}
		b.line("size_t %s = 0;", k)
	case conSum, conProduct:
		b.line("Value %s = w_earth(%d);", res, map[consumerKind]int{conSum: 0, conProduct: 1}[p.consumer])
		// A fold over Earths knows what its identity is and can start there.
		// Anything else has to wait for the first element to find out what
		// zero means, which is a test and two conditional moves per element —
		// on `span 1 n | bend f | sift p | sum` that was the whole gap to Go.
		//
		// Only Earth: `sum` of an empty Thread answers `w_earth(0)` whatever
		// the element type, so starting a Water fold at 0.0 would disagree
		// with the unfused verb on empty input, and the differential tests
		// would be right to say so.
		if p.elem != types.Earth {
			b.line("bool %s_started = false;", res)
		}
	case conSize, conCount:
		b.line("int64_t %s_n = 0;", res)
	case conSeek, conFirst:
		b.line("Value %s = w_stilled();", res)
	case conAny:
		b.line("Value %s = W_SHADOW;", res)
	case conAll:
		b.line("Value %s = W_LIGHT;", res)
	case conSeekIdx, conIdx:
		// Where, rather than what. The count is of elements that reached the
		// consumer, which is not the producer's index when a stage dropped
		// some — the same distinction `dupe` makes.
		b.line("Value %s = w_stilled();", res)
		b.line("int64_t %s_n = 0;", res)
		if p.consumer == conIdx {
			wanted = g.hoist(b, g.expr(b, p.match, sc))
		}
	case conHas:
		b.line("Value %s = W_SHADOW;", res)
		wanted = g.hoist(b, g.expr(b, p.match, sc))
	case conNone:
		b.line("Value %s = W_LIGHT;", res)
	case conNth:
		b.line("Value %s = w_stilled();", res)
		b.line("int64_t %s_n = 0;", res)
		want := "1" // `second`
		if p.at != nil {
			want = fmt.Sprintf("(%s).earth", g.expr(b, p.at, sc))
		}
		wanted = fmt.Sprintf("w%d", g.fresh())
		b.line("int64_t %s = %s;", wanted, want)
	case conBraid:
		b.line("Value %s = %s;", res, g.expr(b, p.braidSeed, sc))
	case conGentle:
		// The accumulator and the answer are the same variable: it holds the
		// last `Woven` until a step hands back a `Gentled`, and whichever it
		// ended on is what the fold answers.
		//
		// `_done` is which of the two it was. Holding that in a bool rather
		// than in a Weaving object is what lets a step assign the two of them
		// instead of building one per turn for the loop to take apart again.
		// See weaving.go; the Weaving is built once, below, because `gentle`
		// answers with one.
		acc := g.expr(b, p.braidSeed, sc)
		b.line("Value %s_acc = %s;", res, acc)
		b.line("Value %s_out = %s_acc;", res, res)
		b.line("bool %s_done = false;", res)
		if braidPlan.lambda != nil && g.splittableWeaving(braidPlan.lambda.Body, sc) {
			if n := g.splitAcc(braidPlan.lambda, sc); n > 0 {
				// The seed is taken apart once, here, and never put together
				// again until the loop is over.
				gentleParts = make([]string, n)
				for i := range gentleParts {
					gentleParts[i] = fmt.Sprintf("%s_acc%d", res, i)
					b.line("Value %s = w_twine_at(%s_acc, %d);", gentleParts[i], res, i)
				}
			}
		}
	case conDupe:
		// `dupe` answers both positions as well as the value, so the loop
		// counts what has passed it. That is not the producer's index: a stage
		// may have dropped elements before this one.
		//
		// The seen-set is a Web from the element to where it was, rather than
		// the Circle it reads as. A Circle is a Web of `Light` anyway, so the
		// slot the position goes in was already there. See wp_dupe.
		b.line("Value %s = w_stilled();", res)
		b.line("Value %s_seen = w_web_empty();", res)
		b.line("int64_t %s_n = 0;", res)
	}

	// A fold whose accumulator is only ever read and updated may write through
	// it. The seed arrives shared, so the first update still copies; what the
	// loop hands back escapes, so it is disowned below — unless the seed was
	// itself an owned accumulator, in which case the value is going straight
	// back into a slot nothing else can see and disowning it would make the
	// enclosing loop copy on its next turn.
	// `gentle` is `braid` that may stop, and it threads its accumulator exactly
	// as `braid` does — the difference is only that its step answers `Woven acc`
	// or `Gentled answer` rather than the accumulator bare, and that the
	// accumulator lives in its own variable because `res` holds the Weaving.
	foldAcc, inherited, foldAt := "", false, -1
	if p.consumer == conBraid || p.consumer == conGentle {
		accVar := res
		if p.consumer == conGentle {
			accVar = res + "_acc"
		}
		// The step as it will actually be emitted: `planCall` has already read a
		// named definition back as the lambda it is, and the proof has to be
		// run over that rather than over the name.
		step := p.braidFn
		if braidPlan.lambda != nil {
			step = braidPlan.lambda
		}
		if name, at, ok := g.foldOwned(step, p.braidSeed); ok {
			foldAcc = accVar
			// When the accumulator is a Twine of state, the thing to mark owned
			// is the half that is a collection, which the step's own pattern
			// binds. The inliner does the marking, since only it knows the C
			// name that half ends up in.
			foldAt = at
			if at >= 0 {
				g.foldOwnedName, g.foldOwnedAt = name, at
				defer func() { g.foldOwnedName, g.foldOwnedAt = "", -1 }()
			}
			if v, isVar := p.braidSeed.(*ast.Var); isVar {
				if cname, bound := sc.lookup(v.Name); bound && g.owned[cname] {
					inherited = true
				}
			}
			g.owned[accVar] = true
			defer delete(g.owned, accVar)
			// A map the loop owns, filled once per element, is told how many
			// are coming: otherwise it rehashes its way up from sixteen, which
			// costs as much again as the inserts. Only when the count is exact
			// — a stage that can drop an element would make it a guess, and
			// guessing high on a long source would reserve a great deal for
			// nothing.
			//
			// The accumulator, not the loop's answer. For a `braid` they are
			// the same variable; for a `gentle` they never were — the answer is
			// the Weaving, and reserving against it read a WData as a WMap.
			// That was undefined behaviour that happened to be harmless, and it
			// only came to light when the Weaving stopped existing until the
			// loop was over.
			if g.foldsIntoMap(p.braidSeed) && n != "" && !p.thins() {
				b.line("%s = w_web_reserve(%s, %s);", accVar, accVar, n)
			}
		}
	}

	// A take stage counts what has passed it, so its counter lives outside. So
	// does a drop's, and a dropwhile's flag: once its test has failed the rest
	// go through however they test, which is what makes it a stage and not a
	// sift.
	takeSeen := make([]string, len(p.stages))
	for idx, st := range p.stages {
		switch st.kind {
		case stageTake, stageDrop:
			takeSeen[idx] = fmt.Sprintf("t%d", g.fresh())
			b.line("int64_t %s = 0;", takeSeen[idx])
		case stageSkip:
			takeSeen[idx] = fmt.Sprintf("t%d", g.fresh())
			b.line("bool %s = false;", takeSeen[idx])
		}
	}

	// A turn whose whole storage is dead when it ends can hand it back, which
	// is what lets a backtracking search forget the branches it abandons.
	region := ""
	if g.turnReleasable(p, braidPlan, sc) {
		// The top-level values the body can reach are built here, outside every
		// region, rather than by whichever turn happens to mention one first —
		// a global holding storage a turn is about to forget would go on
		// holding it for the rest of the program.
		for _, name := range g.topValuesReached(braidPlan.lambda.Body, sc) {
			b.line("(void)%s();", g.cnames[name])
		}
		region = fmt.Sprintf("mk%d", g.fresh())
	}

	i := fmt.Sprintf("i%d", g.fresh())
	x := fmt.Sprintf("x%d", g.fresh())
	// left and right hold a pair the loop has not had to build. elem below
	// builds one on demand, for the consumers that need a whole value; keeping
	// it is decided before the loop, by pairKept.
	var left, right string
	switch {
	case p.producer.isZip() && p.producer.zipFn != nil:
		// `zipwith f a b`: read one element from each and combine them here.
		// f is planned like a stage's function, so a lambda is inlined and
		// nothing is called.
		b.open("for (size_t %s = 0; %s < %s; %s++) {", i, i, n, i)
		a := fmt.Sprintf("l%d", g.fresh())
		bb := fmt.Sprintf("r%d", g.fresh())
		b.line("Value %s = w_thread_at(%s, %s);", a, src, i)
		b.line("Value %s = w_thread_at(%s, %s);", bb, srcB, i)
		b.line("Value %s = %s;", x, g.emitCall(b, zipPlan, []string{a, bb}, sc))
	case p.producer.pairs():
		b.open("for (size_t %s = 0; %s < %s; %s++) {", i, i, n, i)
		left = fmt.Sprintf("l%d", g.fresh())
		right = fmt.Sprintf("r%d", g.fresh())
		if p.producer.isZip() {
			b.line("Value %s = w_thread_at(%s, %s);", left, src, i)
			b.line("Value %s = w_thread_at(%s, %s);", right, srcB, i)
		} else if p.producer.isEnum() {
			b.line("Value %s = w_earth((int64_t)%s);", left, i)
			b.line("Value %s = w_thread_at(%s, %s);", right, src, i)
		} else if p.producer.isCouples() {
			b.line("Value %s = w_thread_at(%s, %s);", left, src, ci)
			b.line("Value %s = w_thread_at(%s, %s);", right, src, cj)
			b.line("if (++%s == w_thread_len(%s)) { %s++; %s = %s + 1; }", cj, src, ci, cj, ci)
		} else {
			b.line("Value %s = %s[%s];", left, keys, i)
			b.line("Value %s = %s[%s];", right, vals, i)
		}
	case p.producer.isGrid():
		b.open("for (size_t %s = 0; %s < %s; %s++) {", i, i, n, i)
		if p.producer.grid.dirs() > 0 {
			rr := fmt.Sprintf("nr%d", g.fresh())
			cc := fmt.Sprintf("nc%d", g.fresh())
			b.line("int64_t %s = %s + w_grid_dr[%s];", rr, gridRow, i)
			b.line("int64_t %s = %s + w_grid_dc[%s];", cc, gridCol, i)
			b.line("if (!w_pattern_in(%s, %s, %s)) continue;", gridPat, rr, cc)
			if p.producer.grid.yieldsKnot() {
				b.line("Value %s = w_knot_make(%s, %s);", x, rr, cc)
			} else {
				b.line("Value %s = w_pattern_cell(%s, %s, %s);", x, gridPat, rr, cc)
			}
		} else if p.producer.grid.yieldsKnot() {
			// The row and column are carried rather than divided out, and they
			// advance before the body so that a stage which skips an element
			// still leaves them pointing at the next cell.
			r := fmt.Sprintf("kr%d", g.fresh())
			c := fmt.Sprintf("kc%d", g.fresh())
			b.line("int64_t %s = (int64_t)(%s / (%s ? %s : 1));", r, i, gridCols, gridCols)
			b.line("int64_t %s = (int64_t)(%s %% (%s ? %s : 1));", c, i, gridCols, gridCols)
			b.line("Value %s = w_knot_make(%s, %s);", x, r, c)
		} else {
			b.line("Value %s = w_pattern_cell(%s, (int64_t)(%s / (%s ? %s : 1)), (int64_t)(%s %% (%s ? %s : 1)));",
				x, gridPat, i, gridCols, gridCols, i, gridCols, gridCols)
		}
	case p.producer.isSpan():
		b.open("for (int64_t %s = (%s).earth; %s <= (%s).earth; %s++) {", i, lo, i, hi, i)
		if region != "" {
			// Everything this turn allocates is handed back at the end of it.
			// See regions.go for what had to be true for that to be allowed.
			b.line("WMark %s = w_mark();", region)
		}
		b.line("Value %s = w_earth(%s);", x, i)
	case p.producer.isFlow():
		b.open("for (;;) {")
		// The seed advances at the top rather than in the loop's increment, so
		// that a stage which skips an element with `continue` still moves on.
		b.open("if (%s) {", stepFn)
		next := g.emitCall(b, flowPlan, []string{seed}, sc)
		b.line("%s = %s;", seed, next)
		b.close("}")
		b.line("%s = true;", stepFn)
		b.line("Value %s = %s;", x, seed)
	case p.producer.isCycle():
		// An empty Thread cycles to nothing rather than for ever, which is the
		// only way this loop ends without something downstream stopping it —
		// so the emptiness test is the loop condition.
		b.open("for (size_t %s = 0; %s != 0; %s = (%s + 1) %% %s) {", i, n, i, i, n)
		b.line("Value %s = w_thread_at(%s, %s);", x, src, i)
	default:
		b.open("for (size_t %s = 0; %s < %s; %s++) {", i, i, n, i)
		b.line("Value %s = w_thread_at(%s, %s);", x, src, i)
	}

	// elem is the element as one value. For a pair producer that costs a Twine,
	// which is exactly what this is all for, so it is called only where
	// something genuinely needs the pair whole.
	elem := func() string {
		if left == "" {
			return x
		}
		arr := fmt.Sprintf("pr%d", g.fresh())
		b.line("Value %s[2] = {%s, %s};", arr, left, right)
		x = fmt.Sprintf("x%d", g.fresh())
		b.line("Value %s = w_twine_copy(%s, 2);", x, arr)
		left, right = "", ""
		return x
	}
	if p.producer.pairs() && !pairKept(p, plans, whilePlans, braidPlan) {
		elem()
	}

	for idx, st := range p.stages {
		switch st.kind {
		case stageSift, stageCull:
			var keep string
			if left != "" {
				keep = g.inlineLambdaPair(b, plans[idx].lambda, nil, left, right, sc)
			} else {
				keep = g.emitCall(b, plans[idx], []string{x}, sc)
			}
			if st.kind == stageSift {
				b.line("if (!(%s).spirit) continue;", keep)
			} else {
				b.line("if ((%s).spirit) continue;", keep)
			}
		case stageBend:
			var mapped string
			if left != "" {
				mapped = g.inlineLambdaPair(b, plans[idx].lambda, nil, left, right, sc)
				left, right = "", ""
			} else {
				mapped = g.emitCall(b, plans[idx], []string{x}, sc)
			}
			next := fmt.Sprintf("x%d", g.fresh())
			b.line("Value %s = %s;", next, mapped)
			x = next
		case stageTake:
			b.line("if (%s >= (%s).earth) break;", takeSeen[idx], takeLimits[idx])
			b.line("%s++;", takeSeen[idx])
		case stageDrop:
			b.open("if (%s < (%s).earth) {", takeSeen[idx], takeLimits[idx])
			b.line("%s++;", takeSeen[idx])
			b.line("continue;")
			b.close("}")
		case stageSkip:
			b.open("if (!%s) {", takeSeen[idx])
			var past string
			if left != "" {
				past = g.inlineLambdaPair(b, whilePlans[idx].lambda, nil, left, right, sc)
			} else {
				past = g.emitCall(b, whilePlans[idx], []string{x}, sc)
			}
			b.line("if ((%s).spirit) continue;", past)
			b.line("%s = true;", takeSeen[idx])
			b.close("}")
		case stageWhile:
			var keep string
			if left != "" {
				keep = g.inlineLambdaPair(b, whilePlans[idx].lambda, nil, left, right, sc)
			} else {
				keep = g.emitCall(b, whilePlans[idx], []string{x}, sc)
			}
			b.line("if (!(%s).spirit) break;", keep)
		case stageScan:
			// A bend with a memory: the accumulator lives outside the loop and
			// the element it yields is the new total.
			acc := scanAccs[idx]
			var step string
			if left != "" {
				step = g.inlineLambdaPair(b, plans[idx].lambda, []string{acc}, left, right, sc)
				left, right = "", ""
			} else {
				step = g.emitCall(b, plans[idx], []string{acc, x}, sc)
			}
			b.line("%s = %s;", acc, step)
			x = acc
		}
	}

	switch p.consumer {
	case conCollect:
		regrow := "w_regrow"
		if p.pack != "" {
			regrow = "w_regrow_packed"
		}
		if cap != "" {
			b.open("if (%s == %s) {", k, cap)
			b.line("%s = %s(%s, %s, %s * 2);", out, regrow, out, k, cap)
			b.line("%s *= 2;", cap)
			b.close("}")
		}
		if p.pack != "" {
			// The tag is the header's, so only the payload is written.
			b.line("%s[%s++] = (%s).earth;", out, k, elem())
			break
		}
		b.line("%s[%s++] = %s;", out, k, elem())
	case conSum, conProduct:
		name := "add"
		verb := "wp_add"
		if p.consumer == conProduct {
			name, verb = "mul", "wp_mul"
		}
		if typed, ok := g.specialCname(name, p.elem); ok {
			verb = typed
		}
		ex := elem()
		if p.elem == types.Earth {
			b.line("%s = %s(%s, %s);", res, verb, res, ex)
			break
		}
		b.open("if (%s_started) {", res)
		b.line("%s = %s(%s, %s);", res, verb, res, ex)
		b.close("} else {")
		b.indent++
		b.line("%s = %s;", res, ex)
		b.line("%s_started = true;", res)
		b.close("}")
	case conSize:
		b.line("%s_n++;", res)
	case conCount:
		hit := g.emitCall(b, predPlan, []string{elem()}, sc)
		b.line("if ((%s).spirit) %s_n++;", hit, res)
	case conSeek:
		ex := elem()
		hit := g.emitCall(b, predPlan, []string{ex}, sc)
		b.open("if ((%s).spirit) {", hit)
		b.line("%s = w_held(%s);", res, ex)
		b.line("break;")
		b.close("}")
	case conFirst:
		b.line("%s = w_held(%s);", res, elem())
		b.line("break;")
	case conAny:
		hit := g.emitCall(b, predPlan, []string{elem()}, sc)
		b.open("if ((%s).spirit) {", hit)
		b.line("%s = W_LIGHT;", res)
		b.line("break;")
		b.close("}")
	case conAll:
		hit := g.emitCall(b, predPlan, []string{elem()}, sc)
		b.open("if (!(%s).spirit) {", hit)
		b.line("%s = W_SHADOW;", res)
		b.line("break;")
		b.close("}")
	case conNone:
		hit := g.emitCall(b, predPlan, []string{elem()}, sc)
		b.open("if ((%s).spirit) {", hit)
		b.line("%s = W_SHADOW;", res)
		b.line("break;")
		b.close("}")
	case conSeekIdx:
		hit := g.emitCall(b, predPlan, []string{elem()}, sc)
		b.open("if ((%s).spirit) {", hit)
		b.line("%s = w_held(w_earth(%s_n));", res, res)
		b.line("break;")
		b.close("}")
		b.line("%s_n++;", res)
	case conIdx:
		b.open("if (w_equal(%s, %s)) {", g.hoist(b, elem()), wanted)
		b.line("%s = w_held(w_earth(%s_n));", res, res)
		b.line("break;")
		b.close("}")
		b.line("%s_n++;", res)
	case conHas:
		b.open("if (w_equal(%s, %s)) {", g.hoist(b, elem()), wanted)
		b.line("%s = W_LIGHT;", res)
		b.line("break;")
		b.close("}")
	case conNth:
		// A position before the start is nowhere, and saying so has to end the
		// loop rather than count towards it: an endless producer would
		// otherwise be asked for an element it can never reach.
		b.line("if (%s < 0) break;", wanted)
		b.open("if (%s_n == %s) {", res, wanted)
		b.line("%s = w_held(%s);", res, elem())
		b.line("break;")
		b.close("}")
		b.line("%s_n++;", res)
	case conBraid:
		var step string
		if left != "" {
			step = g.inlineLambdaPair(b, braidPlan.lambda, []string{res}, left, right, sc)
		} else {
			step = g.emitCall(b, braidPlan, []string{res, x}, sc)
		}
		b.line("%s = %s;", res, step)
		if region != "" {
			// The accumulator is an unboxed Power, so it points at nothing the
			// mark is about to take back. Everything else this turn built is
			// unreachable the moment the line above has run.
			b.line("w_release(%s);", region)
		}
	case conGentle:
		// A step written in the shapes weaving.go knows assigns the
		// accumulator and the flag outright. Anything else builds a Weaving,
		// and the two lines below take it apart exactly as they always did.
		if braidPlan.lambda != nil {
			g.weaving = weavingSplit{
				acc: res + "_acc", out: res + "_out", done: res + "_done",
				parts: gentleParts,
			}
		}
		var step string
		if left != "" {
			step = g.inlineLambdaPair(b, braidPlan.lambda, []string{res + "_acc"}, left, right, sc)
		} else {
			step = g.emitCall(b, braidPlan, []string{res + "_acc", x}, sc)
		}
		g.weaving = weavingSplit{}
		if step != "" {
			tmp := b.tmp()
			b.line("%s = %s;", tmp, step)
			b.line("%s_done = w_data_index(%s) != 0;", res, tmp)
			b.open("if (%s_done) {", res)
			b.line("%s_out = w_data_field(%s, 0);", res, tmp)
			b.close("} else {")
			b.indent++
			b.line("%s_acc = w_data_field(%s, 0);", res, tmp)
			b.close("}")
		}
		b.line("if (%s_done) break;", res)
	case conDupe:
		ex := g.hoist(b, elem())
		first := fmt.Sprintf("fd%d", g.fresh())
		b.line("Value %s;", first)
		b.open("if (w_web_find(%s_seen, %s, &%s)) {", res, ex, first)
		found := fmt.Sprintf("pd%d", g.fresh())
		b.line("Value %s[3] = {w_earth((int64_t)%s_n), %s, %s};", found, res, first, ex)
		b.line("%s = w_held(w_twine_copy(%s, 3));", res, found)
		b.line("break;")
		b.close("}")
		b.line("%s_seen = w_web_put_owned(%s_seen, %s, w_earth(%s_n));", res, res, ex, res)
		b.line("%s_n++;", res)
	}

	b.close("}")

	if p.producer.isItems() {
		// The two arrays w_web_entries filled belong to this loop and to
		// nothing else: the elements were copied out of them, not into.
		b.line("w_free(%s, sizeof(Value) * (%s ? %s : 1));", keys, n, n)
		b.line("w_free(%s, sizeof(Value) * (%s ? %s : 1));", vals, n, n)
	}

	if p.consumer == conGentle && gentleParts != nil {
		// The accumulator, put together once, for whoever reads it after the
		// loop — the Weaving below, and the disown that ends its ownership.
		b.line("Value %s_parts[] = {%s};", res, strings.Join(gentleParts, ", "))
		b.line("%s_acc = w_twine_copy(%s_parts, %d);", res, res, len(gentleParts))
	}

	if p.consumer == conGentle {
		// The Weaving, built once. Inside the loop it was a Value and a bool,
		// which is what let the step assign them rather than allocate an
		// object per turn for the loop to take straight back apart; but
		// `gentle` answers with a Weaving, and `failing` reads which case it
		// was, so one has to exist by the time the loop is over.
		b.line("Value %s = %s_done ? w_data(\"Gentled\", 1, (Value[]){%s_out}, 1)",
			res, res, res)
		b.line("                 : w_data(\"Woven\", 0, (Value[]){%s_acc}, 1);", res)
	}

	switch p.consumer {
	case conCollect:
		// A chain that filtered has left room at the end of the buffer; the
		// tail goes back rather than sitting inside the Thread for ever.
		if p.pack != "" {
			return fmt.Sprintf("w_thread_packed_fit(%s, %s, %s, %s)", out, k, room, p.pack)
		}
		return fmt.Sprintf("w_thread_fit(%s, %s, %s)", out, k, room)
	case conSize, conCount:
		return fmt.Sprintf("w_earth(%s_n)", res)
	default:
		if foldAcc != "" && !inherited {
			// The accumulator leaves the loop, so whoever gets it is a second
			// reader and it must stop being writable.
			//
			// Three shapes. A `gentle` hands back a Weaving rather than the
			// accumulator, so the accumulator is disowned on its own — it is
			// the object the Woven case wraps. An accumulator that is a Twine
			// of state is not itself writable; the half that is a collection
			// is, and that is what has to stop. And the plain case is the
			// accumulator itself.
			leaving := foldAcc
			if foldAt >= 0 {
				leaving = fmt.Sprintf("w_twine_at(%s, %d)", foldAcc, foldAt)
			}
			if p.consumer == conGentle || foldAt >= 0 {
				b.line("w_disown(%s);", leaving)
				return res
			}
			return fmt.Sprintf("w_disown(%s)", res)
		}
		return res
	}
}
