-- Looking inside a function that runs a million times.
--
-- Ghost text answers "what does this line hold", which has one answer for a
-- top-level definition and no answer at all for a line inside a function body:
-- it holds a different thing on every call. So the inside of a recursive
-- function is the one place in a Weave program with no ghost text, and it is
-- where a bug is most likely to be.
--
-- `weave trace -watch f` records what f's names held on each call instead, and
-- this shows those records in a floating window — the same window an LSP hover
-- uses, because it is the same question asked of a different thing.
--
-- It runs on demand rather than on save. Recording per call costs the fusion
-- inside the body, so it is paid for one function at a time and only when it is
-- asked for.

local M = {}

local config = require("weave.config")

--- enclosing finds the top-level definition the cursor is inside, by the only
--- structure the language needs for it: a top-level item starts in column zero
--- and everything under it is indented. Scanning up for the nearest line that
--- starts in column zero is the same rule the compiler's own salvage uses.
---@param buf integer
---@param lnum integer 1-based
---@return string|nil name, integer|nil line
function M.enclosing(buf, lnum)
  local lines = vim.api.nvim_buf_get_lines(buf, 0, lnum, false)
  for i = #lines, 1, -1 do
    local line = lines[i]
    if line ~= "" and not line:match("^[ \t]") then
      -- The nearest line in column zero is the item the cursor is in, whatever
      -- it turns out to be. A comment in column zero ends the item above it
      -- rather than belonging to it, which is how the compiler's own salvage
      -- reads one, and a comment is not a definition — so it has no name and
      -- there is nothing to watch.
      local name = line:match("^([%a_][%w_'?!-]*)")
      if name then
        return name, i
      end
      return nil, nil
    end
  end
  return nil, nil
end

--- parse reads the `@` records `-watch` adds to the trace output.
---
--- One record per name per call: `@LINE<TAB>CALL<TAB>NAME<TAB>VALUE`, with the
--- result of a call under the empty name and the total under `calls` at call
--- zero. Columns come out in the order they were first seen, which is the order
--- they are written in the source.
---@param out string
---@return table[] calls, string[] columns, integer total
function M.parse(out)
  local rows, order, seen = {}, {}, {}
  local calls, total = {}, 0

  for line in vim.gsplit(out, "\n", { plain = true }) do
    local call, name, value = line:match("^@%-?%d+\t(%d+)\t([^\t]*)\t(.*)$")
    if call then
      call = tonumber(call)
      if name == "calls" and call == 0 then
        total = tonumber(value) or 0
      else
        if name == "" then
          name = "="
        end
        if not seen[name] then
          seen[name] = true
          order[#order + 1] = name
        end
        if not rows[call] then
          rows[call] = {}
          calls[#calls + 1] = call
        end
        rows[call][name] = value
      end
    end
  end

  -- What a call answered goes last, wherever it was first seen. It is the
  -- widest thing in the table and the one read least often, so it belongs at
  -- the edge where it can be cut without moving anything else.
  for i, col in ipairs(order) do
    if col == "=" then
      table.remove(order, i)
      order[#order + 1] = "="
      break
    end
  end

  table.sort(calls)
  local out_rows = {}
  for _, call in ipairs(calls) do
    out_rows[#out_rows + 1] = { call = call, values = rows[call] }
  end
  if total == 0 then
    total = #out_rows
  end
  return out_rows, order, total
end

--- width measures a cell as the screen will.
local function width(text)
  return vim.fn.strdisplaywidth(text)
end

--- clip shortens a cell to a column, marking that it was cut.
local function clip(text, w)
  if width(text) <= w then
    return text
  end
  if w <= 1 then
    return "…"
  end
  -- strcharpart counts characters, and a clipped value is usually a rendered
  -- collection whose characters are one column each; a wide one costs a column
  -- of padding and nothing worse.
  return vim.fn.strcharpart(text, 0, w - 1) .. "…"
end

--- fit shrinks column widths until the whole row fits the budget, taking from
--- the widest column each time.
---
--- The widest column is the one holding a rendered collection, and it is the
--- one that can spare the room: `{0 2 7 0 …` says what a Circle is doing as
--- well as the whole of it does, while `call` and a one-digit index say nothing
--- at all once they are cut. Taking from the widest keeps every narrow column
--- whole, which is what makes the table readable down a column.
local function fit(widths, budget, gap)
  local function total()
    local sum = gap * (#widths - 1)
    for _, w in ipairs(widths) do
      sum = sum + w
    end
    return sum
  end

  local floor = 3
  while total() > budget do
    local widest, at = 0, nil
    for i, w in ipairs(widths) do
      if w > widest then
        widest, at = w, i
      end
    end
    if not at or widest <= floor then
      break
    end
    widths[at] = widths[at] - 1
  end
end

--- render lays the calls out as a table: a column per name, a row per call.
---
--- No outer borders and no pipes. A hover window is narrow and every column of
--- it is a column of values not shown, so the table spends its room on the
--- values and marks the columns with space and a rule — which is also what
--- makes it readable at a glance down a column, the way a recursion is read.
---@param name string
---@param rows table[]
---@param columns string[]
---@param total integer how many calls there were, kept and not
---@param budget integer how many columns the window has
---@return string[]
function M.render(name, rows, columns, total, budget)
  local count = ("**%s** — %d call%s"):format(name, total, total == 1 and "" or "s")
  if #rows == 0 then
    return { ("`%s` was never called."):format(name) }
  end

  local header = { "call" }
  for _, col in ipairs(columns) do
    header[#header + 1] = col
  end

  -- The cells, unclipped, so the columns are sized against what is there.
  local body = {}
  local previous = nil
  for _, row in ipairs(rows) do
    if previous and row.call > previous + 1 then
      local gap = { "⋮" }
      for _ = 1, #columns do
        gap[#gap + 1] = "⋮"
      end
      body[#body + 1] = gap
    end
    local cells = { tostring(row.call) }
    for _, col in ipairs(columns) do
      cells[#cells + 1] = row.values[col] or ""
    end
    body[#body + 1] = cells
    previous = row.call
  end

  local widths = {}
  for i, text in ipairs(header) do
    widths[i] = width(text)
  end
  for _, cells in ipairs(body) do
    for i, text in ipairs(cells) do
      widths[i] = math.max(widths[i] or 0, width(text))
    end
  end
  fit(widths, budget, 2)

  -- The call number reads as a count, so it is right-aligned; a value reads as
  -- text and is not.
  local function row_text(cells, right_first)
    local parts = {}
    for i = 1, #header do
      local text = clip(cells[i] or "", widths[i])
      local pad = string.rep(" ", math.max(widths[i] - width(text), 0))
      if i == 1 and right_first then
        parts[i] = pad .. text
      else
        parts[i] = text .. pad
      end
    end
    return (table.concat(parts, "  "):gsub("%s+$", ""))
  end

  local rule = {}
  for i = 1, #header do
    rule[i] = string.rep("─", widths[i])
  end

  -- Fenced, so that the client renders the table as the fixed-width block it
  -- is rather than reading the columns as Markdown and reflowing them.
  local lines = { count, "", "```", row_text(header), row_text(rule) }
  for _, cells in ipairs(body) do
    lines[#lines + 1] = row_text(cells, true)
  end
  lines[#lines + 1] = "```"
  if #rows < total then
    lines[#lines + 1] = ("_%d call%s in between are not kept_")
      :format(total - #rows, total - #rows == 1 and "" or "s")
  end
  return lines
end

--- show opens the window for the function the cursor is in.
---@param buf integer|nil
function M.show(buf)
  buf = buf or vim.api.nvim_get_current_buf()
  local file = vim.api.nvim_buf_get_name(buf)
  if file == "" then
    return
  end
  local lnum = vim.api.nvim_win_get_cursor(0)[1]
  local name = M.enclosing(buf, lnum)
  if not name then
    vim.notify("weave: the cursor is not inside a definition", vim.log.levels.INFO)
    return
  end

  -- Required here rather than at the top so that the parts of this module with
  -- no editor in them can be checked without one. See test/calls_spec.lua.
  local trace = require("weave.trace")
  local cfg = config.get()
  local dir = vim.fs.dirname(file)
  local cmd = {
    cfg.cmd,
    "trace",
    "-watch",
    name,
    "-timeout",
    cfg.timeout_ms .. "ms",
    "-memory",
    tostring(cfg.memory_mb),
    file,
  }
  vim.system(cmd, {
    stdin = trace.source_for(dir),
    cwd = dir,
    timeout = cfg.timeout_ms * 8,
  }, function(res)
    local out = res.stdout or ""
    -- Everything happens on the main loop. This callback runs in a fast event
    -- context, where a Vimscript function may not be called at all, and laying
    -- the table out measures every cell with `strdisplaywidth`.
    vim.schedule(function()
      -- The table is sized to the window it is about to sit in, because a line
      -- wider than that wraps — and a wrapped table is not a table. Four
      -- columns go to the border and the padding either side.
      local budget = math.min(cfg.max_width, math.max(vim.o.columns - 8, 20))
      local rows, columns, total = M.parse(out)
      local lines = M.render(name, rows, columns, total, budget)
      local _, win = vim.lsp.util.open_floating_preview(lines, "markdown", {
        border = "rounded",
        focus = false,
        focusable = true,
        max_width = budget,
      })
      if win and vim.api.nvim_win_is_valid(win) then
        -- Anything still too wide is scrolled to, not folded over.
        vim.wo[win].wrap = false
      end
    end)
  end)
end

return M
