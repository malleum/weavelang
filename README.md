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
- **`_` for the argument you did not name.** `xs where (mod _ 2 | eq 0)`, and
  `web | get _ "a"` when the verb wants the collection first.
- **Real data structures.** `Web` and `Circle` are hash array mapped tries and
  `Taveren` is a leftist heap. A shortest path is one verb: give `dijkstra` the
  steps out of a place and it answers with the cost of reaching everywhere —
  see `examples/maze.weave`.
- **206 verbs**, named after [rask](https://github.com/malleum/rask) wherever
  there is no One-Power word for the job, so the two read alike. `weave verbs`
  prints them all, and [docs/verbs.md](docs/verbs.md) is the same list with
  types, generated from the compiler's own prelude.

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
| Web, Circle, Taveren | working |
| In-place updates, in a loop and in a fold | working |
| User-declared sum types | working |
| Endless `flow` sequences | working |
| `remember` memoisation | working |
| `Weaving`, `rescue`, `dijkstra` | working |
| Tree-sitter grammar, LSP, formatter, REPL | working |
| Tutorial, generated vocabulary reference | working |
| Several bare expressions per file | working |
| Language server inside Markdown code blocks | working |
| A flat hash table for immediate keys | working |
| A memo table of `remember`'s own | working |
| Reusing dead arena memory | working |
| Fusing `zip` and `items` into the loop that unpacks them | working |
| Releasing the Threads a call leaves behind | working |
| Reading a map back without a comparison sort | working |
| Fusing `zipwith` into the loop that consumes it | working |
| A `Held` of a Power is not a box | working |
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

1 of 206 built-ins
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
types and built-in documentation, completion over the 206 built-ins and your
own definitions with their signatures, signature help while applying a verb,
and formatting.

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
is the real one and not the sample. It builds on `weave trace`, which reports
one record per definition and is dull enough to drive from anything.

[docs/nixvim.md](docs/nixvim.md) wires all four pieces up in about thirty
lines.

### Formatting

`weave fmt` throws the layout away and prints the syntax tree, so redundant
parentheses, stray blank lines and inconsistent indentation cannot survive —
there is no code path that would emit them. Comments are kept, and a call
written as a pipeline comes back as one.

It has one setting, and it is about spelling. Weave gives its punctuation words
— `is` for `=`, `gives` for `:`, `through` for `|` — and the words are the
language while the symbols are the shorthand, so the words are what it prints:

```
nums = Source|lines|bend(earth)   ->   nums is Source through lines through bend earth
```

`weave fmt -terse` prints the symbols instead. They are the same tokens, so
either spelling reads back as the other and neither is more correct — it is a
question of whether you would rather read or squint.

A pipeline past 80 columns is broken one stage per line. `-check` exits
non-zero when a file is not already formatted, which is what `make check` runs
over the examples.

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
on each run. Set `WEAVE_CACHE` to move it, or to empty to turn it off.

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

`weave build -no-fuse` turns it off, and the test suite compiles 34 pipeline
shapes both ways and requires identical output.

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

The proof is deliberately narrow. Bind the collection to another name, put it in
a Twine, capture it in a lambda, or take its `cells`/`keys` — which share the
storage — and it copies. `weave build -no-in-place` turns it off, and the
differential suite runs every aliasing hazard both ways.

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
| [docs/verbs.md](docs/verbs.md) | all 206 verbs with types, generated from the prelude |
| [docs/performance.md](docs/performance.md) | Weave against Go, raw and on Advent of Code 2024 |
| [docs/nixvim.md](docs/nixvim.md) | grammar, LSP, formatter and plugin in one nixvim block |
| [SPEC.md](SPEC.md) | the language design, and why each piece is the way it is |

The reference is generated by `make docs`, and a test fails if it is stale, so
a verb cannot be added without being documented. Every ` ```weave ` block in
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
