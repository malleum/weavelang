-- weave.nvim — editor support for the Weave language.
--
--   :WeaveTrace        run the program and show every definition's value
--   :WeaveTraceOff     stop, and clear the ghost text
--   :WeaveTraceToggle
--
--   :AocInput          fetch the puzzle input beside this program
--   :AocProblem        show the day's problem in a split
--   :AocSubmit         submit the answer for the part the cursor is in
--   :AocTime           how long until another answer may be sent
--
-- `weave lsp` provides diagnostics, hover, completion and formatting; this
-- plugin can start it for you, or you can leave that to your own LSP
-- configuration and use this only for the ghost text.

local M = {}

local config = require("weave.config")
local trace = require("weave.trace")
local calls = require("weave.calls")

M.trace = trace
M.calls = calls
M.config = config

--- start_lsp attaches the language server to a buffer.
---@param buf integer
local function start_lsp(buf)
  local cfg = config.get()
  vim.lsp.start({
    name = "weave",
    cmd = { cfg.cmd, "lsp" },
    root_dir = vim.fs.dirname(vim.api.nvim_buf_get_name(buf)),
  }, { bufnr = buf })
end

--- setup registers the commands and autocommands. Calling it is optional: the
--- commands exist either way, and the defaults are what most people want.
---@param opts table|nil
function M.setup(opts)
  local cfg = config.setup(opts)

  -- Also registered in plugin/weave.lua, but repeated here so that a
  -- configuration which loads this module directly still gets the filetype.
  vim.filetype.add({ extension = { weave = "weave" } })
  vim.api.nvim_set_hl(0, "WeaveTrace", { link = "Comment", default = true })

  local group = vim.api.nvim_create_augroup("weave.nvim", { clear = true })

  -- Markdown too, because a ```weave fence is a Weave program and both the
  -- language server and the tracer read it as one. The server is started for
  -- Markdown as well so hover and completion work inside a fence; tracing is
  -- not turned on automatically there, since most Markdown files have no
  -- Weave in them at all — `:WeaveTrace` asks for it.
  vim.api.nvim_create_autocmd("FileType", {
    group = group,
    pattern = { "weave", "markdown" },
    callback = function(args)
      if cfg.lsp then
        start_lsp(args.buf)
      end
      if cfg.aoc.auto_input and vim.bo[args.buf].filetype == "weave" then
        -- Quiet: most Weave files are not an Advent of Code day, and a file
        -- that is has its input already after the first time.
        require("weave.aoc").input(vim.api.nvim_buf_get_name(args.buf), { quiet = true })
      end
      if cfg.auto and vim.bo[args.buf].filetype == "weave" then
        trace.attach(args.buf)
      end
    end,
  })

  local watched = { "*.weave", "*.md", "*.markdown" }

  if cfg.on_save then
    vim.api.nvim_create_autocmd("BufWritePost", {
      group = group,
      pattern = watched,
      callback = function(args)
        trace.on_save(args.buf)
      end,
    })
  end

  vim.api.nvim_create_autocmd("BufDelete", {
    group = group,
    pattern = watched,
    callback = function(args)
      trace.detach(args.buf)
    end,
  })
end

return M
