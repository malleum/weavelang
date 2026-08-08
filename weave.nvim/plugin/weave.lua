-- Commands, registered whether or not setup() was called. `require` is cheap
-- and lazy here: nothing runs until a command does.

if vim.g.loaded_weave_nvim then
  return
end
vim.g.loaded_weave_nvim = true

-- Weave source, by extension. There is nothing else to go on: a Weave program
-- has no shebang and no header. Registering it here rather than in ftdetect/
-- means it works even where filetype detection scripts are not being sourced.
vim.filetype.add({ extension = { weave = "weave" } })

vim.api.nvim_create_user_command("WeaveTrace", function()
  require("weave.trace").attach(vim.api.nvim_get_current_buf())
end, { desc = "Show every definition's value as ghost text" })

vim.api.nvim_create_user_command("WeaveTraceOff", function()
  require("weave.trace").detach(vim.api.nvim_get_current_buf())
end, { desc = "Stop showing definition values" })

vim.api.nvim_create_user_command("WeaveTraceToggle", function()
  require("weave.trace").toggle(vim.api.nvim_get_current_buf())
end, { desc = "Toggle the definition-value ghost text" })

vim.api.nvim_create_user_command("WeaveInput", function()
  local file = vim.api.nvim_buf_get_name(0)
  local input = require("weave.trace").input_for(vim.fs.dirname(file))
  vim.notify(input and ("weave: tracing against " .. input)
    or "weave: no input file found next to this program", vim.log.levels.INFO)
end, { desc = "Report which input file tracing would use" })

-- Advent of Code. Each works out which puzzle the current file is from the
-- directories above it, so none of them takes an argument.

vim.api.nvim_create_user_command("AocInput", function()
  require("weave.aoc").input()
end, { desc = "Fetch the puzzle input beside this program" })

vim.api.nvim_create_user_command("AocProblem", function(a)
  require("weave.aoc").problem(nil, a.bang)
end, { bang = true, desc = "Show the day's problem in a split (! refetches)" })

vim.api.nvim_create_user_command("AocSubmit", function(a)
  require("weave.aoc").submit(nil, a.bang)
end, { bang = true, desc = "Submit the answer for the part the cursor is in (! ignores the cooldown)" })

vim.api.nvim_create_user_command("AocTime", function()
  require("weave.aoc").time()
end, { desc = "How long until another answer may be sent" })
