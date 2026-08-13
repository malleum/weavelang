# weave.nvim

Editor support for [Weave](../README.md): every definition's value shown at the
end of its line, run against the real puzzle input.

![what it does](#) <!-- one line per definition, in Comment colour, at end of line -->

```
nums is Source | lines | bend earth | bend (otherwise 0)   = [1721 979 366 299 675 1456]

part1 is                                                           = 514579
  weave hit is nums | seek (a : nums | any (b : eq 2020 (add a b)))
  ward hit
    Held a : mul a (sub 2020 a)
    Stilled : 0
```

## What it does

On save it runs `weave trace` over the buffer, with **the largest input file in
the program's own directory** on stdin, and puts each definition's value at the
end of the line it was defined on. A definition that takes arguments has no
value, so its inferred type is shown instead.

The largest file is the right one for Advent of Code: the real input is always
bigger than the sample you pasted in to check your answer.

Inside a function body a line has no single value — it holds a different one on
every call — so what is shown there is the **first** value each binding ever
holds. That is the call you would have made by hand, and it survives a
definition that never finishes: a loop that times out still shows what its first
step held.

A definition that runs out of time shows **`⧖`**, and one that asks for more
memory than `memory_mb` shows **`⊘`**. Neither is a value. The rest of the file
is traced without it, so one slow part two costs its own ghost text and nobody
else's — the compiler does that itself, which is why this plugin no longer kills
`weave trace` and throws away everything it had said.

**`:WeaveCalls`** is for the rest of a recursion. Put the cursor in a function
and it shows what that function's names held on *each* call, in the window an
LSP hover uses — a column per name, a row per call:

```
loop — 12 calls

call  d        c              i  h  m  =
────  ───────  ─────────────  ─  ─  ─  ──────────
   1  0 2 7 0  {}             2  7  2  {0 2 7 0}…
   2  2 3 1 2  {0 2 7 0}      1  3  1  {0 2 7 0…
```

It runs on demand rather than on save, and for good reason: recording per call
costs the fusion inside the body, and a watched function is not inlined into a
fused loop, so it is a real difference in what gets compiled. The first calls
and the last are kept with the count between them, since a base case that never
fires shows up in the head and a loop that will not settle shows up in the tail.

## Commands

| | |
|---|---|
| `:WeaveTrace` | start showing values in this buffer |
| `:WeaveTraceOff` | stop, and clear |
| `:WeaveTraceToggle` | |
| `:WeaveInput` | report which input file it would use |
| `:WeaveCalls` | what the enclosing function's names held, call by call |
| `:AocInput` | fetch the puzzle input beside this program |
| `:AocProblem` | show the day's problem in a split (`!` refetches) |
| `:AocSubmit` | submit the answer for the part the cursor is in (`!` sends it anyway) |
| `:AocTime` | what has been answered, and how long until another may be sent |

## Advent of Code

Four commands, and none of them takes an argument: which puzzle a file belongs
to is read off the directories above it. A tree laid out `.../2017/5/pt2.weave`
— or `.../2024/day05/` — needs no configuration at all.

The session cookie is read from a **file**, not a setting, because a setting
ends up committed and a cookie is a credential. By default that is
`advent_of_code/.session` at the top of the repository the file is in.

```
$ mkdir -p advent_of_code && cp /dev/stdin advent_of_code/.session
<paste the session cookie>
$ echo advent_of_code/.session >> .gitignore
```

**`:AocInput`** fetches the input into the program's own directory as `in`. It
is fetched once and never again — the input does not change, and asking twice
is asking the site for something it already gave. Opening a `.weave` file in a
day directory does this by itself, quietly, which is why most days need the
command not at all.

**`:AocProblem`** fetches the day's text, renders it as Markdown into
`problem.md`, and opens it in a vertical split on the right **without taking
the cursor out of the file you are editing**. Every call after the first reads
the saved copy, so the site is asked once however often the window is closed.
Part two only appears once part one is answered — which a right answer now does
for you: the day is refetched and a window already showing it is scrolled to
`--- Part Two ---`. It opens no window that was not open, because a submission
is not a request to read. `:AocProblem!` refetches by hand if you want it.

**`:AocSubmit`** runs the program, takes the answer belonging to the part the
cursor is in, and sends it. A Weave file for a day is one binding for the input
and one bare chain per part, so the first bare chain is part one and the second
is part two — put the cursor in the one you mean. The reply is read back as
right, wrong, too high, too low, or too soon, and the cooldown is remembered in
`.aoc-submitted` beside the program.

Every submission is also appended to `.aoc-answers` beside it: when, which part,
the answer, and what came back. **Neither file should be committed.** That
ledger is what lets `:AocSubmit` **refuse** an answer the record already rules
out — the same answer graded before is graded the same way now, and an answer at
or past a bound already found is past it. `:AocSubmit!` sends it anyway, which
is what the bang already meant for the wait. Only a verdict the site actually
reached counts: "too soon" means the answer was never graded, so repeating one
is allowed.

**`:AocTime`** reads it back:

```
aoc: 3m 20s to wait — last: too high
part 1: 4000 (too low), 9000 (too high), 5500 (wrong)
  between 4000 and 9000
part 2: 17 (right)
```

The bracket is the whole reason the site bothers to say *which way* you were
wrong, and it is thrown away if nobody writes it down.

## Setup

```lua
require("weave").setup()
```

Everything below is a default, so the call above is a complete configuration.

```lua
require("weave").setup({
  cmd = "weave",            -- the compiler; a bare name is looked up on PATH
  auto = true,              -- start tracing when a .weave file is opened
  on_save = true,           -- re-run on write
  input_patterns = { "*.txt", "*.in", "*.input", "input*" },
  timeout_ms = 5000,        -- give up on a *definition* that will not finish
  memory_mb = 6144,         -- and on one that asks for more than this
  call_width = 28,          -- how wide one cell of the :WeaveCalls window may be
  prefix = "  = ",
  highlight = "WeaveTrace", -- links to Comment by default
  max_width = 120,
  lsp = false,              -- start `weave lsp` too; leave off if you already do

  aoc = {
    auto_input = true,      -- fetch the input on opening a day's file
    session_file = "",      -- empty means <git root>/advent_of_code/.session
    input_name = "in",
    problem_name = "problem.md",
    user_agent = "github.com/malleum/weave nvim plugin",  -- put your contact here
    host = "https://adventofcode.com",
    timeout_ms = 20000,
  },
})
```

The user agent names a person because the site asks that automated requests do.
Put your own contact in it.

## The rest of the editor story

- **`weave lsp`** — diagnostics, hover, completion over the built-ins and your
  own definitions, signature help, and formatting. Hover answers where a name is
  *bound* as well as where it is used — a parameter, a `weave` name, a name a
  pattern takes apart — which is the half anybody actually hovers. Set
  `lsp = true` above, or start it however you normally do. If it ever
  misbehaves, `WEAVE_LSP_LOG=/tmp/weave-lsp.log` makes it append every method,
  its timing, and any panic with its stack.
- **`tree-sitter-weave`** — highlighting, folding, and ```weave blocks inside
  Markdown. It lives in [`tree-sitter-weave/`](../tree-sitter-weave), and the
  flake exposes it as a package `nvim-treesitter` can take directly.

## Why it needs a save

Tracing compiles the program and runs it: about 100 ms warm, since the runtime
is cached as object files and only your program is compiled each time. That is
fast enough for a save hook and not fast enough for every keystroke — which is
also the honest thing, because a value that changes as you type mid-expression
is noise.

Note that tracing evaluates **every** definition, including ones the answer
does not depend on. `timeout_ms` is the limit for *one* definition rather than
for the whole file, and `memory_mb` is the same bargain for memory: one that
runs past either is marked and the rest of the file is traced without it. This
plugin's own kill is a backstop at eight times the timeout, for a compiler that
hangs rather than a program that is slow.
