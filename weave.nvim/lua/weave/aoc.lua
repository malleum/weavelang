-- Advent of Code, from inside the editor.
--
-- Four things, and they are all the same shape: work out which puzzle this
-- file is, find the session cookie, ask the site, and put the answer where it
-- belongs.
--
--   :AocInput     fetch the input beside the program, once
--   :AocProblem   fetch the problem into a local Markdown file and show it
--   :AocSubmit    submit the chain the cursor is in
--   :AocTime      how long until another answer may be sent
--
-- The year and the day come from the directory the file is in, so a tree laid
-- out `.../2017/5/day5.weave` needs no configuration at all. The session
-- cookie is read from a file rather than a setting, because a setting ends up
-- committed and a cookie is a credential.
--
-- Every request goes through `curl` in the background. Nothing here blocks the
-- editor, and nothing here writes to a buffer other than the one it made.

local config = require("weave.config")

local M = {}

-- ------------------------------------------------------------------ puzzle

--- puzzle_of works out which puzzle a path belongs to, by looking for a year
--- and a day among the directories above it.
---
--- A year is a four-digit number that could be one; a day is one or two digits
--- between 1 and 25. The nearest day wins, and the nearest year above it, so
--- `.../aoc/2017/05/pt2.weave` and `.../2017/day05/x.weave` both work.
---@param path string
---@return table|nil { year = integer, day = integer, dir = string }
function M.puzzle_of(path)
  if path == nil or path == "" then
    return nil
  end
  local dir = vim.fs.dirname(path)
  local parts = vim.split(vim.fs.normalize(dir), "/", { plain = true })

  local day, year
  for i = #parts, 1, -1 do
    local part = parts[i]
    if day == nil then
      local d = tonumber(part:match("^d?a?y?%-?_?(%d%d?)$") or part:match("^(%d%d?)$"))
      if d and d >= 1 and d <= 25 then
        day = d
      end
    end
    if day ~= nil and year == nil then
      local y = tonumber(part:match("^(%d%d%d%d)$"))
      if y and y >= 2015 and y <= 2100 then
        year = y
      end
    end
  end
  if day == nil or year == nil then
    return nil
  end
  return { year = year, day = day, dir = dir }
end

-- ------------------------------------------------------------------ session

--- session_file finds the file holding the session cookie: the configured
--- path if it is set, else `advent_of_code/.session` at the top of the
--- repository the file is in.
---@param from string
---@return string|nil
function M.session_file(from)
  local cfg = config.get().aoc
  if cfg.session_file and cfg.session_file ~= "" then
    return vim.fn.expand(cfg.session_file)
  end
  local git = vim.fs.find(".git", { path = from, upward = true, type = "directory" })[1]
    or vim.fs.find(".git", { path = from, upward = true, type = "file" })[1]
  if not git then
    return nil
  end
  return vim.fs.dirname(git) .. "/advent_of_code/.session"
end

--- session reads the cookie, or reports why it could not.
---@param from string
---@return string|nil, string|nil
function M.session(from)
  local path = M.session_file(from)
  if not path then
    return nil, "no .git above this file, so there is nowhere to look for a session cookie"
  end
  local f = io.open(path, "r")
  if not f then
    return nil, "no session cookie at " .. path
  end
  local text = f:read("*a") or ""
  f:close()
  text = vim.trim(text)
  if text == "" then
    return nil, path .. " is empty"
  end
  return text, nil
end

-- --------------------------------------------------------------------- http

--- request runs curl in the background and hands the body back.
---
--- The user agent names a person, because the site asks that automated
--- requests do; it is a setting for that reason and not for any other.
---@param opts table { url, session, post, from }
---@param done fun(body: string|nil, err: string|nil)
local function request(opts, done)
  local cfg = config.get().aoc
  local args = {
    "curl", "--silent", "--show-error", "--fail-with-body",
    "--max-time", tostring(math.floor(cfg.timeout_ms / 1000)),
    "--cookie", "session=" .. opts.session,
    "--user-agent", cfg.user_agent,
  }
  if opts.post then
    vim.list_extend(args, { "--data", opts.post })
  end
  table.insert(args, opts.url)

  vim.system(args, { text = true }, function(res)
    vim.schedule(function()
      if res.code ~= 0 then
        done(nil, vim.trim((res.stderr or "") .. " " .. (res.stdout or "")))
        return
      end
      done(res.stdout or "", nil)
    end)
  end)
end

local function url_for(p, tail)
  return string.format("%s/%d/day/%d%s", config.get().aoc.host, p.year, p.day, tail or "")
end

local function say(msg, level)
  vim.notify("aoc: " .. msg, level or vim.log.levels.INFO)
end

-- -------------------------------------------------------------------- input

--- input fetches the puzzle input into the program's own directory, once.
--- A file that is already there is left alone: the input never changes, and
--- asking twice is asking the site for something it already gave.
---@param path string|nil
---@param opts table|nil { quiet = boolean }
function M.input(path, opts)
  opts = opts or {}
  path = path or vim.api.nvim_buf_get_name(0)
  local p = M.puzzle_of(path)
  if not p then
    if not opts.quiet then
      say("cannot tell which puzzle this is: no year and day in the path", vim.log.levels.WARN)
    end
    return
  end
  local out = p.dir .. "/" .. config.get().aoc.input_name
  if vim.uv.fs_stat(out) then
    if not opts.quiet then
      say("the input is already at " .. vim.fs.basename(out))
    end
    return
  end

  local cookie, err = M.session(p.dir)
  if not cookie then
    if not opts.quiet then
      say(err, vim.log.levels.WARN)
    end
    return
  end

  say(string.format("fetching %d day %d…", p.year, p.day))
  request({ url = url_for(p, "/input"), session = cookie }, function(body, rerr)
    if not body then
      say("could not fetch the input: " .. rerr, vim.log.levels.ERROR)
      return
    end
    local f = io.open(out, "w")
    if not f then
      say("could not write " .. out, vim.log.levels.ERROR)
      return
    end
    f:write(body)
    f:close()
    say(string.format("%d day %d: %d bytes into %s", p.year, p.day, #body, vim.fs.basename(out)))
  end)
end

-- ------------------------------------------------------------------ problem

--- problem opens the day's text in a split on the right, without taking the
--- cursor out of the file being edited.
---
--- The first call fetches and saves it; every call after that reads the saved
--- copy, so the site is asked once however often the window is closed. Part two
--- only appears once part one is answered, so `force` fetches it again — which
--- is what a correct submission does for you.
---
--- opts:
---   force        fetch again even though the file is already there
---   only_if_open leave the window alone unless one is already showing it,
---                which is what a refetch after a correct answer wants: the
---                file is brought up to date either way, but nothing appears
---                on screen that was not there before
---   jump         a line to scroll the window to, if it holds one
---   quiet        say nothing on the way
---@param path string|nil
---@param opts table|boolean|nil  a boolean is read as `force`, as the command passes
function M.problem(path, opts)
  if type(opts) == "boolean" or opts == nil then
    opts = { force = opts or false }
  end
  path = path or vim.api.nvim_buf_get_name(0)
  local p = M.puzzle_of(path)
  if not p then
    if not opts.quiet then
      say("cannot tell which puzzle this is: no year and day in the path", vim.log.levels.WARN)
    end
    return
  end
  local out = p.dir .. "/" .. config.get().aoc.problem_name

  if vim.uv.fs_stat(out) and not opts.force then
    M.show(out, opts)
    return
  end

  local cookie, err = M.session(p.dir)
  if not cookie then
    -- The problem is readable without a cookie; only part two needs one.
    cookie = ""
    if not opts.quiet then
      say(err .. " — fetching part one only", vim.log.levels.WARN)
    end
  end

  request({ url = url_for(p), session = cookie }, function(body, rerr)
    if not body then
      say("could not fetch the problem: " .. rerr, vim.log.levels.ERROR)
      return
    end
    local md = M.to_markdown(body, p)
    local f = io.open(out, "w")
    if not f then
      say("could not write " .. out, vim.log.levels.ERROR)
      return
    end
    f:write(md)
    f:close()
    M.show(out, opts)
  end)
end

--- to_markdown pulls the article out of the page and renders it as text.
---
--- This is not a general HTML reader and does not try to be. Advent of Code
--- serves the same handful of tags every year, and the shape of them is stable
--- enough that a dozen substitutions read better than a dependency would.
---@param html string
---@param p table
---@return string
function M.to_markdown(html, p)
  local parts = {}
  for article in html:gmatch("<article.-</article>") do
    parts[#parts + 1] = article
  end
  local body = table.concat(parts, "\n")
  if body == "" then
    body = html
  end

  body = body:gsub("<h2[^>]*>(.-)</h2>", "\n## %1\n")
  body = body:gsub("<em[^>]*class=\"star\"[^>]*>(.-)</em>", "**%1**")
  body = body:gsub("<code>(.-)</code>", "`%1`")
  body = body:gsub("<pre>%s*(.-)%s*</pre>", "\n```\n%1\n```\n")
  body = body:gsub("<em>(.-)</em>", "**%1**")
  body = body:gsub("<li>(.-)</li>", "\n- %1")
  body = body:gsub("<p>", "\n\n")
  body = body:gsub("<span[^>]*>", "")
  body = body:gsub("<a[^>]*>(.-)</a>", "%1")
  body = body:gsub("<[^>]+>", "")

  local entities = {
    ["&gt;"] = ">", ["&lt;"] = "<", ["&amp;"] = "&", ["&quot;"] = '"',
    ["&apos;"] = "'", ["&#39;"] = "'", ["&hellip;"] = "…", ["&mdash;"] = "—",
    ["&ndash;"] = "–", ["&nbsp;"] = " ", ["&eacute;"] = "é",
  }
  body = body:gsub("&[#%w]+;", function(e) return entities[e] or e end)

  body = body:gsub("\n\n\n+", "\n\n")
  return string.format("# Advent of Code %d, day %d\n\n%s\n\n---\n\n%s\n",
    p.year, p.day, vim.trim(body), url_for(p))
end

--- show opens a file in a vertical split on the right and puts the cursor
--- back where it was. Reusing a window that already holds the file means
--- running the command twice does not stack splits.
---
--- `opts.only_if_open` stops it opening one at all, and `opts.jump` scrolls a
--- window that is already there to the first line holding that text — which is
--- how part two lands in view the moment part one is answered.
---@param file string
---@param opts table|nil
function M.show(file, opts)
  opts = opts or {}
  local here = vim.api.nvim_get_current_win()
  for _, win in ipairs(vim.api.nvim_list_wins()) do
    local name = vim.api.nvim_buf_get_name(vim.api.nvim_win_get_buf(win))
    if name == file then
      vim.api.nvim_win_call(win, function() vim.cmd("edit!") end)
      if opts.jump then
        M.jump_to(win, opts.jump)
      end
      vim.api.nvim_set_current_win(here)
      return
    end
  end
  if opts.only_if_open then
    return
  end
  vim.cmd("botright vsplit " .. vim.fn.fnameescape(file))
  vim.wo.wrap = true
  vim.wo.linebreak = true
  vim.bo.filetype = "markdown"
  if opts.jump then
    M.jump_to(vim.api.nvim_get_current_win(), opts.jump)
  end
  vim.api.nvim_set_current_win(here)
end

--- jump_to puts a window's cursor on the first line holding `text`, and that
--- line at the top, so what has just appeared is what you are looking at.
---@param win integer
---@param text string
function M.jump_to(win, text)
  local buf = vim.api.nvim_win_get_buf(win)
  local lnum = M.line_holding(vim.api.nvim_buf_get_lines(buf, 0, -1, false), text)
  if not lnum then
    return
  end
  vim.api.nvim_win_set_cursor(win, { lnum, 0 })
  vim.api.nvim_win_call(win, function() vim.cmd("normal! zt") end)
end

--- line_holding is the 1-based line number of the first line containing text,
--- or nil. Split out so the search is testable without an editor.
---@param lines string[]
---@param text string
---@return integer|nil
function M.line_holding(lines, text)
  for i, line in ipairs(lines) do
    if line:find(text, 1, true) then
      return i
    end
  end
  return nil
end

-- ------------------------------------------------------------------- submit

--- part_at decides which part the cursor is in, by counting the bare
--- expressions above it.
---
--- A Weave file for a day is one binding for the input and one bare chain per
--- part, so the first bare chain is part one and the second is part two.
--- Anything past the second is part two as well, since there are only two.
---@param buf integer
---@param lnum integer 1-based
---@return integer
function M.part_at(buf, lnum)
  local lines = vim.api.nvim_buf_get_lines(buf, 0, lnum, false)
  local bare = 0
  local i = 1
  while i <= #lines do
    local line = lines[i]
    if line ~= "" and not line:match("^%s") and not line:match("^#") then
      -- A definition has an `is` or a `::` at the top level; anything else
      -- starting in column zero is an answer.
      if not line:match("%f[%w]is%f[%W]") and not line:match("::") and not line:match("^%u") then
        bare = bare + 1
      end
    end
    i = i + 1
  end
  return math.min(math.max(bare, 1), 2)
end

--- answer_at runs the program and takes the answer belonging to the part the
--- cursor is in. `weave run` prints one line per bare chain, in order.
---@param buf integer
---@param part integer
---@param done fun(answer: string|nil, err: string|nil)
local function answer_at(buf, part, done)
  local cfg = config.get()
  local file = vim.api.nvim_buf_get_name(buf)
  local trace = require("weave.trace")
  local input = trace.input_for(vim.fs.dirname(file))
  local stdin = input and table.concat(vim.fn.readfile(input, "b"), "\n") or nil

  vim.system({ cfg.cmd, "run", file }, {
    stdin = stdin,
    cwd = vim.fs.dirname(file),
    timeout = cfg.timeout_ms,
  }, function(res)
    vim.schedule(function()
      if res.code ~= 0 then
        done(nil, vim.trim(res.stderr or "the program did not run"))
        return
      end
      local out = {}
      for line in vim.gsplit(res.stdout or "", "\n", { plain = true }) do
        if vim.trim(line) ~= "" then
          out[#out + 1] = vim.trim(line)
        end
      end
      if #out < part then
        done(nil, string.format("the program printed %d answer(s), so there is no part %d", #out, part))
        return
      end
      done(out[part], nil)
    end)
  end)
end

-- ------------------------------------------------------------------ cooldown

local function stamp_file(p)
  return p.dir .. "/.aoc-submitted"
end

--- cooldown_left reports how many seconds are left before another answer may
--- be sent, and what the site said last time.
---@param p table
---@return integer, string
function M.cooldown_left(p)
  local f = io.open(stamp_file(p), "r")
  if not f then
    return 0, ""
  end
  local text = f:read("*a") or ""
  f:close()
  local at, wait, note = text:match("^(%d+)%s+(%d+)%s*(.*)$")
  if not at then
    return 0, ""
  end
  local left = (tonumber(at) + tonumber(wait)) - os.time()
  if left < 0 then
    left = 0
  end
  return left, vim.trim(note)
end

--- Every answer sent for a puzzle, in the order they were sent.
---
--- The stamp file beside it holds one thing — when another answer may go — and
--- is overwritten each time. This is the other half: what was actually sent and
--- what came back. "Wrong" is not the useful part of a wrong answer; *which*
--- answer was wrong is, because the next one has to be a different one, and
--- because a too-high and a too-low between them say where the answer is.
local function history_file(p)
  return p.dir .. "/.aoc-answers"
end

--- history reads back every answer sent for this puzzle.
---@param p table
---@return table[] each { at = integer, part = integer, answer = string, verdict = string }
function M.history(p)
  local f = io.open(history_file(p), "r")
  if not f then
    return {}
  end
  local out = {}
  for line in f:lines() do
    local at, part, answer, verdict = line:match("^(%d+)\t(%d+)\t([^\t]*)\t(.*)$")
    if at then
      out[#out + 1] = {
        at = tonumber(at),
        part = tonumber(part),
        answer = answer,
        verdict = verdict,
      }
    end
  end
  f:close()
  return out
end

--- bracket reads the bounds out of the answers already sent for a part: every
--- "too low" is below the answer and every "too high" is above it.
---
--- This is the whole reason the site bothers to say which way you were wrong,
--- and it is thrown away if nobody writes it down. Only answers that are whole
--- numbers count, since nothing else can be compared.
---@param entries table[]
---@param part integer
---@return integer|nil low, integer|nil high
function M.bracket(entries, part)
  local low, high
  for _, e in ipairs(entries) do
    local n = tonumber(e.answer)
    if e.part == part and n and n == math.floor(n) then
      if e.verdict == "too low" and (not low or n > low) then
        low = n
      elseif e.verdict == "too high" and (not high or n < high) then
        high = n
      end
    end
  end
  return low, high
end

--- tried lists the answers already sent for a part, newest last, without
--- repeating one that was sent twice.
---@param entries table[]
---@param part integer
---@return string[]
function M.tried(entries, part)
  local out, seen = {}, {}
  for _, e in ipairs(entries) do
    if e.part == part and not seen[e.answer] then
      seen[e.answer] = true
      out[#out + 1] = ("%s (%s)"):format(e.answer, e.verdict)
    end
  end
  return out
end

local function remember_submission(p, wait, note, part, answer)
  local f = io.open(stamp_file(p), "w")
  if f then
    f:write(string.format("%d %d %s\n", os.time(), wait, note))
    f:close()
  end
  -- Appended, never rewritten: the point of it is what came before.
  local h = io.open(history_file(p), "a")
  if h then
    h:write(string.format("%d\t%d\t%s\t%s\n", os.time(), part or 0,
      (answer or ""):gsub("[\t\n]", " "), note))
    h:close()
  end
end

--- clock renders a number of seconds the way a person reads one.
---@param secs integer
---@return string
function M.clock(secs)
  if secs <= 0 then
    return "now"
  end
  if secs < 60 then
    return string.format("%ds", secs)
  end
  if secs < 3600 then
    return string.format("%dm %ds", math.floor(secs / 60), secs % 60)
  end
  return string.format("%dh %dm", math.floor(secs / 3600), math.floor(secs % 3600 / 60))
end

--- verdict reads what the site said. The wording has been the same for years,
--- and every branch of it matters: too high and too low are the useful ones.
---@param body string
---@return string, integer  a message, and how long to wait
function M.verdict(body)
  local text = body:gsub("<[^>]+>", " "):gsub("%s+", " ")

  local wait = 60
  local mins = text:match("wait (%d+) minute") or text:match("have (%d+)m")
  if mins then
    wait = tonumber(mins) * 60
  elseif text:match("wait one minute") or text:match("have 1m") then
    wait = 60
  end
  local secs = text:match("(%d+)s left to wait")
  if secs then
    wait = tonumber(secs)
  end

  if text:match("That's the right answer") then
    return "right", 0
  end
  if text:match("answer is too high") then
    return "too high", wait
  end
  if text:match("answer is too low") then
    return "too low", wait
  end
  if text:match("That's not the right answer") then
    return "wrong", wait
  end
  if text:match("You gave an answer too recently") then
    return "too soon", wait
  end
  if text:match("Did you already complete it") then
    return "already answered", 0
  end
  return "unclear — read the reply", wait
end

--- submit sends the answer for the part the cursor is in.
---@param path string|nil
---@param bang boolean|nil skip the cooldown check
function M.submit(path, bang)
  local buf = vim.api.nvim_get_current_buf()
  path = path or vim.api.nvim_buf_get_name(buf)
  local p = M.puzzle_of(path)
  if not p then
    say("cannot tell which puzzle this is: no year and day in the path", vim.log.levels.WARN)
    return
  end

  local left = M.cooldown_left(p)
  if left > 0 and not bang then
    -- The whole report, not just the clock: a submission held back is exactly
    -- when what has already been sent is worth reading.
    say(table.concat(M.report(p), "\n"), vim.log.levels.WARN)
    return
  end

  local cookie, err = M.session(p.dir)
  if not cookie then
    say(err, vim.log.levels.WARN)
    return
  end

  local part = M.part_at(buf, vim.api.nvim_win_get_cursor(0)[1])
  say(string.format("running for part %d…", part))

  answer_at(buf, part, function(answer, aerr)
    if not answer then
      say(aerr, vim.log.levels.ERROR)
      return
    end
    local ruled_out = M.known_false(M.history(p), part, answer)
    if ruled_out and not bang then
      -- The answer is wrong on the evidence already in hand, and sending it
      -- costs a submission and the cooldown that follows one. The bang is
      -- already "send it anyway", so it overrides this as it does the wait.
      say(string.format("part %d: %s — %s. :AocSubmit! to send it anyway",
        part, answer, ruled_out), vim.log.levels.WARN)
      return
    end
    say(string.format("part %d: %s — sending", part, answer))
    request({
      url = url_for(p, "/answer"),
      session = cookie,
      post = string.format("level=%d&answer=%s", part, vim.uri_encode(answer)),
    }, function(body, rerr)
      if not body then
        say("could not submit: " .. rerr, vim.log.levels.ERROR)
        return
      end
      local msg, wait = M.verdict(body)
      remember_submission(p, wait, msg, part, answer)
      local level = vim.log.levels.INFO
      if msg ~= "right" then
        level = vim.log.levels.WARN
      end
      say(string.format("part %d: %s (%s) — %s", part, answer, msg,
        wait > 0 and (M.clock(wait) .. " to wait") or "go on"), level)

      -- A right answer changes the page: part two appears, or the day is
      -- recorded as finished. The saved copy is brought up to date either way,
      -- and a window already showing it is scrolled to what has just arrived —
      -- but none is opened, because a submission is not a request to read.
      if msg == "right" then
        M.problem(path, {
          force = true,
          only_if_open = true,
          quiet = true,
          jump = part == 1 and "Part Two" or nil,
        })
      end
    end)
  end)
end

--- judged reports whether a verdict is one the site actually reached. "Too
--- soon" means the answer was never graded, and an answer nobody graded says
--- nothing about whether it is right.
local function judged(verdict)
  return verdict == "wrong" or verdict == "too high" or verdict == "too low"
    or verdict == "right"
end

--- known_false says why the answers already sent rule this one out, or nothing
--- if they do not.
---
--- Two ways they can. The same answer graded before is graded the same way now,
--- and a bound already found puts everything past it out of reach: an answer at
--- or below a `too low` is too low, whatever else is true of it.
---@param entries table[]
---@param part integer
---@param answer string
---@return string|nil
function M.known_false(entries, part, answer)
  for _, e in ipairs(entries) do
    if e.part == part and e.answer == answer and judged(e.verdict) then
      return ("already sent, and it was %s"):format(e.verdict)
    end
  end

  local n = tonumber(answer)
  if not n or n ~= math.floor(n) then
    return nil
  end
  local low, high = M.bracket(entries, part)
  if low and n <= low then
    return ("%d is not above %d, which came back too low"):format(n, low)
  end
  if high and n >= high then
    return ("%d is not below %d, which came back too high"):format(n, high)
  end
  return nil
end

--- report lays out what is known about a puzzle: when another answer may go,
--- and every answer already sent with what the site said about it.
---
--- Knowing an answer was too high is worth nothing on its own. Knowing *which*
--- answer was too high is worth the next submission, and knowing both bounds is
--- worth all of them — so the bracket is stated outright rather than left to be
--- worked out from a list.
---@param p table
---@return string[]
function M.report(p)
  local left, note = M.cooldown_left(p)
  local lines = {}
  if left == 0 then
    lines[1] = "you may answer now"
  else
    lines[1] = ("%s to wait"):format(M.clock(left))
  end
  if note ~= "" then
    lines[1] = lines[1] .. " — last: " .. note
  end

  local entries = M.history(p)
  for part = 1, 2 do
    local tried = M.tried(entries, part)
    if #tried > 0 then
      lines[#lines + 1] = ("part %d: %s"):format(part, table.concat(tried, ", "))
      local low, high = M.bracket(entries, part)
      if low and high then
        lines[#lines + 1] = ("  between %d and %d"):format(low, high)
      elseif low then
        lines[#lines + 1] = ("  above %d"):format(low)
      elseif high then
        lines[#lines + 1] = ("  below %d"):format(high)
      end
    end
  end
  return lines
end

--- time reports how long until another answer may be sent, and what has already
--- been sent.
---@param path string|nil
function M.time(path)
  local p = M.puzzle_of(path or vim.api.nvim_buf_get_name(0))
  if not p then
    say("cannot tell which puzzle this is: no year and day in the path", vim.log.levels.WARN)
    return
  end
  say(table.concat(M.report(p), "\n"))
end

return M
