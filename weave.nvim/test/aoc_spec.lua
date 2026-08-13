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

-- line_holding: where a refetched problem scrolls to when part two arrives.
local problem = {
  "# Advent of Code 2017, day 5",
  "",
  "## --- Day 5: A Maze of Twisty Trampolines ---",
  "",
  "text",
  "",
  "## --- Part Two ---",
  "",
  "more text",
}
eq("finds part two", aoc.line_holding(problem, "Part Two"), 7)
eq("finds the first match", aoc.line_holding(problem, "##"), 3)
eq("nothing to find", tostring(aoc.line_holding(problem, "Part Three")), "nil")
eq("an empty file", tostring(aoc.line_holding({}, "Part Two")), "nil")
-- The text is looked for plainly, not as a pattern, since a heading is full of
-- punctuation a pattern would read as syntax.
eq("dashes are not a pattern", aoc.line_holding(problem, "--- Part Two ---"), 7)

-- history, bracket, tried and report: what has already been sent.
--
-- "Wrong" is not the useful part of a wrong answer; which answer was wrong is,
-- and a too-high and a too-low between them say where the answer must be.
local dir = os.getenv("TMPDIR") or "/tmp"
local puzzle = { dir = dir, year = 2017, day = 5 }
local answers = dir .. "/.aoc-answers"
os.remove(answers)
os.remove(dir .. "/.aoc-submitted")

eq("nothing sent yet", #aoc.history(puzzle), 0)

local f = assert(io.open(answers, "w"))
f:write("1000\t1\t4000\ttoo low\n")
f:write("1100\t1\t9000\ttoo high\n")
f:write("1200\t1\t5500\twrong\n")
f:write("1300\t1\t5500\twrong\n")
f:write("1400\t2\t17\tright\n")
f:close()

local entries = aoc.history(puzzle)
eq("every answer is read back", #entries, 5)
eq("with its part", entries[5].part, 2)
eq("and what the site said", entries[2].verdict, "too high")

local low, high = aoc.bracket(entries, 1)
eq("a too low is a lower bound", low, 4000)
eq("a too high is an upper one", high, 9000)
eq("and a part with neither has none", tostring(aoc.bracket(entries, 2)), "nil")

local tried = aoc.tried(entries, 1)
eq("three distinct answers", #tried, 3)
eq("the first, with its verdict", tried[1], "4000 (too low)")
eq("the same answer twice is listed once", tried[3], "5500 (wrong)")

local report = aoc.report(puzzle)
eq("no stamp file means you may answer", report[1], "you may answer now")
eq("part one's answers", report[2], "part 1: 4000 (too low), 9000 (too high), 5500 (wrong)")
eq("and where the answer has to be", report[3], "  between 4000 and 9000")
eq("part two's", report[4], "part 2: 17 (right)")

-- Only whole numbers can be compared, so a word answer bounds nothing.
local words = {
  { at = 1, part = 1, answer = "abcdef", verdict = "too high" },
  { at = 2, part = 1, answer = "12", verdict = "too low" },
}
local wlow, whigh = aoc.bracket(words, 1)
eq("a word is not a bound", tostring(whigh), "nil")
eq("but a number beside it still is", wlow, 12)

-- known_false: what the record already rules out, so a submission that cannot
-- be right is not spent finding that out again.
eq("a repeat of a wrong answer", aoc.known_false(entries, 1, "5500"),
  "already sent, and it was wrong")
eq("a repeat of a too high one", aoc.known_false(entries, 1, "9000"),
  "already sent, and it was too high")
eq("at the lower bound", aoc.known_false(entries, 1, "4000"),
  "already sent, and it was too low")
eq("below the lower bound", aoc.known_false(entries, 1, "3999"),
  "3999 is not above 4000, which came back too low")
eq("above the upper bound", aoc.known_false(entries, 1, "12000"),
  "12000 is not below 9000, which came back too high")
eq("inside the bracket is allowed", tostring(aoc.known_false(entries, 1, "6000")), "nil")
eq("a bound is per part", tostring(aoc.known_false(entries, 2, "3999")), "nil")
eq("and a right answer is named as such", aoc.known_false(entries, 2, "17"),
  "already sent, and it was right")

-- An answer nobody graded says nothing. "Too soon" means the site never looked
-- at it, so repeating it has to be allowed.
local ungraded = {
  { at = 1, part = 1, answer = "77", verdict = "too soon" },
  { at = 2, part = 1, answer = "88", verdict = "unclear — read the reply" },
}
eq("a too-soon answer is not ruled out", tostring(aoc.known_false(ungraded, 1, "77")), "nil")
eq("nor an unclear one", tostring(aoc.known_false(ungraded, 1, "88")), "nil")

-- A word answer cannot be bracketed, but it can still be an exact repeat.
local words2 = {
  { at = 1, part = 1, answer = "ZFHKBJZW", verdict = "wrong" },
  { at = 2, part = 1, answer = "40", verdict = "too low" },
}
eq("a repeated word", words2 and aoc.known_false(words2, 1, "ZFHKBJZW"),
  "already sent, and it was wrong")
eq("a different word is not bounded", tostring(aoc.known_false(words2, 1, "ABCDEFGH")), "nil")

os.remove(answers)

print("all ok")
