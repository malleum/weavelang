-- The parts of lua/weave/aoc.lua that are logic rather than editor.
--
-- Everything here runs under a plain Lua with a stand-in for the handful of
-- `vim.*` functions the module touches, so the awkward bits — reading a year
-- and a day out of a path, deciding which part the cursor is in, reading what
-- the site said, turning the page into Markdown — are checked without an
-- editor, a network or a session cookie. The parts that do need those are the
-- parts with nothing in them but a request.
--
-- weave.nvim/aoc_test.go runs this when a Lua is on PATH.
-- Run from the plugin root: lua5.1 test/aoc_spec.lua
package.path = "lua/?.lua;" .. package.path

vim = {
  fs = {
    dirname = function(p) return (p:gsub("/[^/]*$", "")) end,
    basename = function(p) return (p:match("[^/]*$")) end,
    normalize = function(p) return p end,
    find = function() return {} end,
  },
  split = function(s, sep) local out = {} for part in (s.."/"):gmatch("([^/]*)/") do out[#out+1] = part end return out end,
  trim = function(s) return (s:gsub("^%s+", ""):gsub("%s+$", "")) end,
  gsplit = function(s, sep) return string.gmatch(s .. sep, "(.-)" .. sep) end,
  list_extend = function(a, b) for _, v in ipairs(b) do a[#a+1] = v end return a end,
  deepcopy = function(t)
    if type(t) ~= "table" then return t end
    local o = {} for k, v in pairs(t) do o[k] = vim.deepcopy(v) end return o
  end,
  tbl_deep_extend = function(_, a, b)
    local o = vim.deepcopy(a)
    for k, v in pairs(b or {}) do
      if type(v) == "table" and type(o[k]) == "table" then o[k] = vim.tbl_deep_extend("force", o[k], v) else o[k] = v end
    end
    return o
  end,
  log = { levels = { INFO = 2, WARN = 3, ERROR = 4 } },
  notify = function(msg) print("NOTIFY " .. msg) end,
  uv = { fs_stat = function() return nil end },
  fn = { expand = function(s) return s end, fnameescape = function(s) return s end },
  uri_encode = function(s) return s end,
  api = { nvim_buf_get_lines = function(_, from, to, _)
    local out = {}
    for i = from + 1, math.min(to, #BUFFER) do out[#out + 1] = BUFFER[i] end
    return out
  end },
  bo = {}, wo = {},
}

local aoc = require("weave.aoc")

local function eq(what, got, want)
  if tostring(got) ~= tostring(want) then
    print("FAIL " .. what .. ": got " .. tostring(got) .. ", want " .. tostring(want))
    os.exit(1)
  end
  print("ok   " .. what)
end

-- puzzle_of
local cases = {
  { "/home/x/aoc/2017/5/pt2.weave", 2017, 5 },
  { "/home/x/advent/2024/day05/a.weave", 2024, 5 },
  { "/x/2015/25/z.weave", 2015, 25 },
  { "/x/2017/day-9/z.weave", 2017, 9 },
}
for _, c in ipairs(cases) do
  local p = aoc.puzzle_of(c[1])
  eq("puzzle " .. c[1], p and p.year, c[2])
  eq("puzzle day " .. c[1], p and p.day, c[3])
end
eq("no puzzle", aoc.puzzle_of("/home/x/scratch/a.weave"), nil)
eq("day out of range", aoc.puzzle_of("/x/2017/40/a.weave"), nil)

-- clock
eq("clock 0", aoc.clock(0), "now")
eq("clock 45", aoc.clock(45), "45s")
eq("clock 90", aoc.clock(90), "1m 30s")
eq("clock 7200", aoc.clock(7200), "2h 0m")

-- verdict
local right = "<article><p>That's the right answer!  You are one gold star closer.</p></article>"
local high = "<article><p>That's not the right answer; your answer is too high.  Please wait one minute and try again.</p></article>"
local low = "<article><p>That's not the right answer; your answer is too low.  Please wait 3 minutes and try again.</p></article>"
local soon = "<article><p>You gave an answer too recently; you have to wait after submitting. You have 30s left to wait.</p></article>"
local msg, wait = aoc.verdict(right);  eq("verdict right", msg, "right");    eq("verdict right wait", wait, 0)
msg, wait = aoc.verdict(high);         eq("verdict high", msg, "too high");  eq("verdict high wait", wait, 60)
msg, wait = aoc.verdict(low);          eq("verdict low", msg, "too low");    eq("verdict low wait", wait, 180)
msg, wait = aoc.verdict(soon);         eq("verdict soon", msg, "too soon");  eq("verdict soon wait", wait, 30)

-- to_markdown
local page = [[<html><body><article class="day-desc"><h2>--- Day 5: A Maze ---</h2><p>You have <em>a list</em> of offsets, like <code>0 3 0 1 -3</code>.</p><pre><code>0
3
0</code></pre><ul><li>first</li><li>second</li></ul></article></body></html>]]
local md = aoc.to_markdown(page, { year = 2017, day = 5 })
local function has(what, needle)
  if not md:find(needle, 1, true) then print("FAIL markdown " .. what .. "\n" .. md) os.exit(1) end
  print("ok   markdown " .. what)
end
has("title", "# Advent of Code 2017, day 5")
has("heading", "## --- Day 5: A Maze ---")
has("emphasis", "**a list**")
has("code", "`0 3 0 1 -3`")
has("fence", "```")
has("list", "- first")
if md:find("<") then print("FAIL markdown: tags left\n" .. md) os.exit(1) end
print("ok   markdown has no tags left")
-- part_at: the first bare chain is part one, the second is part two.
BUFFER = {
  "nums is Source through earths",
  "",
  "nums through sum",
  "",
  "nums through prod",
  "",
}
eq("part at the first chain", aoc.part_at(0, 3), 1)
eq("part at the second chain", aoc.part_at(0, 5), 2)
eq("part above everything", aoc.part_at(0, 1), 1)

BUFFER = {
  "# a comment, not an answer",
  "Colour is Red | Green",
  "nums is [1 2 3]",
  "f x is add x 1",
  "nums through sum",
  "nums through prod",
}
eq("declarations are not answers", aoc.part_at(0, 5), 1)
eq("and the one after is part two", aoc.part_at(0, 6), 2)

print("all ok")
