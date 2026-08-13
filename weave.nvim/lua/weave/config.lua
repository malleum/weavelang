-- Settings for weave.nvim. Everything has a default that works, so
-- `require("weave").setup()` with no arguments is a complete configuration.

local M = {}

local defaults = {
  --- The compiler. A bare name is looked up on PATH.
  cmd = "weave",

  --- Show each definition's value at the end of its line, from the moment a
  --- Weave file is opened. Off means nothing happens until `:WeaveTrace`.
  auto = true,

  --- Re-run on write. Tracing compiles and runs the program, so this is tied
  --- to saving rather than to typing.
  on_save = true,

  --- Where the input comes from: the largest file in the program's own
  --- directory matching one of these, and failing that the largest file there
  --- that is not Weave source.
  input_patterns = { "*.txt", "*.in", "*.input", "input*" },

  --- Give up on a definition that will not finish. Tracing evaluates every
  --- definition, including ones the answer does not depend on. One that runs
  --- past this shows an hourglass instead of a value, and the rest of the file
  --- is traced without it — so a slow part two costs its own ghost text and
  --- nobody else's.
  timeout_ms = 5000,

  --- The same bargain for memory: a definition that asks for more than this
  --- many megabytes is stopped and shows `⊘`. A half-written definition is as
  --- likely to allocate the whole machine as it is to loop for ever, and this
  --- is running on every save.
  memory_mb = 6144,

  --- How wide one cell of the call window may be. A recursion is read by
  --- comparing calls, which wants columns that line up more than it wants any
  --- one value in full. See :WeaveCalls.
  call_width = 28,

  --- What the ghost text looks like.
  prefix = "  = ",
  highlight = "WeaveTrace",
  max_width = 120,

  --- Advent of Code, from inside the editor. See lua/weave/aoc.lua.
  aoc = {
    --- Fetch the input on opening a Weave file that sits in a `.../YEAR/DAY/`
    --- directory, if it is not already there. It is fetched once and never
    --- again: the input does not change.
    auto_input = true,

    --- Where the session cookie is. Empty means `advent_of_code/.session` at
    --- the top of the repository the file is in — a file rather than a
    --- setting, because a setting ends up committed and a cookie is a
    --- credential.
    session_file = "",

    --- What the fetched files are called, in the program's own directory.
    input_name = "in",
    problem_name = "problem.md",

    --- Named in every request, because the site asks that automated ones name
    --- a person. Put your own contact here.
    user_agent = "github.com/malleum/weave nvim plugin",

    host = "https://adventofcode.com",
    timeout_ms = 20000,
  },

  --- Start the language server for Weave buffers. Leave this off if your
  --- configuration already starts it — two clients on one buffer is twice the
  --- diagnostics.
  lsp = false,
}

local current = vim.deepcopy(defaults)

function M.setup(opts)
  current = vim.tbl_deep_extend("force", vim.deepcopy(defaults), opts or {})
  return current
end

function M.get()
  return current
end

M.defaults = defaults

return M
