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
  is for: `is` for `=`, `gives` for `:`, `through` for `|`, `this` for
  `_`. They are the same tokens, so a line ending in `gives` opens a block
  exactly as one ending in `:` does. **`weave fmt` prints the words**;
  `weave fmt -terse` prints the symbols, and either can be read back. Each word
  was chosen because it could never be a verb, which is the rule that had `of`,
  `at` and `from` cut.
- **Eight words have no symbol at all**: `that` names the second argument a
  bracket group can be handed, and `former`/`latter` and `fore`/`mid`/`aft` name
  the components of the first (§10.1a); `else` and `failing` are particles
  carrying a verb of their own (§14).

```weave
[1 2 3] | bend (x gives mul x x) | sum
```
- **Keywords:** `weave channel ward into where as through else failing
  remember` + the word spellings `is gives this that former latter fore mid aft`, and
  type and constructor names. `pick` and `flow` are ordinary identifiers bound to
  builtins, so they stay usable as pipeline stages and arguments.
- **`is` and `=` are interchangeable** as the binder; `is` is idiomatic.

### Layout

Indentation opens a block only after something that wants one: a line ending in
`is`, a ward arm ending in `:`, or the `ward` line whose arms follow. Anywhere
else a deeper line **continues** the line above, so an application can span
lines:

```weave-part
pick (member seen k)
  (walk rest seen)
  (walk (push rest k) (join seen k))
```

A line that opens with `|`, `where`, `as` or `through` continues the line above
too, so a long pipeline can breathe:

```weave-part
nums is
  Source
    | lines
    | glean earth
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
The core iterable. Verb chains **fuse** into a single pass with no intermediate
Thread at all. Exactly which verbs, since it decides what an endless chain may
be made of:

- **Producers** — a Thread, `span`, `zip`, `zipwith`, `items`, `enum`,
  `couples`, the grid walks `knots`, `nb4`, `nb8`, `around4` and `around8`, and
  the two endless ones, `flow` and `cycle`.
- **Stages** — `bend`, `sift`, `cull`, `take`, `takewhile`, `drop`,
  `dropwhile`, and `scan`, which is a `bend` carrying an accumulator.
- **Consumers** — `braid`, `sum`, `prod`, `len`, `count`, and the ones that can
  answer early: `seek`, `first`, `any`, `all`, `dupe`, `gentle`.

Anything else in a chain ends the fusion and builds the Thread. An early
answer stops the whole chain where it is, which is what lets `flow` and `cycle`
be consumed at all (§5.1.1).

A chain fuses when there is something to save, and there are three ways for
that to be true. Two stages save the Thread between them. A **generated**
producer — a `span`, or one of the grid walks — saves the array it would have
built, so one stage is enough and a bare consumer is enough too: `nb8 g k |
count (eq '#')` builds no eight-element Thread per cell. And a **lambda written
on the spot** saves the closure: the loop inlines it, so `xs | bend (x : mul x
x)` fuses where `xs | bend twice` does not, because there the runtime verb was
going to call the function either way.

`nb4` and `nb8` are the one exception, and only under a fold: the accumulator
may be the very Pattern being read, and writing through it (§13) would then be
seen by the rest of the walk. The verb copies its four or eight values out
first, so it keeps the meaning the fused loop could not.

A Thread is a strict vector, so `flow` is not one you can hold.

**What a fused `gentle` costs.** A `gentle` is Weave's loop, and the loop it
compiles to builds nothing per turn. Its step is compiled in *statement*
position — `Woven x` assigns the accumulator, `Gentled y` assigns the answer and
sets a flag — so the Weaving the loop would otherwise build and take straight
back apart on the same turn never exists; it is put together once when the loop
ends, because `gentle` answers with one and `failing` reads which case it was.
An accumulator that is a Twine of state is carried one component to a variable,
for the same reason and with the same shape: the step takes it apart on the way
in and writes it out on the way out, so the Twine between those two is a thing
nobody ever looks at. The trampoline walk of Advent of Code 2017 day 5 went from
2.9 GB and four allocations a turn to **51.7 KB and nine allocations for the
whole program**.

Two limits, stated plainly because a step outside them compiles exactly as it
did: a step ending in a `ward` is not split, and neither is one handing on a
Twine it did not write out on the spot.

**What a fused fold hands back.** A turn of a fold whose whole storage is dead
when the turn ends gives it back: the arena is a bump pointer, so the loop marks
where it has got to, lets the turn allocate what it likes, and puts the pointer
back. That is the only thing that helps a *backtracking* search, which uses the
collection it was handed once per **option** — the old value has to survive for
the next branch, so it is genuinely not single-threaded and §13 will never bless
it. Advent of Code 2025 day 10 went from 893 ms and 297 MB to **98 ms and 7 MB**.

The condition is that nothing allocated during a turn may be reachable after it,
and four things have to hold for that: the accumulator is an unboxed Power, so
what deliberately outlives the turn carries no pointer into it; the elements
likewise; no stage holds state across turns, since `scan` and `dupe` keep
exactly what a release would take; and nothing reachable stores into a global,
of which a program has two — a `remember` table, which rules the loop out, and
the memoised accessor a top-level value compiles to, which is forced before the
region opens. Anything that cannot be shown to satisfy all four is compiled as
it was. `weave build -no-regions` turns it off.

### 5.1.0 Indexing, membership and editing
`span lo hi` names both ends and includes both, because an input that writes a
range writes `11-22`. When what you want is the *places* of n things rather than
a range between two numbers, that is `under n` — zero up to but not including
n — and `copies n x` is a Thread of the same value that many times, which is
`repeat` for a Thread rather than for text.

A Thread is a strict array, so position is a load: `nth i xs` answers with a
`Hold` because most indices are out of bounds most of the time, and `has x xs`
tests membership. `idx x xs` is the position of the first match.

`weld ys xs` is xs with ys on the end, which is also how you append one
(`weld [x] xs`) and prepend one (`weld xs [x]`). `mend i x xs` replaces a
position, leaving the Thread alone when there is no such position, the way `set`
does for a grid, and `twist i f xs` is `mend` when the new value is worked out
from the old one. `sever n xs` cuts in two and hands back both halves. None of
them changes the Thread you gave: a Thread is a value, and the compiler decides
separately whether the old one is still needed (§13).

Asking *where* rather than *what* has both halves throughout. `idx x xs` is the
first position a value lies at and `idxs x xs` every one; `seekidx p xs` is the
first position passing a test and `siftidx p xs` every one; `highidx` and
`lowidx` are where `high` and `low` found what they found, which asking `idx`
afterwards would answer wrongly when the value repeats.

### 5.1.1 `flow` and `cycle` — the endless Threads
`flow f seed` is `seed, f seed, f (f seed), …`, and `cycle xs` is `xs` over and
over. Neither ends. Neither is ever built: the loop that consumes one holds a
single element at a time, so it must be created and consumed in the same
pipeline, and something must stop it — `take n`, `takewhile p`, `seek p`,
`first`, `any p`, `all p`, `dupe` or `gentle`.

```weave-part
flow (mul 2) 1 | seek (gt 1000)          # Held 1024
cycle [1 2 3] | take 7 | sum             # 13, the wrap-around for free
cycle [1 2 3] | scan add 0 | dupe        # the first running total seen twice
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
`around4`/`around8` for their knots; `sited`/`sites` for where a value is,
which is `cell` asked the other way round. Built from text via `pattern Source`.

`weft fill rows` weaves rows you already have. `warp f rows cols` is the other
constructor: given the shape and a function from a knot to what belongs there,
it lays a grid out before anything is on it — a board, a distance table, a mask.
Written by hand that is a `span` inside a `span` inside a `weft`, plus a fill
value nothing will ever read.

`tallies g` is the running total over a grid — every cell replaced by the total
of the box from the top left corner to it — and `tallied t a b` reads a box back
out of one. Asked once, every "how much is inside this box" afterwards is a
single subtraction whatever the box's size. Three of the four corners sit one
row or one column *before* the box, which is what a hand-written version has to
pad for; `tallied` owns them, so nothing needs a border of zeroes, the knots may
be given either way round, and a box running off the grid is clipped rather than
wrong.

```weave
g is [[1, 2, 3], [4, 5, 6], [7, 8, 9]] | weft 0

t is tallies g

[tallied t (knot 1 1) (knot 2 2), tallied t (knot 0 0) (knot 2 2)]
  | bend air
  | join " "
```
```
28 45
```

### 5.3 `Web k v` — the map/dict
Associations, as a hash array mapped trie: 32-way branching with path copying,
so an insert touches about four nodes for a million entries and building a Web
inside a fold stays linear. A Web the compiler has proved unshared (§13) whose
keys are immediates — `Earth`, `Knot`, `Fire`, `Spirit` — is instead a flat
open-addressed table, which is the same map with none of the trie's cost; it
becomes a trie again the moment it is used persistently. When the values are
unboxed as well, that table packs a slot down to two raw payloads with one
shared tag per column — sixteen bytes rather than thirty-two, and a probe that
is one `int64` compare — and widens back to tagged slots the moment anything
disagrees. `get`, `put`, `known`,
`forget`, `keys`, `vals`, `items`, `merge`, `mapvals`, `freq`, `most`.

`keys`, `vals` and `items` come back in ascending key order. A map has no order
of its own, and giving it one costs a sort but means a program's output depends
on what it put in rather than on how the runtime happened to store it.

### 5.4 `Circle a` — the set
Unique members, sharing the Web's storage — both representations and the
ordering guarantee. `circle`, `member`, `insert`, `remove`, `members`, `union`,
`inter`, `diff`, and `covers outer inner` for containment — the question the
other three left unasked, and `within`'s counterpart for a range.

### 5.5 `Taveren a` — the priority queue (min-heap)
A leftist heap: `push` and `pop` are both O(log n) merges, and nothing is
copied. `pop` → `Hold (a, Taveren a)`. Ordered by `Ord` (§11). For Dijkstra
and cost-BFS.

### 5.5a `Link a` — who is joined to whom
Disjoint sets over the nodes it was built from. `link xs` puts every node in a
circle of its own; `bind l a b` joins the circles holding two of them; `bound l
a b` asks whether two are together yet; `clumped l` hands back the circles, each
once, in the order their first member was given.

It is the one graph question `clumps` cannot answer. `clumps` is asked once, of
a finished graph. A Link is asked *while* the joining happens — which is what
Kruskal's algorithm needs, and every minimum-spanning-tree puzzle with it.

```weave
l is link [1, 2, 3, 4, 5]

joined is bind (bind l 1 2) 4 5

[air (bound joined 1 2), air (bound joined 1 3), air (clumped joined | bend len)]
  | join " "
```
```
Light Shadow [2 1 2]
```

Binding hands back a new Link and leaves the old one alone, like everything
else here. Which values exist never changes, so a bind copies only the two
small arrays that say who answers to whom — eight bytes a node — and a Link
threaded single-threadedly through a loop (§13) copies nothing at all. A node
the Link was not built with does not exist: binding it does nothing, and it is
bound only to itself.

### 5.6 `Knot` — a grid coordinate
`knot row col`; accessors `row` and `col`, and `mdist` for the distance between two.

### 5.7 `Hold a` — Option, `Held a | Stilled`
Replaces null. `Held` = holding a value; `Stilled` = severed, nothing here.
`otherwise d` unwraps with default — `else d` is the same thing as a particle
(§14) — and `holds` tests presence. The *type* is `Hold a`; `Held` and
`Stilled` are its two constructors.

A `Held` costs nothing to build. A Value is a tag, four bytes that used to be
padding, and eight bytes of payload, and a `Held` puts the inner tag in the
spare four and keeps the value where it stands — for *anything*, bar a `Held` of
a `Held`, which is the one case that needs a box. So `cell g k | otherwise 0`,
which is most of how a program reads a grid, allocates nothing at all. The empty
Thread costs nothing either: `else []` is how every program says "nothing was
there", and it is one object for the whole program rather than a fresh header
holding nothing each time it is said.

### 5.8 `Weaving a e` — Result, `Woven a | Gentled e`
`Woven` = success; `Gentled e` = failed, severed with a reason. A `ward` over
the two cases is checked for exhaustiveness like any other sum type. Success
sorts before failure, since `Woven` is declared first.

Both sides come out with a default: `rescue d` takes the `Woven` value, and
`snag d` takes the other — what the weaving stopped on. `failing d` is `snag`
as a particle.

```weave
divide _ 0 is Gentled "divide by zero"
divide a b is Woven (div a b)

divide 10 0 | rescue 0        # 0
divide 10 0 failing "fine"    # "divide by zero"
```

`gentle` is the fold built on this. It is `braid` that may stop: the step
answers `Woven acc` to carry on or `Gentled answer` to end the fold there, and
the fold answers whichever it ended on. It short-circuits, so it can consume an
endless chain (§5.1.1), and `snag` is how the answer comes back out.

```weave
deltas is [1, neg 2, 3, 1]

deltas
  through cycle
  through scan add 0
  through gentle (s n gives pick (member s n) (Gentled n) (Woven (insert s n))) (circle [0])
  failing 0
```
```
2
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
```weave-part
busy g k is
  weave here is at g k
  ward here
    Held '#' : gte 2 (count (eq '#') (nb8 g k))
    _        : Shadow

solve is
  weave nums is Source | lines | glean earth
  channel big n is gt 100 n
  nums | sift big | sum
```
**A binding may take its value apart**, the way a parameter already could:

```weave
f p is
  weave (a, b) is p
  add a b

(width, height) is (7, 3)

[f (1, 2), mul width height]
```

A bare name is still a name — `weave x is 1` binds and does not match — so the
shapes that count are the bracketed ones and `_`, which is what anyone reaches
for. One pattern and no alternative means it has to cover everything the value
could be, so a refutable one is the same soft diagnostic a one-armed `ward`
gets: it compiles, and traps if the value does not match.

The top-level form expands, between parsing and checking, into a hidden
definition holding the whole value and one projection per name. A top-level
value is a memoised accessor, so the expression runs once however many names
read it, the dependency order falls out of the free variables, and the generated
`ward` carries the exhaustiveness check.

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
```weave-part
Source | lines | glean earth | sift (gt 10) | seek (divBy 3) | otherwise 0
```

Four of the commonest stages have a particle of their own, and the same chain
written with them is §14's business. `weave fmt` chooses between the two
spellings by style rather than by what was typed, so a program comes back
consistent either way:
```weave-part
Source through lines through glean earth where gt 10 through seek (divBy 3) else 0
```

## 9. Pattern matching — `ward`

The arms go in an indented block, or bracketed on the `ward` line. The two are
the same ward; which reads better is a question of length, and `weave fmt`
keeps whichever was written for as long as it fits.

```
ward EXPR                       ward EXPR (PATTERN : EXPR) (PATTERN : EXPR)
  PATTERN : EXPR
  PATTERN : EXPR
```
- **Exhaustive:** a `ward` (or multi-clause head) missing a variant — including
  `Stilled` — is a compile error. `_` is the wildcard.
- **Patterns:** literals (`0`, `'#'`, `"x"`, `Light`), constructors with binders
  (`Held n`, `Woven x`, `Gentled e`), Twine and Knot destructuring (`(x, y)`,
  `knot r c`), and `_`.
- The arrow is a single `:`.
- The **block form owns its indentation**, so it cannot sit inside brackets —
  layout is suspended there. The bracketed form can, and is the only one an
  expression has room for: `xs | bend (n gives ward (even n) (Light : n) (Shadow : 0))`.
- A bracketed arm and a lambda are written identically, which is not a
  coincidence: `(x : e)` means the same thing read either way. Position is what
  separates them — inside a ward's head a bracketed group holding a `:` is an
  arm, and everywhere else it is a lambda.

## 10. Expressions

### 10.1 Lambdas
`args : body`, arrow is `:`:
```weave-part
Source | lines | bend (line : len line)
xs | braid (acc x : add acc x) 0
```

### 10.1a The hole words — the arguments you did not name

A one-off function needs no parameter names. Seven words stand for what the
enclosing bracket group is handed, and they answer two different questions —
*which argument*, and *which component of the first one*:

| written | means |
|---|---|
| `_`  `this` | the first argument |
| `that` | the second argument |
| `former`  `latter` | the two halves of a Twine of **two** |
| `fore`  `mid`  `aft` | the three parts of a Twine of **three** |

`_` and `this` are one token — the symbol and its word. `weave fmt` prints
`this` and `weave fmt -terse` prints `_`. The rest have no symbol; `weave fmt`
cannot shorten them and does not try.

**A component word carries its width as well as its position**, and that is the
point rather than an accident. `former` and `latter` are what English calls the
parts of a pair, and the only width it calls them at; `fore`, `mid` and `aft`
are the parts of a three. So `(former)` on its own says both which component and
how many there are, and a group holding one can be read where it stands rather
than after its type is known — which is what makes these work the same in a
pipeline stage, in brackets, and as a function's argument.

One group may not ask for both widths: `add former aft` says two and three in
the same breath and is refused. Nothing names a component of a Twine of four; at
that width a pattern says it more clearly than a word would.

| written with holes | the same thing named |
|---|---|
| `xs where (mod _ 2 \| eq 0)` | `xs where (x : mod x 2 \| eq 0)` |
| `xs \| bend (mul _ _)` | `xs \| bend (x : mul x x)` |
| `web \| get _ "a"` | `get web "a"` |
| `xs \| braid (add this that) 0` | `xs \| braid (a b : add a b) 0` |
| `pairs as add fore aft` | `pairs \| bend ((a, b) : add a b)` |

Two rules and no more:

1. **Brackets bind them.** Every hole inside one bracket group belongs to that
   group, and the group becomes a function of what they name. `(add _ _)` is
   `(x : add x x)`, not a function of two.
2. **A pipeline stage binds them too**, to the value being piped, which is what
   lets the collection-first verbs (`get`, `cell`, `member`, `insert`) sit in a
   chain. The value is bound, not substituted, so it is evaluated once however
   many holes name it.

What the claim *binds* is decided by which words appear, and by nothing else:

- One argument normally; **two as soon as a `that` appears** anywhere in the
  group. `that` is what makes a group take a second argument, so
  `braid (add this that) 0` needs no parameter names.
- The first argument whole, **or taken apart as soon as a component word
  appears** — into two for `former`/`latter`, into three for `fore`/`mid`/`aft`.
  Without one, `this` is the whole value, Twine or not; it is the arrival of a
  component word that asks for the opening, and which word it is says how wide.

All four combine: `(sub that former)` takes a pair and a second argument and
subtracts one from the other. A pipeline stage hands over one value, so a
`that` in a bare stage is an error — brackets are what supply a second.

Because the brackets are what bind them, nesting one call inside another
splits them up: `(eq 0 (mod _ 3))` claims `_` for the *inner* brackets and is a
type error, not a different meaning. Pipe instead — `(mod _ 3 | eq 0)` — which
is also shorter. A hole with nothing to claim it, or one inside a group that
already names its parameters, is an error.

None of the five can be a variable name; all five are keywords. In a pattern
`_` remains the wildcard, and the two never meet, since one is an expression
and the other is a pattern.

### 10.2 `ward` as expression
Every `ward` yields a value (all arms same type).

### 10.3 `pick` — the functional ternary
`pick COND IF_LIGHT IF_SHADOW`, lazy (evaluates only the taken branch):
```weave-part
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
| `Ply` | `take`, `drop`, `sever`, `rev`, `turn`, `weld`, `repeat` | Air, Thread |

`sort xs`, `eq a b`, and auto-output all dispatch through these; no comparator
needs to be passed for the common orderings. `Reckon` is what lets one `add`
serve both Earth and Water without operators or separate names.

`Ply` is the narrower thing `Bulk` is not. `Bulk` says a value has a size; `Ply`
says its elements lie in a known order, so a run of them can be taken, dropped,
cut or turned round. A Web has a size and no order, which is why `len` asks for
`Bulk` and `take` asks for `Ply` — and why `take 5 "hello world"` is a substring
rather than a round trip through `fires` and back. On Air these count runes, not
bytes, so `take` agrees with `len` and a character never comes apart in the
middle.

`weld` and `repeat` fit the Talent for a reason worth naming: neither mentions
the element type. `weld` is `a -> a -> a` and `repeat` is `Earth -> a -> a`, the
same shape `rev` has, so both join text and Thread without a word of extra
machinery. `nth`, `first` and `last` do not fit, and cannot: they answer with an
*element*, and what an element is depends on what it came out of — a Thread of
`a` holds an `a`, some text holds a Fire. That is a relation between two types
rather than a property of one, so it rides as a `Strand` constraint instead,
settled when the call is typed and defaulting to the Thread reading wherever the
container is not known.

Container types inherit a Talent from their contents: `Thread a` has `Eq` only
when `a` does, so `eq [1 2] [1 2]` is fine and `sort [Light Shadow]` is a type
error (`Spirit has no Ord Talent`).

Constraints show up in inferred types: `double n is add n n` reports
`double :: a -> a  where Reckon a`.

## 11.4 Taking a line apart

Most of Advent of Code wants the numbers and not the punctuation, and `earths`
answers that on its own: `"Game 11: 3 blue, 4 red" | earths` is `[11 3 4]`.
`waters` is the same sweep for the other Power, and a run of digits with no
point still counts, since this reads input rather than source:
`"x=1.5 y=-2" | waters` is `[1.5 -2.0]`. When the shape of the line *is* the
point, `delve` says it:

```weave
"Game 11: 3 blue, 4 red" | delve "Game {}: {}"
```
```weave
Held ["11" "3 blue, 4 red"]
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
- **Output** is every bare top-level expression, in the order it was written,
  rendered via `Show`: Earth/Water/Air print bare; `Thread` prints one element
  per line; a `Hold` prints as `Held x` or `Stilled`, so an answer that might
  not be there says so. `air x` forces a `Show` to `Air`. A Thread's elements
  are separated by newlines and the whole output ends with one.

  A chain bound to a name stays quiet; a chain left bare is an answer. So a
  file for one Advent of Code day is one binding for the input and two bare
  chains for the two parts, and both get printed:

  ```weave-part
  digits is [3 1 4 1 5]

  digits through sum
  digits through prod
  ```
  ```
  14
  60
  ```

- **`weave trace`** reports every definition as well, one tab-separated record
  per line — `LINE`, `NAME`, `VALUE` — which is what an editor shows as ghost
  text. A chain written one stage to a line reports what it holds at the end of
  *every* line it spans, so a pipeline reads as a sequence of shapes rather
  than a single answer. A file that does not compile still reports: the
  top-level items the mistake reached are left out and the rest is traced, at
  the lines they are on.

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

> **Implemented for a `Pattern`, `Web`, `Circle` or `Thread` threaded through a
> loop**, which is the case that matters. The general form — every container,
> decided by a reference count maintained everywhere — still needs the counting
> allocator; the runtime bump-allocates and does not free, so what is not
> written through is kept.

Everything is immutable *to you*. An update verb like `set sheet k v` returns "a
new grid," but the runtime checks `sheet`'s refcount:

- **Refcount 1** (last live reference — the old binding is dead after this call):
  mutate the buffer in place, hand back the same memory. **O(1).**
- **Shared** (something else still reads `sheet`): copy first, then mutate the
  copy. **O(n)**; the old grid stays valid for the other reader.

```weave-part
twice g k1 v1 k2 v2 is
  weave g2 is set g k1 v1    # g unused below -> in-place
  weave g3 is set g2 k2 v2   # g2 dead        -> in-place
  g3                         # a chain of O(1) updates, as Go's g[k]=v is
```
Proving it takes two halves, because neither alone is enough.

**Dynamically**, that the collection did not arrive already shared. Everything
is born shared, since the caller usually still holds what it hands over, so the
first update in a loop copies and marks the copy owned and every later one
writes through. This half is what makes the static half safe: calling the
writing form on something shared is never wrong, it simply copies.

**Statically**, that nothing else can reach it. This is the part with rules, and
they are stated as what is *refused* rather than what is allowed:

- A mention is **read-only** unless the verb can keep the collection. Two things
  disqualify a verb, and only two. Nine hand back a *window* on the argument's
  own array — `take`, `drop`, `sever`, `strands`, `takewhile`, `dropwhile`,
  `chunk`, `windows`, `cells` — and three hand the argument itself straight back
  when the update had nowhere to go: `mend`, `twist`, `set`. Everything else in
  the prelude may read it, wherever in the argument list it sits.
- Beyond that, the **type** decides: the collection's own type constructor must
  not occur in the call's result type. That is what stops `copies 3 g`,
  `weld [g] xs` or `put w k g` from quietly keeping it, and it needs no list to
  maintain — a verb that hands the collection back inside something has that
  something's type say so. A result the checker cannot resolve to a concrete
  type counts as mentioning everything.
- **The program's own functions** are read on the same terms, unless they are
  `remember`ed: a memo table keeps its arguments for the rest of the program and
  no type can say that.
- **A second name is a second owner.** Binding it with `weave`, putting it in a
  Twine or a Thread, or capturing it in a lambda all give up, because each
  leaves a reference the analysis cannot follow. A lambda written out *as a
  direct argument to a verb* is the exception: the verb cannot keep it past the
  call, so a mention inside is a read like any other.
- **A chain of updates is one update.** `put (put w a 1) b 2` hands each result
  straight to the next and nothing else sees the links.
- **A sibling argument of the updating call may not read it.** The arguments of
  a tail call are evaluated in order into the loop's slots, so an update at one
  writes through *before* a later one is evaluated — and a later argument that
  reads would see a write that, in the language, has not happened. A read
  anywhere else in the body is fine, because everything else runs first.
- **Handing it straight back unchanged** into its own slot counts as threading
  it, not as duplicating it: the next turn holds exactly the reference this one
  did. That is what a loop with a branch that skips the update looks like, which
  is most loops.
- Some clause must **actually update** it. A loop that only reads gains nothing,
  and is left alone so that nothing is ever disowned that was never owned.

Two shapes carry an update: a **self-tail-recursive parameter**, and a **fold's
accumulator** — `braid` or `gentle`. `gentle` is `braid` that may stop and
threads its accumulator identically; its step answers `Woven acc` to carry on,
which is that fold's tail position, and `Gentled x` to stop, whose `x` is the
answer leaving the fold rather than the accumulator.

A fold's accumulator may be the collection itself, or a **Twine of state with
the collection as one half** — `(board, position)` threaded through a walk,
which is how you carry two things. A Twine the step takes apart on entry and
rebuilds on exit has exactly one reference to each half, so the half that is a
collection is as single-threaded as a bare accumulator. The update has to sit in
that half's slot of the rebuilt Twine, and no other slot may mention it: the
slots are evaluated in order, so a later one would read a write that has not
happened.

The step may be **written out or named**. A definition of one clause, whose
parameters are patterns that cannot fail, which does not call itself and is not
`remember`ed, *is* a lambda under another name and is read back as one — so
lifting a step out to a name, which is what you do the moment it grows past a
line, costs nothing.

**`pick` counts as tail position in both branches here** as it does for tail
calls (§13.1), so the two spellings of one loop compile to the same program.
Without that they differed by three orders of magnitude with nothing in the
source to say which you had written.

**Ownership can cross a call.** A helper that updates a collection and hands it
back is written twice over: a body that keeps the ownership it was given, and
the name everyone else calls, which gives it back. A caller that was itself
holding the collection owned calls the first, so `search (fill board ks) …`
costs one copy rather than one per turn. The two forms are found by a fixpoint
over the call graph, started optimistically and cut back wherever a body turns
out to let its parameter escape.

Three gaps are known and left. A member of a *mutually* tail-recursive group
cannot consume, because the group compiles to one function over a shared slot
array. A consumed parameter has to be a plain name in every clause, since
destructuring one binds a window on its own array. And the fold-over-itself case
knows `braid` and not `gentle`.

**And what no analysis will ever bless**, so that none of this reads as a
promise: a *backtracking* search uses the collection it was handed once per
**option**, not once. The old value has to survive for the next branch, so it is
genuinely not single-threaded and there is nothing here to find. What helps
there is not writing through but forgetting — a fused fold turn handing its
whole storage back at the end of it, which is in §5.1.

`Taveren` is not covered. Its `push` path-copies, but a leftist heap's spine is
short — 200,000 pushes cost 0.02 s and 36 MB — so it is not the cliff the
others were.

## 14. D-particle aliases (optional prose glue)

Canonical is C (pipelines + `ward`). These desugar to canonical — pure sugar,
no new semantics — so easy code can read like prose.

| Particle | Desugars to | Example |
|---|---|---|
| `through f` | `\| f` | `Source through lines` |
| `where p` | `\| sift p` | `lines Source where has3digits` |
| `as f` | `\| bend f` | `lines Source as earth` |
| `else d` | `\| otherwise d` | `cell g k else '.'` |
| `failing d` | `\| snag d` | `harvest earth lines failing 0` |

`through` is the pipe spelled as a word; the other four carry a verb of their
own. `where` and `as` feed the **function**, so a hole in one makes a lambda —
`xs as mul _ 2` maps by `(x : mul x 2)`. `else` and `failing` feed a **value**,
the one to fall back on, so a hole in one has nothing there to claim it.

Which spelling a chain was written with is not a matter of meaning: a particle
desugars to its verb *by name*, so both forms resolve to the same thing even
where the name has been shadowed. `weave fmt` therefore chooses — the wordy
style prints `where p` and `as f`, the terse style `| sift p` and `| bend f`.

Particles sit at exactly the same precedence as `|` and associate left to
right, so they interleave freely: `xs | bend f where p | len`. A line opening
with one continues the line above, which is what lets a long chain be broken
one stage to a line.

The no-op glue words `of`, `at` and `from` were **cut**: a glue word that can
shadow a verb would silently rewrite working code, and readability glue is not
worth that.

## 15. Standard verbs (a sample; data-last)

The authoritative catalogue is `internal/prelude/prelude.go`, which the
compiler parses at start-up — the signatures there *are* these signatures, and
there are 233 of them. **[docs/verbs.md](docs/verbs.md) lists every one, with
its type and what it does**, generated from that same table by `make docs`; a
test fails if it falls out of date, so no verb can exist without appearing
there. `weave docs` serves the same vocabulary as a page with search over every
name, signature and description at once; `weave verbs [search]` prints it at
the terminal,
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
under   : Earth -> Thread Earth                       # 0 .. n-1: the places of n things
copies  : Earth -> a -> Thread a                      # n of the same value
flow    : (a -> a) -> a -> Thread a                   # endless: seed, f seed, ...
len     : a -> Earth                                  # any Bulk type
count   : (a -> Spirit) -> Thread a -> Earth
sum prod : Thread a -> a                              # needs Reckon a
sums prods : Thread a -> Thread a                     # the running totals
scan    : (b -> a -> b) -> b -> Thread a -> Thread b  # braid keeping every one
priors  : (b -> a -> b) -> b -> Thread a -> Thread b  # scan, keeping the seed too
settle  : (a -> a) -> a -> a                          # apply until nothing changes
gentle  : (b -> a -> Weaving b c) -> b -> Thread a -> Weaving b c  # braid that stops
take drop  : Earth -> Thread a -> Thread a
takewhile dropwhile : (a -> Spirit) -> Thread a -> Thread a
zip     : Thread a -> Thread b -> Thread (a, b)
zipwith : (a -> b -> c) -> Thread a -> Thread b -> Thread c
couples : Thread a -> Thread (a, a)                   # every two, each once
index   : Thread a -> Web a Earth                     # where each value sits
squeeze : Thread Earth -> Thread Earth                # a sparse axis made dense
sort    : Thread a -> Thread a                        # needs Ord a
nth     : Earth -> Thread a -> Hold a                 # by position
has     : a -> Thread a -> Spirit                     # membership; needs Eq a
high low : Thread a -> Hold a                         # the largest, the smallest
dupe    : Thread a -> Hold (Earth, Earth, a)          # the first repeat, and where

# Asking where rather than what. Both halves throughout: the first, and all.
idx  idxs    : a -> Thread a -> ...                   # Hold Earth, Thread Earth
seekidx siftidx : (a -> Spirit) -> Thread a -> ...    # the same, by test
highidx lowidx  : Thread a -> Hold Earth              # where high and low found it
glean   : (a -> Hold b) -> Thread a -> Thread b       # bend, keeping the Held
harvest : (a -> Hold b) -> Thread a -> Weaving (Thread b) a   # or say which failed
cycle   : Thread a -> Thread a                        # endless; bound it
all any : (a -> Spirit) -> Thread a -> Spirit

# Building and editing one. A Thread is a value: none of these changes the one
# you gave it.
thread  : (a, a) -> Thread a                          # a pair as a Thread
weld    : Thread a -> Thread a -> Thread a            # weld ys xs: xs then ys
mend    : Earth -> a -> Thread a -> Thread a          # replace one position
twist   : Earth -> (a -> a) -> Thread a -> Thread a   # mend, from the old value
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
base unbase : Earth -> ... -> ...                     # write and read any base 2..36
air   : a -> Air                                      # render anything, needs Show
strip : Air -> Air
earths waters : Air -> Thread Earth, Thread Water     # every number in some text
spans : Air -> Thread (Earth, Earth)                  # every `11-22` in some text
carve : Air -> Air -> Thread Air                      # carve seps text; words, named
delve : Air -> Air -> Hold (Thread Air)               # delve shape text

# Grid (Pattern a)
pattern  : Air -> Pattern Fire
weft     : a -> Thread (Thread a) -> Pattern a        # a Pattern of anything
warp     : (Knot -> a) -> Earth -> Earth -> Pattern a # a Pattern from its knots
spin flip : Pattern a -> Pattern a                    # a quarter turn, a mirror
cell     : Pattern a -> Knot -> Hold a
set      : Pattern a -> Knot -> a -> Pattern a
knots    : Pattern a -> Thread Knot
cells    : Pattern a -> Thread a
sited sites : Pattern a -> a -> ...                   # where a value is: the first, and all
cellwise : (a -> b) -> Pattern a -> Pattern b         # map, keeping the shape
nb4 nb8  : Pattern a -> Knot -> Thread a              # neighbouring cells
around4 around8 : Pattern a -> Knot -> Thread Knot    # neighbouring knots
tallies  : Pattern a -> Pattern a                     # the box above and left of each cell
tallied  : Pattern a -> Knot -> Knot -> a             # a box out of one, in one subtraction

# Ranges. A Twine, inclusive at both ends, which is how every input writes one.
overlaps    : (a, a) -> (a, a) -> Spirit              # needs Ord a
overlapping : (a, a) -> (a, a) -> Hold (a, a)         # the part they share
within      : (a, a) -> (a, a) -> Spirit              # does the first hold the second
spanning    : (a, a) -> (a, a) -> (a, a)              # the smallest round both
holding     : (a, a) -> a -> Spirit
width       : (Earth, Earth) -> Earth
mesh        : Thread (Earth, Earth) -> Thread (Earth, Earth)  # merged, in order

# Web / Circle / Taveren / graphs
get   : Web k v -> k -> Hold v
put   : Web k v -> k -> v -> Web k v
forget : Web k v -> k -> Web k v                      # and remove, for a Circle
freq  : Thread a -> Web a Earth                       # frequency count
push  : Taveren a -> a -> Taveren a
pop   : Taveren a -> Hold (a, Taveren a)

# A graph is a function from a place to the steps out of it — which fits an
# implicit graph, a Pattern or a state machine, as readily as one you built.
dijkstra : (a -> Thread (Earth, a)) -> a -> Web a Earth      where Ord a
reach    : (a -> Thread a) -> a -> Circle a                  where Eq a
route    : (a -> Thread (Earth, a)) -> a -> a -> Hold (Thread a)
toposort : (a -> Thread a) -> Thread a -> Hold (Thread a)     where Eq a
clumps   : (a -> Thread a) -> Thread a -> Thread (Thread a)   where Eq a

# A Link answers the same question while the joining is still going on.
link    : Thread a -> Link a                                  where Eq a
bind    : Link a -> a -> a -> Link a                          where Eq a
bound   : Link a -> a -> a -> Spirit                          where Eq a
clumped : Link a -> Thread (Thread a)

# Hold / Weaving
otherwise : a -> Hold a -> a                           # `else d` as a particle
holds     : Hold a -> Spirit
woven     : Weaving a e -> Spirit                      # `holds`, for the other pair
rescue    : a -> Weaving a e -> a
snag      : e -> Weaving a e -> e                      # `failing d` as a particle

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
