# Weave — Language Spec (v0.2)

Weave is a strict, functional, statically-typed language for solving Advent of
Code with minimal syntax and One-Power-themed vocabulary. It compiles to C
(`clang -O3`) with reference-counted memory and opportunistic in-place mutation.

> Status: **implemented**, except where noted. The compiler in this repository
> parses, type-checks and compiles this language to native code. Sections
> describing optimisations not yet performed say so explicitly.

---

## 1. Philosophy

- **Immutable to you, mutable underneath.** You never mutate; the compiler does,
  in place, when it proves it's safe (§13).
- **Minimal symbols, maximal words.** The symbols you type often are `|`, `is`,
  and `:`. There are **no arithmetic operators** — arithmetic is named verbs
  (`add`, `mul`, …). Everything structural is a One-Power word.
- **Input is a variable, output is the last expression.** `Source` is the input;
  a program's final top-level expression is its output.
- **No null.** Absence is `Stilled`, the empty half of `Held a | Stilled`, and
  the compiler forces you to handle it.
- **No loops.** Tail-recursion (lowered to real loops, §13.1) and sequence verbs
  (`bend`, `sift`, `braid`, `seek`) replace them.

## 2. Design decisions (fixed)

| Concern | Choice |
|---|---|
| Compiler language | Go |
| Backend | emit C, compile with `clang -O3` (swappable to LLVM-IR text later) |
| Memory | automatic reference counting + in-place reuse (FBIP, §13) |
| Evaluation | strict; `Thread` is lazy and fuses (§5.1) |
| Types | static, inferred (Hindley–Milner, level-based generalisation), sum types, exhaustive matching |
| File extension | `.weave` |
| Modules | none (single-file programs) |

## 3. Lexical

- **Comments:** `# to end of line`.
- **Identifiers:** `letter (letter | digit)*`. No apostrophes — One-Power terms
  drop theirs (`taveren`, `angreal`). lowercase = value/function; Uppercase =
  type or constructor.
- **The entire symbol set:** `is = | : :: -> , [ ] { } ( ) _ " ' #`
  (`->` appears only inside `::` type signatures.)
- **The punctuation has a word spelling**, and the words are what the language
  is for: `is` for `=`, `gives` for `:`, `through` for `|`, `this` for `_`. They
  are the same tokens, so a line ending in `gives` opens a block exactly as one
  ending in `:` does. **`weave fmt` prints the words**; `weave fmt -terse`
  prints the symbols, and either can be read back. Each word was chosen because
  it could never be a verb, which is the rule that had `of`, `at` and `from`
  cut.

```weave
[1 2 3] | bend (x gives mul x x) | sum
```
- **Keywords:** `weave channel ward into where as through remember` + type and
  constructor names. `pick` and `flow` are ordinary identifiers bound to
  builtins, so they stay usable as pipeline stages and arguments.
- **`is` and `=` are interchangeable** as the binder; `is` is idiomatic.

### Layout

Indentation opens a block only after something that wants one: a line ending in
`is`, a ward arm ending in `:`, or the `ward` line whose arms follow. Anywhere
else a deeper line **continues** the line above, so an application can span
lines:

```weave
pick (member seen k)
  (walk rest seen)
  (walk (push rest k) (join seen k))
```

A line that opens with `|`, `where`, `as` or `through` continues the line above
too, so a long pipeline can breathe:

```weave
nums is
  Source
    | lines
    | bend earth
    where gt 10
```

### Literals

| Type | Literal | Examples |
|---|---|---|
| Earth (Int) | digits, `_` groups | `42`, `-7`, `1_000_000` |
| Water (Float) | decimal | `3.14`, `1.0`, `-0.5` |
| Fire (Char) | single-quoted | `'#'`, `'a'`, `'\n'` |
| Air (Text) | double-quoted | `"hello"`, `""` |
| Spirit (Bool) | keywords | `Light` (true), `Shadow` (false) |
| Thread | brackets, space- or comma-sep | `[1 2 3]`, `[]`, `[Step North 3, Rest]` |
| Twine | parens, comma-sep | `(1, "a")`, `(x, y)` |
| Web | braces, `k : v` | `{"a" : 1  "b" : 2}` |
| Knot | constructor | `knot 2 3` |

A Thread literal reads one of two ways. **Without a comma**, elements are
separated by spaces and each is an **atom**, so `[a b]` is unambiguously two
elements and an application must be parenthesised: `[(f x) (g y)]`. **With a
comma anywhere at the top level**, elements are full expressions and commas
separate them: `[Step North 3, Rest]` is two. `weave fmt` picks the first form
when every element is an atom and the second otherwise.

Inside `{ }` each key and value is an atom, as in the space-separated form.

## 4. The Five Powers (primitive types)

`Earth`=Int, `Water`=Float, `Fire`=Char, `Air`=Text, `Spirit`=Bool.

A number literal takes its Power from the definition around it. `1` in a
function over Waters *is* a Water, so a Water factorial reads the way an Earth
one does:

```weave
fact 1.0 is 1.0
fact n is mul n (fact (sub n 1))

fact 5.0
```
```weave
120.0
```

A literal that nothing decides is an Earth. That settling happens per
definition, before it is generalised, so a definition mentioning a literal is
monomorphic in that literal's Power — `half x is div x 2` is `Earth -> Earth`,
and a signature is how you say otherwise:

```weave
half :: Water -> Water
half x is div x 2

half 9.0
```
```weave
4.5
```

The reason is the compilation model rather than the type system: a value of
type `a where Reckon a` has no representation to emit, and one C function
cannot hold both an Earth constant and a Water one.

A Water prints as the shortest text that reads back as the same number, and
always with a decimal point, so `1.0` never prints as `1`.

All are **unboxed** (raw `i64`/`f64`/`i32`/pointer/`i1`) — never heap-allocated.
`Light` and `Shadow` are the two `Spirit` values (no `true`/`false`).

## 5. Built-in types

### 5.1 `Thread a` — the sequence
The core iterable. Verb chains (`bend | sift | seek | …`) **fuse** into a single
pass with no intermediate Thread at all; a terminal verb (`braid`, `count`,
`len`, `sum`, `seek`) ends it, and one that can answer early — `seek`, `first`,
`any`, `all` — stops the whole chain there.

A Thread is a strict vector, so `flow` is not one you can hold.

### 5.1.0 Indexing, membership and editing
A Thread is a strict array, so position is a load: `nth i xs` answers with a
`Hold` because most indices are out of bounds most of the time, and `has x xs`
tests membership. `idx x xs` is the position of the first match.

`weld ys xs` is xs with ys on the end, which is also how you append one
(`weld [x] xs`) and prepend one (`weld xs [x]`). `mend i x xs` replaces a
position, leaving the Thread alone when there is no such position, the way `set`
does for a grid. `sever n xs` cuts in two and hands back both halves. None of
them changes the Thread you gave: a Thread is a value, and the compiler decides
separately whether the old one is still needed (§13).

### 5.1.1 `flow` and `cycle` — the endless Threads
`flow f seed` is `seed, f seed, f (f seed), …`, and `cycle xs` is `xs` over and
over. Neither ends. Neither is ever built: the loop that consumes one holds a
single element at a time, so it must be created and consumed in the same
pipeline, and something must stop it — `take n`, `takewhile p`, `seek p`,
`first`, `any p` or `all p`.

```weave
flow (mul 2) 1 | seek (gt 1000)          # Held 1024
cycle [1 2 3] | take 7 | sum             # 13, the wrap-around for free
flow next 27 | takewhile (neq 1) | len   # the Collatz chain from 27
flow ((a, b) : (b, add a b)) (0, 1)
  | bend fst
  | take 10                              # the first ten Fibonacci numbers
```

Binding a flow to a name, or ending a chain with something that must see every
element, is a compile error rather than a program that runs forever.

### 5.2 `Pattern a` — the grid
2D indexed collection (rows × cols), of anything — `pattern text` reads a grid
of Fires, and `weft fill rows` weaves one out of rows of whatever you have, so
`Source | lines | bend earths | weft 0` is a Pattern of Earths. `cell`, `set`,
`cellwise`, `knots`,
`cells`, `rows`, `cols`, `shape`, `inb`; `nb4`/`nb8` for neighbouring cells and
`around4`/`around8` for their knots. Built from text via `pattern Source`.

### 5.3 `Web k v` — the map/dict
Associations, as a hash array mapped trie: 32-way branching with path copying,
so an insert touches about four nodes for a million entries and building a Web
inside a fold stays linear. A Web the compiler has proved unshared (§13) whose
keys are immediates — `Earth`, `Knot`, `Fire`, `Spirit` — is instead a flat
open-addressed table, which is the same map with none of the trie's cost; it
becomes a trie again the moment it is used persistently. `get`, `put`, `known`,
`forget`, `keys`, `vals`, `items`, `merge`, `mapvals`, `freq`, `most`.

`keys`, `vals` and `items` come back in ascending key order. A map has no order
of its own, and giving it one costs a sort but means a program's output depends
on what it put in rather than on how the runtime happened to store it.

### 5.4 `Circle a` — the set
Unique members, sharing the Web's storage — both representations and the
ordering guarantee. `circle`, `member`, `insert`, `remove`, `members`, `union`,
`inter`, `diff`.

### 5.5 `Taveren a` — the priority queue (min-heap)
A leftist heap: `push` and `pop` are both O(log n) merges, and nothing is
copied. `pop` → `Hold (a, Taveren a)`. Ordered by `Ord` (§11). For Dijkstra
and cost-BFS.

### 5.6 `Knot` — a grid coordinate
`knot row col`; accessors `row` and `col`, and `mdist` for the distance between two.

### 5.7 `Hold a` — Option, `Held a | Stilled`
Replaces null. `Held` = holding a value; `Stilled` = severed, nothing here.
`otherwise d` unwraps with default; `holds` tests presence. The *type* is
`Hold a`; `Held` and `Stilled` are its two constructors.

### 5.8 `Weaving a e` — Result, `Woven a | Gentled e`
`Woven` = success; `Gentled e` = failed, severed with a reason. `rescue d`
unwraps with a default, and a `ward` over the two cases is checked for
exhaustiveness like any other sum type. Success sorts before failure, since
`Woven` is declared first.

```weave
divide _ 0 is Gentled "divide by zero"
divide a b is Woven (div a b)

divide 10 0 | rescue 0        # 0
```

### 5.8.1 Taking a Thread apart in a pattern

`[a b]` matches a Thread of exactly two and binds both; `[x ..rest]` matches
one of at least one and binds the remainder, which is a slice of the same
storage and so costs nothing to make.

```weave
total [] is 0
total [x ..rest] is add x (total rest)

pair [a b] is add a b
pair _ is 0

[total [1 2 3 4], pair [3 4], pair [1 2 3]] | bend air | join " "
```
```
10 7 0
```

A Thread's length is not drawn from a finite set, so a list of fixed-length
patterns is never exhaustive and always needs a `_`. `[]` together with
`[x ..rest]` is the exception: between them they name every shape there is,
which the checker works out from the patterns rather than from the type.

### 5.9 `Twine` — `(a, b)`, `(a, b, c)`
Several things wound into one, the way a twine is several strands. Anonymous
and positional: written `(a, b)`, destructured `(x, y) : …`. `zip`, `items`,
`enum` and `pairs` all hand you one.

### 5.10 Declared sum types
A line beginning with an upper-case name declares a type. `is` separates the
head from the alternatives, and `|` reads as "or":

```weave
Direction is North | South | East | West
Move is Step Direction Earth | Rest
Tree a is Leaf | Node (Tree a) a (Tree a)
```

- **Parameters** are lower-case names in the head. A field mentioning a name
  the head does not bind is an error.
- **Fields are type atoms.** An applied type needs brackets — `Node (Tree a) a`
  is two fields, not one.
- **Recursion**, including between declarations, is allowed and needs no
  forward declaration; the checker reads all heads before any fields.
- **Talents are derived**: a declared type has `Eq`, `Ord` and `Show` whenever
  its fields do, so its values sort, print, and serve as `Web` keys with
  nothing written down. `Ord` follows declaration order first and then the
  fields, so `North` sorts before `South`. `Reckon` and `Bulk` are never
  derived: adding two Directions has no meaning.
- **Exhaustiveness** applies exactly as it does to `Hold`: a `ward` or a
  multi-clause head that misses `West` names it.

Longer declarations wrap one alternative per line, which is what `weave fmt`
produces past 80 columns:

```weave
Shape is
  Circle Water
  | Rect Water Water
  | Dot
```

At runtime a value is a constructor index plus its fields. A constructor that
carries nothing is built once and shared, so `North` costs nothing to mention.

## 6. Bindings & functions

### 6.1 Top level — bare, no keyword
The presence of parameters distinguishes a value from a function:
```weave
answer is 42                  # value
square n is mul n n           # function of one arg
```

### 6.2 Multi-clause heads + destructuring (recommended over Clojure-style)
Repeat the name with different patterns; the compiler checks exhaustiveness (or
requires a `_` clause):
```weave
fib 0 is 0
fib 1 is 1
fib n is add (fib (sub n 1)) (fib (sub n 2))

dist (knot r1 c1) (knot r2 c2) is
  add (abs (sub r1 r2)) (abs (sub c1 c2))
```

### 6.3 Local bindings — `weave` (values) and `channel` (functions)
Inside a body, locals are introduced with `weave` / `channel`; the final,
un-`is`'d expression is the return value. (This is where these two words live
now that top level is bare.)
```weave
busy g k is
  weave here is at g k
  ward here
    Held '#' : gte 2 (count (eq '#') (nb8 g k))
    _        : Shadow

solve is
  weave nums is Source | lines | bend earth
  channel big n is gt 100 n
  nums | sift big | sum
```
A `channel` may call itself, so a helper that recurses does not have to be
lifted to the top level:

```weave
total xs is
  channel go acc ys is
    ward ys
      [] : acc
      [x ..rest] : go (add acc x) rest
  go 0 xs

total [1 2 3 4]
```
```weave
10
```

Inline let-expression form uses `into`:
```weave
weave a is 1, b is 2 into add a b
```

## 7. Application, currying, precedence

- Application is juxtaposition: `f a b` == `((f a) b)`. Space binds tighter than
  everything except grouping parens.
- **Currying / partial application:** `gt 10 : Earth -> Spirit`.
- **Data-last convention:** every collection verb takes its data as the *last*
  argument, so partial application + pipe compose (see §8).
- Parentheses group only: `bend (mul 2) xs`.
- Precedence, loosest → tightest: `|` < `is`/`into` binding bodies < application.

## 8. The pipeline `|`

`x | f | g` == `g (f x)` — the value is fed as the **last** argument of each
stage. This is canonical for `map | filter | seek`:
```weave
Source | lines | bend earth | sift (gt 10) | seek (divBy 3) | otherwise 0
```

## 9. Pattern matching — `ward`

```
ward EXPR
  PATTERN : EXPR
  PATTERN : EXPR
```
- **Exhaustive:** a `ward` (or multi-clause head) missing a variant — including
  `Stilled` — is a compile error. `_` is the wildcard.
- **Patterns:** literals (`0`, `'#'`, `"x"`, `Light`), constructors with binders
  (`Held n`, `Woven x`, `Gentled e`), Twine and Knot destructuring (`(x, y)`,
  `knot r c`), and `_`.
- The arrow is a single `:`.
- A `ward` **owns an indented block**, so it cannot sit inside brackets (layout
  is suspended there). To use one mid-expression, bind it first:
  `weave r is ward x ... into f r`.

## 10. Expressions

### 10.1 Lambdas
`args : body`, arrow is `:`:
```weave
Source | lines | bend (line : len line)
xs | braid (acc x : add acc x) 0
```

### 10.1a `_` / `this` — the argument you did not name
`_` in expression position stands for one argument, so a one-off function needs
no parameter name. It has a word spelling, `this`, which is the same token —
`weave fmt` prints the word and `weave fmt -terse` the symbol. **A `_` is
claimed by the brackets closest to it**, or by the pipeline stage it sits in:

```weave
xs where (mod _ 2 | eq 0)      is  xs where (x : mod x 2 | eq 0)
xs | bend (mul _ _)            is  xs | bend (x : mul x x)
web | get _ "a"                is  get web "a"
sheet | cell _ k               is  cell sheet k
```

Two rules and no more:

1. **Brackets bind it.** Every `_` inside one bracket group is the same value,
   and a group holding one becomes a function of that value. `(add _ _)` is
   `(x : add x x)`, not a function of two.
2. **A pipeline stage binds it too**, to the value being piped, which is what
   lets the collection-first verbs (`get`, `cell`, `member`, `insert`) sit in a
   chain. The value is bound, not substituted, so it is evaluated once however
   many `_` name it.

Because the brackets are what bind it, nesting one call inside another splits
them up: `(eq 0 (mod _ 3))` claims `_` for the *inner* brackets and is a type
error, not a different meaning. Pipe instead — `(mod _ 3 | eq 0)` — which is
also shorter. A `_` with nothing to claim it, or one inside a group that
already names its parameter, is an error.

In a pattern `_` remains the wildcard; the two never meet, since one is an
expression and the other is a pattern.

### 10.1b `that` — the second half of a pair
`that` is the one word with no symbol. Writing it says the value arriving is a
two-part Twine that the group or stage wants opened, and binds both halves:
`this` the first, `that` the second, whichever order they are written in.

```weave
pairs | bend (add this that)   is  pairs | bend ((a, b) : add a b)
pairs where gt this that       is  pairs where ((a, b) : gt a b)
(7, 8) | sub this that         is  ward (7, 8) { (a, b) : sub a b }
```

The same two rules bind it: the closest brackets, or the pipeline stage. What
changes is only what the claim binds — one value without a `that` anywhere in
the group, both halves as soon as one appears. So `this` on its own is the
whole value, pair or not; it is the arrival of `that` that asks for the
opening. A `that` with nothing to claim it is an error, and no variable can be
named `that`, since it is a keyword.

### 10.2 `ward` as expression
Every `ward` yields a value (all arms same type).

### 10.3 `pick` — the functional ternary
`pick COND IF_LIGHT IF_SHADOW`, lazy (evaluates only the taken branch):
```weave
pick (gt x 10) "big" "small"
```

### 10.4 Result propagation (`?`)
> **Not implemented, and not certain to be.** The idea was that `EXPR?` would
> short-circuit a `Gentled e` out of the enclosing function and otherwise
> unwrap the `Woven x`. It is the one place the language would gain a
> non-local exit, which cuts against everything else here, and a `ward` over
> the two cases already reads well. Left out until a real program wants it.

## 11. Talents (minimal typeclasses)

Five built-in Talents cover AoC ergonomics. User-defined Talents are not a
thing: nothing in an Advent of Code program has wanted one, and adding them
would mean dictionary passing or specialisation for every constrained call.

| Talent | Provides | Default for |
|---|---|---|
| `Eq` | `eq`, `neq` | all Powers, Twine, Knot |
| `Ord` | `lt`, `lte`, `gt`, `gte`, `sort`, `Taveren` order | Earth, Water, Fire, Air |
| `Show` | display / output rendering | all built-ins |
| `Reckon` | `add`, `sub`, `mul`, `div`, `abs`, `neg`, `sum` | Earth, Water |
| `Bulk` | `len` | Air, Thread, Web, Circle, Taveren, Pattern |

`sort xs`, `eq a b`, and auto-output all dispatch through these; no comparator
needs to be passed for the common orderings. `Reckon` is what lets one `add`
serve both Earth and Water without operators or separate names.

Container types inherit a Talent from their contents: `Thread a` has `Eq` only
when `a` does, so `eq [1 2] [1 2]` is fine and `sort [Light Shadow]` is a type
error (`Spirit has no Ord Talent`).

Constraints show up in inferred types: `double n is add n n` reports
`double :: a -> a  where Reckon a`.

## 11.4 Taking a line apart

Most of Advent of Code wants the numbers and not the punctuation, and `earths`
answers that on its own: `"Game 11: 3 blue, 4 red" | earths` is `[11 3 4]`. When
the shape of the line *is* the point, `delve` says it:

```weave
"Game 11: 3 blue, 4 red" | delve "Game {}: {}"
```
```weave
Held [11 3 blue, 4 red]
```

`{}` keeps a run and everything else has to match exactly. A run stops at the
first place the text after it appears, and the shape has to account for the
whole line — a trailing `{}` is how you say "and the rest". A line that does
not have the shape you said is `Stilled` rather than a guess, which is what
makes `glean` the natural way to read a whole file:

```weave
["3-5", "not a range", "8-1"]
  | glean (l : delve "{}-{}" l)
  | bend (p : p | harvest earth | rescue [] | sum)
```
```weave
[8 9]
```

There is no regular expression engine, and there is not going to be one: it
would be the first part of the language with a sub-syntax of its own, and five
years of Advent of Code has not needed it. The one thing `delve` cannot say is
a literal `{}` in the input, which buys a shape with nothing to escape.

## 12. Source & output

- `Source : Air` — the raw program input. Shape it with `lines`, `fires`,
  `words`, `pattern`, `earth`, `delve`.
- **Output** is the final top-level expression, rendered via `Show`:
  Earth/Water/Air print bare; `Thread` prints one element per line; a `Hold`
  prints as `Held x` or `Stilled`, so an answer that might not be there says so.
  `say x` forces a `Show` to `Air`.
  A Thread's elements are separated by newlines and the whole output ends with
  one. A file may hold several bare expressions; `weave run` prints the last,
  and `weave trace` reports every one.

## 12.1 Primitive specialisation

Arithmetic and comparison verbs are polymorphic, so the runtime versions
dispatch on a value's tag. When inference has determined that a call's operands
are a primitive — Earth, Water, Fire or Spirit — the compiler emits the typed
operation instead, and the dispatch disappears. `eq` at Earth becomes an
integer comparison rather than a call into structural comparison.

This is invisible: the specialised and general forms compute the same answers,
and a definition whose type is still a variable keeps the general verb.

## 13.1 Tail calls

A definition that calls itself in tail position compiles to a jump, not a call:
its parameters become local variables and the body is wrapped in a loop, so
recursion costs what a loop costs and the C stack does not grow.

```weave
count 0 acc is acc
count n acc is count (sub n 1) (add acc n)

count 100000000 0
```

runs in about a third of a second and uses one stack frame. The arguments are
evaluated into temporaries before any parameter is rebound, since they normally
read the values they replace.

`pick` counts as tail position in both branches, because only the taken branch
runs.

**Mutual** tail recursion gets the same guarantee. Definitions that tail-call
each other form a cycle in the tail-call graph, and each cycle compiles into
one function whose loop switches on which member is running:

```weave
even 0 is Light
even n is odd (sub n 1)

odd 0 is Shadow
odd n is even (sub n 1)

even 50000000
```

runs in one frame. Every member keeps its own entry point, so an ordinary call
to one is exactly what it was — the alternative, a trampoline, would have taxed
every call in the program to pay for the few that needed it.

Two things are deliberately left out of a merged cycle. A `remember`ed
definition keeps its calls as calls, since a jump would step around the lookup;
and in-place grid updating is not attempted across a cycle, because proving a
grid is threaded without duplication would have to hold along every path
through every member rather than around one loop.

## 13. In-place mutation (FBIP) — why updates cost what Go costs

> **Implemented for a `Pattern`, `Web` or `Circle` threaded through a loop**,
> which is the case that matters. The general form — every container, decided
> by a reference count maintained everywhere — still needs the counting
> allocator; the runtime bump-allocates and does not free, so what is not
> written through is kept.

Everything is immutable *to you*. An update verb like `set sheet k v` returns "a
new grid," but the runtime checks `sheet`'s refcount:

- **Refcount 1** (last live reference — the old binding is dead after this call):
  mutate the buffer in place, hand back the same memory. **O(1).**
- **Shared** (something else still reads `sheet`): copy first, then mutate the
  copy. **O(n)**; the old grid stays valid for the other reader.

```weave
weave g2 is set g  k1 v1     # g unused below -> in-place
weave g3 is set g2 k2 v2     # g2 dead        -> in-place
g3                            # a chain of O(1) updates, as Go's g[k]=v is
```
Proving it takes two halves, because neither alone is enough. **Statically**,
that the loop never duplicates the collection: every mention of the parameter
must be either the update in the tail call or an argument to a verb that reads
without keeping a reference. Bind it to another name, put it in a Twine,
capture it in a lambda, or take its `cells` or `keys` — which share the storage
— and the analysis gives up. **Dynamically**, that it did not arrive already
shared: the first call usually hands over something the caller still holds, so
a collection is marked shared when it is built and owned only when it is the
copy the update itself just made.

A grid needs one ownership bit, because its cells are one block. A `Web` or
`Circle` is a trie whose nodes are allocated separately and shared between
versions, so the bit is **per node**: an insert writes through the owned prefix
of the path and copies from the first shared node downwards, marking the
children of anything it copies as shared. The first turn of a loop copies, later
turns mostly do not, and no node is ever copied twice however long the loop
runs.

`Taveren` and `Thread` buffers are not yet covered.

## 14. D-particle aliases (optional prose glue)

Canonical is C (pipelines + `ward`). These desugar to canonical — pure sugar,
no new semantics — so easy code can read like prose.

| Particle | Desugars to | Example |
|---|---|---|
| `where p` | `\| sift p` | `lines Source where has3digits` |
| `as f` | `\| bend f` | `lines Source as earth` |
| `through f` | `\| f` | `Source through lines` |
| `otherwise d` | Option default | (also canonical) |

Particles sit at exactly the same precedence as `|` and associate left to
right, so they interleave freely: `xs | bend f where p | len`.

The no-op glue words `of`, `at` and `from` were **cut**: a glue word that can
shadow a verb would silently rewrite working code, and readability glue is not
worth that.

## 15. Standard verbs (a sample; data-last)

The authoritative catalogue is `internal/prelude/prelude.go`, which the
compiler parses at start-up — the signatures there *are* these signatures, and
there are 206 of them. **[docs/verbs.md](docs/verbs.md) lists every one, with
its type and what it does**, generated from that same table by `make docs`; a
test fails if it falls out of date, so no verb can exist without appearing
there. `weave verbs [search]` prints the same reference at the terminal,
`weave repl` answers `:type name`, and the language server completes and
documents the lot.

What follows is the *shape* of the vocabulary rather than the whole of it: one
screen that shows how the pieces fit together, where the full reference is a
list. A test keeps every name below in step with the prelude.

```
# Sequence (Thread a)
bend    : (a -> b) -> Thread a -> Thread b            # map
sift    : (a -> Spirit) -> Thread a -> Thread a       # filter
braid   : (b -> a -> b) -> b -> Thread a -> b         # fold
seek    : (a -> Spirit) -> Thread a -> Hold a
span    : Earth -> Earth -> Thread Earth              # inclusive range
flow    : (a -> a) -> a -> Thread a                   # endless: seed, f seed, ...
len     : a -> Earth                                  # any Bulk type
count   : (a -> Spirit) -> Thread a -> Earth
sum prod : Thread a -> a                              # needs Reckon a
sums prods : Thread a -> Thread a                     # the running totals
take drop  : Earth -> Thread a -> Thread a
takewhile dropwhile : (a -> Spirit) -> Thread a -> Thread a
zip     : Thread a -> Thread b -> Thread (a, b)
zipwith : (a -> b -> c) -> Thread a -> Thread b -> Thread c
sort    : Thread a -> Thread a                        # needs Ord a
nth     : Earth -> Thread a -> Hold a                 # by position
has     : a -> Thread a -> Spirit                     # membership; needs Eq a
glean   : (a -> Hold b) -> Thread a -> Thread b       # bend, keeping the Held
harvest : (a -> Hold b) -> Thread a -> Weaving (Thread b) a   # or say which failed
cycle   : Thread a -> Thread a                        # endless; bound it
all any : (a -> Spirit) -> Thread a -> Spirit

# Building and editing one. A Thread is a value: none of these changes the one
# you gave it.
thread  : (a, a) -> Thread a                          # a pair as a Thread
weld    : Thread a -> Thread a -> Thread a            # weld ys xs: xs then ys
mend    : Earth -> a -> Thread a -> Thread a          # replace one position
sever   : Earth -> Thread a -> (Thread a, Thread a)   # cut in two
strands : (a -> b) -> Thread a -> Thread (Thread a)   # runs of adjacent equals
plait   : Thread a -> Thread a -> Thread a            # zip, flattened
cull    : (a -> Spirit) -> Thread a -> Thread a       # sift the other way round

# The same three verbs one level deeper, for a Thread of Threads. rask's
# equivalents descend to whatever depth the data has, which it can do because
# it is dynamically typed; here the type says how deep the data is, so these
# say exactly one level.
bendr : (a -> b) -> Thread (Thread a) -> Thread (Thread b)
siftr : (a -> Spirit) -> Thread (Thread a) -> Thread (Thread a)
zipr  : (a -> b -> c) -> Thread (Thread a) -> Thread (Thread b) -> Thread (Thread c)

# Text (Air)
lines : Air -> Thread Air
words : Air -> Thread Air
fires : Air -> Thread Fire
split : Air -> Air -> Thread Air                      # split sep text; "" is per character
earth : Air -> Hold Earth                             # also water, fire
air   : a -> Air                                      # render anything, needs Show
strip : Air -> Air
earths : Air -> Thread Earth                          # every Earth in some text
delve : Air -> Air -> Hold (Thread Air)               # delve shape text

# Grid (Pattern a)
pattern  : Air -> Pattern Fire
weft     : a -> Thread (Thread a) -> Pattern a        # a Pattern of anything
spin flip : Pattern a -> Pattern a                    # a quarter turn, a mirror
cell     : Pattern a -> Knot -> Hold a
set      : Pattern a -> Knot -> a -> Pattern a
knots    : Pattern a -> Thread Knot
cells    : Pattern a -> Thread a
cellwise : (a -> b) -> Pattern a -> Pattern b         # map, keeping the shape
nb4 nb8  : Pattern a -> Knot -> Thread a              # neighbouring cells
around4 around8 : Pattern a -> Knot -> Thread Knot    # neighbouring knots

# Web / Circle / Taveren / graphs
get   : Web k v -> k -> Hold v
put   : Web k v -> k -> v -> Web k v
freq  : Thread a -> Web a Earth                       # frequency count
push  : Taveren a -> a -> Taveren a
pop   : Taveren a -> Hold (a, Taveren a)

# A graph is a function from a place to the steps out of it — which fits an
# implicit graph, a Pattern or a state machine, as readily as one you built.
dijkstra : (a -> Thread (Earth, a)) -> a -> Web a Earth      where Ord a
reach    : (a -> Thread a) -> a -> Circle a                  where Eq a
route    : (a -> Thread (Earth, a)) -> a -> a -> Hold (Thread a)
toposort : (a -> Thread a) -> Thread a -> Hold (Thread a)     where Eq a

# Hold / Weaving
otherwise : a -> Hold a -> a
holds     : Hold a -> Spirit
rescue    : a -> Weaving a e -> a

# Numbers / logic (no operators — these are the verbs)
add sub mul div mod  : a -> a -> a                     # needs Reckon
abs neg              : a -> a
min max              : a -> a -> a                     # needs Ord
eq neq               : a -> a -> Spirit                # needs Eq
lt lte gt gte        : a -> a -> Spirit                # needs Ord
even odd             : Earth -> Spirit
divBy                : Earth -> Earth -> Spirit        # divBy d n  ==  n mod d == 0
and or               : Spirit -> Spirit -> Spirit
not                  : Spirit -> Spirit
```

Reading a value out of text is one verb per Power, named for the Power:
`earth :: Air -> Hold Earth`, and likewise `water` and `fire`. Each answers
`Stilled` when the text does not spell one, so a failed read is a value the
caller has to handle rather than an error. Going the other way is
`air :: a -> Air`, which needs `Show` and cannot fail. No verb in the language
takes a type as an argument.

Two argument conventions run side by side, and the difference is deliberate:
**sequence transforms are data-last** so they compose with the pipeline
(`xs | sift even`), while **keyed collections take the collection first** and
are called rather than piped (`get w k`, `cell g k`, `put w k v`). A grid or a
map is usually the fixed thing being consulted, not the thing flowing through.
`_` bridges the two when you want the second form in a chain: `w | get _ "a"`.

### Naming

A verb keeps a One-Power name when it has one — `bend`, `sift`, `braid`,
`flow`, `span`, `knot`, `circle`, `web`, `taveren`, and the `Hold`/`Weaving`
verbs. Everywhere else the name is the one
[rask](https://github.com/malleum/rask) uses, so the two languages read the
same: `len`, `prod`, `rev`, `flat`, `strip`, `idx`, `chunk`, `pivot`, `group`,
`freq`, `items`, `cell`, `nb4`, `nb8`, `r`, `c`, `join`, `insert`, `remove`.

Three of rask's names could not be taken. Weave has no operators, so `add`,
`sub` and the comparisons are arithmetic and cannot also be set insertion —
`insert` and `remove` carry rask's list-verb names for the Circle instead. And
`set` is the grid update, so building a Circle stays `circle`.

## 15.1 `remember` — memoisation

Manual, and deliberately so. A compiler can prove a Weave function is pure —
every function is — but purity is not the question. Whether a call repeats with
the same arguments depends on the call graph at run time, not on the source,
and a memo table is unbounded: it hashes every argument it ever sees and never
forgets one. Guessing wrong turns a fast function into a slow one that also
leaks.

So it is a marker on the definition:

```weave
remember fib 0 is 0
fib 1 is 1
fib n is add (fib (sub n 1)) (fib (sub n 2))
```

- It marks the **definition**, not one equation, so it goes once, on the first
  clause. Writing it on a later clause works and `weave fmt` moves it.
- Every argument must have the **Eq** Talent, since a remembered call is looked
  up by them. A function argument is therefore a compile error.
- A definition with **no arguments** is already computed once, the first time
  it is used, so the marker on one is an error rather than a no-op.
- The table is a `Web` from the arguments — a Twine of them, past the first —
  to the result, so it works on a `Knot`, a Twine, or a declared sum type with
  no further machinery. It is never pruned: that is the cost the marker asks
  you to accept.
- A remembered definition keeps its self-calls as calls rather than turning a
  tail call into a jump, since a jump would step around the lookup.

What the compiler does without being asked is the bounded version: constant
folding, and common subexpressions within one expression. Those need no table.

## 16. Worked example — AoC 2020 Day 1 (both parts)

```weave
# input: one Earth per line. part1: two entries summing to 2020, multiplied.
#        part2: three entries summing to 2020, multiplied.

nums is Source | lines | bend earth | bend (otherwise 0)

part1 is
  weave pair is
    nums | seek (a :
      nums | any (b : eq 2020 (add a b)))
  ward pair
    Held a  : mul a (sub 2020 a)
    Stilled : 0

part2 is
  weave hit is
    combos 3 nums | seek (t : eq 2020 (sum t))
  ward hit
    Held t  : prod t
    Stilled : 0

[part1, part2] | bend air | join "\n"
```
