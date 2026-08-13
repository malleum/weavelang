# TODO

Everything that has come up in Weave's design and is not built yet, in the
order it was asked for. Anything already working is in the README's status
table instead.

Prose does not follow a change here. A feature lands in the compiler and gets
a line under **Documentation owed** at the bottom of this file; the SPEC, the
README and the tutorial are brought up to date in batches, once the feature has
been used enough to be worth writing about.

## Done

One line each; the detail lives in the commits.

- [x] **Verbs named after the Powers, and `parse` retired.**
- [x] **Indexing, membership, and the endless Thread that repeats** — `nth`, `has`, `glean`, `cycle`, `weft`, `digit`.
- [x] **A rename ledger**, so a retired name cannot come back anywhere.
- [x] **The REPL accepts an incomplete definition**: exhaustiveness is a soft diagnostic.
- [x] **The language server's empty replies** now send `result: null`.
- [x] **The language server inside ` ```weave ` blocks in Markdown.**
- [x] **More than one bare expression per file.**
- [x] **A pipeline blames the stage that failed**, not the head of the chain.
- [x] **The graph verbs, and `spin`/`flip`/`perms`** — `reach`, `route`, `toposort`.
- [x] **`weave test`**, over the `.in`/`.out` pairs beside a program.
- [x] **A local function can call itself.**
- [x] **`harvest`**, `glean` but `Gentled` with the element that would not convert.
- [x] **A number literal takes its Power from context.**
- [x] **In-place updates inside a fold**, so `braid` is as fast as the recursion.
- [x] **`Twine`, not "tuple".**
- [x] **Taking a Thread apart in a pattern** — `[a b]` and `[x ..rest]`.
- [x] **`gives`, `is` and the hole words**, spellings of `:`, `=` and `_`.
- [x] **A Water prints exactly**: the shortest text that reads back the same.
- [x] **`inc` and `dec`.**
- [x] **Vim motions and history in `weave repl`**, on raw `syscall.Termios`.
- [x] **`commentstring` for Neovim.**
- [x] **HAMT nodes grow with headroom**, so a path copy is not a realloc per insert.
- [x] **A benchmark suite against Go**, `make bench`.
- [x] **Documentation that cannot rot**: generated `docs/verbs.md`, a staleness test, and a test that every ` ```weave ` block compiles and prints what it says.
- [x] **`dijkstra` owns its working structures.**
- [x] **Specialise a verb standing on its own as a stage.**
- [x] **In-place updates for `Web` and `Circle`**, one ownership bit per trie node.
- [x] **Mutual tail calls**, compiled as one loop with a state variable.
- [x] **Unambiguous rendering**: `Show` prints text quoted inside a collection, and only inside one — at the top level the text is the answer.
- [x] **`weave fmt -`**, reading stdin.
- [x] **Pairwise and deeper verbs** — `pairs`, `cross`, `combos`, `windows`, `flat`.
- [x] **weave.nvim, and the nixvim wiring.**
- [x] **`Weaving` at runtime, and `dijkstra`.**
- [x] **`remember` — memoisation**, keyed on the arguments.
- [x] **`flow` — endless sequences**, which exist only as a fused loop.
- [x] **Tree-sitter grammar**, with the highlighter queries generated from it.
- [x] **`_` for the argument you did not name.**
- [x] **User-defined sum types**, with Maranget exhaustiveness.
- [x] **A flat hash table for a map the compiler owns** (dual representation).
- [x] **`remember` has a table of its own**, not the general map.
- [x] **The arena reuses what is provably dead** (free lists).
- [x] **A pair that is taken apart is never built** — `zip` and `items` fusion.
- [x] **A function hands back the Threads it built and nobody else can see** (escape analysis).
- [x] **Structured input parsing: `delve`.**
- [x] **A map is read back without a comparison sort** (radix-sorted iteration).
- [x] **`zipwith` is a producer, not a call.**
- [x] **A Thread can be edited** — `weld`, `mend`, `sever`, `strands`, `plait`, `cull`, `thread`.
- [x] **`split ""` was handing back Fires typed as Air.**
- [x] **The boxing was already gone; the fold's flag was not.**
- [x] **A `Held` of a Power is not a box.**
- [x] **`weave run` prints every bare expression**, not only the last.
- [x] **Ghost text per pipeline stage**, one value per source line a chain spans.
- [x] **`this`, `that`, `fore` and `aft`** — the first two arguments, and the two halves of the first.
- [x] **`as` implies a verb**: `as f` is `through bend f`, as `where p` is `through sift p`.
- [x] **`scan` is a fusible stage**, one running total per element, so `sums` is `scan add 0`.
- [x] **`dupe`**, the first element seen before — a `seek` with a memory.
- [x] **`gentle`**, a braid that stops when the step answers `Gentled`.
- [x] **The formatter reaches for the words**: `where`/`as` in the wordy style, `| sift`/`| bend` in the terse one, and the hole words wherever a lambda's parameters can be dropped.
- [x] **The formatter defaults to the wordy spelling**; `weave fmt -terse` prints the symbols.
- [x] **The formatter refuses a program it cannot parse**, rather than printing it with the unreadable part missing.
- [x] **Ghost text on an endless chain**: a chain holding a `flow` or a `cycle` is traced whole, since a prefix of one would not compile.
- [x] **`else` and `failing`**, particles for `through otherwise` and `through snag`.
- [x] **`snag`**, `rescue` from the other side: the value a Weaving stopped on, or a default.
- [x] **`siftidx` and `idxs`**, the all-of-them twins of `seekidx` and `idx`.
- [x] **Taking out is as cheap as putting in.** `forget` and `remove` write through on an owned Web or Circle, and a flat map forgets flatly rather than turning itself into a trie to lose one key. Both were ~50× slower and ~12× the memory of their adding twins.
- [x] **Advent of Code, from inside the editor** — `:AocInput`, `:AocProblem`, `:AocSubmit`, `:AocTime`, with the puzzle read off the directory names and the session cookie read from a file.
- [x] **Ghost text survives a compile error.** `weave trace` leaves out the top-level items the mistake reached and reports the rest, at the lines they are on, so the values stay put while a line is being typed.
- [x] **The formatter breaks one long stage across lines**, at its arguments, with a further indent so a continuation cannot be read as the next stage.
- [x] **A ward's arms may be bracketed on its own line** — `ward c (Light : 1) (_ : 0)` — which is also the only form an expression has room for.
- [x] **`weave docs`**, the reference as a searchable page served on localhost (or written out with `-o`), generated from the prelude and the lexer's keyword table: every verb, a second gloss for someone who already writes Weave, the eight language cameos, and the death gate.
- [x] **`high`, `low`, `highidx`, `lowidx`, `seekidx`** — the largest and smallest elements, and where they and the first match are.
- [x] **`dupe` says where as well as what**, answering `Hold (Earth, a)`.
- [x] **`twist i f xs`**, `mend` when the new value is worked out from the old one.
- [x] **Range verbs** — `overlaps`, `overlapping`, `within`, `spanning`, `holding`, `width`, over inclusive Twines of anything ordered.
- [x] **A Thread updates in place.** `mend` threaded through a loop writes through instead of copying the whole array; 200,000 updates of a 1024-element Thread went from 20.3 s and 3139 MB to 0.00 s and 10 MB.
- [x] **`pick`'s branches are tail position for the in-place analysis** too, as they already were for tail calls. The same loop was 0.00 s written with `ward` and 23 s written with `pick`.
- [x] **Filler words: no more for now.** A word that could ever be a verb silently rewrites working code, so `of`, `at`, `from`, `in`, `by`, `with`, `and`, `or` and `not` are permanently out.
- [x] **`thread` casts a Twine to a Thread.**
- [x] **Completions say they are plain text.**
- [x] **The grammar knows a ward's bracketed arms.** `ward c (Light : 1) (_ : 0)` is a `ward` with `ward_arm_inline` arms rather than `c` applied to two lambdas, so structural selection and folding see the right shape; a ward is an atom, as it is in the compiler's parser, but only in its inline form.
- [x] **An editor answers in the reference's voice.** Hover, completion and signature help show `weave docs`' gloss rather than the plain description diagnostics keep, and a test fails if a verb has no gloss to show.
- [x] **The flat table packs itself when keys and values are both unboxed.** Sixteen-byte slots instead of thirty-two, one shared tag per column, one int64 compare to probe; day 22 went from 622.5 ms and 387 MB to 516.9 ms and 261 MB, and `raw/mapbuild` from 175.0 ms and 98 MB to 125.2 ms and 50 MB.
- [x] **Six verbs out of Advent of Code 2025** — `settle` (the fixed point), `clumps` (connected components), `couples` (every pair as Twines), `index` (value to position), `squeeze` (a sparse axis made dense) and `mesh` (overlapping ranges merged). Days 4, 5, 8 and 9 rewritten on them; day 5's part two went from a seven-line hand-written sweep to one line. `settle` never returns when nothing settles, exactly as a recursion that never bottoms out never returns.
- [x] **Every Eq type really compares.** `w_compare`'s default arm answered *equal* for anything it had not been taught, and a Pattern is Eq — so two grids were the same grid whatever they held, `settle` over a grid stopped after one round, and a Pattern in a Circle collided with every other Pattern. Patterns now compare in reading order, with shape breaking a tie.
- [x] **The grid producers fuse.** `knots` and the four neighbour verbs join `span`, `flow`, `zip` and `items` as producers the loop generates rather than builds, and a *generated* producer is now worth fusing with no stages at all. Advent of Code 2025 day 4: 628 ms to 93 ms.
- [x] **`couples` and `enum` are pair producers**, so the Twine every caller takes straight apart is never built. Day 8: 592 ms to 485 ms.
- [x] **A lambda written on the spot fuses on its own.** One stage, or a consumer's predicate, is enough when the function is a lambda: what goes away is a heap closure built every time the enclosing function runs, plus an indirect call per element. Day 12's peak heap went from 2.5 GB to 1.6 GB and its nine million closures to none.
- [x] **`tallies` and `tallied`, the summed-area table**, and **`carve`**, which is `words` with the separators named. Day 9's twelve hand-written lines of running sums and four-corner arithmetic became two, and its padding went away entirely — `tallied` owns the corners, so only the flood's own border is left to shift by. Day 10's eight-line `tokens`, six chained `replace` calls before `words`, became `carve "[](){} " l`. The year is 368 lines now, from 380.
- [x] **`Link`, the disjoint sets** — `link`, `bind`, `bound`, `clumped`. The question `clumps` cannot be asked: `clumps` answers who is joined to whom once the joining has finished, and Kruskal's algorithm asks it while it is still going on. Day 8's hand-rolled union-find over a Web — `root`, `merge`, `sizes`, `add1` and a tuple threaded through the loop to carry the map back out — became four named operations, 1174 characters to 952. Which values exist never changes, so a bind copies only two int32 arrays, and a Link threaded through a loop copies nothing.
- [x] **A loop that skips the update keeps the in-place path.** Handing a collection straight back into its own slot is as single-threaded as writing to it — the next turn holds exactly the reference this one did — and the analysis was reading that bare mention as a second reference. So a loop with a branch that skips the update, which is most loops, copied on *every* turn including the ones that did update. Found by `Link`; it was always true of Webs, Circles, Patterns and Threads.
- [x] **The vocabulary audited for duplicates, gaps and over-specific verbs.** `head` and `tail` retired — exact spellings of `first` and `drop 1`. `bin` retired in favour of `base`/`unbase`, which do any base from 2 to 36 and, unlike `bin`, can read one back. `sited`/`sites` added for the grid search two programs wrote by hand. `earths`, `waters`, `spans`, `contains` refiled under Text and `most` under Maps, where their types always said they belonged. `pairs`/`couples` and `freq`/`index` — same signature, opposite meaning — now name each other in their own descriptions.
- [x] **SPEC.md's examples are checked.** They never were: the doc-example test globbed `docs/*.md` and README.md, and the spec sits at the root. Four of its chains had stopped compiling. A ```weave-part fence marks an illustration written with names it never defines, which is most of what a spec shows — parsed, so the syntax cannot rot, but not run.
- [x] **The prose caught up with the code** — the `Ply` Talent, `spans`, `priors`, `settle`, `clumps`, `couples`, `index`, `squeeze`, `mesh`, `carve`, `tallies`, `tallied`, `Link`, `sited`, `base`, the packed flat table, `weave build -tally`, the fusion rules and Pattern comparison, across README, SPEC and the tutorial.
- [x] **Five more from the audit, found by measuring rather than guessing** — `under` (the places of n things: `span 0 (sub n 1)` was written eight times across four programs), `copies` (`repeat` for a Thread), `warp` (a grid laid out from its knots, which `weft` cannot do), `woven` (`holds` for the other half of the pair) and `covers` (the containment `union`/`inter`/`diff` left unasked). Day 8 also turned out to be writing `sort | rev | take 3` for a `top 3` that already existed. Advent of Code 2025 is 345 lines now, from 380 when the round started.
- [x] **In-place updating is a blacklist now, not a whitelist.** A verb may read a threaded collection unless it can *keep* it: nine hand back a window on the argument's array and three hand the argument itself back, and beyond those the type decides — the collection's own type constructor must not occur in the call's result. That covers the whole prelude and the program's own helpers, where the old rule blessed about seventeen verbs per collection and refused the rest. `gentle` joined `braid` as a fold that owns its accumulator.
- [x] **A miscompile the widening uncovered, present since the analysis was written.** The arguments of a tail call are evaluated in order into the loop's slots, so an update at one writes through before a later one is evaluated: `fill (sub n 1) (put w n n) (add acc (len w))` read the map after the write it had not yet performed, and answered 209 where the copying build answered 190. `len` was on the old whitelist, so this was always reachable — the differential suite found it the moment reading was wide enough to make it easy to write. A sibling argument of the updating call may no longer mention the collection at all.
- [x] **A Twine of state may be a fold's accumulator.** Carrying `(collection, position)` through a walk is how you carry two things, and it used to be the worst shape in the language — the whole collection copied once per element. A Twine the step takes apart on entry and rebuilds on exit has one reference to each half, so the half that is a collection is as single-threaded as a bare accumulator. The update must sit in that half's slot and no other slot may mention it, since the slots are evaluated in order.
- [x] **A named fold step is read back as the lambda it is.** One clause, patterns that cannot fail, not recursive, not `remember`ed — then a definition *is* a lambda under another name, and inlining it puts the body back where the closure, the call per element and the ownership analysis all already work. Lifting a step out to a name, which is what you do the moment it grows past a line, used to cost all three.
- [x] **A reported program went from 10.1 s and 1.2 GB to 6 ms and 1.8 MB** — a walk over 20,000 elements carrying `(thread, index)` through a `gentle` with a named step. It needed both of the above and `gentle` being wired in at all.
- [x] **Chained updates, and a lambda a verb is holding.** `put (put w a 1) b 2` is as single-threaded as one update — each link hands its result to the next. And a lambda written out as a direct argument to a verb cannot outlive the call, so a mention of the collection inside it is a read like any other: `span a b | bend (j : nth j xs)` reads a Thread through a closure that is gone before the next statement, and refusing it used to cost the whole loop.
- [x] **`w_disown` covers `W_THREAD`.** It had four of the five ownable types, so an owned Thread escaped a loop still writable and a second loop over it wrote through the first loop's result. One word; it hid because the shape that shows it has to read the older value *after* the later loop has run, and nobody writes the test that way.
- [x] **Consumed parameters, so the copy that fix reintroduces lands only where it is needed.** A function whose collection parameter is single-threaded within its body and whose result is that collection is written twice: `wu_f_move` keeps the ownership it was handed, `wu_f` is `w_disown(wu_f_move(…))`, and only a call site the analysis has itself cleared uses the first. That carries the ownership across a call boundary — `fill occ ks is ks | braid … occ` handed a board it owns hands one back — and an optimistic fixpoint makes it work through recursion, which is what a loop written as a function needs. Ownership now composes: a variable the generator holds, an update that writes through, and a call the collection was handed over to are all "owned", so `mend 1 2 (one t)` writes through what `one` gave back.
- [x] **Day 12 was depending on the missing disown, and it was wrong.** A minimal backtracking search over a board answered `Shadow Shadow` in place where copying answered `Light Light`: the board `fill` returned stayed writable, so a branch that failed left its placements behind and the search pruned cells that were never occupied. It got the right answer on the real input by luck. See the rewrite below.
- [x] **A `Held` of anything is not a box.** Only a Held of one of the Powers kept its value where it stood; everything else — a Thread, a Web, some text — allocated a WHold. The payload is eight bytes either way, an int64 or a pointer, so size was never the reason. The one shape that genuinely cannot be flattened is a Held of a Held, since there is one `aux` field and the inner one is already using it. Advent of Code 2025 day 8 was building six million WHolds, one per element it read out of a Thread of Threads: 183 MB of its 435.
- [x] **The empty Thread is one object for the whole program.** `else []` is how every program says "nothing was there", and it allocated a header holding nothing every time it was said — another 183 MB of day 8's 435. It is never owned, so an update to it copies like any other shared collection, and the release path knows to leave it alone.
- [x] **A Thread of Earths is packed**: eight bytes to the element, the shared tag in the header, and one predictable branch in the accessor every reader already went through. `earths`, a fused chain whose result is `Thread Earth`, and `rev` build them; `take`, `drop`, `sever`, `chunk`, `windows`, `strands`, `takewhile` and `dropwhile` hand back a window on the payloads rather than unpacking; everything else asks `thr_boxed` and gets an ordinary Thread once. Day 22: 682 ms and 566 MB to 546 ms and 351 MB.
- [x] **`dupe` says where the repeat was seen before**, so the length of a cycle is one subtraction rather than a second pass over the Thread. It costs nothing: a Circle is a Web whose values are all `Light`, and the flat table's cell is two int64s either way, so the slot the position went in was already there holding a constant.
- [x] **Every parameter is highlighted as a parameter**, not just the first. A tree-sitter field that repeats binds once per pattern, so `(definition parameters: (identifier) @variable.parameter)` captured `l` in `rvrs l i t` and left `i` and `t` to fall back to plain `@variable` — two colours for one thing. A quantified capture takes the run, and a second pattern takes the run after a leading pattern, which is how a clause dispatching on `step 0 xs p` gets its names back.
- [x] **The hole words redone.** `it` retired — nought uses, and a third spelling of `_`. `former`/`latter` became `fore`/`aft` with `mid` between them at three, which is what `dupe` now answers with. The point is that the two questions no longer wear one costume: `this`/`that` are demonstratives and say *which argument*, `fore`/`mid`/`aft` are positional and say *which component of the first*. `aft` is the last of however many there are, so `add fore aft` reads on a pair and `fore mid aft` on a triple, and it is a `mid` beside them that settles which. Nothing names the middle of four, deliberately: at that width a pattern says it better.
- [x] **A word per width, which is what finally worked.** `former`/`latter` are the halves of a Twine of two and `fore`/`mid`/`aft` the parts of one of three, so a word carries its *width* as well as its position and a group holding one can be desugared where it stands. Two earlier spellings named a position relative to a width — "the latter of three", "the last of however many" — and a relative word cannot be resolved until the type is known, which is later than the parser and, for a bracket group whose parameter type is settled before its argument arrives, later than it can be asked for at all. Each of them worked in a pipeline stage and failed in brackets or as a function's argument. This one works in all three, at both widths, which the tests now check one by one. Mixing the widths in one group is refused rather than guessed at.
- [x] **`solve`, and it answers with a Weaving.** A Pattern of Earths read as an augmented matrix: `Woven x` when the equations pin one whole-number answer down, `Gentled (point, directions)` when they leave a solution set. The Gentled side carries a point as well as the directions because the answer is point + span(directions); an empty point means no whole answer was found, and empty directions beside it mean there was nothing to find. Integer elimination throughout — rows combined by what their gcd leaves and divided by their content after — and anything that would leave an Earth stops the program rather than wrapping. Forty lines of Advent of Code 2025 day 10 were this.
- [x] **Every verb that answers from one element may stop an endless one.** The list was `take`, `takewhile`, `seek`, `first`, `any`, `all`, `dupe` and `gentle`; `seekidx`, `none`, `idx`, `has`, `nth` and `second` stop just as surely and were refused — `flow (mul 2) 1 | idx 1024` would not build. Being on the list means being a fused consumer, since only a consumer the loop knows about can end it, so each of them gained a loop arm. `nth` rides `Strand` rather than Thread, so it is the one that has to be asked whether it is walking text.
- [x] **A saturating application does not copy the closure.** `w_apply` copied the closure and its slots on every argument, including the last, where nothing has to outlive the call. A predicate given its captured argument and then called once per element — `knots g | sift (reachable g)` — cost two allocations *per element*: 87 MB of day 4's 105, and a third of its runtime.
- [x] **A read through a lambda is a read even when the call builds a collection.** The result-type rule is about the collection being *handed over*; a collection only mentioned inside a lambda the call is holding was never given to the callee to put anywhere, and the walk of the lambda's body already refuses every way the closure could hand it out. All that is left to rule out is the closure surviving, which needs a function in the result. Day 10's back-substitution reads the solution vector inside a `bend`, and the old rule saw a Thread in that `bend`'s result and gave up — a copy of the whole vector per row.
- [x] **A collection the loop owns may leave inside a constructor.** `Held xs` is how a search answers, and it is the same escape a bare mention is, so it gets the same disown. Refusing it was the rest of what day 10's back-substitution was paying.
- [x] **Sorting is Weave's own, and an Earth key is sorted by its digits.** `qsort` compares through a function pointer, moves elements byte-wise, and is not required to be stable — and stability is not optional for a verb that orders by a derived key and is then walked. So: a bottom-up merge sort, stable by construction; and past 256 elements, when every key is an Earth, a least significant digit radix sort over the bytes that actually vary, which is three passes rather than nineteen and has no branch in the inner loop. Day 8's half a million edges went from 171 ms to 58 ms.
- [x] **A binding may take its value apart**, the way a parameter already could. `weave (a, b) is p` locally, and `(width, height) is dimsOf Source` at the top level. A bare name is still a name — `weave x is 1` binds and does not match — so the shapes that count are the bracketed ones and `_`, which is what anyone reaches for. One pattern and no alternative means it has to cover everything, so a refutable one is the same soft diagnostic a one-armed ward gets. The top-level form expands, between parsing and checking, into a hidden definition holding the value and a projection per name: a top-level value is a memoised accessor, so the expression runs once however many names read it, the dependency order falls out of the free variables, and the generated ward carries the exhaustiveness check. `weave trace` reports the line once, under the pattern, holding the whole Twine — the projections report nothing. The formatter reads the parse tree, so it still prints what was written.
- [x] **A right answer fetches part two on its own.** `:AocSubmit` reported the verdict and left you to run `:AocProblem!` yourself, which is a step nobody wants to remember at the moment they have just solved something. A correct answer now refetches the day and writes it, and a window already showing the problem is reloaded and scrolled to `--- Part Two ---`. It opens no window that was not open: a submission is not a request to read.
- [x] **`repeat` lays a Thread end to end too.** It was text-only, so a Thread had to be written `copies n xs | flat` — which builds the outer Thread of n copies purely to throw it away. Its shape is `a -> a` with the count, which is a `Ply` shape, so it carries the Talent like `turn`, `take`, `rev` and `weld`. `copies` keeps the other question: n of the same *element*.
- [x] **`nth`, `first` and `last` read text as well as a Thread.** They answer with an *element*, and what an element is depends on what it came out of — a Thread of `a` holds an `a`, some text holds a Fire. That is a relation between two types rather than a property of one, so no Talent can say it, which is why `Ply` carries `take`, `drop`, `sever` and `rev` and stopped short of these three. It rides as a `Strand c e` constraint instead: recorded when the verb is instantiated, settled when the call is typed, and defaulting to the Thread reading anywhere the container is not known — which is exactly what the language did before, so nothing that worked stops working. Text is read by rune, so a multibyte character is one element.
- [x] **`under n` is generated rather than built.** It is the span 0 to n-1 written the way anyone who wants "the places of n things" writes it, and it was the only allocation left in a Knot Hash written to update in place: 3,858 Threads for 1.4 MB in a loop that needs none. That program now holds 202 KB where it held 1.7 MB.
- [x] **`drop` and `dropwhile` are fusible stages**, the mirror of `take` and `takewhile`. Being stages is what lets an endless producer survive one: `cycle xs | drop 7 | first` used to be refused with "this pipeline would never finish", because the chain broke at the `drop` and the `cycle` had to be built — even though the hint it printed named `first` as a valid ender.
- [x] **`weld` carries `Ply`, so text welds as readily as a Thread.** It never names the element type — its shape is `a -> a -> a`, the same as `rev`'s — so it always fitted the Talent and had simply been left out of the round that added it.
- [x] **`turn` and `wrap`, for the ring shapes.** `turn n xs` shifts a Thread round — what comes off the front goes on the back, a negative count turns the other way, and any count works however far past the length it is. It carries `Ply`, so text turns by rune too. `wrap i xs` is `nth` on a ring: the index goes round rather than falling off, so `wrap (neg 1)` is the last strand and only an empty Thread answers `Stilled`. Between them they are what a `Wheel` type would have been wanted for, without a second sequence type whose verbs each need their own ruling — and without pretending an array with modulo indexing is the O(1) splice a marble-mania ring actually needs.
- [x] **A line that asks for the whole machine reports `⊘`, and the rest of the file still reports.** The same bargain as the hourglass, for the other way a half-written definition ruins an afternoon. Time is the tracer's to keep, since a program that will not finish cannot be asked to notice; memory is the program's own, since only it knows what it has taken — every byte a program gets from the operating system passes through one place in the arena, and that place counts and stops. It comes in through `WEAVE_MEM_CAP` so that a traced program and a run one are the same program, and out through an exit code of its own so a ceiling is not mistaken for an ordinary failure. `weave trace -memory` is 6144 MB by default and the plugin passes its own.
- [x] **A line that runs out of time reports the hourglass, and the rest of the file still reports.** `weave trace -timeout` runs the program under a limit, keeps every record that arrived, gives the first item that never reported a `⧖`, blanks it out of the source and runs again — the same bargain Salvage makes with a line that will not compile, for the same reason. The runtime flushes each record as it writes it, so a killed run keeps what it had already said. Four turns, so a file that is slow all the way down still comes back. The editor plugin passes its own timeout down and reads records whatever the exit status, instead of killing the compiler and throwing away everything it had.
- [x] **Hovering a name where it is bound says what it is.** The checker records types for uses, and a binder is not a use of itself, so a parameter, a `weave` name or a name a pattern takes apart answered nothing at all — worst of all for a parameter used nowhere yet, which is exactly when the question gets asked. Every binder is recorded now, and a binder is answered as itself: a parameter called `sum` is a parameter, not the verb it shadows.
- [x] **The formatter no longer turns a Thread of one into a Thread of two.** `[(inc x)]` came out `[inc x]`, which reads back as two elements. Elements are comma-separated when any of them is not an atom — but the comma that does the separating is never written for a single element, so a Thread of one is always written the space-separated way and keeps the brackets that make it one thing.
- [x] **`weave trace -watch f` shows what a function's names held, call by call.** Ghost text answers "what does this line hold", which has one answer for a top-level definition and no answer at all for a line inside a function body — so the inside of a recursion was the one place in a program with no ghost text, and it is where a bug is most likely to be. Every binder in the watched function records on every call: parameters, `weave` names, the names a pattern takes apart, and what the call answered. The call number is a local rather than a global, so a recursion unwinds correctly and call one answers last. It is bounded at both ends — the first 24 calls written as they happen, the last 24 held in a ring, the count between them — because the head is where a base case that never fires shows up, the tail is where a loop that will not settle does, and neither is in the middle. Records go out on the same stream marked `@`, so anything reading the by-line records skips them. Two things had to give way: a watched function is not inlined into a fused loop, since a `gentle` step lifted out to a name is the commonest thing to want to watch and inlining it would put the body back where it was invisible; and the counter is bumped inside the tail loop, because each turn of that loop is a call. `:WeaveCalls` runs it on demand and shows the table in the window an LSP hover uses.
- [x] **A binding inside a function body reports the first value it ever holds.** Ghost text and nothing asked for: a `weave` inside a recursion now says what it was the first time through, on its own line, where before it said nothing at all. One value where there are many, and the first rather than the last because the first is the call you would have made by hand — the rest are what `-watch` is for. The flag is per binding site rather than per function, which costs one predictable branch and is right for every shape at once: a lambda inside the body, a binding under a `ward`, a self tail call coming round again. It reports out of a fused loop too, since a step lifted out to a name is inlined and that is exactly where a body is hardest to see — but only for a body that came from a *named* definition, because a lambda written out on the spot has the call's own lines and the chain already reports those. It survives the line that never finishes: a `gentle` that times out still shows what its first step held.
- [x] **The language server survives what any one message does to it.** Reported as "the LSP goes iffy and `:LspRestart` will not fix it, I have to restart nvim" — which is the signature of the *process* dying rather than the server misbehaving: a client whose process was reaped cannot be revived by restarting it, only by an editor that starts a fresh one. Three ways it could die, all now closed: a panic anywhere in the front end took the process with it, and is caught, logged with its stack, and answered as an error on that request; a body that would not parse as JSON was fatal, and is now stepped over, since the body was read whole and the stream is still in step; and a header block with no `Content-Length` was read as end of input, so a stray blank line stopped the server outright. Only losing the framing is still fatal, because once the stream is out of step there is no honest way to find the next message. `WEAVE_LSP_LOG` names a file to write every method, its timing and any panic to — stdout is the protocol and stderr is the editor's, so a server with something to say to a person needed somewhere else to say it.
- [x] **The call window fits the window.** `:WeaveCalls` laid out a table as wide as its widest value, which wrapped, and a wrapped table is not a table — nine columns of a real program came back as unreadable ribbon. Columns are sized against the room there is now, the widest giving way first because it is the one that can spare it: `{0 2 7 0 …` says what a Circle is doing as well as the whole of it does, while a one-digit index says nothing at all once it is cut. No pipes and no outer border, a rule under the header, the call number right-aligned as a count, what the call answered moved to the last column, the block fenced so the client renders it fixed-width instead of reflowing it as Markdown, and wrapping turned off in the float so anything still too wide is scrolled to rather than folded over.
- [x] **`:AocTime` says what you already answered, not just that it was wrong.** The verdict alone is the least useful half of a wrong answer: "too high" tells you nothing you can act on, and *which* answer was too high tells you what the next one has to be. Every submission is now appended to `.aoc-answers` beside the program — when, which part, the answer, the verdict — and the report lists the distinct answers tried per part with what came back. The bracket is stated outright rather than left to be worked out: a `too low` is a lower bound and a `too high` an upper one, so two of them say the answer is *between 4000 and 9000*, which is the whole reason the site bothers to say which way you were wrong. Only whole numbers count as bounds, since nothing else compares. A submission held back by the cooldown prints the same report, because that is exactly when it is worth reading.
- [x] **`:AocSubmit` refuses an answer already known to be wrong.** Two ways the record can rule one out before the request goes: the same answer graded before is graded the same way now, and a bound already found puts everything past it out of reach — an answer at or below a `too low` is too low whatever else is true of it. Both cost a submission and the ten minutes after it, which is the whole point of not spending them. Only a verdict the site actually reached counts: `too soon` means the answer was never graded, so repeating it has to be allowed. `:AocSubmit!` sends it anyway, which is what the bang already meant for the wait.
- [x] **A fused `gentle` builds neither its Weaving nor its state Twine.** The largest measured miss in the compiler, and it is gone: the trampoline walk of 2017 day 5 went from **2.9 GB and 5.5 s to 51.7 KB and 0.46 s** — nine allocations for the whole program, twelve times faster. Four allocations a turn, 112 million of them, every one dead before the next turn began. Two objects, and both were built only to be taken straight back apart by the very next lines of the loop. The Weaving: `res = step(...); if (w_data_index(res)) break; res_acc = w_data_field(res, 0);`. And the state Twine, taken apart by the step's own pattern on the way in and written out again on the way out. So the step is compiled in *statement* position instead — `Woven x` assigns, `Gentled y` assigns and sets a flag — and a state Twine is carried one component to a variable. Both are put together once when the loop ends, because `gentle` answers with a Weaving and `failing` reads which case it was. Every component is worked out before any is assigned, since the second half of a state Twine routinely reads the first and writing it early would be reading the next turn's. It is all-or-nothing per loop: the shape is checked before a line is emitted, so a step ending in a `ward`, or handing on something that is not a Twine written out on the spot, compiles exactly as it did. Eight shapes went into the differential corpus, including the ones it declines — and they earned their keep at once, turning up a bug older than this work: a `gentle` folding into a Web reserved its capacity against the loop's *answer* rather than its accumulator, which for a `gentle` means reading a WData as a WMap. Undefined behaviour that happened to be harmless, invisible until the Weaving stopped existing until the loop was over.
- [x] **A fused fold hands back what its turn allocated, so a backtracking search forgets the branches it abandons.** Measured first: of the whole of 2025, day 10 was 816 ms of 1.2 s and 296 MB, and 70% of that was `wp_mend` *copying* — the branch shape no ownership analysis can ever bless, because a backtrack uses the collection it was handed once per **option** and the old value has to survive for the next one. So instead of freeing those copies one at a time, the turn forgets them all at once: the arena is a bump pointer, so a turn marks where it has got to, allocates whatever it likes, and puts the pointer back. **893 ms to 98 ms and 297 MB to 7 MB**, same compiler, one flag apart.

  The whole difficulty is the one condition that makes it sound — nothing allocated during a turn may be reachable after it — and that is a question about the loop, not the arena. Four things have to hold, each of them a way for something to escape: the accumulator must be an unboxed Power, so what deliberately outlives the turn carries no pointer into it; the elements likewise; no stage may hold state across turns, since `scan` and `dupe` keep exactly what a release would take; and nothing reachable may store into a global, of which a Weave program has two — a `remember` table, which rules the loop out, and the memoised accessor a top-level value compiles to, which is dealt with by forcing those values before the region opens, where they were always going to end up. Anything that cannot be shown to satisfy all four compiles exactly as it did.

  Chunks carry a serial so a release frees what was taken since the mark wherever it sits in the list, and the free lists are emptied only if something was freed while the region was open — a block freed before it was opened lives outside it and is still good. `-no-regions` turns it off, and nine shapes went into the differential corpus including the four kinds it declines.

## Performance

- [ ] **"However you write it" still needs a real reference count.** The static
  proof is wide now — a blacklist plus a type rule for reading, folds and tail
  recursions and Twines of state for threading, and consumed parameters for
  carrying ownership across a call — but it is still a proof, and a proof has
  shapes it cannot see. The one that matters is a collection genuinely used
  twice on a branch that only ever takes one of them, which no amount of
  single-threading analysis will bless. A reference count would; weave.h already
  sizes the field for it, and the ownership bit is the degenerate one-bit case.

  Three known gaps, each cheap on its own if the count is not built:

  - A member of a *mutually* tail-recursive group cannot consume a parameter.
    The group compiles to one C function over a shared slot array, and a second
    entry point per member is not a shape that fits.
  - `isFoldOf` — a fold *over* the threaded collection, seeded with it — knows
    `braid` and not `gentle`.
  - A consumed parameter must be a plain name in every clause. Destructuring one
    binds a window on its own array, and handing that back would be a second way
    to see storage the caller has given up.

- [ ] **Monomorphised container storage — the rest of the Thread half.** The
  packed layout is built and measured: a `Thread Earth` stores eight bytes to
  the element with the tag they share held once in `obj.kind`, `elems == NULL`
  says which layout a Thread has, and `thr_at` rebuilds a Value from the two.
  Advent of Code 2024 day 22 went from 682 ms and 566 MB to 546 ms and 351 MB.

  What packs today is `earths`, any fused chain whose result type is
  `Thread Earth`, and `rev`. What is left is the rest of the producers, and each
  of them is the same shape of change — a packed branch beside the boxed one:

  - **`sort` and `sortby`.** A sort of Earths is the most-written line in Advent
    of Code that still unpacks, and the radix sort underneath already works on
    an `int64` key. It wants its own pass over payloads rather than Values.
  - **`flat`, `uniq`, `plait`, `pivot`, `col`.** All of them build a Thread out
    of elements they read one at a time, so they can build a packed one when the
    elements they read are packed. `flat` is the one that matters: a Thread of
    Threads flattened is how a parsed input becomes a working set.
  - **Water and Fire.** The layout is written for any Power that lives in the
    Value itself, and `packedElem` in the compiler and `wp_waters` in the
    runtime turn one on each. Nothing in the benchmarks wants them yet, which is
    why they are not on.
  - **Twine.** A `Twine (Earth, Earth)` is 64 bytes where 16 would do, and that
    is what day 22's remaining 351 MB is mostly made of. It is the same trick
    against a different header, and it is the next one worth measuring.

  The one thing to keep in mind when adding a producer: the payload array is
  freed at the size the elements round up to, and an odd number of them does not
  end on the sixteen-byte boundary the allocator rounds to. `w_thread_packed_fit`
  handles that for a buffer; anything that allocates payloads directly has to
  agree with it. Getting it wrong hands out a block sitting on top of the
  elements, which is how the first version of this failed.

- [ ] **Reclaim memory.** The runtime allocates from an arena and does not
  collect. The `weave repl` worry in the earlier version of this item was
  misplaced — the REPL runs each line as a fresh process, so the arena dies with
  it. The real case is one long program with a large working set, and the
  dominant source of garbage there was path-copying a threaded collection, which
  in-place updating now removes for `Pattern`, `Web` and `Circle` (2M Web
  inserts went from 44.6 s and 8.9 GB to 0.50 s and 99 MB), and which `dijkstra`
  avoids outright by owning both its structures (a 360,000-node graph went from
  2.2 GB to 263 MB). The free lists in front of the arena then took back what a
  growing structure abandons.

  The escape analysis above then took the case no *local* rule could see: a
  value live right up until the call that made it returns, and dead immediately
  after.

  `weave build -tally` now says where the memory is, which massif could not:
  the arena bump-allocates, so a profiler sees one 1 MB `malloc` charged to
  whichever call happened to exhaust the previous chunk. On day 22 that call is
  `wp_rev`, and the profile reads as a 131 MB leak in `rev`, which allocates one
  array and hands it straight back. The tally reads instead:

      heap high-water mark 189.6 MB, arena holds 135.0 MB from the OS for
      62.3 MB live, 72.7 MB of it idle; 126.0 MB live outside it

           at peak     allocated    blocks  site
           124.9 MB     125.1 MB      2001  collections.c:694
            60.9 MB      60.9 MB      2000  program.c:202

  Both of those are one buyer's working set, kept two thousand times over, and
  neither is `rev`:

  - [ ] **The Web a fused `bend` produces per iteration is never freed.**
    `collections.c:694` is `flat_new`'s table. `buyers | bend offers | braid ...`
    fuses, so only one `offers` Web is *reachable* at a time — and all 2000 are
    live, because releasing a Web is not something the compiler knows how to do.
    This is `escape.go` again with `Web` added to what `freshThread` covers,
    except that a Web's storage is only dead once the entries drawn out of it
    are, so it wants care rather than a new line in a table.

  - [ ] **A fused stage feeding a verb loses its buffer.** `program.c:202` is the
    `zipwith` buffer inside `weave k is zipwith (...) hi lo | rev`. The release
    analysis frees `weave` bindings by name, and this array has no name: it is
    the fused argument of `rev`, born and abandoned inside one expression. Any
    `producer | stage` handed straight to a verb that copies does this.

    `fuse.go`'s `conCollect` case is where the buffer is sealed and `app` in
    `codegen.go` is where the verb call is written, so the shape is: bind the
    chain to a temporary, call, release the temporary.

    The trap is the predicate. `escape.go`'s `freshThread` is *not* it, and
    reaching for it would compile a use-after-free. It answers "does this verb
    allocate the array it returns", which is a different question from "can the
    result reach the argument's array" — `wp_chunk` and `wp_windows` are both in
    the table and both build their elements as `w_thread(t->items + i, len)`,
    slices straight into the argument. (The existing analysis is unaffected: it
    only releases bindings in a function whose result type cannot carry a
    Thread, so nothing sliced can outlive the free.) What this needs is a
    second, smaller table, each entry checked against prelude.c by hand.

  Past those two it is a `Taveren` or a `Thread` threaded through a loop the
  *user* wrote, which is the ownership bit rather than escape: `push` would need
  the per-node flag the trie has, driven by the same analysis. Past *that* it is
  a real collector. The standing guess that the bulk was `Held` boxes from
  `get w k | otherwise 0` is now disproved: they do not appear in the report at
  all, because an Earth in a `Hold` rides in the `Value` and never boxes.

## Asked for, not built yet

These came out of using the language. They are in the order they were raised.

### The build

- [x] **`make test` cannot finish on slow hardware, and now does.** It ran
  `go test ./...` with no `-timeout`, so every package got Go's default ten
  minutes — and `internal/build` takes about twenty-four here, dying with a
  panic that reads exactly like a real failure and is not one. Both halves of
  the question are answered: the differential suite skips under `-short`, which
  is what `make test` passes, and it runs on every push instead. `make
  test-full` is the whole thing with a timeout that fits it.

### Editing and tooling

Nothing owed. The grammar and the language server are level with the
language again.

### The language

Three questions were raised here and all three are answered: keep things as
they are. The reasoning is kept because it is the reasoning against changing
them, and the next person to want the change should read it first.

- [x] **SETTLED, as it stands: `high`/`low` are what you asked for as `max`/`min`.** The
  names were already taken: `max a b` and `min a b` are the binary verbs, and
  a name cannot mean two things. So the Thread versions are `high` and `low`,
  and the positions are `highidx`, `lowidx` and `seekidx`, following the `-idx`
  convention you suggested. `dupe` answers `Hold (Earth, a)`.

  Asked and answered: `high`/`low` stay, and `max`/`min` stay binary. Should
  that ever be reversed, the *binary* pair is what gets renamed —
  `larger`/`smaller` — and the ledger keeps the old names from coming back. It
  is a five-minute change either way; it just cannot be both.

- [x] **SETTLED, as it stands: a replace verb for a Thread, and finding in text.** The
  find-all half is built — `siftidx` is every position passing a test and
  `idxs` is every position a value occurs at, so `seek`/`sift` is no longer
  the only pair with both halves. What is left is two things I did not build
  because I am not convinced either earns a name:

  - **Replacing a value in a Thread.** `bend (pick (eq old this) new this) xs`
    already says it, in one pass, and reads as what it does. A verb would save
    four words and add one more thing to remember.
  - **Positions of a *substring* in text.** `fires text | idxs 'X'` covers a
    character, which is most of it. A run of more than one — finding every
    `XMAS` — has no short spelling, and the honest answer is that it wants a
    rune index rather than a byte one, which is a wart to design around rather
    than a line to write.

  Replacing in text needs nothing: `replace` already swaps *every* occurrence.

  Asked and answered: neither is built. `bend (pick ...)` and `fires | idxs`
  cover them, and a substring search wants the rune-index question settled
  first.

- [x] **SETTLED, as it stands: thread verbs on Twines, and `fore`/`aft` on Threads.**
  Two asks pointing the same way: that a two-part Twine and a two-element
  Thread be interchangeable where it does not matter.

  Both need the same thing and it is not small. `fore`/`aft` desugar in
  the *parser*, to a `(fore, aft)` pattern, before any type is known — a
  Thread would need `[fore aft ..]` instead, and nothing at that stage
  can tell which. Likewise `sum (a, b)` would need `sum` to accept two
  unrelated types.

  The honest routes are: (a) a `Pair` Talent that both satisfy, with every
  verb that wants one written against it — real work, and it changes the
  signatures people read; (b) a type-directed elaboration after checking, so
  the hole words resolve from `info.Types` — smaller, but it means holes stop
  being a purely syntactic thing, which is what makes them easy to explain
  today; (c) leave it, and lean on `thread (a, b)`, which already casts a
  Twine of two of the same type to a Thread.

  Which of those is worth it depends on how often it actually bites.

  Asked and answered: (c) — leave it, and lean on `thread (a, b)`. Worth
  logging the cases as they come up rather than guessing.

### Verbs the vocabulary is missing

Everything this section asked for is built — the six from reading the four
longest Advent of Code 2025 programs, `Link` for the running question they did
not cover, and `carve`, `tallies`, `tallied`, `sited`, `sites`, `base` and
`unbase` from the vocabulary audit. What is left is two shapes the *Talent
machinery* cannot say, both recorded under **Sharp edges** below rather than
here, since neither is a missing verb.

### What else the language needs

Nothing outstanding. Structured input parsing was the last item here, and it
is `delve`, above. What is left in this file is all compiler work under code
that already reads the way it should.

## From Advent of Code 2025

All twelve days, both parts, are in `aoc/2025/`, verified against the examples
in the puzzle text. These are what writing them turned up, worst first. Nothing
here is a verb for one puzzle: each is something that came up more than once,
or that would have come up in any year.

### Bugs

- [x] **`spans` reads a range, and `earths` keeps reading a sign.** Both
  readings of a dash are needed in the same year — `x=-5` is one number and
  `11-22` is two — so rather than guess from context they get a verb each.
  `earths`' doc line now points at `spans`. Days 2 and 5 lost their parsing
  entirely: `bounds is Source through spans`.

### Missing, and general

- [x] **A substring is one verb.** `take`, `drop`, `sever` and `rev` carry the
  new `Ply` Talent and work on Air as readily as on a Thread, by rune and not by
  byte, sharing storage the way the Thread versions do. Not `Bulk`: a Web has a
  length and no order, and `take 3 someWeb` should not type-check. Not `nth`,
  `first` or `takewhile` either — those answer with an *element*, and a Talent
  cannot say that a Thread's element is `a` while Air's is `Fire` without
  associated types.

- [x] **`nth`, `first` and `last` read text.** What `Ply` could not carry, and
  it did not need associated types after all: a `Strand c e` constraint beside
  the Talents, settled when the call is typed and defaulting to the Thread
  reading anywhere the container is not known.

- [x] **`priors`**, `scan` with the value it started from kept, so it is one
  longer than the Thread. That is the shape a prefix sum wants: a range's total
  is one subtraction and the empty range needs no special case. Day 9's table
  went from five lines to `barred through bend (priors add 0) through priors
  (zipwith add) zeroRow`.

- [x] **SETTLED, as it stands: `fore`/`aft` stop at two.** A three-part
  Twine has no hole words, so `sortby (fore)` on `(distance, i, j)` is written
  `sortby ((d, _, _) : d)`. Asked and answered: leave it. A pair has a first and
  a second; a triple has no names anyone would agree on, and inventing two more
  words for one shape is worse than the pattern that already says it.

- [x] **Merging a Thread of ranges is `mesh`**, and this entry had been left
  open long after it was built. The "build a Thread from the front" complaint it
  started as was withdrawn — both ends are already one call, `weld [x] xs` and
  `weld xs [x]`, so a `cons` would be a third way to say the second — and what
  was underneath it was interval merging, the most repeated subroutine in the
  event: 2021 day 5, 2022 day 4, 2023 day 5, 2025 day 5. Day 5 part two is
  `ranges | mesh | bend width | sum` and has been since.

### Sharp edges that cost real time

- [x] **SETTLED, as it stands: a ring is read round and written straight.**
  `wrap i xs` indexes with wrapping and `mend i x xs` does not, so a swap on a
  ring says the wrapping twice in two spellings. Asked and answered: leave it.
  Making `mend` wrap would change what an out-of-range index means for every
  caller, and a wrapping twin is a fifth verb in that family for one shape.

- [x] **SETTLED, as it stands: the comparison verbs read backwards.** `gt 0 x`
  is `x > 0`, which is right for pipelines and unarguable, but both arguments
  are Earth so a swap type-checks and runs. The checker cannot help: the rule
  proposed here was "warn when the first argument is the more complex
  expression", and every case actually reported was two plain names, so it would
  catch a shape nobody writes and miss every shape that was written. The runtime
  half — `weave trace` reporting sub-expressions — is a new record shape for the
  trace format and for the editor plugin that reads it. Asked and answered:
  leave it.
- [ ] **Backtracking search keeps every branch it abandons — for the shapes
  regions do not reach.** Largely answered: a fused fold whose turn is
  self-contained now hands its storage back, which took day 10 from 893 ms and
  297 MB to 98 ms and 7 MB. What is left is every search that does not sit in
  one: a recursion that is not a fused fold at all, a fold over a Thread rather
  than a span, one with a stage in the chain, one whose accumulator is a
  collection. Each is a separate widening of the same four conditions in
  regions.go, and each wants its own measurement first.

  On the real input day 12's packing held 2.5 GB at exit and freed none of it.

  The closure half of that is fixed, and not by the escape analysis this item
  first proposed: a lambda passed to `any` or `all` is now *inlined into a fused
  loop*, so it is never allocated at all. Nine million closures became none and
  the peak fell to 1.6 GB. That is the better answer — freeing a thing is worse
  than not building it — and it says the same trick is worth reaching for
  wherever an escape question looks hard.

  Day 12 itself no longer shows it: with the disown fixed the search allocated a
  board per node and needed more memory than the machine had, so it was
  rewritten to carry the board as three row bitmasks and the counts as one
  packed number, and it now allocates nothing inside the search at all. That is
  the honest answer for a *hot* search and it is not an answer for the language:
  it took hand-packing state into integers, which is exactly what nobody should
  have to do.

  In-place updating cannot help here and it is worth saying why. A backtrack
  uses the board it was handed once *per option*, not once — the old reference
  has to survive for the next branch — so it is genuinely not single-threaded
  and no analysis will ever say otherwise. What a search wants is the branch's
  allocations going away when the branch does, which is the **reclaim memory**
  item above, not this one. Of everything in this file, that is still what most
  decides whether a search-shaped puzzle can be written naturally.

### Found again on a real input

The twelve days were written against the worked examples. Running them on a real
one, and checking every answer against an independent reference, turned up two
more things — and made the case for one already listed.

- [x] **`uniq` was quadratic.** It compared each element against everything it
  had already kept. Nobody notices that on a hand-written Thread; it was forty
  of the forty-four seconds day 2 took, over eighty thousand candidates. It
  keeps a Circle of what it has seen now — `Eq` is all the type asked for and
  all a Circle key asks for — and day 2 went from 44 s to 219 ms. Order is
  unchanged: the first of each value, where it first appeared.

- [x] **Text is quoted inside a collection now, and the Done list is true
  again.** `["a b", "c"]` and `["a", "b", "c"]` both printed `[a b c]`. Quoting
  is a different question from the bracketing that was already there, so they
  are two functions: a comma or a ` : ` keeps neighbours apart on its own, which
  is why a Twine and a Web quote without also bracketing. At the top level text
  stays bare — a program that prints a line wants the line, not the line in
  quotes.

- [x] **A ward's two shapes are told apart by the line, not by the brackets.**
  `ward seekidx (r : test r) rows` read the lambda as an inline arm and the rest
  as nothing at all, because the old rule — the subject stops at the first
  bracketed group holding a `:` — cannot tell an arm from a lambda. The rule now
  is: an indented block after the ward's line means the arms are down there and
  *all* of the line is the subject; otherwise the arms are the run of bracketed
  `pattern : body` groups the line **ends** with. A lambda in the middle of a
  subject has something after it, so it is never mistaken for an arm.

  This is also the rule the tree-sitter grammar already used, so the compiler
  and the editor agree where before they quietly did not.

- [ ] **Three of twelve days needed a different algorithm at full size**, and
  each was a case the example could not have shown: day 9's plane is 10¹⁰ tiles
  rather than 126, day 10 has a machine with more counter states than there are
  atoms in a person, day 8's constant is given per input. None of that is the
  language's fault. It does say what a `weave test` fixture is worth: the
  examples all passed while three programs were wrong.

### What the four biggest programs suggest for the vocabulary

Lines are `weave fmt -terse`, comments and blanks dropped; times are the real
input. This is the reading that produced the six verbs — the numbers are as
they stood *before* them and before the fusion work; the current ones are in
**Measured again**, below.

| day | lines | time | | day | lines | time |
|---|---|---|---|---|---|---|
| 10 | 78 | 1.0 s | | 6 | 18 | 9 ms |
| 12 | 60 | 9.0 s | | 1 | 16 | 7 ms |
| 9 | 51 | 213 ms | | 11 | 15 | 6 ms |
| 8 | 31 | 513 ms | | 4 | 15 | **4.4 s** |
| 7 | 21 | 10 ms | | 3 | 13 | 6 ms |
| 2 | 19 | 94 ms | | 5 | 10 | 5 ms |

Days 10, 12, 9 and 8 are both the longest and the slowest. Day 4 is the odd one
out — fifteen lines and four seconds, because the loop *is* the program.

Reading those five for what a verb could have carried:

- [x] **`combos 2` was already there and I wrote it out twice.** Days 8 and 9
  both open with `span 0 (sub n 2) through mapcat (i gives span (add i 1) ...)`,
  which is `combos 2` exactly. Two lines each, in the two longest programs, for
  a verb that exists. Worth knowing *why* it was not reached for: `combos`
  answers `Thread (Thread a)`, so a pair comes apart as `[a b]` and not
  `(a, b)`, and every other pairing verb in the language — `pairs`, `cross`,
  `zip` — answers Twines. That is the whole reason it did not look like the
  right tool.

The six verbs this argued for are built and listed under **Done**; what is left
below is what reading them *ruled out*.

Two that reading these did *not* support, worth writing down so they are not
proposed again: day 10's integer elimination and day 12's polyomino packing are
each one puzzle's algorithm. Day 12's eight orientations look general, but
`spin` and `flip` already cover a Pattern, and what day 12 needs is a *set of
cells* — the gap is a conversion, not a verb.

### The formatter

- [x] **Every code line in the repository is inside the margin now.** The three
  that were not turned out to be two missing cases, and neither was the one this
  item guessed — a destructuring lambda head already broke fine.

  The first was a **chain ending in a hole word**. `xs | aft` desugars in the
  *parser* to a match on the pair it opens, so by the time the formatter sees
  it the chain is a Ward with a chain inside it and stops looking like a
  pipeline at all — one `| aft` at the end of a line kept the whole thing on
  one line however long it got. Reading those back as stages fixed both of day
  1's lines.

  The second was a **bracketed literal argument**. A Thread literal broke one
  element to a line at the top level and nowhere else, and a Twine literal never
  did. Both break now, with the comma leading so that every line after the first
  opens no block and continues the one above — the form a Thread already used.
  That also has to see through the hole words: `as (a, b)` is a lambda whose
  printed text *is* its body, so the seam to break at is the body's.

- [x] **A long call breaks at its arguments**, the same seam a pipeline stage
  already broke at. It only broke pipelines before, so a nested `pick` — which
  is how a Weave program says "otherwise" — ran to whatever length it liked:
  270 characters in day 12, 217 in day 9, and every one of the twelve solutions
  failed `weave fmt -check` on a form nobody would have chosen. A canonical form
  worse than what people write by hand is one they stop running, and then it
  stops being canonical.

  Three things can now be broken into, each inside the brackets it already sat
  in: a call, a pipeline, and a lambda whose body is either. The closing bracket
  is carried down and lands on the last line written, however deep the breaking
  went. What did not fit goes one argument to a line at a further indent, which
  reparses because a deeper line that opens no block continues the one above it.

  `make fmtcheck` covers `aoc/` as well as `examples/` — the only programs here
  written to be read rather than to show a feature off, and so the ones worth
  holding the canonical form to. Three code lines are still over the margin; see
  the open item above. The rest of what runs long is comments, which the
  formatter does not reflow and probably should not.

### Measured again, after the boxing round

All twelve of 2025 on the real inputs, terse and comment-free, best of three on
a container whose timings swing by a factor of five — so the line counts are
exact and the milliseconds are the floor, not the average. Peak heap is what
`weave build -tally` reports; the "was" column is the same programs before this
round of work on what a value costs to hold.

| day |  loc | ms | was | peak | was | what dominates |
|-----|-----:|---:|----:|-----:|----:|----------------|
| 1   |  20  |   5 |   5 |   2.2 MB |   2.2 MB | — |
| 2   |  19  |  49 |  48 |  31.8 MB |  31.8 MB | generating the repeated-block candidates |
| 3   |  14  |   5 |   4 | 832.0 KB | 863.2 KB | — |
| 4   |  12  |  61 |  97 |  17.7 MB | 104.9 MB | a partial application per cell, now free |
| 5   |   7  |   4 |   4 | 134.9 KB | 134.9 KB | — |
| 6   |  18  |   6 |   7 |   2.8 MB |   4.4 MB | — |
| 7   |  23  |   5 |   9 |   1.2 MB |   1.2 MB | — |
| 8   |  27  | 190 | 433 |  76.6 MB | 435.2 MB | building 500k edges; the sort is a quarter of what it was |
| 9   |  42  | 182 | 196 |  47.7 MB |  78.0 MB | the flood fill and the running-sum table |
| 10  |  78  | 223 | 314 | 296.7 MB | 509.6 MB | the null-space sweep, one board copy a try |
| 11  |  18  |   4 |   5 | 795.5 KB | 913.1 KB | — |
| 12  |  93  |  75 |1141 |   1.4 MB |   1.6 GB | the backtrack, rewritten to carry numbers — and it was wrong |

Total **0.81 s** for the year from 371 lines, against 2.26 s and 2.7 GB of peak
heap when the round started. The milliseconds are the floor across three sweeps
rather than one, because the container drifted by half again between them; the
peaks are exact.

Against Go, on this machine, best of five (`python3 bench/run.py -n 5`):

| benchmark | weave | rss | go | rss | ratio |
|---|---|---|---|---|---|
| `fib 32` | 13.9 ms | 9 M | 16.3 ms | 9 M | **0.85×** |
| `chain` | 35.1 ms | 9 M | 55.3 ms | 9 M | **0.64×** |
| `chainalloc` | 34.7 ms | 9 M | 875.3 ms | 683 M | **0.04×** |
| `loop` | 117.7 ms | 9 M | 111.2 ms | 9 M | 1.06× |
| `collatz` | 41.3 ms | 9 M | 81.3 ms | 9 M | **0.51×** |
| `mapbuild` | 280.5 ms | 50 M | 547.6 ms | 45 M | **0.51×** |
| `text` | 255.1 ms | 241 M | 338.2 ms | 95 M | **0.75×** |
| 2024 day 1 | 3.6 ms | 9 M | 3.9 ms | 9 M | **0.92×** |
| 2024 day 2 | 5.7 ms | 9 M | 4.7 ms | 9 M | 1.22× |
| 2024 day 10 | 4.6 ms | 9 M | 4.5 ms | 9 M | 1.00× |
| 2024 day 11 | 61.1 ms | 20 M | 54.9 ms | 11 M | 1.11× |
| 2024 day 22 | 921.2 ms | 567 M | 368.5 ms | 9 M | 2.50× |

`mapbuild` moved most: 0.92× and 98 M before, 0.51× and 50 M now. Day 22 is
still the one that loses, and still for the reason performance.md gives. Nothing
in this suite sorts, so none of it shows the sort work.

What that leaves:

- [x] **Day 12 allocated 1.6 GB and freed none of it**, 1.18 GB of it nineteen
  million Twines — `o as (add (div pos w) fore, add (mod pos w) aft)`, one
  per cell of every orientation tried. It was also *wrong*: the whole search
  depended on an owned Thread escaping `fill` still writable, and once that was
  fixed it needed more than eight gigabytes on the worked example. Rewritten to
  carry three row bitmasks and a packed count instead of a board and a Thread of
  counts, it allocates nothing inside the search: 1.1 s for 15 million
  placements on the worked example, 90 ms on the real input. Checked against the
  old algorithm — which is correct, only slow, now that the disown is right — on
  720 random small inputs.

  The shape it was written in is still the one anybody would write, and it is
  still 64 bytes per pair where 16 would do. That part is the **monomorphised
  container storage** item above arriving from the other direction, and it is
  what would let the natural spelling stand.

- [ ] **Day 8 is entirely `couples` + `sortby` on half a million triples.** The
  union-find walk over them is free by comparison; cutting the program to just
  the edge list and its sort reproduces the whole runtime. `Link` duly bought
  clarity and no time at all — as predicted.

  Two thirds of what it was holding turned out to be boxing rather than sorting,
  and both are gone: a `Held` of a heap value no longer allocates, and the empty
  Thread `else []` hands back is one object. 435 MB became 69 MB without
  touching the program.

  The sort itself was then measured rather than assumed, and the claim recorded
  here was wrong: `sortby` already worked the key out once per *element*, and
  `Keyed` was an array slot, not a box. The cost was libc's `qsort` — an
  indirect call per comparison, generic byte-wise moves, and no guarantee of
  stability. A stable merge sort with a radix path for Earth keys took it from
  171 ms to 58 ms, and day 8 to 190 ms.

  What is left is the other half of the day: 125 ms building half a million
  `(distance, i, j)` triples, which is a boxed 3-Twine each. That is the
  **monomorphised container storage** item and nothing else.

- [ ] **Integer linear systems.** Day 10 part two is 40 of its 87 lines: Gauss
  elimination over the integers with gcd row reduction, a null-space sweep and
  back-substitution. It is genuinely common — 2023 day 24, 2024 day 13 — but it
  is also genuinely large, and a verb that solved it would have to decide what
  to do about the null space, which is the part each puzzle uses differently.
  Recorded as observed, not proposed.

  Asked: does this mean a shelf of linear-algebra verbs on Threads or Patterns?
  Answered no, and the reason is worth keeping. Nothing in the 40 lines is a
  matrix operation. There is no `matmul`, no determinant, no inverse; a
  determinant over the rationals would not even answer the question, because
  the puzzles want the *integer* solutions and that is a different problem. The
  40 lines are one algorithm — integer Gauss with gcd reduction — and the verbs
  it is written out of already exist. Adding `matmul`, `transpose`, `identity`
  and friends would leave those 40 lines exactly as they are.

  So if anything is added here it is the one thing that is actually wanted:
  a verb that takes a Pattern of Earth as an augmented matrix and hands back
  the integer solutions, and whose whole difficulty is what it says about the
  null space. That is a design question, not a shelf of functions. Two shapes
  worth weighing when it comes up: hand back the particular solution plus a
  basis for the null space and let the puzzle pick, or hand back `Stilled` for
  anything not uniquely determined and make the under-determined case somebody
  else's problem. The first is honest and the second is what four puzzles out
  of five actually need. Not started; recorded so the next look starts here.

- [ ] **`weave trace` gives up the fusion `weave run` gets.** Asked why the
  trampoline walk shows no ghost text at all when `weave run` answers it in
  10 s: because trace mode compiles a pipeline stage by stage so that every
  source line has a value to report, and a chain compiled that way is not a
  chain any more. Measured on that program: 26.5 s under `weave trace` against
  10.1 s under `weave run`, so 2.6× — and `-O0`, which trace also uses, is part
  of the rest. The time limit below means this no longer costs the file its
  ghost text, but the gap itself is real and unaddressed.

  There is a cheap half of it available. Staging is only needed for a chain
  that spans more than one line; a definition written on one line has exactly
  one place to report and could be compiled whole, fused, exactly as `weave
  run` compiles it. Nothing has been measured to say what share of a real file
  that is.

- [ ] **Thread destructing** in a lambda works fine, but list destructing into 
  multiple top level variables from a Thread gives a warning

- [ ] **Formatter** does the formatter even allow the 1 line local variable 
  (weave ... into) syntax?

### Thematic Vocabulary Suggestions (Collision-Free)
| Current Verb | Proposed WoT Verb | Lore Reason / Context |
| :--- | :--- | :--- |
| **Pattern** | | |
| `flip` | **`mirror`** | Flipping a grid left-to-right behaves like a look into the Portal Stone Mirror Worlds (*Portal Stones* / *Tel'aran'rhiod*). |
| **Graph Traversals (The Ways & Skimming)** | | |
| `dijkstra` | **`skimming`** | Skimming requires navigating the dark space to calculate the distance/cost to everywhere else. |
| `route` | **`waygate`** | Yields the explicit, node-by-node path to safely reach a destination. |
| `clumps` | **`enclaves`** | Groups of nodes (like Ogier *Stedding* enclaves) that are internally connected but cut off from the rest of the graph. |
| `toposort` | **`chronicle`** | Ordering a directed acyclic graph so every edge points forward—mapping the linear history of an Age. |
| **Sequences & Threads (Textiles & Healing)** | | |
| `flat` | **`unfurl`** | Taking a bundled Thread of Threads and unfurling it into one continuous line. |
| `uniq` | **`distil`** | Filtering out all repeated impurities until only the pure, distinct essence remains. |
| `compact` | **`heal`** | Dropping the `Stilled` entries and unwrapping the living values that are left. |
| `take` | **`shear`** | Cutting a thread cleanly to keep only the front elements. |
| `drop` | **`shed`** | Letting the beginning of a thread fall away. |
| `strands` | **`skeins`** | Breaking a Thread into adjacent runs of matching elements (a skein of yarn). |
| `rev` | **`unwind`** | Turning a thread back-to-front or rolling a spindle backward. |
| **Maps & Webs (Channeling & Ties)** | | |
| `get` | **`draw`** | Reaching into a Web (or an *angreal*) to fetch a value out. |
| `put` | **`anchor`** | Fixing a specific key and value together permanently (Tying off a weave). |
| `forget` | **`banish`** | Completely severing a tie out of the Web. |
| **Grouping & Pairing (The Tower & The Bond)** | | |
| `group` | **`ajah`** | Gathering elements by a derived characteristic or "color", exactly how the Tower divides its initiates. |
| `zip` | **`bond`** | Pairs two threads together perfectly element-by-element, like an Aes Sedai and her Warder. |
| `zipwith` | **`meld`** | Combining two threads using a specific function (like linking *saidin* and *saidar* to create a new weave). |
| **The Wheel, Time & Fate** | | |
| `cycle` | **`wheel`** | Endless repetition of the same Thread. *The Wheel of Time turns...* |
| `settle` | **`age`** | Applying a function over and over until nothing changes anymore—waiting for an Age to fully pass and stabilize. |
| `sort` | **`compel`** | Forcing threads into a rigid, exact, predetermined order (Compulsion). |
| `squeeze` | **`fold`** | Turning a sparse, massive axis into a dense one, skipping the empty space—exactly how Traveling *folds* the Pattern. |
| **Text & Air (Weather & Illusions)** | | |
| `strip` | **`scour`** | Scouring away the surrounding impurities or whitespace using Air. |
| `replace` | **`mask`** | Swapping one pattern for another everywhere it appears (like the *Mirror of Mists* illusion). |
| **Handling Weavings (Success & Failure)** | | |
| `rescue` | **`embrace`** | Unwrapping a successful `Woven` result (Embracing the Source). |
| `snag` | **`shield`** | Extracting the failure reason when a weave is `Gentled` (catching the shield). |
| **Logic & Choice** | | |
| `all` | **`aligned`** | Does every single thread in the sequence satisfy the condition uniformly? |
| `any` | **`glimpse`** | Does even a single element match the test? A brief flicker of vision. |

| Current Verb | Proposed WoT Verb | Lore Reason / Context |
| :--- | :--- | :--- |
| **Sequences & Threads** | | |
| `copies` | **`reflections`** | Creating identical copies of a thread, like shifting into a mirror world or looking through a Portal Stone. |
| `count` | **`score`** | Tallying up how many elements align with a specific condition. |
| `sum` | **`total`** | Accumulating all values of a thread together into one final result. |
| `prod` | **`yield`** | Combining all numbers by multiplication to find the compound output. |
| `span` | **`stride`** | An inclusive stride of whole numbers traversing from one point to another. |
| `cross` | **`interlace`** | Generating every possible combination between two threads, interlacing them completely. |
| `combos` | **`alliances`** | Finding all unique sub-groupings or subsets of a specific size. |
| `perms` | **`turnings`** | Producing every possible ordering of a thread, representing all the ways the Wheel could arrange those elements. |
| `chunk` | **`batches`** | Sectioning a long thread into clean, uniformly sized batches. |
| `windows` | **`panes`** | Sliding overlapping views across a thread, like checking consecutive panes of a window. |
| `pivot` | **`skew`** | Swapping rows and columns across nested threads to shift the orientation. |
| **Text & Air** | | |
| `split` | **`slice`** | Splitting a block of Air or text into separate fragments along a clean line. |
| `join` | **`knit`** | Knitting separate fragments of text back together into a single cohesive weave. |

## Additional todo ideas

+ Heuristic Pathfinding (A*)
Weave provides dijkstra and route out of the box, which is incredible for standard shortest-path problems. However, Dijkstra scales poorly on massively open grid searches. AoC frequently designs pathfinding puzzles (like 2021 Day 15 or 2022 Day 12) where Dijkstra will time out or eat up memory, requiring A* search with a Manhattan distance heuristic. Weave leaves A* implementation entirely to the user.

+ Shoelace Formula & Pick's Theorem
AoC has a recurring geometry pattern where you trace a massive boundary on a grid and are asked for the total enclosed area (e.g., 2023 Day 18). This requires the Shoelace formula paired with Pick's Theorem. While the math isn't difficult to write using Earth verbs, a native area or enclose verb for a Thread Knot would optimize for a major, recurring trope.

+ Prime
`primes` infinite generator via sieve, and `isprime` via algo

+ Binary search
`bidx` and `bisect`

+ Chinese remainder theorem?

+ Advanced Modular Arithmetic?
AoC loves scheduling puzzles that boil down to solving systems of modular congruences (e.g., 2020 Day 13). While Weave has mod, lcm, and gcd, it lacks a native Chinese Remainder Theorem (crt) solver or a modular inverse function. Without these, you have to write a sieve or the extended Euclidean algorithm from scratch using Weave's recursive style, which is highly error-prone under time pressure.

+ Infinite Cellular Automata ?
Pattern is bounded and dense, making it perfect for fixed worksheets. But for Conway's Game of Life puzzles that expand infinitely in all directions (e.g., 2020 Day 17), you have to abandon Pattern and fall back to using a Circle Knot (a set of active coordinates). While doable, you lose access to Pattern's slick nb8 and around8 verbs, forcing you to manually map neighbor offsets over your sets.

Here is a brief markdown bullet point list of the 5 primitives optimized for the geometric and vector-heavy puzzles seen in 2024 and 2025:

* **Grid Raycasting (`cast`)**
* **Signature:** `cast :: Pattern a -> Knot -> Knot -> Thread Knot`
* **Purpose:** Generates a line-of-sight path of `Knot`s outward from a starting coordinate along a directional delta until it hits the grid edge. Perfect for guard patrols and antinodes.


* **Knot Vector Math (`shift` / `kscale`)**
* **Signatures:** `shift :: Knot -> Knot -> Knot` and `kscale :: Earth -> Knot -> Knot`
* **Purpose:** Eliminates the verbose manual unpacking and rebuilding of coordinates by allowing direct vector addition and scalar multiplication on `Knot` structures.


* **Sparse Grid Bounding Box (`extent`)**
* **Signature:** `extent :: Thread Knot -> Hold (Knot, Knot)`
* **Purpose:** Finds the top-left and bottom-right corners of a sparse set of active coordinates (`Circle Knot`), acting as a clean bridge to convert infinite cellular automata back into dense `Pattern` boundaries.


* **2D sliding windows (`patches`)**
* **Signature:** `patches :: Earth -> Pattern a -> Pattern (Pattern a)`
* **Purpose:** Elegant 2D extension of the 1D `windows` verb, extracting overlapping sub-grid blocks to make 3x3 convolutions and local neighborhood checks trivial.


* **Modular Exponentiation (`modpow`)**
* **Signature:** `modpow :: Earth -> Earth -> Earth -> Earth`
* **Purpose:** Executes $O(\log N)$ fast modular exponentiation for massive scale cycles, serving as a core mathematical building block for encryption/scheduling loops without requiring rigid CRT data structures.

## Documentation owed

A feature lands in the compiler and in this file; the prose waits until the
feature has been used enough to be worth writing about. These are the prose
changes owed, to be done in batches.

- [ ] `solve`, once it has been used on a day or two: what the Weaving's two
  sides mean, what an empty point says, and the worked cycle-skipping shape.
- [ ] The bracket rule as it now bites at three: `add fore (mul mid aft)`
  claims the inner words for the inner brackets, so the outer group asks for a
  width of three with one word and the inner one asks again. SPEC states the
  rule but does not show it going wrong at this width.
- [ ] The verbs that can stop an endless producer, wherever SPEC.md and the
  tutorial list them — SPEC.md:170 and :257 name six, and there are thirteen.
  The diagnostic's own hint is already right.
- [ ] `dupe`'s middle field, wherever the tutorial and SPEC describe it: what
  the second position means, and the cycle-length idiom it is there for. The
  generated verb table and the two printed example outputs are already correct;
  it is the prose around them that is not.
- [ ] The packed Thread layout, in whatever part of SPEC.md or docs/performance.md
  describes how a Thread is stored: that a `Thread Earth` costs eight bytes to
  the element and not sixteen, which verbs keep it packed and which quietly
  widen it, and that none of this is visible to a program.

- [x] What the formatter breaks into, and that every line is inside the margin.
- [x] `turn`, `wrap`, `wind` and `repeat`; `weld` and `repeat` under `Ply`; the
  fusible-stage list gaining `drop` and `dropwhile`; and why `nth`, `first` and
  `last` ride a `Strand` constraint instead of the Talent.
- [x] `:AocSubmit` refetching part two on a right answer.
- [x] A binding that takes its value apart, in SPEC.md and the tutorial.
- [x] The benchmark table in docs/performance.md, remeasured.
- [x] What a value costs to hold: a `Held` of anything is unboxed bar a Held of
  a Held, and the empty Thread is one object for the whole program.
- [x] Consumed parameters, and the backtrack that no analysis will ever bless.
- [x] `weave trace -timeout` and `-memory`, the `⧖` and the `⊘`, and what the
  plugin's `timeout_ms` now means.
- [x] Hover on a binder.
- [x] `:AocTime`, `.aoc-answers`, the bracket, and `:AocSubmit` refusing an
  answer the record rules out.
- [x] What a fused `gentle` costs, and what a fused fold hands back.
- [x] That `weave build -tally` does not hear about a release.
- [x] `WEAVE_LSP_LOG`, and `WEAVE_MEM_CAP` beside `WEAVE_CACHE`.
- [x] First values inside a function body, and `weave trace -watch` with
  `:WeaveCalls`.

One question is still open, recorded here because the prose had to pick an
answer and did: a function's **parameters** do not report as ghost text, because
they are bound on the definition's own line and that line already carries the
inferred type. Three ways out — leave it (the call window has them), let a line
carry several records and have the plugin join them, or drop the type from a
function's line in favour of its first arguments. The documents describe the
first, which is what is built.

What follows is the standing note about what the build already keeps in step,
so that the next batch knows what it does not have to do.

What is *not* on this list, because the build already holds it: `docs/verbs.md`
is generated by `make docs`, the verb count in README/SPEC/tutorial is checked
by a test, the highlighter's built-in list has to equal the prelude's, the
rename ledger keeps a retired name out of every document and every program, and
every ` ```weave ` block in every document — now including SPEC.md — is compiled
and its output checked. Those are kept in step as each change lands.

The batch before last caught up: the hole words, `as`,
`else`, `failing`, `scan`, `dupe`, `gentle`, `snag`, `waters`, the where-verbs,
ranges, `twist`, the fusible-stage list, Thread in-place updating, the one-line
`ward`, `weave run` printing every bare expression, `weave trace` per line and
surviving an error, the formatter's rewrites, `weave docs`, and the Advent of
Code commands.
