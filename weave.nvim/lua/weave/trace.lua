-- Ghost text showing what every definition in the buffer evaluated to.
--
-- It runs `weave trace` over the program with the directory's largest input
-- file on stdin, and puts each record at the end of the line it came from.
-- That is the whole idea: the answer to "what is in `nums` right now" should
-- be on the screen, not something you go and print.

local M = {}

local config = require("weave.config")

local ns = vim.api.nvim_create_namespace("weave-trace")

-- enabled is per buffer, so one file can be left running while another is not.
local enabled = {}

--- clear removes the ghost text from a buffer.
function M.clear(buf)
  if vim.api.nvim_buf_is_valid(buf) then
    vim.api.nvim_buf_clear_namespace(buf, ns, 0, -1)
  end
end

--- input_for finds the file to feed the program: the largest one in the
--- buffer's own directory that is not itself Weave source. Advent of Code
--- inputs sit next to the program, and the real one is always the big one —
--- the sample you pasted in to check your answer is not.
---@param dir string
---@return string|nil
function M.input_for(dir)
  local best, best_size = nil, -1

  local function consider(path)
    local stat = vim.uv.fs_stat(path)
    if not stat or stat.type ~= "file" then
      return
    end
    if stat.size > best_size then
      best, best_size = path, stat.size
    end
  end

  for _, pattern in ipairs(config.get().input_patterns) do
    for _, path in ipairs(vim.fn.glob(dir .. "/" .. pattern, false, true)) do
      consider(path)
    end
  end
  if best then
    return best
  end

  -- Nothing matched the usual names, so fall back to anything in the directory
  -- that is not source and not hidden.
  local entries = vim.uv.fs_scandir(dir)
  if not entries then
    return nil
  end
  while true do
    local name, kind = vim.uv.fs_scandir_next(entries)
    if not name then
      break
    end
    if kind == "file" and not name:match("^%.") and not name:match("%.weave$") then
      consider(dir .. "/" .. name)
    end
  end
  return best
end

--- parse turns `weave trace` output into records.
---@param out string
---@return table[]
local function parse(out)
  local records = {}
  for line in vim.gsplit(out, "\n", { plain = true }) do
    local lnum, name, value = line:match("^(%d+)\t([^\t]*)\t(.*)$")
    if lnum then
      records[#records + 1] = {
        lnum = tonumber(lnum),
        name = name,
        value = value,
      }
    end
  end
  return records
end

--- render puts one piece of virtual text at the end of each traced line.
local function render(buf, records)
  M.clear(buf)
  if not vim.api.nvim_buf_is_valid(buf) then
    return
  end
  local cfg = config.get()
  local last = vim.api.nvim_buf_line_count(buf)

  for _, rec in ipairs(records) do
    if rec.lnum >= 1 and rec.lnum <= last then
      local text = rec.value
      if #text > cfg.max_width then
        text = text:sub(1, cfg.max_width - 1) .. "…"
      end
      vim.api.nvim_buf_set_extmark(buf, ns, rec.lnum - 1, 0, {
        virt_text = { { cfg.prefix .. text, cfg.highlight } },
        virt_text_pos = "eol",
        hl_mode = "combine",
      })
    end
  end
end

--- blocks_in finds the ```weave fences in a Markdown buffer.
---
--- Each is its own program, so each is traced on its own and its records are
--- shifted down to where the block sits. This is the same extraction the
--- language server does, and for the same reason: the file is Markdown, but
--- the code in it is Weave.
---@param buf integer
---@return table[] each { start = 0-based line of the first line of code, src = string }
function M.blocks_in(buf)
  local lines = vim.api.nvim_buf_get_lines(buf, 0, -1, false)
  local out, i = {}, 1
  while i <= #lines do
    if vim.trim(lines[i]) == "```weave" then
      local j = i + 1
      while j <= #lines and vim.trim(lines[j]) ~= "```" do
        j = j + 1
      end
      if j > #lines then
        break
      end
      out[#out + 1] = {
        start = i, -- 0-based line of lines[i + 1]
        src = table.concat(vim.list_slice(lines, i + 1, j - 1), "\n") .. "\n",
      }
      i = j + 1
    else
      i = i + 1
    end
  end
  return out
end

--- source_for reads the input file the program should see, if there is one.
local function source_for(dir)
  local input = M.input_for(dir)
  if not input then
    return ""
  end
  local f = io.open(input, "rb")
  if not f then
    return ""
  end
  local data = f:read("*a") or ""
  f:close()
  return data
end

--- trace_file runs `weave trace` over a path and hands the records back.
local function trace_file(path, dir, stdin, offset, done)
  local cfg = config.get()
  vim.system({ cfg.cmd, "trace", path }, {
    stdin = stdin,
    cwd = dir,
    timeout = cfg.timeout_ms,
  }, function(res)
    local records = {}
    if res.code == 0 then
      records = parse(res.stdout or "")
      for _, rec in ipairs(records) do
        rec.lnum = rec.lnum + offset
      end
    end
    done(records)
  end)
end

--- run traces a buffer and shows the result.
---
--- A file being edited does not compile most of the time, and `weave trace`
--- knows it: it leaves out the definitions the mistake reached and reports the
--- rest, at the lines they are on. So the ghost text stays put while a line is
--- being typed rather than blinking out on every keystroke. The error itself is
--- left to the language server, which is already reporting it in this window.
---@param buf integer|nil
function M.run(buf)
  buf = buf or vim.api.nvim_get_current_buf()
  local file = vim.api.nvim_buf_get_name(buf)
  if file == "" then
    return
  end
  local dir = vim.fs.dirname(file)
  local stdin = source_for(dir)

  if file:match("%.weave$") then
    trace_file(file, dir, stdin, 0, function(records)
      vim.schedule(function()
        render(buf, records)
      end)
    end)
    return
  end

  if not file:match("%.mk?d$") and not file:match("%.markdown$") then
    return
  end

  -- Markdown: one program per fence, each written out and traced on its own.
  local blocks = M.blocks_in(buf)
  if #blocks == 0 then
    M.clear(buf)
    return
  end
  local all, left = {}, #blocks
  for _, b in ipairs(blocks) do
    local path = vim.fn.tempname() .. ".weave"
    local f = io.open(path, "w")
    if not f then
      left = left - 1
    else
      f:write(b.src)
      f:close()
      trace_file(path, dir, stdin, b.start, function(records)
        vim.list_extend(all, records)
        os.remove(path)
        left = left - 1
        if left == 0 then
          vim.schedule(function()
            render(buf, all)
          end)
        end
      end)
    end
  end
  if left == 0 then
    vim.schedule(function()
      render(buf, all)
    end)
  end
end

--- enabled_for reports whether a buffer is showing ghost text.
function M.enabled_for(buf)
  return enabled[buf] == true
end

--- attach turns tracing on for a buffer and runs it once.
function M.attach(buf)
  enabled[buf] = true
  M.run(buf)
end

--- detach turns it off and clears what is there.
function M.detach(buf)
  enabled[buf] = nil
  M.clear(buf)
end

function M.toggle(buf)
  buf = buf or vim.api.nvim_get_current_buf()
  if M.enabled_for(buf) then
    M.detach(buf)
  else
    M.attach(buf)
  end
end

--- on_save is the autocommand hook: it only does work for buffers that asked.
function M.on_save(buf)
  if M.enabled_for(buf) then
    M.run(buf)
  end
end

return M
