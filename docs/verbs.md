# The Weave vocabulary

Every built-in, with its type. This file is generated — `weave verbs -md` — from
`internal/prelude/prelude.go`, which the compiler parses at start-up, so the
signatures here are the signatures the type checker uses and cannot drift from
them.

At a terminal, `weave verbs` prints the same thing, and `weave verbs Knot` filters
it — by name, by type, or by description, so searching for a type answers
"what works on one of these".

Two conventions run through the whole table, and the difference is deliberate.
**Sequence transforms are data-last**, so partial application composes with the
pipeline: `sift even` is a function still waiting for its Thread. **Keyed
collections take the collection first**, because a grid or a map is usually the
fixed thing being consulted rather than the thing flowing through. `_` bridges
the two when you want the second form in a chain: `w | get _ "a"`.

## Contents

- [Input](#input) — 1
- [Sequences](#sequences) — 76
- [Ranges](#ranges) — 6
- [Text](#text) — 23
- [Absence and failure](#absence-and-failure) — 4
- [Grids](#grids) — 22
- [Maps](#maps) — 10
- [Sets](#sets) — 8
- [Priority queues and graphs](#priority-queues-and-graphs) — 7
- [Numbers](#numbers) — 33
- [Comparison](#comparison) — 6
- [Logic](#logic) — 4
- [Characters](#characters) — 6
- [Constructors](#constructors) — 7

## Input

Read once, whatever the program does with it.

| | |
|---|---|
| `Source :: Air` | the program's input |

## Sequences

`Thread a`. A chain of these fuses into a single pass with no intermediate Thread, and one that can answer early — `seek`, `first`, `any`, `all` — stops the whole chain there.

| | |
|---|---|
| `bend :: (a -> b) -> Thread a -> Thread b` | map a function over a Thread |
| `sift :: (a -> Spirit) -> Thread a -> Thread a` | keep the elements that satisfy a test |
| `braid :: (b -> a -> b) -> b -> Thread a -> b` | fold a Thread into a single value |
| `seek :: (a -> Spirit) -> Thread a -> Hold a` | the first element satisfying a test |
| `span :: Earth -> Earth -> Thread Earth` | the inclusive range between two Earths |
| `flow :: (a -> a) -> a -> Thread a` | the endless Thread seed, f seed, f (f seed), ... |
| `len :: a -> Earth  where Bulk a` | how many elements a collection or text holds |
| `count :: (a -> Spirit) -> Thread a -> Earth` | how many elements satisfy a test |
| `sum :: Thread a -> a  where Reckon a` | add every element together |
| `prod :: Thread a -> a  where Reckon a` | multiply every element together |
| `sums :: Thread a -> Thread a  where Reckon a` | the running totals |
| `prods :: Thread a -> Thread a  where Reckon a` | the running products |
| `take :: Earth -> Thread a -> Thread a` | the first n elements |
| `drop :: Earth -> Thread a -> Thread a` | everything after the first n elements |
| `takewhile :: (a -> Spirit) -> Thread a -> Thread a` | the leading run that satisfies a test |
| `dropwhile :: (a -> Spirit) -> Thread a -> Thread a` | everything after that leading run |
| `zip :: Thread a -> Thread b -> Thread (a, b)` | pair two Threads element by element |
| `zipwith :: (a -> b -> c) -> Thread a -> Thread b -> Thread c` | combine two Threads element by element |
| `thread :: (a, a) -> Thread a` | the two halves of a Twine as a Thread |
| `weld :: Thread a -> Thread a -> Thread a` | weld ys xs: xs with ys on the end |
| `mend :: Earth -> a -> Thread a -> Thread a` | mend i x xs: xs with position i replaced, or xs when there is no such position |
| `sever :: Earth -> Thread a -> (Thread a, Thread a)` | cut a Thread in two at a position |
| `strands :: (a -> b) -> Thread a -> Thread (Thread a)  where Eq b` | runs of adjacent elements whose derived key agrees |
| `plait :: Thread a -> Thread a -> Thread a` | plait as bs: one from each in turn, stopping with the shorter. `zip` flattened |
| `cull :: (a -> Spirit) -> Thread a -> Thread a` | keep what the test turns down: sift the other way round |
| `bendr :: (a -> b) -> Thread (Thread a) -> Thread (Thread b)` | transform every element one level deeper |
| `siftr :: (a -> Spirit) -> Thread (Thread a) -> Thread (Thread a)` | keep the elements passing a test, one level deeper |
| `zipr :: (a -> b -> c) -> Thread (Thread a) -> Thread (Thread b) -> Thread (Thread c)` | combine two Threads of Threads element by element |
| `sort :: Thread a -> Thread a  where Ord a` | order a Thread |
| `sortby :: (a -> b) -> Thread a -> Thread a  where Ord b` | order by a derived key |
| `all :: (a -> Spirit) -> Thread a -> Spirit` | does every element satisfy the test |
| `any :: (a -> Spirit) -> Thread a -> Spirit` | does any element satisfy the test |
| `none :: (a -> Spirit) -> Thread a -> Spirit` | does no element satisfy the test |
| `first :: Thread a -> Hold a` | the first element, if there is one |
| `second :: Thread a -> Hold a` | the second element, if there is one |
| `last :: Thread a -> Hold a` | the final element, if there is one |
| `head :: Thread a -> Hold a` | the first element, if there is one |
| `tail :: Thread a -> Thread a` | everything after the first element |
| `rev :: Thread a -> Thread a` | the same elements, back to front |
| `flat :: Thread (Thread a) -> Thread a` | flatten a Thread of Threads |
| `uniq :: Thread a -> Thread a  where Eq a` | drop repeated elements |
| `enum :: Thread a -> Thread (Earth, a)` | pair each element with its position |
| `scan :: (b -> a -> b) -> b -> Thread a -> Thread b` | braid keeping every running total |
| `gentle :: (b -> a -> Weaving b c) -> b -> Thread a -> Weaving b c` | braid that stops when the step answers Gentled |
| `dupe :: Thread a -> Hold (Earth, a)  where Eq a` | where the first repeat is, and what it is |
| `high :: Thread a -> Hold a  where Ord a` | the largest element |
| `low :: Thread a -> Hold a  where Ord a` | the smallest element |
| `highidx :: Thread a -> Hold Earth  where Ord a` | where the largest element is |
| `lowidx :: Thread a -> Hold Earth  where Ord a` | where the smallest element is |
| `seekidx :: (a -> Spirit) -> Thread a -> Hold Earth` | where the first element satisfying a test is |
| `twist :: Earth -> (a -> a) -> Thread a -> Thread a` | twist i f xs: xs with position i put through f, or xs when there is no such position |
| `top :: Earth -> Thread a -> Thread a  where Ord a` | the n largest elements |
| `bot :: Earth -> Thread a -> Thread a  where Ord a` | the n smallest elements |
| `maxby :: (a -> b) -> Thread a -> Hold a  where Ord b` | the element with the largest key |
| `minby :: (a -> b) -> Thread a -> Hold a  where Ord b` | the element with the smallest key |
| `pairs :: Thread a -> Thread (a, a)` | each element paired with the next |
| `cross :: Thread a -> Thread b -> Thread (a, b)` | every combination of two Threads |
| `combos :: Earth -> Thread a -> Thread (Thread a)` | every n-element combination |
| `perms :: Thread a -> Thread (Thread a)` | every ordering of a Thread |
| `compact :: Thread (Hold a) -> Thread a` | drop the Stilled entries, unwrap the rest |
| `mapcat :: (a -> Thread b) -> Thread a -> Thread b` | bend then flatten |
| `chunk :: Earth -> Thread a -> Thread (Thread a)` | split into runs of n |
| `windows :: Earth -> Thread a -> Thread (Thread a)` | every overlapping run of n |
| `pivot :: Thread (Thread a) -> Thread (Thread a)` | swap rows and columns |
| `group :: (a -> b) -> Thread a -> Web b (Thread a)  where Eq b` | gather by a derived key |
| `idx :: a -> Thread a -> Hold Earth  where Eq a` | where a value first occurs |
| `nth :: Earth -> Thread a -> Hold a` | the element at a position, if there is one |
| `has :: a -> Thread a -> Spirit  where Eq a` | does this Thread hold this value |
| `glean :: (a -> Hold b) -> Thread a -> Thread b` | bend, keeping only what came back Held |
| `harvest :: (a -> Hold b) -> Thread a -> Weaving (Thread b) a` | glean, but Gentled with the first element that would not convert |
| `cycle :: Thread a -> Thread a` | the same Thread over and over, endlessly |
| `freq :: Thread a -> Web a Earth  where Eq a` | count how often each element occurs |
| `most :: Web a Earth -> Hold a` | the key with the highest count |
| `contains :: Air -> Air -> Spirit` | contains needle haystack |
| `earths :: Air -> Thread Earth` | every Earth appearing in some text |
| `waters :: Air -> Thread Water` | every Water appearing in some text |

## Ranges

| | |
|---|---|
| `overlaps :: (a, a) -> (a, a) -> Spirit  where Ord a` | do two inclusive ranges meet at all |
| `overlapping :: (a, a) -> (a, a) -> Hold (a, a)  where Ord a` | the range two inclusive ranges share, if they meet |
| `within :: (a, a) -> (a, a) -> Spirit  where Ord a` | within outer inner: does the first range hold all of the second |
| `spanning :: (a, a) -> (a, a) -> (a, a)  where Ord a` | the smallest range holding both, gaps included |
| `holding :: (a, a) -> a -> Spirit  where Ord a` | is this value inside an inclusive range |
| `width :: (Earth, Earth) -> Earth` | how many Earths an inclusive range holds |

## Text

`Air`. Text is a `Bulk` type, so `len` counts its characters.

| | |
|---|---|
| `lines :: Air -> Thread Air` | split text into lines |
| `words :: Air -> Thread Air` | split text on whitespace |
| `fires :: Air -> Thread Fire` | the characters of some text |
| `blocks :: Air -> Thread Air` | split text on blank lines |
| `split :: Air -> Air -> Thread Air` | split text on a separator |
| `join :: Air -> Thread Air -> Air` | join text with a separator |
| `strip :: Air -> Air` | remove surrounding whitespace |
| `air :: a -> Air  where Show a` | render any value as text |
| `earth :: Air -> Hold Earth` | the Earth this text spells, if it spells one |
| `water :: Air -> Hold Water` | the Water this text spells, if it spells one |
| `fire :: Air -> Hold Fire` | the one Fire this text holds, if it holds one |
| `upper :: Air -> Air` | the same text in upper case |
| `lower :: Air -> Air` | the same text in lower case |
| `padl :: Earth -> Fire -> Air -> Air` | pad on the left to a width |
| `padr :: Earth -> Fire -> Air -> Air` | pad on the right to a width |
| `starts :: Air -> Air -> Spirit` | starts prefix text: does text begin with prefix |
| `ends :: Air -> Air -> Spirit` | ends suffix text: does text end with suffix |
| `cutstart :: Air -> Air -> Air` | remove a prefix if it is there |
| `cutend :: Air -> Air -> Air` | remove a suffix if it is there |
| `replace :: Air -> Air -> Air -> Air` | replace needle with text everywhere |
| `delve :: Air -> Air -> Hold (Thread Air)` | take a line apart against a shape: `{}` keeps a run, everything else must match |
| `repeat :: Earth -> Air -> Air` | text repeated n times |
| `bin :: Earth -> Air` | the binary digits of an Earth |

## Absence and failure

`Hold a` is `Held a | Stilled` and `Weaving a e` is `Woven a | Gentled e`. There is no null: the compiler makes you handle the empty case.

| | |
|---|---|
| `otherwise :: a -> Hold a -> a` | unwrap a Hold, or use a default |
| `holds :: Hold a -> Spirit` | does this Hold contain a value |
| `rescue :: a -> Weaving a e -> a` | unwrap a Weaving, or use a default |
| `snag :: e -> Weaving a e -> e` | the value a Weaving stopped on, or a default |

## Grids

`Pattern a`, indexed by `Knot`. A grid threaded through a loop is updated in place rather than copied.

| | |
|---|---|
| `pattern :: Air -> Pattern Fire` | read text as a Pattern of Fires |
| `weft :: a -> Thread (Thread a) -> Pattern a` | weave rows into a Pattern, padding short rows |
| `spin :: Pattern a -> Pattern a` | a quarter turn clockwise |
| `flip :: Pattern a -> Pattern a` | mirrored left to right |
| `cell :: Pattern a -> Knot -> Hold a` | the cell at a knot, if in bounds |
| `set :: Pattern a -> Knot -> a -> Pattern a` | a grid with one cell replaced |
| `cellwise :: (a -> b) -> Pattern a -> Pattern b` | transform every cell, keeping the grid's shape |
| `knots :: Pattern a -> Thread Knot` | every coordinate of a grid |
| `cells :: Pattern a -> Thread a` | every cell of a grid |
| `rows :: Pattern a -> Earth` | how many rows a grid has |
| `cols :: Pattern a -> Earth` | how many columns a grid has |
| `shape :: Pattern a -> (Earth, Earth)` | the rows and columns of a grid |
| `inb :: Pattern a -> Knot -> Spirit` | is this knot inside the grid |
| `nb4 :: Pattern a -> Knot -> Thread a` | the four orthogonal neighbours |
| `nb8 :: Pattern a -> Knot -> Thread a` | the eight surrounding neighbours |
| `around4 :: Pattern a -> Knot -> Thread Knot` | the four neighbouring knots in bounds |
| `around8 :: Pattern a -> Knot -> Thread Knot` | the eight neighbouring knots in bounds |
| `row :: Knot -> Earth` | the row of a knot |
| `col :: Knot -> Earth` | the column of a knot |
| `dirs4 :: Thread Knot` | the four orthogonal steps |
| `dirs8 :: Thread Knot` | the eight surrounding steps |
| `mdist :: Knot -> Knot -> Earth` | the Manhattan distance between two knots |

## Maps

`Web k v`, a hash array mapped trie. Keyed access takes the collection first, so `w | get _ k` is how it joins a pipeline.

| | |
|---|---|
| `web :: Thread (k, v) -> Web k v  where Eq k` | build a Web from pairs |
| `get :: Web k v -> k -> Hold v  where Eq k` | look up a key |
| `put :: Web k v -> k -> v -> Web k v  where Eq k` | a Web with one key set |
| `known :: Web k v -> k -> Spirit  where Eq k` | is this key present |
| `forget :: Web k v -> k -> Web k v  where Eq k` | a Web with one key removed |
| `keys :: Web k v -> Thread k` | every key |
| `vals :: Web k v -> Thread v` | every value |
| `items :: Web k v -> Thread (k, v)` | every key and value together |
| `merge :: Web k v -> Web k v -> Web k v  where Eq k` | merge two Webs, the second winning |
| `mapvals :: (v -> w) -> Web k v -> Web k w  where Eq k` | transform every value, keeping the keys |

## Sets

`Circle a`, sharing the map's trie.

| | |
|---|---|
| `circle :: Thread a -> Circle a  where Eq a` | gather a Thread into a Circle |
| `member :: Circle a -> a -> Spirit  where Eq a` | is this value in the Circle |
| `insert :: Circle a -> a -> Circle a  where Eq a` | a Circle with one value added |
| `remove :: Circle a -> a -> Circle a  where Eq a` | a Circle with one value removed |
| `members :: Circle a -> Thread a` | every value in a Circle |
| `union :: Circle a -> Circle a -> Circle a  where Eq a` | everything in either Circle |
| `inter :: Circle a -> Circle a -> Circle a  where Eq a` | everything in both Circles |
| `diff :: Circle a -> Circle a -> Circle a  where Eq a` | everything in the first but not the second |

## Priority queues and graphs

`Taveren a` is a leftist heap. Every graph verb here takes the same thing — a function from a place to the steps out of it — which works on an implicit graph, a grid or a state machine, as readily as on one you built. An explicit graph is a `Web k (Thread k)`, which `group` and `mapvals` make in a line.

| | |
|---|---|
| `taveren :: Thread a -> Taveren a  where Ord a` | build a queue from a Thread |
| `push :: Taveren a -> a -> Taveren a  where Ord a` | add a value to the queue |
| `pop :: Taveren a -> Hold (a, Taveren a)  where Ord a` | take the smallest value |
| `dijkstra :: (a -> Thread (Earth, a)) -> a -> Web a Earth  where Ord a` | cheapest cost to every node reachable from here, given a step function |
| `reach :: (a -> Thread a) -> a -> Circle a  where Eq a` | every place reachable from here, given a step function |
| `route :: (a -> Thread (Earth, a)) -> a -> a -> Hold (Thread a)  where Ord a` | the cheapest path from one place to another, if there is one |
| `toposort :: (a -> Thread a) -> Thread a -> Hold (Thread a)  where Eq a` | the nodes ordered so every edge points forwards, or Stilled on a cycle |

## Numbers

There are no operators anywhere in Weave: arithmetic is these verbs. The `Reckon` Talent is Earth and Water.

| | |
|---|---|
| `add :: a -> a -> a  where Reckon a` | add two numbers |
| `sub :: a -> a -> a  where Reckon a` | subtract the second from the first |
| `mul :: a -> a -> a  where Reckon a` | multiply two numbers |
| `div :: a -> a -> a  where Reckon a` | divide the first by the second |
| `mod :: Earth -> Earth -> Earth` | remainder after division |
| `gcd :: Earth -> Earth -> Earth` | greatest common divisor |
| `lcm :: Earth -> Earth -> Earth` | least common multiple |
| `inc :: a -> a  where Reckon a` | one more |
| `dec :: a -> a  where Reckon a` | one less |
| `abs :: a -> a  where Reckon a` | magnitude, without sign |
| `neg :: a -> a  where Reckon a` | the negation of a number |
| `min :: a -> a -> a  where Ord a` | the smaller of two values |
| `max :: a -> a -> a  where Ord a` | the larger of two values |
| `even :: Earth -> Spirit` | is this divisible by two |
| `odd :: Earth -> Spirit` | is this not divisible by two |
| `divBy :: Earth -> Earth -> Spirit` | divBy d n: is n divisible by d |
| `sign :: a -> Earth  where Reckon a` | -1, 0 or 1 |
| `sqrt :: a -> Water  where Reckon a` | square root |
| `cbrt :: a -> Water  where Reckon a` | cube root |
| `ceil :: a -> Earth  where Reckon a` | round up |
| `floor :: a -> Earth  where Reckon a` | round down |
| `round :: a -> Earth  where Reckon a` | round to nearest |
| `clamp :: a -> a -> a -> a  where Ord a` | clamp lo hi x: hold x between two bounds |
| `pow :: a -> a -> a  where Reckon a` | pow base exponent |
| `bor :: Earth -> Earth -> Earth` | bitwise or |
| `band :: Earth -> Earth -> Earth` | bitwise and |
| `bxor :: Earth -> Earth -> Earth` | bitwise exclusive or |
| `bnot :: Earth -> Earth` | bitwise complement |
| `shl :: Earth -> Earth -> Earth` | shl n x: shift x left by n |
| `shr :: Earth -> Earth -> Earth` | shr n x: shift x right by n |
| `pi :: Water` | the circle constant |
| `e :: Water` | the base of the natural logarithm |
| `inf :: Water` | positive infinity |

## Comparison

`Eq` and `Ord` are Talents, so these work on anything that has them — including a type you declared, which derives both.

| | |
|---|---|
| `eq :: a -> a -> Spirit  where Eq a` | are two values equal |
| `neq :: a -> a -> Spirit  where Eq a` | are two values different |
| `lt :: a -> a -> Spirit  where Ord a` | lt b a: is a less than b |
| `lte :: a -> a -> Spirit  where Ord a` | lte b a: is a at most b |
| `gt :: a -> a -> Spirit  where Ord a` | gt b a: is a greater than b |
| `gte :: a -> a -> Spirit  where Ord a` | gte b a: is a at least b |

## Logic

`Spirit` is `Light` and `Shadow`. `pick` is the conditional, and evaluates only the branch it takes.

| | |
|---|---|
| `and :: Spirit -> Spirit -> Spirit` | are both true |
| `or :: Spirit -> Spirit -> Spirit` | is either true |
| `not :: Spirit -> Spirit` | the opposite |
| `pick :: Spirit -> a -> a -> a` | pick c a b: a when c is Light, else b |

## Characters

`Fire`.

| | |
|---|---|
| `isDigit :: Fire -> Spirit` | is this character a digit |
| `isAlpha :: Fire -> Spirit` | is this character a letter |
| `isSpace :: Fire -> Spirit` | is this character whitespace |
| `ord :: Fire -> Earth` | the code point of a Fire |
| `spark :: Earth -> Fire` | the Fire with a code point |
| `digit :: Fire -> Hold Earth` | the value of a decimal Fire |

## Constructors

How a value of a built-in sum type is made, and how a pattern takes it apart.

| | | |
|---|---|---|
| `Light :: Spirit` | `Spirit` | truth |
| `Shadow :: Spirit` | `Spirit` | falsehood |
| `Held :: a -> Hold a` | `Hold` | a value that is present |
| `Stilled :: Hold a` | `Hold` | nothing here: Weave's answer to null |
| `Woven :: a -> Weaving a e` | `Weaving` | a successful result |
| `Gentled :: e -> Weaving a e` | `Weaving` | a failed result, with a reason |
| `knot :: Earth -> Earth -> Knot` | `Knot` | a grid coordinate |

## Special forms

One name the checker handles itself, because it does not have an ordinary type.

| | |
|---|---|
| `Source :: Air` | the program's input, read once |
