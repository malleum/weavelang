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

## Commands

| | |
|---|---|
| `:WeaveTrace` | start showing values in this buffer |
| `:WeaveTraceOff` | stop, and clear |
| `:WeaveTraceToggle` | |
| `:WeaveInput` | report which input file it would use |
| `:AocInput` | fetch the puzzle input beside this program |
| `:AocProblem` | show the day's problem in a split (`!` refetches) |
| `:AocSubmit` | submit the answer for the part the cursor is in (`!` ignores the cooldown) |
| `:AocTime` | how long until another answer may be sent |

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
Part two only appears once part one is answered, so `:AocProblem!` refetches.

**`:AocSubmit`** runs the program, takes the answer belonging to the part the
cursor is in, and sends it. A Weave file for a day is one binding for the input
and one bare chain per part, so the first bare chain is part one and the second
is part two — put the cursor in the one you mean. The reply is read back as
right, wrong, too high, too low, or too soon, and the cooldown is remembered in
`.aoc-submitted` beside the program. **`:AocTime`** says how long is left of it.

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
  timeout_ms = 5000,        -- give up on a program that will not finish
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
  own definitions, signature help, and formatting. Set `lsp = true` above, or
  start it however you normally do.
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
does not depend on, so a definition that loops forever will hit `timeout_ms`
rather than the program finishing. That is the trade for seeing everything.
