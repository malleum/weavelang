# Weave, from the beginning

This assumes you have written a little Haskell or Elm or OCaml at some point and
liked it, but could not currently write out the definition of a fold from
memory. Nothing here relies on remembering that. Every idea is introduced with a
program you can run.

Every code block below is a complete program — you can paste any of them into a
file and run it — and they are all compiled by the test suite, so none of them
can quietly stop working.

```sh
weave run hello.weave          # compile and run, with stdin as the input
weave hello.weave              # the same thing
weave repl                     # try things out
weave verbs Knot               # what works on coordinates?
```

---

## 1. A program is some definitions and one expression

The last bare expression in the file is what the program prints. Everything
above it is named for your convenience.

```weave
answer is 42

answer
```

```
42
```

There is no `main`, no `print`, no `return`. If you want to see something, make
it the last line.

Definitions can come in any order — this is the same program:

```weave
answer

answer is 42
```

`is` binds. So does `=`, if you prefer, but `is` is idiomatic and `weave fmt`
will rewrite `=` to `is`.

---

## 2. There are no operators

None. Not one. Arithmetic is verbs:

```weave
add 2 3
```

```
5
```

```weave
mul (add 2 3) (sub 10 4)
```

```
30
```

That looks like a lot of brackets, and it would be if you wrote everything this
way. You mostly will not — see §4.

The reason for it is that the symbols you *do* type are then free to mean
something structural. In Weave the only punctuation you use often is `|`, `is`
and `:`.

The full arithmetic vocabulary is `add sub mul div mod`, plus `abs neg min max
gcd lcm pow sqrt` and the rest. `weave verbs Numbers` lists them.

---

## 3. Five primitive types, named after the Five Powers

| | | |
|---|---|---|
| `Earth` | integer | `42`, `-7`, `1_000_000` |
| `Water` | float | `3.14`, `1.0` |
| `Fire` | character | `'#'`, `'a'`, `'\n'` |
| `Air` | text | `"hello"` |
| `Spirit` | boolean | `Light` and `Shadow` |

```weave
[air 42, air 3.5, air 'x', "hi", air Light] | join " "
```

```
42 3.5 x hi Light
```

Types are inferred, never written — but they are strict, and you will hear about
it if you get one wrong:

```
add 1 "x"
   |     ^
`add` expects Earth here, but found Air
```

You *can* write a signature if you want one as documentation. The compiler
checks it against what it infers:

```weave
double :: Earth -> Earth
double n is mul n 2

double 21
```

---

## 4. The pipeline, `|`

`x | f` is `f x`. That is the whole rule, with one wrinkle worth knowing: the
piped value goes in as the **last** argument.

```weave
[3 1 2] | sort
```

```
1
2
3
```

Since it goes in last, a verb applied to some of its arguments is a stage:

```weave
[1 2 3 4 5] | sift even | sum
```

```
6
```

`sift even` is `sift` waiting for its Thread. `[1 2 3 4 5] | sift even` supplies
it. This is why the sequence verbs all take their data last — it is what makes
them compose.

A pipeline can be broken across lines by starting each line with `|`:

```weave
Source
  | lines
  | bend earth
  | bend (otherwise 0)
  | sum
```

Read that top to bottom: take the input, split it into lines, parse each one as
an integer, replace anything that failed to parse with 0, add them up.

Three words carry a verb of their own, so the commonest stages read as prose.
`through` is `|` spelled out; `where p` is `| sift p`; `as f` is `| bend f`:

```weave
["1", "x", "2", "3"]
  as earth
  as otherwise 0
  where gt 0
  through sum
```

```
6
```

Every one of them sits at the same precedence as `|`, so they interleave
freely, and `weave fmt` prints the words wherever the plain pipe would do.

---

## 5. Sequences are `Thread`s

A Thread is the list type. The three verbs you will use constantly:

```weave
[1 2 3] | bend (x : mul x 10)
```

```
10
20
30
```

`bend` is map. The `(x : ...)` is a lambda — parameters, then `:`, then the
body.

```weave
[1 2 3 4 5 6] | sift (x : gt 3 x)
```

```
4
5
6
```

`sift` is filter. Note `gt 3 x` reads "x is greater than 3" — comparison verbs
are data-last too, so `sift (gt 3)` means "keep the ones over 3".

```weave
[1 2 3 4] | braid (total x : add total x) 0
```

```
10
```

`braid` is fold. It takes the combining function, the starting value, and the
Thread. You will rarely need it, because `sum`, `prod`, `len`, `count`, `most`
and friends already exist.

Building a range:

```weave
span 1 5
```

```
1
2
3
4
5
```

There is no positional indexing verb. To get the *n*th element, drop the ones
before it:

```weave
span 10 20 | drop 3 | first
```

```
Held 13
```

That `Held` is the next section.

---

## 6. There is no null

A verb that might not have an answer returns a `Hold`: either `Held x` or
`Stilled`.

```weave
[1 2 3] | seek (gt 10)
```

```
Stilled
```

```weave
[1 2 3] | seek (gt 1)
```

```
Held 2
```

You cannot use the value without dealing with the empty case. Usually you want
a default:

```weave
[] | first | otherwise 0
```

```
0
```

And when you want to do different things in the two cases, you match — which is
the next section.

---

## 7. Pattern matching with `ward`

```weave
describe h is
  ward h
    Held 0 : "zero"
    Held n : "something"
    Stilled : "nothing"

[describe (Held 0), describe (Held 5), describe Stilled] | join " "
```

```
zero something nothing
```

`ward` takes the value, then one indented arm per case: the pattern, `:`, the
result.

**The compiler checks you handled everything.** Delete the `Stilled` line from
that program and you get:

```
`describe` does not handle every case: `Stilled` is unmatched
 hint: add an arm for `Stilled`
```

This is the single most useful thing the type system does for you. It also
catches an arm that can never be reached.

Patterns can be literals, constructors with names bound to their fields,
Twines, and `_` for "anything":

```weave
sign2 0 is "zero"
sign2 n is pick (gt 0 n) "positive" "negative"

[sign2 0, sign2 5, sign2 (neg 5)] | join " "
```

That is the other form: **a definition can have several clauses**, each with its
own patterns, tried in order. It is usually tidier than a `ward`.

`pick c a b` is the conditional — `c` decides, and only the branch taken is
evaluated.

---

## 8. Recursion instead of loops

There are no loops. When a sequence verb does not fit, you write a function that
calls itself.

The shape to remember: **one clause for the base case, one for the step**.

```weave
countdown 0 is []
countdown n is flat [[n], countdown (sub n 1)]

countdown 5
```

```
5
4
3
2
1
```

The important variant is the one with an **accumulator** — a parameter carrying
the answer so far:

```weave
total 0 acc is acc
total n acc is total (sub n 1) (add acc n)

total 100 0
```

```
5050
```

Look at where the recursive call sits in that second clause: it is the *whole*
body, not part of a larger expression. That is a **tail call**, and Weave
compiles it to a jump — the same machine code a `for` loop would produce, in one
stack frame. `total 100000000 0` runs in a third of a second and does not
overflow anything.

Compare the first version: `flat [[n], countdown (sub n 1)]` has the call
*inside* a `flat`, so there is work left to do after it comes back. That is not a
tail call and it does use stack. For a few thousand elements that is fine; for a
few million it is not.

The rule of thumb: **if you are recursing over something big, carry an
accumulator and make the recursive call the last thing that happens.**

Definitions that call *each other* in tail position get the same treatment, so
this also runs in one frame:

```weave
even2 0 is Light
even2 n is odd2 (sub n 1)

odd2 0 is Shadow
odd2 n is even2 (sub n 1)

even2 1000000
```

```
Light
```

---

## 9. `_`, for the argument you did not name

Writing `(x : mod x 2 | eq 0)` gets tedious. `_` stands for the argument:

```weave
span 1 10 | sift (mod _ 2 | eq 0) | sum
```

```
30
```

The rule: **a `_` is claimed by the brackets around it**, and the group becomes
a function of it. Every `_` in one group is the same value, so `(add _ _)` is
"double".

A pipeline stage claims one too, which is how the collection-first verbs join a
chain:

```weave
web [("a", 1), ("b", 2)] | get _ "a" | otherwise 0
```

```
1
```

`get` takes the map first, so it would not normally fit in a pipeline. `get _
"a"` puts the piped value where the `_` is.

`_` also has a word spelling, `this`, and `:` has one too, `gives`. They are
the same tokens — `weave fmt` prints the words, `weave fmt -terse` the symbols
— and on a line you are going to read a hundred times, the words usually win:

```weave
[1 2 3 4] where (mod this 2 | eq 0) | bend (x gives mul x 10) | sum
```
```
60
```

Three more words have no symbol. `that` is the *second* argument, so a
two-argument function needs no parameter names either:

```weave
[1 2 3 4] | braid (add this that) 0
```
```
10
```

`this` is the first argument and `that` the second, whichever order they are
written in — `braid (sub that this) 0` is a different fold, not an error.

`former` and `latter` are the two halves of the first argument. Writing either
says the value arriving is a two-part Twine the group wants opened:

```weave
[(1, 5), (3, 2), (4, 4)] as add former latter | sum
```
```
19
```

Without a `former` or `latter`, the argument stays whole, pair or not. All four
combine: `(sub that former)` takes a pair and a second argument and subtracts
one from the other.

One thing to watch: because the *brackets* bind it, nesting one call inside
another splits them up. `(eq 0 (mod _ 3))` claims the `_` for the inner
brackets, which is a type error. Pipe instead — `(mod _ 3 | eq 0)` — which is
shorter anyway. The compiler says so when you get it wrong.

---

## 10. Reading the input

`Source` is the whole of standard input, as `Air`.

```weave
Source | lines | len
```

The usual first move is to turn it into something structured:

```weave
Source | lines | bend earth | bend (otherwise 0) | sum
```

Reading a value out of text is one verb per Power, named for it: `earth`,
`water`, `fire`. Each returns a `Hold`, because reading
can fail.

When the input is a mess of numbers with punctuation between them, `earths` pulls
them all out and handles minus signs:

```weave
"move 3 from 1 to 7" | earths
```

```
3
1
7
```

When the *shape* of the line is the point rather than the numbers in it,
`delve` says the shape and hands back the runs you marked with `{}`:

```weave
"Game 11: 3 blue, 4 red" | delve "Game {}: {}"
```

```
Held [11 3 blue, 4 red]
```

Everything that is not `{}` has to match exactly, a run stops at the first place
the text after it appears, and the shape has to account for the whole line — so
a trailing `{}` is how you say "and the rest". A line that does not have the
shape you said comes back `Stilled` rather than as a guess, which makes `glean`
the natural way to read a file where only some lines are interesting:

```weave
["3-5", "not a range", "8-1"]
  | glean (l : delve "{}-{}" l)
  | bend (p : p | harvest earth | rescue [] | sum)
```

```
8
9
```

There is no regular expression engine, and there is not going to be one.
Between `earths` and `delve`, five years of Advent of Code has not wanted it.

Other things you will reach for: `words`, `blocks` (split on blank lines),
`split`, `strip`, `fires`. `split "" text` gives you one piece per character,
as text rather than as `Fire`s — `fires` is the one that gives you those.

### Earths and Waters

A number literal takes its Power from what is around it, so `1` inside a
function over Waters is a Water:

```weave
fact 1.0 is 1.0
fact n is mul n (fact (sub n 1))

fact 5.0
```
```
120.0
```

A literal nothing decides is an Earth, and that decision is made per
definition — so a definition that mentions a literal is fixed to one Power.
`half x is div x 2` is `Earth -> Earth`; to get the other one, say so:

```weave
half :: Water -> Water
half x is div x 2

half 9.0
```
```
4.5
```

A Water prints as the shortest text that reads back as the same number, and
always with a decimal point, so you can tell the two apart at a glance:

```weave
[div 1.0 3.0, 0.1, 1.0]
```
```
0.3333333333333333
0.1
1.0
```

### Taking a Thread apart

A pattern can be a Thread, which saves a lot of `nth`:

```weave
pair [a b] is add a b
pair _ is 0

total [] is 0
total [x ..rest] is add x (total rest)

[pair [3 4], pair [1 2 3], total [1 2 3 4]] | bend air | join " "
```
```
7 0 10
```

`[a b]` matches a Thread of exactly two. `..rest` takes everything left over —
and because a Thread is an array, that is a slice of the same storage, not a
copy. `[a b ..]` throws the rest away without naming it.

A list of fixed-length patterns can never be exhaustive, so it always needs a
`_`. `[]` with `[x ..rest]` is the one pair that is complete on its own.

### Building one up

A Thread is a value, so nothing changes the one you have — these hand back a new
one, and the compiler decides separately whether the old one is still needed.

```weave
xs is [1 2 3]

[ air (xs | weld [4 5])          # weld ys xs: xs with ys on the end
, air (xs | weld [0])            # so appending one is welding a Thread of one
, air ([0] | weld xs)            # and prepending is welding the other way
, air (xs | mend 1 99)           # replace a position
, air (xs | sever 1)             # cut in two, both halves
, air (plait xs [7 8 9])         # one from each in turn: zip, flattened
, air (span 1 8 | cull even)     # keep what the test turns down
] | join "\n"
```
```
[1 2 3 4 5]
[1 2 3 0]
[0 1 2 3]
[1 99 3]
([1], [2 3])
[1 7 2 8 3 9]
[1 3 5 7]
```

`mend` at a position that is not there leaves the Thread alone, the way `set`
does for a grid. And `strands` breaks a Thread into runs of adjacent elements that
have the same key, which is what counting repeats wants:

```weave
"aaabbc" | fires | strands (c : c) | bend len
```
```
3
2
1
```

---

## 11. Grids

A `Pattern` is a 2-D grid, indexed by `Knot` (a row/column pair). `Source
through pattern` reads the input as one.

```weave
sheet is "ab\ncd" | pattern

[rows sheet, cols sheet] | bend air | join " "
```

```
2 2
```

```weave
sheet is "abc\ndef" | pattern

cell sheet (knot 1 2) | otherwise ' ' | air
```

```
f
```

`cell` returns a `Hold` because the knot might be off the edge. The verbs you
want: `knots` (every coordinate), `cells` (every value), `around4`/`around8`
(the neighbouring knots that are in bounds), `nb4`/`nb8` (their values),
`cellwise` (map over every cell, keeping the shape), `set`.

Counting the `#`s that have at least two `#` neighbours:

```weave
sheet is "#.#\n###\n#.#" | pattern

busy k is
  ward cell sheet k
    Held '#' : gte 2 (nb4 sheet k | count (eq '#'))
    _ : Shadow

sheet | knots | count busy
```

```
3
```

`set g k v` returns a *new* grid — but when the compiler can see you are
threading one grid through a loop and never keeping the old version, it updates
in place. So a loop of grid updates costs what it would in Go, without you
writing anything different.

---

## 12. Maps, sets, queues

```weave
counts is [1 1 2 3 3 3] | freq

[get counts 3 | otherwise 0, len counts] | bend air | join " "
```

```
3 3
```

`freq` counts occurrences into a `Web` (a map). Note `get counts 3` — **keyed
collections take the collection first**, unlike the sequence verbs. That is
deliberate: a map is usually the fixed thing you are consulting, not the thing
flowing through. Use `_` when you want it in a pipeline.

```weave
seen is circle [1 2 3]

[member seen 2, member seen 9] | bend air | join " "
```

```
Light Shadow
```

`Circle` is a set. `Taveren` is a priority queue, but you often will not need it
directly — see the next section.

Both are hash array mapped tries, and both get the same in-place treatment as
grids when threaded through a loop, so building a set of a million things is
linear rather than a million copies.

---

## 13. Shortest paths

`dijkstra` wants one thing: a function from a place to the `(cost, place)` steps
leading out of it. It answers with the cost of reaching everywhere.

```weave
steps n is pick (gte 5 n) [] [(1, add n 1), (3, add n 2)]

get (dijkstra steps 0) 5 | otherwise (neg 1)
```

```
5
```

Because it returns the whole map, a second question about the same graph is a
lookup rather than another search.

For a maze, the step function is "the open neighbours, one step each":

```weave
sheet is "S.#\n..E" | pattern

open k is neq '#' (cell sheet k | otherwise '#')

at m is sheet | knots | seek (k : eq m (cell sheet k | otherwise ' ')) | otherwise (knot 0 0)

steps k is around4 sheet k | sift open | bend (n : (1, n))

get (dijkstra steps (at 'S')) (at 'E') | otherwise (neg 1)
```

```
3
```

---

## 14. Your own types

```weave
Direction is North | East | South | West

turn North is East
turn East is South
turn South is West
turn West is North

turn (turn North)
```

```
South
```

A constructor can carry values, and a type can take parameters and refer to
itself:

```weave
Tree a is Leaf | Node (Tree a) a (Tree a)

total Leaf is 0
total (Node l v r) is add v (add (total l) (total r))

total (Node (Node Leaf 1 Leaf) 2 (Node Leaf 3 Leaf))
```

```
6
```

Declared types **sort, print and hash for free** — you get `Eq`, `Ord` and
`Show` as long as the fields have them, so a declared value can go straight into
a set or be used as a map key:

```weave
Colour is Red | Green | Blue

[Blue, Red, Green, Red] | sort | bend air | join " "
```

```
Red Red Green Blue
```

And of course `ward` over one is checked for exhaustiveness, so adding a fourth
`Direction` turns every match on it into a compile error until you have handled
it. That is the main reason to declare one.

---

## 15. Endless sequences

`flow f seed` is `seed, f seed, f (f seed), …` and never ends.

```weave
flow (mul 2) 1 | seek (gt 1000)
```

```
Held 1024
```

Because it never ends, something has to stop it: `take`, `takewhile`, `seek`,
`first`, `any` or `all`. If nothing does, that is a compile error rather than a
program that hangs.

```weave
next n is pick (even n) (div n 2) (add 1 (mul 3 n))

flow next 27 | takewhile (neq 1) | len
```

```
111
```

---

## 16. Remembering answers

Some recursions ask the same question over and over. Put `remember` on the
definition and the answers are kept:

```weave
remember fib 0 is 0
fib 1 is 1
fib n is add (fib (sub n 1)) (fib (sub n 2))

fib 60
```

```
1548008755920
```

Without the marker that is about two billion calls. With it, sixty.

It goes once, on the first clause. Every argument has to be comparable, since
that is what the lookup uses — so a function argument is a compile error, and so
is `remember` on something that takes no arguments, because that is computed
once anyway.

---

## 17. Local names

Inside a definition, an indented block can name things before its result:

```weave
answer is
  weave nums is [1 2 3 4]
  weave total is sum nums
  mul total (len nums)

answer
```

```
40
```

`weave` names a value; `channel` names a function. The last line of the block is
what the block is.

There is a one-line form too, for when a block would be overkill:

```weave
answer is weave x is 6, y is 7 into mul x y

answer
```

---

## 18. Putting it together

Advent of Code 2020 day 1: find the two entries that sum to 2020 and multiply
them.

```weave
nums is [1721 979 366 299 675 1456]

part1 is
  weave hit is nums | seek (a : nums | any (b : eq 2020 (add a b)))
  ward hit
    Held a : mul a (sub 2020 a)
    Stilled : 0

part1
```

```
514579
```

In a real solution `nums` would be `Source | lines | bend earth | bend
(otherwise 0)`.

Day 8, a little machine, using a declared type so the compiler holds you to the
instruction set:

```weave
Op is Nop Earth | Acc Earth | Jmp Earth

arg line is line | earths | first | otherwise 0

read line is
  ward line | words | first | otherwise ""
    "acc" : Acc (arg line)
    "jmp" : Jmp (arg line)
    _ : Nop (arg line)

program is
  "nop +0\nacc +1\njmp +4\nacc +3\njmp -3\nacc -99\nacc +1\njmp -4\nacc +6"
    | lines
    | bend read

at n is program | drop n | first

step pc acc seen is pick (member seen pc) acc (advance pc acc (insert seen pc))

advance pc acc seen is
  ward at pc
    Stilled : acc
    Held (Nop _) : step (add pc 1) acc seen
    Held (Acc n) : step (add pc 1) (add acc n) seen
    Held (Jmp n) : step (add pc n) acc seen

step 0 0 (circle [])
```

```
5
```

`step` and `advance` call each other in tail position, so that runs in one stack
frame however long the program loops for.

---

## Where to go next

- **`weave verbs`** — the whole vocabulary, searchable by type. `weave verbs
  Web` when you cannot remember what maps can do.
- **[verbs.md](verbs.md)** — the same thing as a document.
- **[../SPEC.md](../SPEC.md)** — the language definition, including how the
  optimisations work and why.
- **`weave repl`** — give it your puzzle input and iterate a line at a time.
- **[nixvim.md](nixvim.md)** — editor setup: diagnostics as you type, and each
  definition's value shown at the end of its line.

Two habits worth forming early:

1. **Let `weave check` tell you what you missed.** Exhaustiveness errors are the
   language's main contribution to getting an answer right the first time.
2. **Reach for a verb before writing recursion.** There are 206 of them, and
   `weave verbs <type>` finds the one you want faster than writing it does.
