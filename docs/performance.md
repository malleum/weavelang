# How fast is Weave?

Weave compiles to C and hands the result to `clang -O3`, so the interesting
question is not whether it beats an interpreter — it is whether a language with
no loops, no mutation and immutable collections can stay close to the same
program written in Go.

The short answer: on straight-line arithmetic, on Thread pipelines and now on
maps, Weave is as fast as Go and often faster. It holds two to three times the
memory, and one benchmark — Advent of Code 2024 day 22 — is still four times
slower. Both come back to what the runtime allocates and what it can prove is
dead, which is measured at the bottom.

## Method

Every benchmark is a pair of programs — one Weave, one Go — that read the same
input on stdin and print the same answer. The runner refuses to report a timing
for a pair that disagrees, so a fast wrong answer cannot slip in.

```
python3 bench/run.py            # everything
python3 bench/run.py -n 9 day11 # one benchmark, best of nine
```

- Weave is built with `weave build -opt -O3`, which is `clang -O3` underneath.
- Go is built with `go build`, no flags.
- Timing is wall clock, best of five, measured around `fork`/`exec` so process
  start-up is included in both.
- Memory is the child's own peak resident set, from `wait4`.
- Measured on an Intel Xeon at 2.10 GHz, 4 cores, 16 GB, Ubuntu 24.04,
  clang 18.1.3, Go 1.24.7.

Best-of-five rather than an average: the fastest run is the one least disturbed
by everything else on the machine, and it is the number that reproduces.

The Advent of Code inputs are **generated**, not real. Real inputs are per-user
and not redistributable, so `bench/gen.py` produces inputs of the same shape and
size as the 2024 puzzles — 1000 rows for day 1, a 55×55 height map for day 10,
2000 buyers for day 22. The answers differ from yours; the work does not.

## Raw benchmarks

| benchmark | weave | rss | go | rss | ratio |
|---|---|---|---|---|---|
| `fib 32` — naive recursion | 10.7 ms | 9 M | 14.0 ms | 9 M | **0.76×** |
| `chain` — map/filter/sum over 20 M | 27.5 ms | 9 M | 27.6 ms | 9 M | **1.00×** |
| `chainalloc` — the same, written with intermediate slices | 27.2 ms | 9 M | 641.8 ms | 683 M | **0.04×** |
| `loop` — 100 M tail-recursive steps | 84.9 ms | 9 M | 75.9 ms | 9 M | 1.12× |
| `collatz` — longest chain below 300 000 | 47.7 ms | 9 M | 83.0 ms | 9 M | **0.57×** |
| `mapbuild` — 2 M map insertions | 149.3 ms | 98 M | 161.7 ms | 47 M | **0.92×** |
| `text` — 16 MB, 3.2 M words | 184.6 ms | 241 M | 201.4 ms | 87 M | **0.92×** |

Ratios below 1.00 are Weave being faster.

**`fib`** is 2.7 million calls with nothing to optimise, so it measures call
overhead and integer arithmetic and little else.

```weave
fib 0 is 0
fib 1 is 1
fib n is add (fib (sub n 1)) (fib (sub n 2))

n is Source | strip | earth | otherwise 0

fib n
```

Weave wins here because the C it emits is a plain recursive function over
`int64_t` with no interface dispatch and no stack-growth check, and clang
inlines one level of it. Go's goroutine stacks are cheap but not free.

**`chain` and `chainalloc`** are the same computation twice.

```weave
n is Source | strip | earth | otherwise 0

span 1 n | bend (x : mod (mul x x) 1000003) | sift even | sum
```

Weave fuses that whole pipeline into one loop with no intermediate Thread, so
it costs what the hand-written Go loop costs — 27.8 ms against 25.4 ms, which
is as close as two compilers get on the same loop. `chainalloc` is the Go
program written the way the Weave program *reads*: build the range, map it,
filter it, sum it. That costs 655 ms and 683 MB. Fusion is the difference
between the two Go columns, and it is why Weave can afford to have no loops at
all.

The `mod` is not decoration. Without it the sum of squares up to twenty million
does not fit in an int64, and the first version of this benchmark overflowed —
both languages wrapped identically, so the harness compared two wrong answers
and agreed. `weave build -overflow` is what found it. See below.

**`loop`** is a dead heat, within noise either way. A tail-recursive Weave
function compiles to a `for(;;)` with assignments, which is what the Go program
already was, so this measures nothing but the C compiler against the Go
compiler on identical code.

```weave
count 0 acc is acc
count n acc is count (sub n 1) (add acc (mod n 7))

count 100000000 0
```

**`collatz`** is branch-heavy integer work over 300 000 starting values.

**`text`** splits 16 MB into 3.2 million words and counts the long ones. Weave
is faster but holds 255 MB against Go's 103 MB, because Weave materialises the
word Thread and never frees it.

## Advent of Code 2024

| day | weave | rss | go | rss | ratio |
|---|---|---|---|---|---|
| 1 — historian hysteria | 2.4 ms | 9 M | 2.3 ms | 9 M | 1.04× |
| 2 — red-nosed reports | 3.4 ms | 9 M | 2.6 ms | 9 M | 1.32× |
| 10 — hoof it | 3.1 ms | 9 M | 2.5 ms | 9 M | 1.25× |
| 11 — plutonian pebbles | 31.8 ms | 20 M | 33.4 ms | 11 M | **0.95×** |
| 22 — monkey market | 539.5 ms | 387 M | 197.5 ms | 9 M | 2.73× |

Days 1, 2 and 10 finish in single-digit milliseconds in both languages, and at
that size the numbers are mostly process start-up and parsing. The 1.4× is real
but it is 1.4× of nothing; both are far below the threshold where anyone
notices. This is the honest shape of most Advent of Code days — the puzzle is
the hard part, not the runtime.

Days 11 and 22 are where the work is, and they are where Weave loses.

**Day 11** blinks a handful of stones 75 times. The stones never interact, so
the answer is a sum over each starting stone, and the same (stone, blinks)
question recurs constantly — which is what `remember` is for:

```weave
remember stones s 0 is 1
stones 0 n is stones 1 (sub n 1)
stones s n is
  weave d is digits s
  weave half is pow10 (div d 2)
  pick (even d)
    (add (stones (div s half) (sub n 1)) (stones (mod s half) (sub n 1)))
    (stones (mul s 2024) (sub n 1))

digits n is pick (gt 9 n) (add 1 (digits (div n 10))) 1

pow10 0 is 1
pow10 k is mul 10 (pow10 (sub k 1))

start is Source | earths

[start | bend (s : stones s 25) | sum, start | bend (s : stones s 75) | sum]
  | bend air
  | join "\n"
```

Without `remember` this is 2^75 work; with it, it is about 130 000 distinct
subproblems. The Go program uses a `map[[2]int]int` for the same thing, and
Weave now uses something very close to it: a memo table is private to one
definition, is never copied and is never pruned, so it is a plain
open-addressed table probed straight from the argument array rather than a Web.
It used to be a Web, and the two-argument key was a Twine allocated on every
call — hit or miss — only to be hashed and thrown away. That was 402 ms and
527 MB; it is now 29 ms and 20 MB, which is a tie.

**Day 22** is the most expensive of the four: four million rounds of
shift/xor/mask for part 1, and four million map insertions for part 2. The
first half is a fair fight — the secret-number loop on its own runs in the same
time in both languages. The second half is not, and this is now the one
benchmark Weave loses badly.

It used to be far worse. This program is written as two nested folds, and until
the in-place analysis learned to look inside one it path-copied the whole map
on every step; the version measured here is the idiomatic one, and it is both
faster and lighter than the hand-unrolled tail recursion it replaced.

What was left, once the map was dealt with, turned out not to be `zipwith`
either. Counting every allocation the program made said where the time went:
**19.6 million blocks of 32 bytes, 597 MB of them**, against 24 000 large ones.
Those 32-byte blocks were *pairs* — a `WTwine` header and its two-element array
— and almost all were built only to be taken apart on the very next line:

```c
Value h67 = wp_zip(l65, l66);        // builds 2000 Twines
...
  Value b72 = w_twine_at(x71, 0);    // and immediately unpacks them
  Value b73 = w_twine_at(x71, 1);
```

Both halves of the program did it: `zip k v | braid (w (key, price) : …)` per
buyer, and `items o | braid (u (k, v) : …)` in the fold that merges them.
Deleting the first from the source took the program from 3.58 s to 1.27 s;
deleting the second took it to 1.83 s.

So the fusion machinery learned both. `zip a b` is a two-source producer and
`items w` walks the map's keys and values as parallel arrays, and where the
function that consumes an element destructures a two-element Twine, its halves
are bound straight from the sources — the pair never exists. Day 22 went from
1489 ms and 1522 MB to **1186 ms and 925 MB**, and the pair-building
disappeared from the generated C entirely.

`zipwith` was measured and left alone. Its three result arrays are each read
more than once — `d` is read as `drop 1 d`, `drop 2 d` and `drop 3 d` — so a
fused `zipwith` would still have to write them out. It removes one array of
nine and none of the pairs. That is what day 22's remaining gap is made of.

## Where the gap is

Every benchmark Weave used to lose was a benchmark that built a big map or set.
That is no longer true of the time — `mapbuild` and day 11 both now finish
ahead of Go — but it is still true of the memory, and one benchmark is still
badly behind.

What changed is that a map is no longer always a trie. `Web` and `Circle` are
persistent hash array mapped tries because the language promises immutability,
and the in-place analysis (SPEC §13) already proves, for the loops that matter,
that nothing else can see one. For an *owned* map whose keys are immediates —
`Earth`, `Knot`, `Fire`, `Spirit`, which is very nearly every key anyone
writes — the map is instead a flat open-addressed table: one array, no node per
entry, nothing to path-copy, one cache miss per lookup instead of four. It
turns back into a trie the moment it is used persistently, so nothing the trie
guaranteed is lost.

All the timings on this page come from one machine in one sitting, which is
what makes the ratios worth reading; the absolute milliseconds do not compare
with an earlier version of this document.

That caveat is stronger than it sounds. The same `loop` binary, byte-identical
generated C, measured 54.7 ms at one point in an afternoon and 82.3 ms at
another — and Go's moved by much less over the same span. A ratio within about
ten percent of parity should be read as a tie, and `loop` has landed on both
sides of it.

What remains is the allocator, and here the honest answer is that it is
partly fixed and partly not.

The arena still does not collect, and never will — tracing would cost every
program something to help the few that need it. What it does now is *reuse*.
The places that know a block is dead the instant it happens — a buffer that has
just outgrown its array, a trie node an owned insert has just replaced, a hash
table a grow has just rehashed out of — hand it back, and the next allocation
of that size takes it. The lists are exact-size rather than rounded, because a
loop that builds one Thread per turn asks for the same length every time.

That is worth having and it is not large: day 22 dropped 63 MB, `text` 14 MB,
and the rest is unchanged. It cannot be large, because the blocks it reclaims
are the ones a *growing* structure abandons, and most of what a Weave program
allocates is not that. Day 22's 1.5 GB is intermediate values that are live
right up until the call that made them returns — no local rule can see they are
dead, and the only thing that could is an escape analysis the compiler does not
have.

The values that are live right up until the call that made them returns, and
dead immediately afterwards, are the escape analysis's, described next.

### A `Held` of a Power is not a box

`cell g k | otherwise 0` and `get w k | otherwise 0` are most of how Advent of
Code reads a grid or a map, and both allocated a `Hold` in order to take it
apart on the next instruction. Measured, that was 16.3% of day 10 and 118 MB of
day 22.

There was room to fix it without growing anything. A `Value` is a four-byte tag
and an eight-byte payload, which is sixteen bytes with four of them padding.
Spend the padding:

```c
typedef struct {
  uint32_t tag;
  uint32_t aux;   // what a Hold has inside
  union { int64_t earth; double water; ... };
} Value;
```

A `Held` of one of the Powers is now that value where it stands, with its own
tag moved into `aux` and `W_HOLD` in `tag`. `Stilled` is a reserved `aux`. Only
a `Held` of something already on the heap — a Thread, some text, a Web — still
needs a `WHold`, and nothing outside the four functions that build and read a
Hold can tell the difference.

The interesting part was what it broke. `Stilled` used to be "the pointer field
is zero", and the pointer field is the same eight bytes as `earth` — so
**`Held 0` and `Stilled` had the same representation** under the old test, and
every `if (h.obj)` in the runtime was a latent bug waiting for someone to store
a zero in a map. There were seven of them: `otherwise`, `holds`, `glean`,
`harvest`, `compact`, the shortest-path walk-back, and `toposort`'s degree
count. All seven now ask `w_is_held`, and twenty cases pin them.

Day 22 gave back 117 MB — almost exactly the 118 MB the profile attributed to
`w_held` — and went from 3.1× Go to 2.7×. Day 10 went from 1.32× to 1.25×. The
instruction count moved less than the profile suggested, 5.4% rather than 16%,
because only the allocation goes away; the work of assembling the value stays,
inline and free of a call.

### The boxing was already gone, and the fold's flag was not

Every value is a 16-byte tagged union, and the plan of record was to unbox it —
monomorphise every polymorphic function, change the calling convention, give the
collections typed storage. It was written down as worth 1.6× on recursive
arithmetic and 2.3× on a fused loop, and it was the last thing on the list.

Reading the assembly first turned out to matter. Here is the whole inner loop of
`span 1 n | bend (x : mod (mul x x) 1000003) | sift even | sum`, as clang
compiles what Weave emits:

```
.LBB0_10:
	movq	%r8, %rdi
	imulq	%r8, %rdi
	movq	%rdi, %rax
	mulq	%r10
	shrq	$19, %rdx
	imulq	$1000003, %rdx, %rax
	subq	%rax, %rdi
	testb	$1, %dil
	jne	.LBB0_12
# %bb.11:
	testb	$1, %bl                 <-- the fold's `started` flag
	movzbl	%bl, %ebx
	cmovel	%r11d, %ebx
	cmoveq	%r9, %rsi
	addq	%rdi, %rsi
```

There is not a tag in it. The `Value` struct never reaches memory: the typed
helpers are `static inline`, clang sees through all of them, and everything
lives in registers. **The unboxing that mattered for speed was already
happening**, done by the C compiler for free, and had been all along.

What was actually costing `chain` its gap to Go is the four instructions in the
middle: a test and two conditional moves for a flag saying whether the fold had
seen its first element yet. `sum` is `Reckon a`, so the loop could not start the
accumulator at zero — it might be summing Waters — and waited to be told.

But the fused loop usually knows: `p.elem` already records the element type,
which is what picks `w_add_e` over the tag-dispatching `wp_add`. When that type
is Earth the identity is known too, so the fold starts there and adds
unconditionally. Only Earth: `sum` of an *empty* Thread answers an Earth `0`
whatever the element type, so starting a Water fold at `0.0` would disagree with
the unfused verb, and the differential tests would be right to say so.

The flag goes, and clang then turns the filter branchless as well:

```
.LBB0_14:
	movq	%rdi, %r8
	imulq	%rdi, %r8
	...
	testb	$1, %r8b
	je	.LBB0_16
	xorl	%r8d, %r8d              <-- odd: add zero instead of branching
.LBB0_16:
	addq	%r8, %rsi
```

`chain` went from 1.09× to 0.98×, and with it the last raw benchmark Weave was
losing. Measured on an idle machine, best of 21: Weave is 54.7 ms on `loop`
against Go's 65.0, and steady to within a millisecond across runs where Go
varies by fifteen.

Boxing has not stopped mattering — it has moved. A `Thread Earth` still stores
sixteen bytes per `int64`, and a `Web Earth Earth` stores thirty-two per entry.
That is what the two-to-three times memory is, and on day 22 it is why
`flat_put` and `map_lookup` are 29% of the run: half the bytes they touch are
tags. Fixing *that* is monomorphised container storage, which is a real project
with a memory payoff rather than an instruction-count one — and now that the
loops are settled, it is the only unboxing left worth costing out.

### `zipwith` is a producer, not a call

A chain fuses into one loop until it reaches a stage that needs two Threads at
once, and `zipwith` is that stage. It was left alone once already, on the
grounds that fusing it would save no allocation — its result arrays are read
more than once, so the loop has to write them out either way. That was true and
it was the wrong question. What `zipwith` costs is not the array, it is the
closure and the call: a function value built before the loop and reached
through `w_call` once per element.

So it is now a producer, like `zip` with the combining done on the spot, and its
function is planned exactly like a stage's — which means a lambda is inlined
into the loop body and specialised against its inferred types:

```weave
p is [3 1 4 1 5 9 2 6]

zipwith (a b : sub b a) p (drop 1 p)
```

```c
for (size_t i = 0; i < n; i++) {
  Value l = w_thread_at(p, i);
  Value r = w_thread_at(shifted, i);
  o[k++] = w_sub_e(r, l);        // no closure, no call, typed subtraction
}
```

Two smaller things came out of the same profile. The prelude's own `call1` and
`call2` went through `w_call` rather than the inline fast paths beside it, so
every higher-order verb in the runtime paid a function call in front of the
closure dispatch — two lines, and 6% of day 22. And `w_thread_len` and
`w_thread_at` were out-of-line functions wrapping a single dereference, which a
fused loop calls once or twice per element; inlining them was another 12%.

Whole-program instructions fell from 510M to 416M, and the day from 3.3× Go to
3.0×. That last figure is the honest one: the instruction count fell by 18% and
the wall clock by 7%, because what is left is memory-bound rather than
instruction-bound.

### A map is read back without a comparison sort

`keys`, `vals` and `items` hand a map's entries back in ascending key order.
That is what makes the flat table and the trie indistinguishable from inside the
language, and what makes a program's output depend on what it put in rather than
on how the runtime stored it. It is a good guarantee, and for a while it was a
quarter of day 22's running time.

Two things fixed that. Nothing that moves is bigger than it has to be: the sort
orders *ranks* — a key's ordering value and where its entry sits, sixteen bytes,
four to a cache line — and the entries themselves never move at all. For a flat
map they are not even copied; the ranks carry slot numbers and the table is read
where it lies.

And it is a radix sort over only the bytes that actually differ:

```c
uint64_t varying = ones ^ zeros;   // a bit is set where the keys disagree
```

Keys agree on their high bits far more often than not. Every Earth key a program
builds is small and positive, so five of its eight bytes are the same in all of
them, and those passes are skipped — day 22's keys take three passes over the
array rather than eleven comparisons per element.

```
before                     after
24.3%  cells_sort           7.6%  ranks_sort
 2.2%  web_cells            3.5%  web_entries_of
```

Whole-program instruction count fell 26%, and the day went from 4.4× Go to 3.3×.
A key that is not an immediate — text, a Twine, a declared type — has no
one-word ordering and keeps the comparison sort, which is one more reason to key
a map on Earths or Knots.

### A function hands back the Threads it built

The free lists can only take back what the *runtime* knows is dead. The larger
case is a function that builds half a dozen intermediate Threads, uses them, and
returns something with nothing to do with any of them — day 22 doing that two
thousand times over was 694 MB of Thread arrays nobody ever asked for again.

The answer comes mostly from the types. If a function's result cannot *contain*
a Thread — `Web Earth Earth` cannot, a type variable might, a function might have
captured one — then no Thread the function built can be reached through what it
returns. Weave has no assignment and no mutable globals, so being returned is
the only way a value outlives its call.

The one exception is a `remember`ed function, whose table keeps its arguments
for the rest of the program. That is why a local handed to a *user* function is
left alone, while one handed to a prelude verb is not: a verb cannot outlive the
call, so whatever it does with the Thread, the result is still something this
function either drops or returns — and the return has already been ruled out.

So a `weave` binding is released just before the return when it is a Thread, was
bound to a verb that allocates a fresh array, and is only ever passed to prelude
verbs. The slicing verbs are deliberately not on that list: `drop 1 xs` shares
`xs`'s storage, so freeing one would free the other.

```weave
offers s is
  weave p is flow (n : mod (mul n 7) 101) s | take 20 | bend (x : mod x 10)
  weave d is zipwith (a b : sub b a) p (drop 1 p)
  weave k is rev d
  zip k p | braid (w (key, price) : put w key price) (web [])

offers 3 | items | len
```

The three Threads are `Thread Earth`; the result is `Web Earth Earth`, which
cannot hold a Thread; so all three go back:

```c
  Value z29 = r25;             // the Web the fold built
  w_thread_release(l14);       // p
  w_thread_release(l18);       // d
  w_thread_release(l19);       // k
  return z29;
```

Three smaller things came out of the same measurement. A buffer that doubled
hands back the slack as it seals, so a `sift` that thinned a million elements to
ten does not keep the million. A fused loop over `items` frees the two arrays
`w_web_entries` gave it. And a fold into a map whose length is known reserves it
up front instead of rehashing its way up from sixteen, which was costing as much
again as the inserts themselves.

Day 22: 1186 ms and 925 MB to 827 ms and 505 MB. `weave build -no-release` turns
the whole thing off, and the differential tests compile every program both ways.

### A pair that is taken apart is never built

`zip a b` and `items w` are producers the fused loop generates rather than
values it builds. Where the function consuming an element destructures a
two-element Twine, its halves are bound straight from the two sources:

```weave
ks is [1 2 3]
ps is [10 20 30]

zip ks ps | braid (w (key, price) : put w key price) (web [])
```

```c
for (size_t i = 0; i < n; i++) {
  Value l = w_thread_at(ks, i);       // no Twine, no Thread of Twines
  Value r = w_thread_at(ps, i);
  acc = w_web_put_owned(acc, l, r);
}
```

`items` gets a runtime call of its own, `w_web_entries`, which hands back the
keys and the values as two parallel arrays in the order `items` would have
given. It reads the map into fresh arrays before the loop starts, so a fold
that writes back into the same map it is walking is still safe.

If anything in the chain wants the pair whole — a `first`, a collected result,
a function that does not take it apart — the loop builds one at the top of the
body and everything proceeds as before. Deciding that before the loop rather
than per element is what keeps the fast path free of tests.

### Folds update in place too

The in-place analysis used to recognise only a collection threaded through a
**tail recursion**. The same loop written as a `braid` path-copied, because the
accumulator lived inside the runtime's fold where the compiler could not see
it — so the idiomatic spelling was the slow one, which is the wrong way round.

It sees it now. A chain ending in `braid` is fused whatever it is over, so the
accumulator is an ordinary C variable and the same proof applies: the body may
read it, and must otherwise only update it and hand the result back. A fold
*over* the accumulator counts as an update too, which is what makes two nested
folds writing to one map cost one copy rather than one per turn.

```weave
ks is span 1 2000000 | bend (k : mod (mul k 2654435761) 1000003)

ks | braid (w k : put w k 1) (web []) | len
```
```
1000003
```

| | before | after |
|---|---|---|
| that fold, 2 M inserts | 13.8 s, 9052 M | 0.42 s, 99 M |
| AoC 2024 day 22 | 6.2 s, 4198 M | 1.8 s, 1647 M |

The proof is deliberately narrow, and `weave build -no-in-place` turns it off —
the differential suite compiles every program both ways and requires the same
answer. Bind the accumulator to another name, capture it in a lambda, take its
`keys`, or return something that is not the update, and it copies.

## What an overflow check costs

An `Earth` is an `int64` and int64 arithmetic wraps. `weave build -overflow`
compiles `add`, `sub` and `mul` at Earth with `__builtin_*_overflow`, so a
number too large stops the program and names the verb instead of silently
becoming a negative one.

It is off by default because it is not free. Measured by building each
benchmark both ways and running them alternately, so both see the same machine.
These were taken before the flat map table and the memo table below, so the
`mapbuild` and day 11 rows are slower than those benchmarks are now; the shape
of the answer — which is what the section is about — is unchanged:

| benchmark | plain | checked | cost |
|---|---|---|---|
| `fib 32` | 9.2 ms | 12.6 ms | +36% |
| `loop` — 100 M tail-recursive steps | 70.5 ms | 92.3 ms | +31% |
| `collatz` | 41.1 ms | 71.8 ms | +75% |
| `mapbuild` — 2 M map insertions | 405.8 ms | 406.9 ms | +0.3% |
| day 1 | 3.1 ms | 2.9 ms | −5% |
| day 2 | 3.8 ms | 3.7 ms | −2% |
| day 10 | 2.9 ms | 3.0 ms | +2% |
| day 11 | 386.4 ms | 393.4 ms | +2% |

The shape is clear: a loop that does nothing but arithmetic pays a third to
three quarters, because the check is a real fraction of the work and the branch
stops clang vectorising. A program that also touches memory pays nothing
measurable — the negative numbers are noise, not a speed-up.

So it is cheap for the programs anyone actually writes, and expensive for the
ones that exist to measure arithmetic. Whether that trade is worth taking by
default is a judgement about how often a wrapped answer would go unnoticed;
2024 day 11 already produces numbers in the 10^14 range, and this document
found one in its own benchmark suite.

## Compile time and binary size

Weave goes through C, which sounds slow and is not:

```
$ weave build bench/weave/day22.weave
123 ms
```

The C runtime is compiled once and cached, so a rebuild is one `clang -O3` of a
few hundred lines of generated C. The executable is 80 KB, against 2.3 MB for
the equivalent Go program, because there is no runtime to carry — no scheduler,
no garbage collector, and no reflection metadata.

## Reproducing

```
python3 bench/gen.py     # regenerate the inputs
python3 bench/run.py     # build both languages and time everything
```

The sources are in `bench/weave` and `bench/go`. If you add a benchmark, add
both halves: the runner exists to make it impossible to publish a timing for
two programs that do not agree on the answer.
