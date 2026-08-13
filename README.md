<div align="center">

```
 ██╗    ██╗███████╗ █████╗ ██╗   ██╗███████╗
 ██║    ██║██╔════╝██╔══██╗██║   ██║██╔════╝
 ██║ █╗ ██║█████╗  ███████║██║   ██║█████╗
 ██║███╗██║██╔══╝  ██╔══██║╚██╗ ██╔╝██╔══╝
 ╚███╔███╔╝███████╗██║  ██║ ╚████╔╝ ███████╗
  ╚══╝╚══╝ ╚══════╝╚═╝  ╚═╝  ╚═══╝  ╚══════╝
 ╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲
```

**A strict, functional, statically-typed language for Advent of Code.**
Minimal syntax · One-Power vocabulary · compiled through C by `clang -O3`

[Tutorial](docs/tutorial.md) ·
[Vocabulary](docs/verbs.md) ·
[Specification](SPEC.md) ·
[Performance](docs/performance.md) ·
[Editor setup](docs/nixvim.md)

</div>

```weave
# Sum every number in the input.
nums is
  Source
    | lines
    | bend earth
    | bend (otherwise 0)

nums | sum
```

New to the language? [docs/tutorial.md](docs/tutorial.md) starts from nothing
and works up to two solved Advent of Code puzzles. The full design lives in
[SPEC.md](SPEC.md). The short version:

- **Input is a variable, output is the last expression.** `Source` holds the
  input; the program's final bare expression is what it prints.
- **No loops, no operators, no null.** Recursion and sequence verbs replace
  loops; arithmetic is named (`add`, `mul`); absence is `Stilled`, which the
  compiler makes you handle.
- **Your own sum types.** `Move is Step Direction Earth | Rest` declares a
  type that sorts, prints and hashes without a line of boilerplate, and that
  the exhaustiveness checker holds you to.
- **Immutable to you, mutable underneath.** `set pattern k v` mutates in place
  when the old value is dead, so updates cost what they cost in Go.
- **Lazy where it matters.** `bend | sift | seek` fuses into a single pass
  with no intermediate collections, and `flow` gives you an endless sequence
  the compiler makes you bound.
- **Words for the arguments you did not name.** `this` is the first, `that` the
  second, `fore`, `mid` and `aft` the components of a Twine:
  `xs where (mod this 2 | eq 0)`, `braid (add this that) 0`,
  `pairs as add fore aft`. `web | get _ "a"` puts the piped value where
  the hole is, for the verbs that want the collection first.
- **Real data structures.** `Web` and `Circle` are hash array mapped tries,
  `Taveren` is a leftist heap, `Pattern` is a grid you can lay out from its
  knots or ask a summed-area question of, and `Link` is disjoint sets. A shortest path is
  one verb: give `dijkstra` the steps out of a place and it answers with the
  cost of reaching everywhere — see `examples/maze.weave`. `clumps` gives the
  groups of a graph that can reach one another, and a `Link` answers the same
  question while the joining is still going on, which is what Kruskal's
  algorithm needs.
- **233 verbs**, named after [rask](https://github.com/malleum/rask) wherever
  there is no One-Power word for the job, so the two read alike. `weave verbs`
  prints them all, and [docs/verbs.md](docs/verbs.md) is the same list with
  types, generated from the compiler's own prelude.
- **The ring shapes, without a ring type.** `turn n xs` shifts a sequence round
  — what comes off the front goes on the back, a negative count turns the other
  way, and any count works however far past the length it is. `wrap i xs` is
  `nth` on a ring, so `wrap (neg 1)` is the last strand and only an empty Thread
  answers `Stilled`; `wind` is the same for writing, so `wind (neg 1) v xs`
  replaces the last one. Between them they are what a circular list type would
  have been wanted for, without a second sequence type whose every verb needs
  its own ruling — and without pretending an array with modulo indexing is the
  O(1) splice a marble ring actually needs. `repeat n xs` lays a sequence end to
  end, which saves building the outer Thread that `copies n xs | flat` throws
  away. All four are `Ply`, so text turns, wraps and repeats by rune too.

## Status

Working end to end: Weave programs compile to native executables and run.

```sh
$ printf '1721\n979\n366\n299\n675\n1456\n' | weave run examples/aoc2020-day01.weave
514579
```

| Phase | State |
|---|---|
| Chain fusion, tail calls | working |
| Lexer, with indentation layout | working |
| Parser and syntax tree | working |
| Diagnostics with source excerpts | working |
| Type inference and Talents | working |
| Exhaustiveness and reachability | working |
| C backend, runtime, `weave run` | working |
| Web, Circle, Taveren, Link | working |
| In-place updates, in a loop and in a fold | working |
| User-declared sum types | working |
| Endless `flow` sequences | working |
| `remember` memoisation | working |
| `Weaving`, `rescue`, `dijkstra` | working |
| Tree-sitter grammar, LSP, formatter, REPL | working |
| Tutorial, generated vocabulary reference, `weave docs` | working |
| Several bare expressions per file | working |
| Language server inside Markdown code blocks | working |
| A flat hash table for immediate keys | working |
| A memo table of `remember`'s own | working |
| Reusing dead arena memory | working |
| Fusing `zip`, `items`, `enum` and `couples` into the loop that unpacks them | working |
| Releasing the Threads a call leaves behind | working |
| Reading a map back without a comparison sort | working |
| Fusing `zipwith` into the loop that consumes it | working |
| A `Held` of a Power is not a box | working |
| Fusing the grid walks, and a lambda written on the spot | working |
| Summed-area tables over a grid (`tallies`) | working |
| Monomorphised container storage | next |
| Structured input parsing (`delve`) | working |

`weave` on its own prints the command list:

```
 ██╗    ██╗███████╗ █████╗ ██╗   ██╗███████╗
 ██║    ██║██╔════╝██╔══██╗██║   ██║██╔════╝
 ██║ █╗ ██║█████╗  ███████║██║   ██║█████╗
 ██║███╗██║██╔══╝  ██╔══██║╚██╗ ██╔╝██╔══╝
 ╚███╔███╔╝███████╗██║  ██║ ╚████╔╝ ███████╗
  ╚══╝╚══╝ ╚══════╝╚═╝  ╚═╝  ╚═══╝  ╚══════╝
 ╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲
 a functional language for Advent of Code, compiled through C  · dev

commands

  run      file.weave     compile and run, feeding stdin to Source
  build    file.weave     compile to a native executable
  check    file.weave     parse and check, reporting any errors
  fmt      file.weave     print in canonical form (-w rewrites; - reads stdin)
  repl     [input]        evaluate definitions and expressions as you type them
  test     file.weave     run a program against the .in/.out files beside it
  trace    file.weave     print every definition's value, one record per line

  verbs    [search]       the built-in vocabulary, with types
  lsp                     run the language server on stdin and stdout
  version                 print the compiler version

  parse    file.weave     print the syntax tree (for debugging the compiler)
  lex      file.weave     print the token stream (for debugging the compiler)

  weave file.weave is short for weave run file.weave
```

Output is coloured when it is going to a terminal, and plain when it is not.
`NO_COLOR`, `TERM=dumb` and `WEAVE_COLOR=never` all turn it off.

`weave build -tally` compiles a program that keeps its own books: every live
block is recorded against the source line that asked for it, and the breakdown
at the high-water mark goes to stderr when it ends. The arena bump-allocates out
of megabyte chunks, so an ordinary heap profiler sees one `malloc` charged to
whichever call happened to exhaust the last chunk and nothing at all about what
went in it. It is a debugging build — the bookkeeping costs more than the
allocation does — so never time one.

`weave build -overflow` compiles Earth arithmetic with an overflow check, so a
number too large for an `int64` stops the program and names the verb instead of
wrapping round to a wrong answer. It costs a third to three quarters on a loop
that does nothing but arithmetic, and nothing measurable on a program that also
touches memory — [docs/performance.md](docs/performance.md) has the table.

`weave verbs` searches the vocabulary by name, type or description:

```
$ weave verbs dijkstra
Priority queues and graphs
  dijkstra :: (a -> Thread (Earth, a)) -> a -> Web a Earth  where Ord a
    cheapest cost to every node reachable from here, given a step function

1 of 233 built-ins
```

`weave docs` serves the same vocabulary as a page, with search over every name,
signature and description at once:

```
$ weave docs
the reference is at http://127.0.0.1:7373
```

It is one file with nothing fetched from anywhere, so it works with the network
off; `weave docs -o page.html` writes it out. It is a reference rather than an
introduction — every verb there is explained by other verbs and every type by
other types, which is no use at all until you already write Weave, and exactly
what you want once you do.

### Editor support

Two halves. `tree-sitter-weave/` is the grammar — highlighting, folding,
structural selection, and ` ```weave ` blocks inside Markdown. The flake exposes it as
`packages.tree-sitter-weave`, built from the committed `src/parser.c`, so
nothing needs the tree-sitter CLI to install it.

Its scanner ports the compiler's layout algorithm, and `go test` runs the
grammar over every example, so the two cannot drift apart silently.

`weave lsp` speaks the Language Server Protocol on stdio. Everything it reports
comes from the compiler's own front end, so the editor and `weave check` cannot
disagree — a full parse and type-check takes about 4 ms, so it re-checks on
every keystroke rather than debouncing.

It provides diagnostics (with the compiler's hints), hover showing inferred
types and built-in documentation, completion over the 233 built-ins and your
own definitions with their signatures, signature help while applying a verb,
and formatting.

Hover answers where a name is **bound** as well as where it is used — a
parameter in the list, a `weave` name, a name a pattern takes apart — which is
the half anybody actually hovers, and the one case where the answer is not
already on the screen. A binder is answered as itself: a parameter called `sum`
is a parameter, not the verb it shadows.

It also works **inside ` ```weave ` blocks in a Markdown file**: the server
finds the fences and checks each as its own program, reporting diagnostics at
the line they sit on in the document. `weave.nvim` traces them the same way, so
a block's definitions get their values beside them.

```lua
vim.lsp.config.weave = { cmd = { "weave", "lsp" }, filetypes = { "weave", "markdown" } }
vim.lsp.enable("weave")
```

`weave.nvim/` shows every definition's value at the end of its line, run
against the largest input file next to the program — which for Advent of Code
is the real one and not the sample. A chain written one stage to a line gets a
value for *every* line it spans, so a pipeline reads as a sequence of shapes.
It builds on `weave trace`, which reports one record per line and is dull
enough to drive from anything.

A file being edited does not compile most of the time, so `weave trace` leaves
out the top-level items the mistake reached and reports the rest, at the lines
they are on. The values stay put while a line is being typed instead of
blinking out on every keystroke.

A definition that will not *finish* is the same problem wearing different
clothes, and gets the same answer. `-timeout` gives one definition a limit and
`-memory` gives it a ceiling in megabytes; a definition that runs past either
reports **`⧖`** for the clock or **`⊘`** for the ceiling instead of a value, and
the rest of the file is traced without it. Neither mark is a value, and a
half-written definition is as likely to ask for every byte in the machine as it
is to loop for ever — which matters when this runs on every save.

Ghost text reaches inside a function body too, for the **first** value each
binding there ever holds. A line inside a function has no single value, so it
used to have none at all; the first is the one you can reason about, because it
is the call you would have made by hand. It survives the line that never
finishes: a loop that times out still shows what its first step held.

For the rest of a recursion there is **`weave trace -watch f`**, and
`:WeaveCalls` in the editor, which put what `f`'s names held on *each* call into
a floating window — a column per name, a row per call, bounded at both ends with
the count between them. It runs on demand rather than on save, because recording
per call costs the fusion inside the body, and a watched function is not inlined
into a fused loop, which is a real difference in what gets compiled. Its records
are the ordinary trace format with a call number in front:
`@LINE⇥CALL⇥NAME⇥VALUE`, marked so that anything reading the by-line records
skips them.

The plugin also does Advent of Code: `:AocInput` fetches the input beside the
program, `:AocProblem` opens the day's text in a split without taking the
cursor out of the file you are editing, and `:AocSubmit` sends the answer for
the part the cursor is in — refusing one the record already rules out, since a
repeat of a wrong answer is wrong and an answer past a bound already found is
past it. A right answer refetches the day on its own, so part two arrives
without asking. `:AocTime` says what has already been sent for each part with
what the site said about it, and the bracket the too-highs and too-lows put the
answer in. Which puzzle a file belongs to is read off the directories above it.

[docs/nixvim.md](docs/nixvim.md) wires all four pieces up in about thirty
lines.

### Formatting

`weave fmt` throws the layout away and prints the syntax tree, so redundant
parentheses, stray blank lines and inconsistent indentation cannot survive —
there is no code path that would emit them. Comments are kept, and a call
written as a pipeline comes back as one.

It has one setting, and it is about spelling. Weave gives its punctuation words
— `is` for `=`, `gives` for `:`, `through` for `|`, `this` for `_` — and the
words are the language while the symbols are the shorthand, so the words are
what it prints:

```
nums = Source|lines|bend(earth)   ->   nums is Source through lines as earth
```

That is a stronger claim than it looks, because the formatter *chooses* the
spelling rather than keeping the one you typed:

- **`sift` becomes `where` and `bend` becomes `as`** in the wordy style, and
  back into verbs in the terse one. Which you wrote is not a matter of meaning
  — a particle desugars to its verb by name, so both resolve to the same thing
  even where the name has been shadowed.
- **A lambda becomes a hole group** wherever that reads back as the same
  function: `(x : add x 1)` comes back as `(add this 1)` and
  `((a, b) : add a b)` as `(add fore aft)`. Whether it can is not a
  property of the lambda alone — a hole is claimed by the brackets closest to
  it, so an occurrence inside a nested group would be claimed by that instead.
  Rather than enumerate the cases, the rewrite is checked by doing it: print
  the candidate, read it back, and keep it only if what comes back is the
  candidate. `(x : add x (mul x 2))` fails that check and keeps its parameter.

`weave fmt -terse` prints the symbols and the verbs instead. They are the same
tokens, so either spelling reads back as the other and neither is more correct
— it is a question of whether you would rather read or squint.

A pipeline past 80 columns is broken one stage per line, and a stage that is
still too long is broken again at its arguments, one to a line at a further
indent. A bracketed literal — a Thread or a Twine — breaks one element to a
line with the comma leading, so that every line after the first opens no block
and continues the one above it. And a chain ending in a hole word is still a
chain: `xs | aft` desugars in the parser to a match on the pair it opens, so
the formatter used to stop seeing a pipeline at all and let the whole thing run
off the edge. Every code line in this repository is inside the margin.

`-check` exits non-zero when a file is not already formatted, which is what
`make check` runs over the examples and the Advent of Code solutions.

It refuses a program it cannot parse rather than printing it. The parser
recovers by leaving what it could not read out of the tree, and printing that
tree would quietly delete the line the mistake was on — the one thing a
formatter must never do.

### Testing against the samples

Advent of Code hands you a sample and its answer every day. `weave test` runs
the program against the fixtures beside it — `day05.in` and `day05.out` next to
`day05.weave`, or a `testdata/` directory — so re-checking the sample after a
change is one command rather than a decision:

```
$ weave test examples/*.weave
ok   aoc2020-day01
ok   fib
FAIL maze
     expected 4
          got 3

7 passed, 1 failed
```

A numbered pair — `day05.1.in` and `day05.1.out` — is a second case, which is
how a day with two parts or two samples fits. An input with no matching output
is not a case, so the real puzzle input can sit in the same directory.

### The REPL

`weave repl` keeps the definitions you enter and compiles the whole program
each time, so what it prints is exactly what `weave run` would. Give it your
puzzle input and iterate:

```
$ weave repl input.txt
weave> nums is Source | lines | bend earth | bend (otherwise 0)
nums :: Thread Earth
weave> nums | sift even | sum
12345
weave> :type bend
bend :: (a -> b) -> Thread a -> Thread b
```

A line ending in `is` or `:` starts a block, ended by a blank line. `:help`
lists the commands: `:list`, `:drop`, `:clear`, `:type`, `:source`, `:quit`.

The line editor has vi bindings, the way fish and zsh do: `Esc` for normal
mode, then `h`/`l`/`w`/`b`/`e`/`0`/`^`/`$` to move, `x`/`X`/`D`/`C`/`S`,
`d` and `c` with a motion, `r`, `p`, `u`, and `i`/`a`/`I`/`A` to go back to
inserting. The arrow keys walk the history, as do `k` and `j`; it is kept in
your user cache directory, so it survives the session. `Ctrl-A`/`E`/`U`/`K`/`W`
work in insert mode, `Ctrl-C` abandons the line and `Ctrl-D` on an empty one
leaves. When standard input is not a terminal the REPL reads it as a script,
which is how it is tested.

A definition is allowed to be incomplete while you are still writing it:

```
weave> fib 0 is 1
note: `fib` does not handle every case
      add a `_` arm to cover the rest
fib :: Earth -> Earth
weave> fib n is mul n (fib (sub n 1))
fib :: Earth -> Earth
weave> fib 5
120
```

`weave check` and `weave build` still refuse that first line — a program that
stops halfway is not what anyone wanted — but in a REPL the next line is the
other clause. If you run something that reaches the case you have not written,
the program stops there and says `no clause of `fib` matched`.

Turnaround is about 90ms per line: the C compiler runs at `-O0` and the
runtime is compiled once into cached object files.

### Compile times

The runtime is cached as object files under your user cache directory, keyed by
its contents and the flags it was built with, so only your program is compiled
on each run. Set `WEAVE_CACHE` to move it, or to empty to turn it off; a cache
directory that cannot be made is not an error, since caching is an optimisation
and the build has a perfectly good uncached path.

Two more environment variables the runtime reads. `WEAVE_MEM_CAP` is a ceiling,
in bytes, on what a compiled program may take from the operating system: it
stops rather than growing past it, and exits with a code of its own so that a
ceiling is not mistaken for an ordinary failure. `weave trace` sets it; nothing
else does, because a batch job given an input is entitled to whatever that input
costs. `WEAVE_LSP_LOG` names a file for `weave lsp` to append to — every method
with its timing, and any panic with its stack. Standard output is the protocol
and standard error belongs to the editor, so a server with something to say to a
person needs somewhere else to say it, and it is the first thing to reach for
when the language server misbehaves.

| | cold | warm |
|---|---|---|
| `weave run` on a small program | 0.95 s | 0.13 s |

### Speed today

Every benchmark is a pair of programs, one Weave and one Go, reading the same
input and required to print the same answer. Best of five, process start-up
included. [docs/performance.md](docs/performance.md) has the full set, the
method, and the Advent of Code days.

| Benchmark | Weave | Go |
|---|---|---|
| `fib 32`, argument read at runtime | **9.4 ms** | 10.5 ms |
| map + filter + sum over 20M | **25.8 ms** | 26.2 ms |
| the same, written with intermediate slices in Go | **25.7 ms** | 621 ms |
| longest Collatz chain below 300 000 | **46.1 ms** | 74.6 ms |
| 3.2M words of text, counted | **177 ms** | 179 ms |
| 100M tail-recursive steps | **54.7 ms** | 65.0 ms |
| 2M map insertions | **170 ms** | 183 ms |

Weave now wins every one of these. Seven optimisations got it there, each
described below: chains fuse into a single loop, primitive operations are
specialised to their inferred types, self and mutual tail calls become jumps, a
collection threaded through a loop is updated in place, a pair that is taken
apart on the spot is never built, `zipwith` combines two Threads without a
closure or a call, a `Held` of one of the Powers is not a box, and a function
hands back the Threads it built that nothing else can reach.

The last row used to be the honest weak spot, at 406 ms against Go's 154. A map
the compiler has proved unshared, with immediate keys, is no longer a
persistent trie but a flat open-addressed table, and `remember` no longer keeps
its results in a map at all. Advent of Code 2024 day 11 went from 402 ms to
27 ms with that second change alone.

That table packs itself further when the values are unboxed too: a slot then
holds two raw payloads rather than two tagged Values, with one shared tag per
column, so it is sixteen bytes instead of thirty-two and a probe is a single
`int64` compare. It widens back to tagged slots the moment anything disagrees,
so the packing is an experiment the table abandons rather than a promise the
type system has to keep. Day 22 went from 622.5 ms and 387 MB to 516.9 ms and
261 MB.

The arena does not collect, but it does reuse: the places that know a block is
dead — a buffer that has just outgrown its array, a trie node an owned insert
has replaced — hand it back for the next allocation of that size.

One benchmark is still behind, Advent of Code 2024 day 22, now at 2.7× — down
from 12.6× when the work started. Five measurements took it there. It was
allocating 19.6 million 32-byte pairs that `zip` and `items` built only to be
unpacked on the next line, so both became producers the fused loop generates and
the pair stopped existing. It was keeping 694 MB of intermediate Threads that
die with the call that built them, which the compiler now proves from the
result's type and hands back at the return. And a quarter of it was sorting
map entries on the way out of `items`, which is now a radix sort over ranks
rather than a comparison sort over whole entries. And `zipwith` was reaching its
function through a closure once per element, which fusing it removed.

A `Held` of one of the Powers stopped being a box, which gave it back 117 MB.

What is left is the value representation, but not where the list expected: clang
already unboxes the loops — the generated assembly has no tags in it at all — so
what remains is that a `Web Earth Earth` spends thirty-two bytes an entry on
sixteen bytes of data. That is a container question rather than a loop one, and
it is the two-to-three times memory as well.

What remains is the value representation itself: every value is a 16-byte
tagged union. Measured against hand-written C, that is worth about 1.6× on
recursive arithmetic and about 2× on a fused loop — real, but it needs
monomorphisation, which duplicates every polymorphic function per instantiation
and changes the calling convention, the collection storage and the interface to
every optimisation above. It is not built, and the note in
[TODO.md](TODO.md) says what the measurements were.

### Fusion

A pipeline compiles to a single loop, not a call per stage:

```weave
span 1 1000000 | bend (x : mul x x) | sum
```

becomes one counted loop with the span generated in the header, the lambda
inlined to a multiplication, and no Thread allocated anywhere. Chains ending in
`seek`, `any`, `all` or `first` stop early, so a search no longer maps the
elements it never looks at.

A chain fuses when there is something to save. Two stages save the Thread
between them; a producer the loop *generates* — a `span`, or one of the grid
walks `knots`, `nb4`, `nb8`, `around4`, `around8` — saves the array it would
have built, so one stage is enough and a bare consumer is enough too; and a
lambda written on the spot saves the closure, which is what a backtracking
search spends its memory on. `enum` and `couples` join `zip` and `items` as
producers that yield *pairs*, so a Twine the next stage takes straight apart is
never built at all.

Three measurements from Advent of Code 2025, all on real input: day 4's
eight-neighbour count went from 628 ms to 93 ms once the neighbour walk stopped
allocating a Thread per cell, day 8 from 592 ms to 485 ms on the pairs, and day
12's peak heap from 2.5 GB to 1.6 GB as nine million closures stopped being
built.

`weave build -no-fuse` turns it off, and the differential suite compiles every
pipeline shape both ways and requires identical output.

### Primitive specialisation

The prelude's `add` branches on whether either operand is Water, and `eq` calls
a structural comparison that switches on tags. Neither is needed once the types
are known, and the checker records the type of every expression, so

```weave
span 1 20 | bend (x : mul x x) | sift (gt 50) | sum
```

compiles with `w_mul_e`, `w_gt_e` and `w_add_e` — an integer multiply, a
compare instruction and an integer add, with no tag dispatch anywhere. Where a
type is a variable rather than a primitive, nothing is specialised and the
general verb runs exactly as before, so the pass never has to prove anything
the type checker has not already proved.

`weave build -no-specialize` turns it off.

### In-place updates

`set g k v` returns a new grid, so the runtime copies the cell array; `put w k v`
and `insert c x` path-copy a trie. In a loop both are ruinous, and since the
runtime never frees, the copies stay. When the compiler can prove a loop threads
a collection without ever duplicating it, and the runtime confirms it did not
arrive shared, the update writes through instead:

| | copying | in place |
|---|---|---|
| 20000 updates to a 200x200 grid | > 2 min | 12 ms |
| 2M inserts into a Web, as a loop | 44.6 s, 8.9 GB | 0.50 s, 99 MB |
| 2M inserts into a Web, as a `braid` | 13.8 s, 9.1 GB | 0.42 s, 99 MB |

Grids carry one ownership bit, since the cells are one block. A Web or Circle
carries one per trie node: an insert writes through the owned prefix of the path
and copies from the first shared node down, so the first turn of a loop copies,
later turns mostly do not, and no node is copied twice however long the loop
runs.

The proof used to be a whitelist: about seventeen blessed verbs per collection
could read it and the rest of the prelude could not, so a loop that asked its
map anything unusual copied on every turn with nothing in the source to say why.
It is now stated the other way round. A verb may read the collection unless it
can *keep* it, and only two things disqualify one — nine hand back a window on
the argument's own array (`take`, `drop`, `sever`, `strands`, `takewhile`,
`dropwhile`, `chunk`, `windows`, `cells`) and three hand the argument itself
back when the update had nowhere to go (`mend`, `twist`, `set`). Beyond that the
*type* decides: the collection's own type constructor must not occur in the
call's result. That rule needs no list to maintain, covers the program's own
helper functions, and cannot be quietly narrowed by adding a verb.

Four more shapes joined it. `gentle` threads its accumulator exactly as `braid`
does and had simply been left out. A loop that only *sometimes* updates keeps
the fast path, since handing the collection straight back into its own slot is
as single-threaded as writing to it. A fold's accumulator may be a **Twine of
state** with the collection as one half — `(board, position)` threaded through a
walk — which is how you carry two things and used to be the worst shape in the
language. And a **named** step function of one clause is read back as the lambda
it is and inlined, so lifting a step out to a name costs nothing.

A walk over 20,000 elements carrying `(thread, index)` through a `gentle` went
from 10.1 s and 1.2 GB to 6 ms and 1.8 MB.

Widening it uncovered a miscompile that had been there since the analysis was
written. The arguments of a tail call are evaluated in order into the loop's
slots, so an update at one writes through before a later one is evaluated — and
`fill (sub n 1) (put w n n) (add acc (len w))` read the map *after* the write it
had not yet conceptually performed. The differential suite caught it the moment
the reading rule was wide enough to make it easy to write.

Bind the collection to another name, put it in a Twine, capture it in a lambda,
or read it in a sibling argument of the update, and it copies. `weave build
-no-in-place` turns it off, and the differential suite runs every aliasing
hazard both ways.

`dijkstra` skips the analysis entirely: its frontier never leaves the function
and its distance map does not escape until it is returned, so both are owned by
construction.

### Tail calls

A definition that calls itself in tail position becomes a jump, so recursion
costs what a loop costs:

```weave
count 0 acc is acc
count n acc is count (sub n 1) (add acc n)

count 100000000 0
```

runs in 0.34 s in one stack frame. Before this, a five-million-step count
segfaulted at every optimisation level — clang does not reliably do it for us.
`pick` is a tail position in both branches.

Definitions that tail-call *each other* get the same treatment. They form a
cycle in the tail-call graph, and each cycle is compiled into one C function
whose loop switches on which member is running, so `even`/`odd` handing control
back and forth fifty million times runs in one frame. Each member keeps its own
entry point, so an ordinary call is unchanged — the alternative, a trampoline,
would have cost every call an allocation whether it needed one or not.

## Documentation

| | |
|---|---|
| [docs/tutorial.md](docs/tutorial.md) | learn the language, from `Source` to two solved puzzles |
| [docs/verbs.md](docs/verbs.md) | all 233 verbs with types, generated from the prelude |
| `weave docs` | the same vocabulary as a page on localhost, with search |
| [docs/performance.md](docs/performance.md) | Weave against Go, raw and on Advent of Code 2024 |
| [docs/nixvim.md](docs/nixvim.md) | grammar, LSP, formatter and plugin in one nixvim block |
| [SPEC.md](SPEC.md) | the language design, and why each piece is the way it is |

The reference is generated by `make docs`, and a test fails if it is stale, so
a verb cannot be added without being documented. `weave docs` builds the same
thing as one self-contained page — no font, no script, no stylesheet fetched
from anywhere — with search over every name, signature and description at once.
There each name carries a second description written for someone who already
writes Weave: every verb explained by other verbs and every type by other
types, which is a poor introduction and a good reference. Every ` ```weave ` block in
every one of these files is compiled by the test suite, and where a block shows
its output, that output is checked — the examples cannot rot.

## Building

With Go 1.24+:

```sh
make build     # -> bin/weave
make check     # fmt, vet, tests, and every example
```

With Nix:

```sh
nix build          # -> result/bin/weave, with clang on its PATH
nix develop        # dev shell: go, gopls, staticcheck, clang
nix flake check    # builds the compiler and runs the tests
```

## Layout

```
cmd/weave/        the CLI
internal/token/   token kinds and source positions
internal/lexer/   scanner, including the indentation layout algorithm
internal/ast/     syntax tree, s-expression dumper, free-variable analysis
internal/parser/  recursive-descent parser
internal/diag/    diagnostics with source excerpts and hints
internal/types/   type representation, unification, Talents
internal/prelude/ the built-in vocabulary, as signatures
internal/check/   type inference and exhaustiveness checking
internal/codegen/ C emitter
internal/rt/      the C runtime, embedded into the compiler binary
internal/build/   the pipeline: check, emit, clang
tree-sitter-weave/ the tree-sitter grammar, for editors
weave.nvim/       the Neovim plugin: values as ghost text
examples/         programs that double as the regression suite
testdata/         input and expected output for each example
bench/            the Weave and Go benchmark pairs, and the runner
docs/             tutorial, vocabulary reference, performance, editor setup
```

Every example must parse, type-check, compile, and produce its expected
output: `internal/parser/examples_test.go` and `internal/build/build_test.go`
enforce it, so a change that breaks a realistic program fails the build.
