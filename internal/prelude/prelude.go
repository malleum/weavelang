// Package prelude holds Weave's built-in vocabulary as data: the type of every
// standard verb, every data constructor, and which constructors belong to which
// type. Signatures are written in Weave's own `::` notation so that this file
// stays readable next to the spec, and the checker parses them at start-up.
//
// Two conventions run through the table:
//
//   - Collection verbs are data-last, so partial application composes with the
//     pipeline: `sift even` is a function awaiting its Thread.
//   - Grid verbs take the Pattern first (`at g k`), because grid code reads
//     better that way and a Pattern is rarely the thing being piped.
package prelude

// Entry is one built-in name, its type, and any Talent constraints.
type Entry struct {
	Name string
	// Sig is the type in `::` notation.
	Sig string
	// Where lists Talent constraints, e.g. "Ord a" or "Eq k, Show v".
	Where string
	// Doc is a one-line description, used by diagnostics and by the reference
	// `weave verbs` prints.
	Doc string
	// Group is the heading this verb appears under in that reference. It lives
	// on the entry rather than in a table of its own so that adding a verb
	// without filing it somewhere is impossible.
	Group string
}

// Values are the built-in functions and constants.
var Values = []Entry{
	// Input
	{"Source", "Air", "", "the program's input", "Input"},

	// Sequences
	{"bend", "(a -> b) -> Thread a -> Thread b", "", "map a function over a Thread", "Sequences"},
	{"sift", "(a -> Spirit) -> Thread a -> Thread a", "", "keep the elements that satisfy a test", "Sequences"},
	{"braid", "(b -> a -> b) -> b -> Thread a -> b", "", "fold a Thread into a single value", "Sequences"},
	{"seek", "(a -> Spirit) -> Thread a -> Hold a", "", "the first element satisfying a test", "Sequences"},
	{"span", "Earth -> Earth -> Thread Earth", "", "the inclusive range between two Earths", "Sequences"},
	{"flow", "(a -> a) -> a -> Thread a", "", "the endless Thread seed, f seed, f (f seed), ...", "Sequences"},
	{"len", "a -> Earth", "Bulk a", "how many elements a collection or text holds", "Sequences"},
	{"count", "(a -> Spirit) -> Thread a -> Earth", "", "how many elements satisfy a test", "Sequences"},
	{"sum", "Thread a -> a", "Reckon a", "add every element together", "Sequences"},
	{"prod", "Thread a -> a", "Reckon a", "multiply every element together", "Sequences"},
	{"sums", "Thread a -> Thread a", "Reckon a", "the running totals", "Sequences"},
	{"prods", "Thread a -> Thread a", "Reckon a", "the running products", "Sequences"},
	{"take", "Earth -> Thread a -> Thread a", "", "the first n elements", "Sequences"},
	{"drop", "Earth -> Thread a -> Thread a", "", "everything after the first n elements", "Sequences"},
	{"takewhile", "(a -> Spirit) -> Thread a -> Thread a", "", "the leading run that satisfies a test", "Sequences"},
	{"dropwhile", "(a -> Spirit) -> Thread a -> Thread a", "", "everything after that leading run", "Sequences"},
	{"zip", "Thread a -> Thread b -> Thread (a, b)", "", "pair two Threads element by element", "Sequences"},
	{"zipwith", "(a -> b -> c) -> Thread a -> Thread b -> Thread c", "", "combine two Threads element by element", "Sequences"},
	{"thread", "(a, a) -> Thread a", "", "the two halves of a Twine as a Thread", "Sequences"},
	{"weld", "Thread a -> Thread a -> Thread a", "", "weld ys xs: xs with ys on the end", "Sequences"},
	{"mend", "Earth -> a -> Thread a -> Thread a", "", "mend i x xs: xs with position i replaced, or xs when there is no such position", "Sequences"},
	{"sever", "Earth -> Thread a -> (Thread a, Thread a)", "", "cut a Thread in two at a position", "Sequences"},
	{"strands", "(a -> b) -> Thread a -> Thread (Thread a)", "Eq b", "runs of adjacent elements whose derived key agrees", "Sequences"},
	{"plait", "Thread a -> Thread a -> Thread a", "", "plait as bs: one from each in turn, stopping with the shorter. `zip` flattened", "Sequences"},
	{"cull", "(a -> Spirit) -> Thread a -> Thread a", "", "keep what the test turns down: sift the other way round", "Sequences"},
	{"bendr", "(a -> b) -> Thread (Thread a) -> Thread (Thread b)", "", "transform every element one level deeper", "Sequences"},
	{"siftr", "(a -> Spirit) -> Thread (Thread a) -> Thread (Thread a)", "", "keep the elements passing a test, one level deeper", "Sequences"},
	{"zipr", "(a -> b -> c) -> Thread (Thread a) -> Thread (Thread b) -> Thread (Thread c)", "", "combine two Threads of Threads element by element", "Sequences"},
	{"sort", "Thread a -> Thread a", "Ord a", "order a Thread", "Sequences"},
	{"sortby", "(a -> b) -> Thread a -> Thread a", "Ord b", "order by a derived key", "Sequences"},
	{"all", "(a -> Spirit) -> Thread a -> Spirit", "", "does every element satisfy the test", "Sequences"},
	{"any", "(a -> Spirit) -> Thread a -> Spirit", "", "does any element satisfy the test", "Sequences"},
	{"none", "(a -> Spirit) -> Thread a -> Spirit", "", "does no element satisfy the test", "Sequences"},
	{"first", "Thread a -> Hold a", "", "the first element, if there is one", "Sequences"},
	{"second", "Thread a -> Hold a", "", "the second element, if there is one", "Sequences"},
	{"last", "Thread a -> Hold a", "", "the final element, if there is one", "Sequences"},
	{"head", "Thread a -> Hold a", "", "the first element, if there is one", "Sequences"},
	{"tail", "Thread a -> Thread a", "", "everything after the first element", "Sequences"},
	{"rev", "Thread a -> Thread a", "", "the same elements, back to front", "Sequences"},
	{"flat", "Thread (Thread a) -> Thread a", "", "flatten a Thread of Threads", "Sequences"},
	{"uniq", "Thread a -> Thread a", "Eq a", "drop repeated elements", "Sequences"},
	{"enum", "Thread a -> Thread (Earth, a)", "", "pair each element with its position", "Sequences"},
	{"scan", "(b -> a -> b) -> b -> Thread a -> Thread b", "", "braid keeping every running total", "Sequences"},
	{"gentle", "(b -> a -> Weaving b c) -> b -> Thread a -> Weaving b c", "", "braid that stops when the step answers Gentled", "Sequences"},
	{"dupe", "Thread a -> Hold (Earth, a)", "Eq a", "where the first repeat is, and what it is", "Sequences"},
	{"high", "Thread a -> Hold a", "Ord a", "the largest element", "Sequences"},
	{"low", "Thread a -> Hold a", "Ord a", "the smallest element", "Sequences"},
	{"highidx", "Thread a -> Hold Earth", "Ord a", "where the largest element is", "Sequences"},
	{"lowidx", "Thread a -> Hold Earth", "Ord a", "where the smallest element is", "Sequences"},
	{"seekidx", "(a -> Spirit) -> Thread a -> Hold Earth", "", "where the first element satisfying a test is", "Sequences"},
	{"twist", "Earth -> (a -> a) -> Thread a -> Thread a", "", "twist i f xs: xs with position i put through f, or xs when there is no such position", "Sequences"},

	// Ranges. A Twine is the range, inclusive at both ends, which is how every
	// input that has one writes it.
	{"overlaps", "(a, a) -> (a, a) -> Spirit", "Ord a", "do two inclusive ranges meet at all", "Ranges"},
	{"overlapping", "(a, a) -> (a, a) -> Hold (a, a)", "Ord a", "the range two inclusive ranges share, if they meet", "Ranges"},
	{"within", "(a, a) -> (a, a) -> Spirit", "Ord a", "within outer inner: does the first range hold all of the second", "Ranges"},
	{"spanning", "(a, a) -> (a, a) -> (a, a)", "Ord a", "the smallest range holding both, gaps included", "Ranges"},
	{"holding", "(a, a) -> a -> Spirit", "Ord a", "is this value inside an inclusive range", "Ranges"},
	{"width", "(Earth, Earth) -> Earth", "", "how many Earths an inclusive range holds", "Ranges"},
	{"top", "Earth -> Thread a -> Thread a", "Ord a", "the n largest elements", "Sequences"},
	{"bot", "Earth -> Thread a -> Thread a", "Ord a", "the n smallest elements", "Sequences"},
	{"maxby", "(a -> b) -> Thread a -> Hold a", "Ord b", "the element with the largest key", "Sequences"},
	{"minby", "(a -> b) -> Thread a -> Hold a", "Ord b", "the element with the smallest key", "Sequences"},
	{"pairs", "Thread a -> Thread (a, a)", "", "each element paired with the next", "Sequences"},
	{"cross", "Thread a -> Thread b -> Thread (a, b)", "", "every combination of two Threads", "Sequences"},
	{"combos", "Earth -> Thread a -> Thread (Thread a)", "", "every n-element combination", "Sequences"},
	{"perms", "Thread a -> Thread (Thread a)", "", "every ordering of a Thread", "Sequences"},
	{"compact", "Thread (Hold a) -> Thread a", "", "drop the Stilled entries, unwrap the rest", "Sequences"},
	{"mapcat", "(a -> Thread b) -> Thread a -> Thread b", "", "bend then flatten", "Sequences"},
	{"chunk", "Earth -> Thread a -> Thread (Thread a)", "", "split into runs of n", "Sequences"},
	{"windows", "Earth -> Thread a -> Thread (Thread a)", "", "every overlapping run of n", "Sequences"},
	{"pivot", "Thread (Thread a) -> Thread (Thread a)", "", "swap rows and columns", "Sequences"},
	{"group", "(a -> b) -> Thread a -> Web b (Thread a)", "Eq b", "gather by a derived key", "Sequences"},
	{"idx", "a -> Thread a -> Hold Earth", "Eq a", "where a value first occurs", "Sequences"},
	{"nth", "Earth -> Thread a -> Hold a", "", "the element at a position, if there is one", "Sequences"},
	{"has", "a -> Thread a -> Spirit", "Eq a", "does this Thread hold this value", "Sequences"},
	{"glean", "(a -> Hold b) -> Thread a -> Thread b", "", "bend, keeping only what came back Held", "Sequences"},
	{"harvest", "(a -> Hold b) -> Thread a -> Weaving (Thread b) a", "", "glean, but Gentled with the first element that would not convert", "Sequences"},
	{"cycle", "Thread a -> Thread a", "", "the same Thread over and over, endlessly", "Sequences"},
	{"freq", "Thread a -> Web a Earth", "Eq a", "count how often each element occurs", "Sequences"},
	{"most", "Web a Earth -> Hold a", "", "the key with the highest count", "Sequences"},
	{"contains", "Air -> Air -> Spirit", "", "contains needle haystack", "Sequences"},
	{"earths", "Air -> Thread Earth", "", "every Earth appearing in some text", "Sequences"},
	{"waters", "Air -> Thread Water", "", "every Water appearing in some text", "Sequences"},

	// Text
	{"lines", "Air -> Thread Air", "", "split text into lines", "Text"},
	{"words", "Air -> Thread Air", "", "split text on whitespace", "Text"},
	{"fires", "Air -> Thread Fire", "", "the characters of some text", "Text"},
	{"blocks", "Air -> Thread Air", "", "split text on blank lines", "Text"},
	{"split", "Air -> Air -> Thread Air", "", "split text on a separator", "Text"},
	{"join", "Air -> Thread Air -> Air", "", "join text with a separator", "Text"},
	{"strip", "Air -> Air", "", "remove surrounding whitespace", "Text"},
	{"air", "a -> Air", "Show a", "render any value as text", "Text"},
	{"earth", "Air -> Hold Earth", "", "the Earth this text spells, if it spells one", "Text"},
	{"water", "Air -> Hold Water", "", "the Water this text spells, if it spells one", "Text"},
	{"fire", "Air -> Hold Fire", "", "the one Fire this text holds, if it holds one", "Text"},
	{"upper", "Air -> Air", "", "the same text in upper case", "Text"},
	{"lower", "Air -> Air", "", "the same text in lower case", "Text"},
	{"padl", "Earth -> Fire -> Air -> Air", "", "pad on the left to a width", "Text"},
	{"padr", "Earth -> Fire -> Air -> Air", "", "pad on the right to a width", "Text"},
	{"starts", "Air -> Air -> Spirit", "", "starts prefix text: does text begin with prefix", "Text"},
	{"ends", "Air -> Air -> Spirit", "", "ends suffix text: does text end with suffix", "Text"},
	{"cutstart", "Air -> Air -> Air", "", "remove a prefix if it is there", "Text"},
	{"cutend", "Air -> Air -> Air", "", "remove a suffix if it is there", "Text"},
	{"replace", "Air -> Air -> Air -> Air", "", "replace needle with text everywhere", "Text"},
	{"delve", "Air -> Air -> Hold (Thread Air)", "", "take a line apart against a shape: `{}` keeps a run, everything else must match", "Text"},
	{"repeat", "Earth -> Air -> Air", "", "text repeated n times", "Text"},
	{"bin", "Earth -> Air", "", "the binary digits of an Earth", "Text"},

	// Absence and failure
	{"otherwise", "a -> Hold a -> a", "", "unwrap a Hold, or use a default", "Absence and failure"},
	{"holds", "Hold a -> Spirit", "", "does this Hold contain a value", "Absence and failure"},
	{"rescue", "a -> Weaving a e -> a", "", "unwrap a Weaving, or use a default", "Absence and failure"},
	{"snag", "e -> Weaving a e -> e", "", "the value a Weaving stopped on, or a default", "Absence and failure"},

	// Grids
	{"pattern", "Air -> Pattern Fire", "", "read text as a Pattern of Fires", "Grids"},
	{"weft", "a -> Thread (Thread a) -> Pattern a", "", "weave rows into a Pattern, padding short rows", "Grids"},
	{"spin", "Pattern a -> Pattern a", "", "a quarter turn clockwise", "Grids"},
	{"flip", "Pattern a -> Pattern a", "", "mirrored left to right", "Grids"},
	{"cell", "Pattern a -> Knot -> Hold a", "", "the cell at a knot, if in bounds", "Grids"},
	{"set", "Pattern a -> Knot -> a -> Pattern a", "", "a grid with one cell replaced", "Grids"},
	{"cellwise", "(a -> b) -> Pattern a -> Pattern b", "", "transform every cell, keeping the grid's shape", "Grids"},
	{"knots", "Pattern a -> Thread Knot", "", "every coordinate of a grid", "Grids"},
	{"cells", "Pattern a -> Thread a", "", "every cell of a grid", "Grids"},
	{"rows", "Pattern a -> Earth", "", "how many rows a grid has", "Grids"},
	{"cols", "Pattern a -> Earth", "", "how many columns a grid has", "Grids"},
	{"shape", "Pattern a -> (Earth, Earth)", "", "the rows and columns of a grid", "Grids"},
	{"inb", "Pattern a -> Knot -> Spirit", "", "is this knot inside the grid", "Grids"},
	{"nb4", "Pattern a -> Knot -> Thread a", "", "the four orthogonal neighbours", "Grids"},
	{"nb8", "Pattern a -> Knot -> Thread a", "", "the eight surrounding neighbours", "Grids"},
	{"around4", "Pattern a -> Knot -> Thread Knot", "", "the four neighbouring knots in bounds", "Grids"},
	{"around8", "Pattern a -> Knot -> Thread Knot", "", "the eight neighbouring knots in bounds", "Grids"},
	{"row", "Knot -> Earth", "", "the row of a knot", "Grids"},
	{"col", "Knot -> Earth", "", "the column of a knot", "Grids"},
	{"dirs4", "Thread Knot", "", "the four orthogonal steps", "Grids"},
	{"dirs8", "Thread Knot", "", "the eight surrounding steps", "Grids"},
	{"mdist", "Knot -> Knot -> Earth", "", "the Manhattan distance between two knots", "Grids"},

	// Maps
	{"web", "Thread (k, v) -> Web k v", "Eq k", "build a Web from pairs", "Maps"},
	{"get", "Web k v -> k -> Hold v", "Eq k", "look up a key", "Maps"},
	{"put", "Web k v -> k -> v -> Web k v", "Eq k", "a Web with one key set", "Maps"},
	{"known", "Web k v -> k -> Spirit", "Eq k", "is this key present", "Maps"},
	{"forget", "Web k v -> k -> Web k v", "Eq k", "a Web with one key removed", "Maps"},
	{"keys", "Web k v -> Thread k", "", "every key", "Maps"},
	{"vals", "Web k v -> Thread v", "", "every value", "Maps"},
	{"items", "Web k v -> Thread (k, v)", "", "every key and value together", "Maps"},
	{"merge", "Web k v -> Web k v -> Web k v", "Eq k", "merge two Webs, the second winning", "Maps"},
	{"mapvals", "(v -> w) -> Web k v -> Web k w", "Eq k", "transform every value, keeping the keys", "Maps"},

	// Sets
	{"circle", "Thread a -> Circle a", "Eq a", "gather a Thread into a Circle", "Sets"},
	{"member", "Circle a -> a -> Spirit", "Eq a", "is this value in the Circle", "Sets"},
	{"insert", "Circle a -> a -> Circle a", "Eq a", "a Circle with one value added", "Sets"},
	{"remove", "Circle a -> a -> Circle a", "Eq a", "a Circle with one value removed", "Sets"},
	{"members", "Circle a -> Thread a", "", "every value in a Circle", "Sets"},
	{"union", "Circle a -> Circle a -> Circle a", "Eq a", "everything in either Circle", "Sets"},
	{"inter", "Circle a -> Circle a -> Circle a", "Eq a", "everything in both Circles", "Sets"},
	{"diff", "Circle a -> Circle a -> Circle a", "Eq a", "everything in the first but not the second", "Sets"},

	// Priority queues and graphs
	{"taveren", "Thread a -> Taveren a", "Ord a", "build a queue from a Thread", "Priority queues and graphs"},
	{"push", "Taveren a -> a -> Taveren a", "Ord a", "add a value to the queue", "Priority queues and graphs"},
	{"pop", "Taveren a -> Hold (a, Taveren a)", "Ord a", "take the smallest value", "Priority queues and graphs"},
	{"dijkstra", "(a -> Thread (Earth, a)) -> a -> Web a Earth", "Ord a", "cheapest cost to every node reachable from here, given a step function", "Priority queues and graphs"},
	{"reach", "(a -> Thread a) -> a -> Circle a", "Eq a", "every place reachable from here, given a step function", "Priority queues and graphs"},
	{"route", "(a -> Thread (Earth, a)) -> a -> a -> Hold (Thread a)", "Ord a", "the cheapest path from one place to another, if there is one", "Priority queues and graphs"},
	{"toposort", "(a -> Thread a) -> Thread a -> Hold (Thread a)", "Eq a", "the nodes ordered so every edge points forwards, or Stilled on a cycle", "Priority queues and graphs"},

	// Numbers
	{"add", "a -> a -> a", "Reckon a", "add two numbers", "Numbers"},
	{"sub", "a -> a -> a", "Reckon a", "subtract the second from the first", "Numbers"},
	{"mul", "a -> a -> a", "Reckon a", "multiply two numbers", "Numbers"},
	{"div", "a -> a -> a", "Reckon a", "divide the first by the second", "Numbers"},
	{"mod", "Earth -> Earth -> Earth", "", "remainder after division", "Numbers"},
	{"gcd", "Earth -> Earth -> Earth", "", "greatest common divisor", "Numbers"},
	{"lcm", "Earth -> Earth -> Earth", "", "least common multiple", "Numbers"},
	{"inc", "a -> a", "Reckon a", "one more", "Numbers"},
	{"dec", "a -> a", "Reckon a", "one less", "Numbers"},
	{"abs", "a -> a", "Reckon a", "magnitude, without sign", "Numbers"},
	{"neg", "a -> a", "Reckon a", "the negation of a number", "Numbers"},
	{"min", "a -> a -> a", "Ord a", "the smaller of two values", "Numbers"},
	{"max", "a -> a -> a", "Ord a", "the larger of two values", "Numbers"},
	{"even", "Earth -> Spirit", "", "is this divisible by two", "Numbers"},
	{"odd", "Earth -> Spirit", "", "is this not divisible by two", "Numbers"},
	{"divBy", "Earth -> Earth -> Spirit", "", "divBy d n: is n divisible by d", "Numbers"},
	{"sign", "a -> Earth", "Reckon a", "-1, 0 or 1", "Numbers"},
	{"sqrt", "a -> Water", "Reckon a", "square root", "Numbers"},
	{"cbrt", "a -> Water", "Reckon a", "cube root", "Numbers"},
	{"ceil", "a -> Earth", "Reckon a", "round up", "Numbers"},
	{"floor", "a -> Earth", "Reckon a", "round down", "Numbers"},
	{"round", "a -> Earth", "Reckon a", "round to nearest", "Numbers"},
	{"clamp", "a -> a -> a -> a", "Ord a", "clamp lo hi x: hold x between two bounds", "Numbers"},
	{"pow", "a -> a -> a", "Reckon a", "pow base exponent", "Numbers"},
	{"bor", "Earth -> Earth -> Earth", "", "bitwise or", "Numbers"},
	{"band", "Earth -> Earth -> Earth", "", "bitwise and", "Numbers"},
	{"bxor", "Earth -> Earth -> Earth", "", "bitwise exclusive or", "Numbers"},
	{"bnot", "Earth -> Earth", "", "bitwise complement", "Numbers"},
	{"shl", "Earth -> Earth -> Earth", "", "shl n x: shift x left by n", "Numbers"},
	{"shr", "Earth -> Earth -> Earth", "", "shr n x: shift x right by n", "Numbers"},
	{"pi", "Water", "", "the circle constant", "Numbers"},
	{"e", "Water", "", "the base of the natural logarithm", "Numbers"},
	{"inf", "Water", "", "positive infinity", "Numbers"},

	// Comparison
	{"eq", "a -> a -> Spirit", "Eq a", "are two values equal", "Comparison"},
	{"neq", "a -> a -> Spirit", "Eq a", "are two values different", "Comparison"},
	{"lt", "a -> a -> Spirit", "Ord a", "lt b a: is a less than b", "Comparison"},
	{"lte", "a -> a -> Spirit", "Ord a", "lte b a: is a at most b", "Comparison"},
	{"gt", "a -> a -> Spirit", "Ord a", "gt b a: is a greater than b", "Comparison"},
	{"gte", "a -> a -> Spirit", "Ord a", "gte b a: is a at least b", "Comparison"},

	// Logic
	{"and", "Spirit -> Spirit -> Spirit", "", "are both true", "Logic"},
	{"or", "Spirit -> Spirit -> Spirit", "", "is either true", "Logic"},
	{"not", "Spirit -> Spirit", "", "the opposite", "Logic"},
	{"pick", "Spirit -> a -> a -> a", "", "pick c a b: a when c is Light, else b", "Logic"},

	// Characters
	{"isDigit", "Fire -> Spirit", "", "is this character a digit", "Characters"},
	{"isAlpha", "Fire -> Spirit", "", "is this character a letter", "Characters"},
	{"isSpace", "Fire -> Spirit", "", "is this character whitespace", "Characters"},
	{"ord", "Fire -> Earth", "", "the code point of a Fire", "Characters"},
	{"spark", "Earth -> Fire", "", "the Fire with a code point", "Characters"},
	{"digit", "Fire -> Hold Earth", "", "the value of a decimal Fire", "Characters"},
}

// Ctor is a data constructor: how a value of a sum type is built, and how it is
// taken apart in a pattern.
type Ctor struct {
	Name string
	// Sig is the constructor's type, e.g. "a -> Hold a".
	Sig string
	// Owner names the type constructor this belongs to.
	Owner string
	// Arity is how many fields the constructor carries.
	Arity int
	Doc   string
}

// Ctors are the built-in data constructors. `knot` is lower-case because it
// reads better applied (`knot 2 3`), and Weave allows a lower-case constructor
// in pattern position for exactly this case.
var Ctors = []Ctor{
	{"Light", "Spirit", "Spirit", 0, "truth"},
	{"Shadow", "Spirit", "Spirit", 0, "falsehood"},

	{"Held", "a -> Hold a", "Hold", 1, "a value that is present"},
	{"Stilled", "Hold a", "Hold", 0, "nothing here: Weave's answer to null"},

	{"Woven", "a -> Weaving a e", "Weaving", 1, "a successful result"},
	{"Gentled", "e -> Weaving a e", "Weaving", 1, "a failed result, with a reason"},

	{"knot", "Earth -> Earth -> Knot", "Knot", 2, "a grid coordinate"},
}

// TypeCtors lists, for each type constructor with a finite set of data
// constructors, what that set is. Exhaustiveness checking uses this to decide
// whether a `ward` covers every case. Types absent from this map (Earth, Air,
// Thread, ...) have unbounded value sets, so a match on them always needs a
// wildcard or variable arm.
var TypeCtors = map[string][]string{
	"Spirit":  {"Light", "Shadow"},
	"Hold":    {"Held", "Stilled"},
	"Weaving": {"Woven", "Gentled"},
	"Knot":    {"knot"},
}
