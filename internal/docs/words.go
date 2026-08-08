package docs

// The parts of the language that are not verbs: the words the lexer reserves,
// the shapes a value can have, and the Talents a type can carry.
//
// These live here rather than in the prelude because the prelude holds things
// with types, and none of these has one. A test walks the lexer's own keyword
// table against this list, so a word cannot be reserved without turning up on
// the page.

var keywords = []Word{
	{Name: "is", Section: "Keywords",
		Means: "binds a name. `=` is the same token.",
		Gloss: "everything at the top level is one of these."},
	{Name: "weave", Section: "Keywords",
		Means: "binds a local value.",
		Gloss: "the block form of `is`, and where a definition keeps its workings."},
	{Name: "channel", Section: "Keywords",
		Means: "binds a local function.",
		Gloss: "`weave` for something that takes arguments. It may call itself."},
	{Name: "into", Section: "Keywords",
		Means: "ends an inline `weave`.",
		Gloss: "only needed on one line; a block ends where the indent does."},
	{Name: "ward", Section: "Keywords",
		Means: "matches a value against patterns.",
		Gloss: "every arm answers, every case is covered, and the compiler is the one who checks."},
	{Name: "remember", Section: "Keywords",
		Means: "marks a definition memoised.",
		Gloss: "the answers are kept, keyed on the arguments. Every argument must therefore have Eq, and a definition taking none is already computed once."},
	{Name: "gives", Section: "Keywords",
		Means: "`:` spelled as a word.",
		Gloss: "the same token. A line ending in one opens a block."},

	{Name: "through", Section: "Particles",
		Means: "`|` spelled as a word.",
		Gloss: "`x through f` is `f x`, and the value goes in last."},
	{Name: "where", Section: "Particles",
		Means: "`through sift`.",
		Gloss: "feeds the test, not the value, so a hole in it makes a function."},
	{Name: "as", Section: "Particles",
		Means: "`through bend`.",
		Gloss: "feeds the function, for the same reason."},
	{Name: "else", Section: "Particles",
		Means: "`through otherwise`.",
		Gloss: "feeds the value to fall back on. `cell g k else '.'`"},
	{Name: "failing", Section: "Particles",
		Means: "`through snag`.",
		Gloss: "`else` on the other side of a Weaving."},

	{Name: "_", Section: "Holes",
		Means: "the first argument.",
		Gloss: "claimed by the brackets closest to it, or by the pipeline stage it sits in. Nesting one call inside another splits them up."},
	{Name: "it", Section: "Holes",
		Means: "`_` spelled as a word.",
		Gloss: "the same token."},
	{Name: "this", Section: "Holes",
		Means: "`_` spelled as a word.",
		Gloss: "the same token again. This is the one `weave fmt` prints."},
	{Name: "that", Section: "Holes",
		Means: "the second argument.",
		Gloss: "writing it is what makes the group take two. `braid (add this that) 0`"},
	{Name: "former", Section: "Holes",
		Means: "the first half of the first argument.",
		Gloss: "writing it says the value arriving is a Twine of two and asks for it opened."},
	{Name: "latter", Section: "Holes",
		Means: "the second half.",
		Gloss: "the other half of the same asking."},

	{Name: "..", Section: "Symbols",
		Means: "the rest of a Thread, in a pattern.",
		Gloss: "`[x ..rest]` matches one or more and binds what is left, sharing the storage."},
	{Name: "::", Section: "Symbols",
		Means: "a type signature.",
		Gloss: "optional. The checker works the type out either way; writing it says which one you meant."},
	{Name: "|", Section: "Symbols",
		Means: "the pipeline, and the bar between a sum type's constructors.",
		Gloss: "two jobs, never in the same place."},
}

var talentWords = []Word{
	{Name: "Eq", Means: "can be compared for sameness.",
		Gloss: "structural, all the way down. What a Web key and a `remember` argument both need."},
	{Name: "Ord", Means: "can be ordered.",
		Gloss: "what `sort`, `high`, `low`, `top`, `bot` and a Taveren want. Eq comes with it."},
	{Name: "Show", Means: "can be rendered.",
		Gloss: "what `air` and the printed answer use. Text inside a collection comes back quoted, so what is printed reads back."},
	{Name: "Reckon", Means: "can be counted with.",
		Gloss: "Earth and Water, and nothing else. A bare number literal starts here and settles on Earth if nothing decides otherwise."},
	{Name: "Bulk", Means: "has a length.",
		Gloss: "what `len` asks for. Air, Thread, Web, Circle, Pattern, Taveren."},
}

// shapeOrder is the order the types are laid out in, which is the order they
// are worth meeting rather than alphabetical.
var shapeOrder = []string{
	"Earth", "Water", "Fire", "Air", "Spirit",
	"Thread", "Twine", "Web", "Circle", "Pattern", "Knot", "Taveren",
	"Hold", "Weaving",
}

var shapeKinds = map[string]string{
	"Earth":   "Power",
	"Water":   "Power",
	"Fire":    "Power",
	"Air":     "Power",
	"Spirit":  "Power",
	"Thread":  "a -> Thread a",
	"Twine":   "(a, b), (a, b, c), …",
	"Web":     "k -> v -> Web k v",
	"Circle":  "a -> Circle a",
	"Pattern": "a -> Pattern a",
	"Knot":    "Knot",
	"Taveren": "a -> Taveren a",
	"Hold":    "a -> Hold a",
	"Weaving": "a -> e -> Weaving a e",
}

var shapeGlosses = map[string]string{
	"Earth":  "the whole number. Sixty-four bits, wrapping, unless the program was built to stop instead.",
	"Water":  "the number with a point in it. Printed as the shortest text that reads back the same, always with the point, so it is never mistaken for the Earth it is not.",
	"Fire":   "the one character. A code point, not a byte: `fires` on Air knows the difference.",
	"Air":    "the text. Immutable, and every verb over it hands back a new one.",
	"Spirit": "Light or Shadow, and nothing between them.",

	"Thread":  "the sequence. Strict, so it is all there; `take`, `drop`, `sever` and `tail` share its storage rather than copying it, and `mend` writes through when nothing else can see it.",
	"Twine":   "several strands wound into one, and they need not be of a kind. Two is the usual number, and `former` and `latter` open one without a pattern; `thread` casts a Twine of two of a kind to the Thread it already was.",
	"Web":     "the map. A trie until the compiler owns it, a flat table after, and it reads back in ascending key order either way.",
	"Circle":  "the set. A Web with nothing on the other side of the key.",
	"Pattern": "the grid, indexed by Knot. Its cells are one block, which is why `set` can write through and why `cells` ends that.",
	"Knot":    "a row and a column. Ordered, so it is a Web key and a Taveren value without another line.",
	"Taveren": "the queue that answers smallest first, whatever order things went in. `dijkstra` is built on one and owns it.",

	"Hold":    "there or not there. Weave's answer to null, and the reason a lookup cannot lie about having failed.",
	"Weaving": "done or stopped, with the reason carried. `rescue` takes one side and `snag` the other.",
}
