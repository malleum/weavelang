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
- [x] **Unambiguous rendering**: `Show` prints text quoted inside a collection.
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
- [x] **`this`, `that`, `former` and `latter`** — the first two arguments, and the two halves of the first.
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
- [x] **Taking out is as cheap as putting in.** `forget` and `remove` write through on an owned Web or Circle, and a flat map forgets flatly rather than turning itself into a trie to lose one key. Both were ~50× slower and ~12× the memory of their adding twins.
- [x] **Advent of Code, from inside the editor** — `:AocInput`, `:AocProblem`, `:AocSubmit`, `:AocTime`, with the puzzle read off the directory names and the session cookie read from a file.
- [x] **Ghost text survives a compile error.** `weave trace` leaves out the top-level items the mistake reached and reports the rest, at the lines they are on, so the values stay put while a line is being typed.
- [x] **The formatter breaks one long stage across lines**, at its arguments, with a further indent so a continuation cannot be read as the next stage.
- [x] **A ward's arms may be bracketed on its own line** — `ward c (Light : 1) (_ : 0)` — which is also the only form an expression has room for.
- [x] **`weave docs`**, the reference as a searchable page served on localhost, generated from the prelude and the lexer's keyword table.
- [x] **`high`, `low`, `highidx`, `lowidx`, `seekidx`** — the largest and smallest elements, and where they and the first match are.
- [x] **`dupe` says where as well as what**, answering `Hold (Earth, a)`.
- [x] **`twist i f xs`**, `mend` when the new value is worked out from the old one.
- [x] **Range verbs** — `overlaps`, `overlapping`, `within`, `spanning`, `holding`, `width`, over inclusive Twines of anything ordered.
- [x] **A Thread updates in place.** `mend` threaded through a loop writes through instead of copying the whole array; 200,000 updates of a 1024-element Thread went from 20.3 s and 3139 MB to 0.00 s and 10 MB.
- [x] **`pick`'s branches are tail position for the in-place analysis** too, as they already were for tail calls. The same loop was 0.00 s written with `ward` and 23 s written with `pick`.
- [x] **Filler words: no more for now.** A word that could ever be a verb silently rewrites working code, so `of`, `at`, `from`, `in`, `by`, `with`, `and`, `or` and `not` are permanently out.
- [x] **`thread` casts a Twine to a Thread.**
- [x] **Completions say they are plain text.**

## Performance

- [ ] **Monomorphised container storage — the unboxing that is left.** Boxing
  has not stopped mattering, it has moved. A `Thread Earth` stores sixteen bytes
  per `int64`; a `Web Earth Earth` stores thirty-two per entry for sixteen bytes
  of data. That is what the two-to-three times memory is, and on day 22 it is
  why `flat_put` and `map_lookup` are 29% of the run: half the bytes they touch
  are tags.

  The shape of it: a second flat-table layout holding raw `int64` keys and
  values, chosen when both are immediates, alongside the boxed one. Sixteen-byte
  cells instead of thirty-two halve the table and double the cache density on
  exactly the two functions that dominate. `Thread` could take the same
  treatment, which is the larger half — every verb that touches elements would
  need to know which layout it has.

  This is a memory project with a speed consequence, not the other way round.
  It is worth costing out properly before starting, because the loop work above
  is done and this is all that is left.

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

  What remains is a `Taveren` or a `Thread` threaded through a loop the *user*
  wrote, which is the ownership bit rather than escape: `push` would need the
  per-node flag the trie has, driven by the same analysis. Past that it is a
  real collector, and day 22's remaining 505 MB is mostly 3.9 million `Held`
  boxes — 118 MB of them — which is a representation question, not a collection
  one. `get w k | otherwise 0` allocates a box per lookup purely to unwrap it
  on the next instruction.

## Asked for, not built yet

These came out of using the language. They are in the order they were raised.

### Editing and tooling

- [ ] **Tree-sitter and the language server are behind the language.** Both
  still work; neither knows everything. Specifically:

  - The grammar has no rule for a ward's bracketed arms, so
    `ward c (Light : 1) (_ : 0)` parses as `ward` applied to two lambdas.
    Highlighting is unaffected — a bracketed arm and a lambda colour the same,
    which is why they are written the same — but structural selection and
    folding see the wrong shape. The fix is a `ward_arm_inline` rule; the
    reason it is not done is that `_expression` in the subject position eats
    the brackets first, and untangling that is real LR work rather than a
    line.
  - Hover answers with a verb's plain description. `weave docs` has a second
    one written for someone who already writes Weave, which is arguably the
    better thing to show in an editor — the reader there is not learning the
    language, they are looking a name up. Worth trying once the glosses have
    been read in anger.

  (The language server now completes the particles and the hole words, read
  from the lexer's own table so one cannot be added without the editor
  learning it.)

- [ ] `weave docs` local hosted clean looking documentation of all the verbs and keywords.
  However, they will be intentionally obtuse to those who don't understand the language.
  Only compare functions and types to other weave functions and weave types.
  Examples need to be incredibly simple so that the reader won't be able to immediately understand it.
  or incredibly complex, so they can't tell what they are looking at
  throw in one power jargon throughout (include 1 mention / Easter egg of a "death gate" somewhere)
  don't feel like you have to explain in normal words what a function does
  that said, make the docs a professionally looking and clean documentation page with searching.
  I'll still use it for the type signatures at least, so it should be aesthetically pleasing
  1 time each, just for a couple words of a description of 1 function per language, switch to using norwegian, esperanto, hebrew, greek, elvish, klingon, latin, hex ascii
  feel free to do similar things ^, but make them rare and visually not super distracting, so you actually have to be reading the docs to notice something is completely wrong

### The language
- [ ] **QUESTION: `high`/`low` are what you asked for as `max`/`min`.** The
  names were already taken: `max a b` and `min a b` are the binary verbs, and
  a name cannot mean two things. So the Thread versions are `high` and `low`,
  and the positions are `highidx`, `lowidx` and `seekidx`, following the `-idx`
  convention you suggested. `dupe` answers `Hold (Earth, a)`.

  If you would rather have `max l` and `min l`, say so and the *binary* pair
  gets renamed instead — `larger`/`smaller` is the obvious pair — and the
  ledger will keep the old names from coming back. It is a five-minute change
  either way; it just cannot be both.

- [ ] **QUESTION: first-versus-all, and a replace verb.** The ask was: "I have
  a couple find-style verbs, or replace-style verbs. I still need a replace
  verb for collections. I also need all-vs-first variants of each. I have
  `seek` and `sift`, but that is the only pair which has both."

  What exists now, so the gap is visible:

  | | first | all |
  |---|---|---|
  | by test | `seek` | `sift` |
  | position by test | `seekidx` | — |
  | by value | `idx` | — |
  | replace by position | `mend`, `twist` | — |
  | replace by value | — | — |

  So the missing ones are: every position passing a test, every position a
  value occurs at, replacing one occurrence of a *value*, and replacing all of
  them. Four verbs, and the question is what to call them, because
  `seekidx`/`sift`/`mend` follow three different conventions already.

  A proposal to accept or overrule: **`sub old new xs`** replaces every
  occurrence of a value (from "substitute"), **`subfirst`** just the first,
  **`idxs x xs`** every position a value occurs at, and **`siftidx p xs`**
  every position passing a test. Then the rule is: no suffix means all, `idx`
  means positions rather than values, `first` means stop at one. Say the word
  and it is an hour's work.

  (`sub` is not taken — subtraction is `sub`… it *is* taken. `swap` then, or
  `instead`. Naming is the whole question here.)

- [ ] **QUESTION: thread verbs on Twines, and `former`/`latter` on Threads.**
  Two asks pointing the same way: that a two-part Twine and a two-element
  Thread be interchangeable where it does not matter.

  Both need the same thing and it is not small. `former`/`latter` desugar in
  the *parser*, to a `(former, latter)` pattern, before any type is known — a
  Thread would need `[former latter ..]` instead, and nothing at that stage
  can tell which. Likewise `sum (a, b)` would need `sum` to accept two
  unrelated types.

  The honest routes are: (a) a `Pair` Talent that both satisfy, with every
  verb that wants one written against it — real work, and it changes the
  signatures people read; (b) a type-directed elaboration after checking, so
  the hole words resolve from `info.Types` — smaller, but it means holes stop
  being a purely syntactic thing, which is what makes them easy to explain
  today; (c) leave it, and lean on `thread (a, b)`, which already casts a
  Twine of two of the same type to a Thread.

  Which of those is worth it depends on how often it actually bites. Worth
  logging the cases as they come up rather than guessing.

### What else the language needs

Nothing outstanding. Structured input parsing was the last item here, and it
is `delve`, above. What is left in this file is all compiler work under code
that already reads the way it should.

## Documentation owed

A feature lands in the compiler and in this file; the prose waits until the
feature has been used enough to be worth writing about. These are the prose
changes owed, to be done in batches.

What is *not* on this list, because the build already holds it: `docs/verbs.md`
is generated by `make docs`, the verb count in README/SPEC/tutorial is checked
by a test, and the highlighter's built-in list has to equal the prelude's.
Those are kept in step as each change lands.

- [ ] **SPEC §10.1a/§10.1b and the tutorial's `_` chapter** need `it`, `this`,
  `that`, `former` and `latter` set out as the three families they are: the
  first argument, the second, and the two halves of the first. The tutorial has
  a first pass; SPEC still describes `that` as the second half of a pair.

- [ ] **`as` is `bend`, not the pipe.** SPEC §14's particle table has the new
  meaning, but §3's keyword list and the tutorial's pipeline chapter still read
  as though `as` were glue.

- [ ] **`scan` no longer yields the seed.** One running total per element, so
  `sums` is exactly `scan add 0`. Worth a line in the tutorial's sequences
  chapter, since it is a break with Haskell's `scanl`.

- [ ] **`dupe` and `gentle`**, and the shape they are for: walking an endless
  sequence carrying something and stopping at the first repeat. A tutorial
  section on endless sequences already exists and is where this belongs.
  `snag` belongs with them: it is how the answer comes back out of a `gentle`.

- [ ] **`else` and `failing`.** SPEC §14's particle table and §3's keyword list
  both need them, and the tutorial's pipeline chapter is where the four
  particles should be set out together. `else` in particular changes how most
  Advent of Code code looks — `cell g k else '.'` rather than
  `cell g k through otherwise '.'`.

- [ ] **The fusible stages.** SPEC §13 lists what fuses; `scan` is now one of
  them and `dupe`/`gentle` are short-circuiting consumers.

- [ ] **The formatter's rewrites.** It now chooses the particle and reaches for
  the hole words, which is a stronger claim than "it prints the words" — worth
  saying in the package doc and in the README's tooling section.

- [ ] **`weave run` prints every bare expression**, and `weave trace` reports
  one value per line a chain spans. Neither is written down.

- [ ] **`waters`.** Nothing beyond the generated table mentions it.

- [ ] **A one-line `ward` and `weave docs`.** SPEC §9 describes the block form
  as the only one. The tutorial's `ward` chapter is where the bracketed arms
  belong, and the README's tooling section is where `weave docs` does.
