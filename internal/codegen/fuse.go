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
)

// shortCircuits reports whether the consumer can stop before the end.
func (c consumerKind) shortCircuits() bool {
	switch c {
	case conSeek, conAny, conAll, conFirst, conDupe, conGentle:
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
}

func (s source) isSpan() bool  { return s.lo != nil }
func (s source) isFlow() bool  { return s.flowFn != nil }
func (s source) isCycle() bool { return s.cycle != nil }
func (s source) isZip() bool   { return s.zipA != nil }
func (s source) isItems() bool { return s.items != nil }

// pairs reports whether the producer yields two halves rather than one value.
// A `zipwith` reads two Threads but yields what its function made of them.
func (s source) pairs() bool { return (s.isZip() && s.zipFn == nil) || s.isItems() }

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
	// pred is the predicate of count, seek, any and all.
	pred ast.Expr
	// braidFn and braidSeed belong to conBraid.
	braidFn, braidSeed ast.Expr
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
		if c, isConsumer := consumers[name]; isConsumer && len(e.Args) == c.arity {
			p.consumer = c.kind
			switch c.kind {
			case conCount, conSeek, conAny, conAll:
				p.pred = e.Args[0]
			case conBraid, conGentle:
				p.braidFn, p.braidSeed = e.Args[0], e.Args[1]
			}
			// For sum and product the call's own type is the element type,
			// which is what the fold operates on.
			p.elem = g.primitiveType(e)
			rest, stages := g.peelStages(e.Args[c.arity-1], sc)
			p.stages = stages
			p.producer = g.recogniseSource(rest, sc)
			// One stage is enough to save an intermediate Thread — and a flow
			// has to be fused however few stages there are, since there is no
			// other way to run it. A fold is fused whatever it is over: the
			// loop replaces a closure call per element, and it is the only
			// shape in which the accumulator can be updated in place.
			return p, g.acceptEndless(e, p) &&
				(len(p.stages) >= 1 || p.producer.endless() ||
					p.consumer == conBraid || p.consumer == conGentle ||
					p.consumer == conDupe || p.producer.zipFn != nil)
		}
	}

	// Otherwise the chain's result is itself a Thread.
	p.consumer = conCollect
	rest, stages := g.peelStages(e, sc)
	p.stages = stages
	p.producer = g.recogniseSource(rest, sc)
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
	if p.producer.pairs() {
		return p, len(p.stages) >= 1
	}
	return p, len(p.stages) >= 2
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
		"end the chain with `take n`, `takewhile p`, `seek p`, `first`, `any p` or `all p`",
		"this pipeline would never finish: `%s` is endless", verb)
	return false
}

// recogniseSource spots a producer that can be generated rather than built.
func (g *gen) recogniseSource(e ast.Expr, sc *scope) source {
	if app, ok := e.(*ast.App); ok {
		if name, isVerb := g.verbCallee(app, sc); isVerb {
			switch {
			case name == "span" && len(app.Args) == 2:
				return source{lo: app.Args[0], hi: app.Args[1]}
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
		case name == "take" && one:
			reversed = append(reversed, stage{kind: stageTake, fn: app.Args[0]})
		case name == "takewhile" && one:
			reversed = append(reversed, stage{kind: stageWhile, fn: app.Args[0]})
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
	inner := newScope(sc)
	var binds []binding
	for i, p := range lam.Params {
		cond, bs := g.match(p, args[i])
		if cond != "" {
			b.line("if (!(%s)) w_fail(\"argument did not match\");", cond)
		}
		binds = append(binds, bs...)
	}
	g.emitBound(b, binds, inner)
	return g.expr(b, lam.Body, inner)
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
		bind(p, lead[i])
	}
	tw := lam.Params[len(lam.Params)-1].(*ast.PTwine)
	bind(tw.Elems[0], left)
	bind(tw.Elems[1], right)
	g.emitBound(b, binds, inner)
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
	case conCount, conSeek, conAny, conAll:
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
		case stageTake:
			takeLimits[i] = g.hoist(b, g.expr(b, st.fn, sc))
		case stageWhile:
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
	var flowPlan callPlan
	switch {
	case p.producer.isZip():
		src = g.hoist(b, g.expr(b, p.producer.zipA, sc))
		srcB = g.hoist(b, g.expr(b, p.producer.zipB, sc))
		n = fmt.Sprintf("n%d", g.fresh())
		other := fmt.Sprintf("n%d", g.fresh())
		b.line("size_t %s = w_thread_len(%s);", n, src)
		b.line("size_t %s = w_thread_len(%s);", other, srcB)
		b.line("if (%s < %s) %s = %s;", other, n, n, other)
	case p.producer.isItems():
		web := g.hoist(b, g.expr(b, p.producer.items, sc))
		keys = fmt.Sprintf("ka%d", g.fresh())
		vals = fmt.Sprintf("va%d", g.fresh())
		n = fmt.Sprintf("n%d", g.fresh())
		b.line("Value *%s;", keys)
		b.line("Value *%s;", vals)
		b.line("size_t %s = w_web_entries(%s, &%s, &%s);", n, web, keys, vals)
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
	switch p.consumer {
	case conCollect:
		out = fmt.Sprintf("o%d", g.fresh())
		k = fmt.Sprintf("k%d", g.fresh())
		if p.producer.endless() {
			// An endless producer has no length to size against, so the
			// buffer grows.
			cap = fmt.Sprintf("cap%d", g.fresh())
			b.line("size_t %s = 16;", cap)
			b.line("Value *%s = (Value *)w_alloc(sizeof(Value) * %s);", out, cap)
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
			b.line("Value *%s = (Value *)w_alloc(sizeof(Value) * (%s ? %s : 1));", out, count, count)
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
	case conBraid:
		b.line("Value %s = %s;", res, g.expr(b, p.braidSeed, sc))
	case conGentle:
		// The accumulator and the answer are the same variable: it holds the
		// last `Woven` until a step hands back a `Gentled`, and whichever it
		// ended on is what the fold answers.
		acc := g.expr(b, p.braidSeed, sc)
		b.line("Value %s_acc = %s;", res, acc)
		b.line("Value %s = w_data(\"Woven\", 0, &%s_acc, 1);", res, res)
	case conDupe:
		// `dupe` answers where the repeat is as well as what it is, so the
		// loop counts what has passed it. That is not the producer's index: a
		// stage may have dropped elements before this one.
		b.line("Value %s = w_stilled();", res)
		b.line("Value %s_seen = w_circle_empty();", res)
		b.line("int64_t %s_n = 0;", res)
	}

	// A fold whose accumulator is only ever read and updated may write through
	// it. The seed arrives shared, so the first update still copies; what the
	// loop hands back escapes, so it is disowned below — unless the seed was
	// itself an owned accumulator, in which case the value is going straight
	// back into a slot nothing else can see and disowning it would make the
	// enclosing loop copy on its next turn.
	foldAcc, inherited := "", false
	if p.consumer == conBraid {
		if _, ok := g.foldOwned(p.braidFn, p.braidSeed); ok {
			foldAcc = res
			if v, isVar := p.braidSeed.(*ast.Var); isVar {
				if cname, bound := sc.lookup(v.Name); bound && g.owned[cname] {
					inherited = true
				}
			}
			g.owned[res] = true
			defer delete(g.owned, res)
			// A map the loop owns, filled once per element, is told how many
			// are coming: otherwise it rehashes its way up from sixteen, which
			// costs as much again as the inserts. Only when the count is exact
			// — a stage that can drop an element would make it a guess, and
			// guessing high on a long source would reserve a great deal for
			// nothing.
			if g.foldsIntoMap(p.braidSeed) && n != "" && !p.thins() {
				b.line("%s = w_web_reserve(%s, %s);", res, res, n)
			}
		}
	}

	// A take stage counts what has passed it, so its counter lives outside.
	takeSeen := make([]string, len(p.stages))
	for idx, st := range p.stages {
		if st.kind == stageTake {
			takeSeen[idx] = fmt.Sprintf("t%d", g.fresh())
			b.line("int64_t %s = 0;", takeSeen[idx])
		}
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
		} else {
			b.line("Value %s = %s[%s];", left, keys, i)
			b.line("Value %s = %s[%s];", right, vals, i)
		}
	case p.producer.isSpan():
		b.open("for (int64_t %s = (%s).earth; %s <= (%s).earth; %s++) {", i, lo, i, hi, i)
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
		if cap != "" {
			b.open("if (%s == %s) {", k, cap)
			b.line("%s = w_regrow(%s, %s, %s * 2);", out, out, k, cap)
			b.line("%s *= 2;", cap)
			b.close("}")
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
	case conBraid:
		var step string
		if left != "" {
			step = g.inlineLambdaPair(b, braidPlan.lambda, []string{res}, left, right, sc)
		} else {
			step = g.emitCall(b, braidPlan, []string{res, x}, sc)
		}
		b.line("%s = %s;", res, step)
	case conGentle:
		var step string
		if left != "" {
			step = g.inlineLambdaPair(b, braidPlan.lambda, []string{res + "_acc"}, left, right, sc)
		} else {
			step = g.emitCall(b, braidPlan, []string{res + "_acc", x}, sc)
		}
		b.line("%s = %s;", res, step)
		b.line("if (w_data_index(%s) != 0) break;", res)
		b.line("%s_acc = w_data_field(%s, 0);", res, res)
	case conDupe:
		ex := elem()
		b.open("if (w_web_has(%s_seen, %s)) {", res, ex)
		pair := fmt.Sprintf("pd%d", g.fresh())
		b.line("Value %s[2] = {w_earth((int64_t)%s_n), %s};", pair, res, ex)
		b.line("%s = w_held(w_twine_copy(%s, 2));", res, pair)
		b.line("break;")
		b.close("}")
		b.line("%s_seen = wp_insert_owned(%s_seen, %s);", res, res, ex)
		b.line("%s_n++;", res)
	}

	b.close("}")

	if p.producer.isItems() {
		// The two arrays w_web_entries filled belong to this loop and to
		// nothing else: the elements were copied out of them, not into.
		b.line("w_free(%s, sizeof(Value) * (%s ? %s : 1));", keys, n, n)
		b.line("w_free(%s, sizeof(Value) * (%s ? %s : 1));", vals, n, n)
	}

	switch p.consumer {
	case conCollect:
		// A chain that filtered has left room at the end of the buffer; the
		// tail goes back rather than sitting inside the Thread for ever.
		return fmt.Sprintf("w_thread_fit(%s, %s, %s)", out, k, room)
	case conSize, conCount:
		return fmt.Sprintf("w_earth(%s_n)", res)
	default:
		if foldAcc != "" && !inherited {
			// The accumulator leaves the loop, so whoever gets it is a second
			// reader and it must stop being writable.
			return fmt.Sprintf("w_disown(%s)", res)
		}
		return res
	}
}
