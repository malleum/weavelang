; Weave syntax highlighting.
;
; Captures use nvim-treesitter's names, so a stock colourscheme renders this
; without further configuration. Patterns are ordered general first and
; specific last, which is the order Neovim resolves in: a later capture over
; the same node wins.

; ------------------------------------------------------------------- names

(identifier) @variable
(constructor) @constructor
(type_name) @type

; A verb being applied, and a bare verb standing as a pipeline stage.
(application function: (identifier) @function.call)
(pipeline (identifier) @function.call)

; ----------------------------------------------------------- the built-ins
;
; Names the compiler already knows. Colouring them apart from the program's
; own definitions is the quickest way to spot a mistyped verb.

; `gives` and `this` are spellings of `:` and `_`, so they colour as the
; symbols they stand for rather than as words. `that` is the second argument a
; group can be handed; `former` and `latter` are the halves of a Twine of two
; and `fore`, `mid` and `aft` the parts of one of three. They colour with them.
["gives" ".."] @operator
["this" "that" "former" "latter" "fore" "mid" "aft"] @variable.builtin

((identifier) @function.builtin
  (#any-of? @function.builtin
    "bend" "sift" "braid" "seek" "span" "under" "copies" "flow" "settle" "len" "count" "sum" "prod"
    "take" "drop" "zip" "zipwith" "sort" "all" "any" "first" "last" "rev"
    "weld" "mend" "sever" "strands" "plait" "cull" "thread" "turn" "wrap"
    "flat" "uniq" "bendr" "siftr" "zipr" "sums" "prods"
    "freq" "most" "chunk" "windows" "pivot" "sortby" "group" "idx"
    "second" "none" "enum" "scan" "priors" "gentle" "dupe" "top" "bot" "pairs" "couples" "cross"
    "high" "low" "highidx" "lowidx" "seekidx" "siftidx" "idxs" "index" "squeeze" "twist"
    "combos" "perms" "compact" "takewhile" "dropwhile" "mapcat" "maxby" "minby"
    "nth" "has" "glean" "harvest" "cycle" "wind"

    "lines" "words" "carve" "fires" "split" "strip" "join" "air" "earths" "spans" "waters" "contains"
    "earth" "water" "fire"
    "blocks" "upper" "lower" "padl" "padr" "starts" "ends" "cutstart" "cutend"
    "replace" "delve" "ord" "spark" "digit" "repeat" "base" "unbase"

    "pattern" "weft" "warp" "spin" "flip" "cell" "set" "knots" "tallies" "tallied" "sited" "sites" "cells" "cellwise" "nb4" "nb8" "rows" "cols" "knot"
    "row" "col" "shape" "inb" "dirs4" "dirs8" "around4" "around8" "mdist"

    "get" "put" "known" "forget" "keys" "vals" "items" "web" "merge" "mapvals"
    "circle" "member" "insert" "remove" "members" "union" "inter" "diff" "covers"
    "taveren" "push" "pop" "dijkstra" "reach" "route" "toposort" "clumps"
    "link" "bind" "bound" "clumped"

    "add" "sub" "mul" "div" "mod" "gcd" "lcm" "abs" "neg" "min" "max"
    "inc" "dec" "even" "odd" "divBy" "sign" "sqrt" "cbrt" "ceil" "floor" "round" "clamp"
    "pow" "bor" "band" "bxor" "bnot" "shl" "shr" "pi" "e" "inf"
    "solve"

    "eq" "neq" "lt" "lte" "gt" "gte" "and" "or" "not" "pick"
    "otherwise" "holds" "woven" "rescue" "snag"
    "overlaps" "overlapping" "within" "spanning" "holding" "width" "mesh"
    "isDigit" "isAlpha" "isSpace"))

; The Five Powers and the built-in type constructors.
((type_name) @type.builtin
  (#any-of? @type.builtin
    "Earth" "Water" "Fire" "Air" "Spirit"
    "Thread" "Pattern" "Web" "Circle" "Taveren" "Link" "Knot" "Hold" "Weaving"))

((constructor) @constructor.builtin
  (#any-of? @constructor.builtin "Held" "Stilled" "Woven" "Gentled"))

; The two Spirit values are constructors, but they read as the language's own
; truth and falsehood.
((constructor) @boolean
  (#any-of? @boolean "Light" "Shadow"))

; Source is the program's input, not a constructor.
((constructor) @variable.builtin
  (#eq? @variable.builtin "Source"))

; ------------------------------------------------------- what a name names

(definition name: (identifier) @variable)
(definition
  name: (identifier) @function
  parameters: (_))

(local_binding name: (identifier) @variable)
(local_binding
  name: (identifier) @function
  parameters: (_))

(inline_binding name: (identifier) @variable)
(inline_binding
  name: (identifier) @function
  parameters: (_))

(signature name: (identifier) @function)

; The `+` is not decoration. A field that repeats binds once per pattern, so
; without it only the *first* parameter is captured and the rest fall back to
; the plain `(identifier) @variable` above — which is why `rvrs l i t` used to
; colour `l` differently from `i` and `t`.
(definition parameters: (identifier)+ @variable.parameter)
(local_binding parameters: (identifier)+ @variable.parameter)
(inline_binding parameters: (identifier)+ @variable.parameter)
(lambda parameters: (identifier)+ @variable.parameter)

; And again for a list that opens with a pattern rather than a name, which is
; how a clause dispatches: `step 0 xs p`, `f [a] b c`. A quantified capture
; matches one run of adjacent children, so the run after the first one needs
; asking for separately. A second leading pattern is where this stops, and a
; definition with two of those is one to write out.
(definition parameters: (_) parameters: (identifier)+ @variable.parameter)
(local_binding parameters: (_) parameters: (identifier)+ @variable.parameter)
(inline_binding parameters: (_) parameters: (identifier)+ @variable.parameter)
(lambda parameters: (_) parameters: (identifier)+ @variable.parameter)

(type_declaration name: (constructor) @type.definition)
(type_declaration parameters: (identifier)+ @variable.parameter)
(constructor_declaration name: (constructor) @constructor)

; ---------------------------------------------------------------- keywords

[
  "is"
  "="
  "weave"
  "channel"
  "into"
  "remember"
] @keyword

"ward" @keyword.conditional

; The pipeline and the particles that read as prose.
[
  "|"
  "where"
  "as"
  "through"
  "else"
  "failing"
] @operator

[
  "::"
  "->"
  ":"
  ","
] @punctuation.delimiter

[
  "("
  ")"
  "["
  "]"
  "{"
  "}"
] @punctuation.bracket

(wildcard) @character.special
(hole) @character.special

; ---------------------------------------------------------------- literals

(integer) @number
(float) @number.float
(character) @character
(text) @string

(comment) @comment @spell
