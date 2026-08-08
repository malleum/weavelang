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

; `gives`, `it` and `this` are spellings of `:` and `_`, so they colour as the
; symbols they stand for rather than as words. `that`, `former` and `latter`
; are the other arguments a group can be handed, and colour with them.
["gives" ".."] @operator
["it" "this" "that" "former" "latter"] @variable.builtin

((identifier) @function.builtin
  (#any-of? @function.builtin
    "bend" "sift" "braid" "seek" "span" "flow" "len" "count" "sum" "prod"
    "take" "drop" "zip" "zipwith" "sort" "all" "any" "first" "last" "rev"
    "weld" "mend" "sever" "strands" "plait" "cull" "thread"
    "flat" "uniq" "bendr" "siftr" "zipr" "sums" "prods"
    "freq" "most" "chunk" "windows" "pivot" "sortby" "group" "idx"
    "head" "tail" "second" "none" "enum" "scan" "gentle" "dupe" "top" "bot" "pairs" "cross"
    "high" "low" "highidx" "lowidx" "seekidx" "twist"
    "combos" "perms" "compact" "takewhile" "dropwhile" "mapcat" "maxby" "minby"
    "nth" "has" "glean" "harvest" "cycle"

    "lines" "words" "fires" "split" "strip" "join" "air" "earths" "waters" "contains"
    "earth" "water" "fire"
    "blocks" "upper" "lower" "padl" "padr" "starts" "ends" "cutstart" "cutend"
    "replace" "delve" "ord" "spark" "digit" "repeat" "bin"

    "pattern" "weft" "spin" "flip" "cell" "set" "knots" "cells" "cellwise" "nb4" "nb8" "rows" "cols" "knot"
    "row" "col" "shape" "inb" "dirs4" "dirs8" "around4" "around8" "mdist"

    "get" "put" "known" "forget" "keys" "vals" "items" "web" "merge" "mapvals"
    "circle" "member" "insert" "remove" "members" "union" "inter" "diff"
    "taveren" "push" "pop" "dijkstra" "reach" "route" "toposort"

    "add" "sub" "mul" "div" "mod" "gcd" "lcm" "abs" "neg" "min" "max"
    "inc" "dec" "even" "odd" "divBy" "sign" "sqrt" "cbrt" "ceil" "floor" "round" "clamp"
    "pow" "bor" "band" "bxor" "bnot" "shl" "shr" "pi" "e" "inf"

    "eq" "neq" "lt" "lte" "gt" "gte" "and" "or" "not" "pick"
    "otherwise" "holds" "rescue" "snag"
    "overlaps" "overlapping" "within" "spanning" "holding" "width"
    "isDigit" "isAlpha" "isSpace"))

; The Five Powers and the built-in type constructors.
((type_name) @type.builtin
  (#any-of? @type.builtin
    "Earth" "Water" "Fire" "Air" "Spirit"
    "Thread" "Pattern" "Web" "Circle" "Taveren" "Knot" "Hold" "Weaving"))

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

(definition parameters: (identifier) @variable.parameter)
(local_binding parameters: (identifier) @variable.parameter)
(inline_binding parameters: (identifier) @variable.parameter)
(lambda parameters: (identifier) @variable.parameter)

(type_declaration name: (constructor) @type.definition)
(type_declaration parameters: (identifier) @variable.parameter)
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
