-- The parts of lua/weave/calls.lua that are logic rather than editor: finding
-- the definition the cursor is inside, reading the records `-watch` writes, and
-- laying them out as a table.
--
-- Everything here runs under a plain Lua with a stand-in for the handful of
-- `vim.*` functions the module touches, exactly as test/aoc_spec.lua does.
--
-- weave.nvim/calls_test.go runs this when a Lua is on PATH.
-- Run from the plugin root: lua5.1 test/calls_spec.lua
package.path = "lua/?.lua;" .. package.path

local BUFFER = {}

vim = {
  fs = { dirname = function(p) return (p:gsub("/[^/]*$", "")) end },
  gsplit = function(s, sep) return string.gmatch(s .. sep, "(.-)" .. sep) end,
  trim = function(s) return (s:gsub("^%s+", ""):gsub("%s+$", "")) end,
  fn = {
    -- Close enough for these: a character is a column, so a continuation byte
    -- of a multibyte one is not. `⋮` has to measure 1 or every column it sits
    -- in comes out three wide.
    strdisplaywidth = function(s) return #(s:gsub("[\128-\191]", "")) end,
    strcharpart = function(s, from, len) return s:sub(from + 1, from + len) end,
  },
  deepcopy = function(t)
    local function copy(v)
      if type(v) ~= "table" then return v end
      local out = {}
      for k, inner in pairs(v) do out[k] = copy(inner) end
      return out
    end
    return copy(t)
  end,
  tbl_deep_extend = function(_, a, b)
    local out = {}
    for k, v in pairs(a) do out[k] = v end
    for k, v in pairs(b or {}) do out[k] = v end
    return out
  end,
  api = {
    nvim_buf_get_lines = function(_, from, to, _)
      local out = {}
      for i = from + 1, math.min(to, #BUFFER) do out[#out + 1] = BUFFER[i] end
      return out
    end,
  },
}

local calls = require("weave.calls")

local function width(s)
  return vim.fn.strdisplaywidth(s)
end

local function eq(what, got, want)
  if tostring(got) ~= tostring(want) then
    print("FAIL " .. what .. ": got " .. tostring(got) .. ", want " .. tostring(want))
    os.exit(1)
  end
  print("ok   " .. what)
end

-- enclosing: a top-level item starts in column zero and everything under it is
-- indented, which is the only structure this needs.
BUFFER = {
  "digits is Source through earths",       -- 1
  "",                                      -- 2
  "next (v, i) c is",                      -- 3
  "  weave n is nth i v",                  -- 4
  "  pick (holds n) (Woven (i, n)) c",     -- 5
  "",                                      -- 6
  "# a comment in column zero",            -- 7
  "gentle next (digits, 0) failing 0",     -- 8
}
eq("inside a function body", calls.enclosing(0, 4), "next")
eq("on the head line itself", calls.enclosing(0, 3), "next")
eq("a definition with no body", calls.enclosing(0, 1), "digits")
eq("a comment is not a definition", tostring(calls.enclosing(0, 7)), "nil")

-- parse: `@LINE<TAB>CALL<TAB>NAME<TAB>VALUE`, the result under the empty name
-- and the total under `calls`.
local out = table.concat({
  "3\tnext\t(Thread Earth, Earth) -> a",    -- an ordinary record, not ours
  "@3\t1\tv\t[3 3 2 -1]",
  "@3\t1\ti\t0",
  "@4\t1\tn\tHeld 3",
  "@3\t1\t\tWoven ([4 3 2 -1], 3)",
  "@3\t2\tv\t[4 3 2 -1]",
  "@3\t2\ti\t3",
  "@4\t2\tn\tHeld -1",
  "@3\t2\t\tWoven ([4 3 2 0], 2)",
  "@0\t0\tcalls\t9",
}, "\n")

local rows, columns, total = calls.parse(out)
eq("two calls", #rows, 2)
eq("the total is what the program said", total, 9)
eq("columns are in the order they were written", table.concat(columns, ","), "v,i,n,=")
eq("a parameter", rows[1].values.v, "[3 3 2 -1]")
eq("a weave binding", rows[2].values.n, "Held -1")
eq("what the call answered", rows[1].values["="], "Woven ([4 3 2 -1], 3)")
eq("call numbers are kept", rows[2].call, 2)

-- An ordinary by-line record is not one of these, and a record for a function
-- that was never called is nothing at all.
local none, _, none_total = calls.parse("1\tdigits\t[1 2 3]\n")
eq("by-line records are left alone", #none, 0)
eq("and there is no total", none_total, 0)

-- render: a column per name, a row per call, sized to the room there is.
local lines = calls.render("next", rows, columns, total, 80)
eq("names the function and the count", lines[1], "**next** — 9 calls")
eq("fenced, so the columns are not read as Markdown", lines[3], "```")
eq("a header row", lines[4], "call  v           i  n        =")
eq("a rule under it", lines[5], "────  ──────────  ─  ───────  ─────────────────────")
eq("the first call", lines[6], "   1  [3 3 2 -1]  0  Held 3   Woven ([4 3 2 -1], 3)")
eq("the second", lines[7], "   2  [4 3 2 -1]  3  Held -1  Woven ([4 3 2 0], 2)")
eq("and closes the fence", lines[8], "```")
eq("and says what is missing", lines[9], "_7 calls in between are not kept_")

-- A gap row stands where the ring skipped the middle, so the jump from the
-- first calls to the last is visible rather than silent.
local skipped = {
  { call = 1, values = { n = "1" } },
  { call = 9, values = { n = "9" } },
}
local gapped = calls.render("f", skipped, { "n" }, 9, 80)
eq("a gap row", gapped[7], "   ⋮  ⋮")

-- The table is sized to the room there is: the widest column gives way first,
-- because it is the one that can spare it. A narrow column stays whole.
local wide = {
  { call = 1, values = { n = string.rep("x", 40), i = "7" } },
  { call = 2, values = { n = string.rep("y", 40), i = "8" } },
}
local tight = calls.render("f", wide, { "n", "i" }, 2, 24)
eq("the header fits the budget", width(tight[4]) <= 24, "true")
eq("and so does a row", width(tight[6]) <= 24, "true")
eq("the narrow column is whole", tight[4], "call  n                i")
eq("the wide one is cut", tight[6], "   1  xxxxxxxxxxxxxx…  7")

-- A function nobody called says so rather than showing an empty table.
eq("never called", calls.render("f", {}, {}, 0, 80)[1], "`f` was never called.")

print("all ok")
