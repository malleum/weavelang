package docs

// The glosses are the reference's second voice.
//
// Every verb already carries a plain one-line description, written for someone
// reading a compiler error, and that stays: it is what the signature is looked
// up alongside. The gloss is the other thing — what the verb is *called*, in
// the vocabulary the language is named out of. It explains nothing to anyone
// who does not already know, which is the point: the reference is for people
// who write Weave, and a verb whose name is `braid` is not clarified by being
// told it is a fold.
//
// A gloss compares a verb only to other Weave verbs and Weave types. Nothing
// here mentions another programming language, because in here there is not
// one.
//
// Eight of them say two or three of their words in a language that is not
// English, once each and never twice. They are not marked and they are not
// translated. You will find them by reading.

// glosses maps a verb to its gloss. A verb without one falls back to its
// group's, below, which is why adding a verb can never leave a blank card.
var glosses = map[string]string{
	// ------------------------------------------------------------- input
	"Source": "what was handed to the weave before it began. Air, always; shape it yourself.",

	// --------------------------------------------------------- sequences
	"bend":      "every strand put through the same turn, and the Thread comes back the length it went in.",
	"sift":      "the Thread minus what the test turned down. `cull` is this one inverted.",
	"braid":     "the Thread wound down to one. Every other consumer here is this one with its answer decided in advance.",
	"seek":      "the first strand the test accepts, and nothing after it is looked at. `seekidx` says where instead.",
	"span":      "an ascending Thread with nothing between the ends. Built in the loop, not before it.",
	"under":     "the places of that many things: zero up to it, and not it. `span` names both ends because an input that writes one does.",
	"copies":    "the same strand over and over, that many times. `repeat` is this for Air.",
	"flow":      "Ú-veth. It exists only inside a chain that ends it — `take`, `takewhile`, `seek`, `first`, `any`, `all`, `dupe`, `gentle`.",
	"len":       "how much is there. Bulk, so it answers for Air as readily as for a Web.",
	"count":     "`len` after `sift`, in one pass and without the Thread in between.",
	"sum":       "`braid add 0`, except that it knows what zero means for the Power it is over.",
	"prod":      "`sum` under multiplication.",
	"sums":      "`scan add 0`. The two are the same verb and one of them is shorter.",
	"prods":     "`sums` under multiplication.",
	"take":      "the front of it, sharing its storage. It bounds an endless producer, and on Air it counts runes.",
	"drop":      "what `take` left, sharing the same storage.",
	"takewhile": "`take` that decides for itself where to stop.",
	"dropwhile": "the Thread from the first strand the test turns down.",
	"turn":      "the wheel turned: what came off the front goes on the back, and a negative count turns the other way.",
	"wrap":      "`nth` on a ring: the index goes round rather than falling off, so `neg 1` is the last strand.",
	"zip":       "two Threads read at once as Twines, shortest first to run out. The Twine is never built if nothing wants it whole.",
	"zipwith":   "`zip` with the pairing already done, so no Twine exists at any point.",
	"thread":    "a Twine of two of a kind, read as the Thread it already was.",
	"weld":      "two Threads made one. Sømmen synes ikke.",
	"mend":      "one strand replaced. On a Thread nothing else can see, it is written where it lies.",
	"sever":     "`take` and `drop` at once, one pass, both halves on the original storage.",
	"strands":   "runs of neighbours whose key agrees, in the order they lay.",
	"plait":     "`zip` that keeps going after the shorter Thread has run out.",
	"cull":      "`sift` refusing what `sift` would keep.",
	"bendr":     "`bend` that also descends into the Threads it finds.",
	"siftr":     "`sift`, all the way down.",
	"zipr":      "`zip`, all the way down.",
	"sort":      "ascending, by the Ord the elements already have.",
	"sortby":    "`sort` against a key drawn from each element rather than the element.",
	"all":       "does nothing turn the test down. Stops at the first that does.",
	"any":       "does anything satisfy it. Stops at the first that does.",
	"none":      "`any` inverted — ghobe' — and it stops in the same place.",
	"first":     "the strand at the head, Held, or Stilled when there is no head.",
	"second":    "`nth 1`, spelled for the case that comes up.",
	"last":      "the strand at the far end.",
	"rev":       "the same strands, laid the other way, on storage of its own. Air turns round by rune, not by byte.",
	"flat":      "one layer of Thread taken off.",
	"uniq":      "repeats dropped, unua okazo kept, order untouched.",
	"enum":      "each strand twined with where it lies.",
	"scan":      "`braid` that answers every total it passed through, one per strand. `sums` is this at `add 0`.",
	"priors":    "`scan` that keeps where it started, so it is one longer than what it walked. The shape a range asks its total in.",
	"gentle":    "`braid` that may stop. The step answers Woven to go on and Gentled to end it there; read the end back out with `snag`.",
	"dupe":      "where the Thread first repeats itself, and what it repeated. `seek` with a memory, which no test alone can be.",
	"high":      "the largest strand. `maxby` with the strand as its own key.",
	"low":       "the smallest. `minby`, likewise.",
	"highidx":   "where `high` found it. Asking `idx` afterwards is a second pass and answers wrongly when the value repeats.",
	"lowidx":    "where `low` found it, for the same reason.",
	"seekidx":   "where `seek` would have stopped.",
	"siftidx":   "where every strand `sift` would have kept lies. `seek`/`sift` was the one pair the language had both halves of; this is the other half of `seekidx`.",
	"idxs":      "every place a value lies, not only the first. `idx` is this one stopping early.",
	"twist":     "`mend` when the new strand is drawn from the old one.",
	"top":       "the largest few, largest first.",
	"bot":       "the smallest few.",
	"maxby":     "the strand whose key is largest. `high` is this with the strand as its own key.",
	"minby":     "the strand whose key is smallest.",
	"pairs":     "each strand twined with the one after it, so one shorter than the Thread. `couples` is every two.",
	"cross":     "every Twine of one from each.",
	"combos":    "every choice of that many, order disregarded.",
	"perms":     "every ordering. Crescit ut necesse est.",
	"compact":   "the Held kept and unwrapped, the Stilled dropped. `glean` is this with a `bend` in front.",
	"mapcat":    "`bend` then `flat`, without the Thread of Threads in between.",
	"chunk":     "cut into lengths of that many, the last one short if it must be.",
	"windows":   "every run of that many, overlapping, in order.",
	"pivot":     "rows for columns. A Thread of Threads turned on its side.",
	"group":     "a Web from a key to every strand that answered to it.",
	"idx":       "where a value first lies. `seekidx` takes a test instead.",
	"nth":       "the strand at a place, Held, or Stilled when there is no such place.",
	"has":       "is that value anywhere in the Thread.",
	"glean":     "`bend` keeping only what came back Held, which is how a Thread changes its Power.",
	"harvest":   "`glean` that refuses to lose anything: Gentled with the first strand that would not convert.",
	"cycle":     "the Thread over and over, endlessly. Bounded exactly as `flow` is.",
	"freq":      "a Web from each value to how often it lay there. `index` remembers where instead.",
	"most":      "the key `freq` counted highest.",
	"contains":  "is that run of text laid inside this one, unbroken.",
	"earths":    "every Earth the Air holds, sign and all. Most inputs are a shape wrapped round a few numbers.",
	"spans":     "every `11-22` the Air holds. `earths` reads that dash as a sign, and has to: `x=-5` is one number.",
	"waters":    "`earths` for the other Power. A run of digits with no point still counts.",

	// ------------------------------------------------------------ ranges
	"overlaps":    "do the two meet at all. `overlapping` says how much.",
	"overlapping": "the range they share, Held, or Stilled when they do not meet.",
	"within":      "does the first hold every part of the second.",
	"spanning":    "the smallest range round both, gap and all.",
	"holding":     "is the value inside the range, ends included.",
	"width":       "how many Earths lie in the range. Zero when it is turned round.",

	// -------------------------------------------------------------- text
	"lines":    "Air cut at every break.",
	"words":    "Air cut at every run of space.",
	"fires":    "Air read as the Fires it is made of.",
	"blocks":   "Air cut where a line was left empty, which is how a paragraph-shaped input arrives.",
	"split":    "Air cut at every occurrence. Given nothing to cut at, one Air per Fire.",
	"join":     "the Thread laid back into Air with that between each.",
	"strip":    "the רווח at either end taken off.",
	"air":      "anything at all, rendered the way `weave run` would render it.",
	"earth":    "the Earth this Air spells, Held, or Stilled when it spells none.",
	"water":    "the same for the other Power.",
	"fire":     "the one Fire this Air holds, if it holds exactly one.",
	"upper":    "raised.",
	"lower":    "lowered.",
	"padl":     "widened on the left to that width, or left alone when it is wide enough.",
	"padr":     "widened on the right.",
	"starts":   "does the Air begin that way.",
	"ends":     "does it end that way.",
	"cutstart": "that prefix taken off, or the Air unchanged when it was not there.",
	"cutend":   "the same at the other end.",
	"replace":  "every occurrence swapped.",
	"delve":    "an Air read against a shape: `{}` keeps a run and everything else must match exactly. Stilled when it does not.",
	"repeat":   "the Air over and over, that many times. Finite, unlike `cycle`.",

	// ------------------------------------------------- absence and failure
	"otherwise": "the Held unwrapped, or the value you brought. `else` is this spelled as a particle.",
	"woven":     "did it come back Woven. `holds` asks the same of a Hold.",
	"holds":     "is there anything in there.",
	"rescue":    "`otherwise` for a Weaving, taking the Woven side.",
	"snag":      "`rescue` from the other side: what the weaving caught on. `failing` is this spelled as a particle.",

	// ------------------------------------------------------------- grids
	"pattern":  "Air read as a Pattern, one Fire to a cell.",
	"warp":     "a grid laid out before anything is on it: the shape, and what belongs at each knot. `weft` weaves rows you already have.",
	"weft":     "Threads woven into a Pattern, short rows filled out with what you brought.",
	"spin":     "a quarter turn, sunwise.",
	"flip":     "mirrored, left for right.",
	"cell":     "what lies at that Knot, Held, or Stilled when the Knot is outside.",
	"set":      "one cell replaced. On a Pattern nothing else can see, it is written where it lies.",
	"cellwise": "`bend` over a Pattern, the shape kept.",
	"knots":    "every Knot in the Pattern, row by row.",
	"cells":    "every cell, row by row. It hands back the Pattern's own storage, which is why naming it ends the Pattern's ownership.",
	"rows":     "how many rows.",
	"cols":     "how many columns.",
	"shape":    "`rows` and `cols` twined.",
	"inb":      "is that Knot inside.",
	"row":      "the row half of a Knot.",
	"col":      "the column half.",
	"mdist":    "how many `nb4` steps lie between two Knots, walls disregarded.",
	"nb4":      "what lies at the four Knots that share an edge, those outside the Pattern left out.",
	"nb8":      "the same for the eight that share an edge or a corner.",
	"around4":  "`nb4` answering with the Knots rather than what is in them.",
	"around8":  "`nb8`, likewise.",
	"dirs4":    "the four steps, as Knots to be added to another.",
	"dirs8":    "the eight.",

	// -------------------------------------------------------------- maps
	"web":     "a Web gathered out of a Thread of Twines. The last of any repeated key wins.",
	"get":     "what that key holds, Held, or Stilled when it holds nothing.",
	"put":     "a key set. On a Web nothing else can see, the path is written rather than copied.",
	"known":   "is that key in there.",
	"forget":  "that key taken out.",
	"keys":    "every key, ascending.",
	"vals":    "every value, in the order of their keys.",
	"items":   "every key twined with its value, ascending. The Twine is never built if nothing wants it whole.",
	"merge":   "two Webs made one, the second winning every key they share.",
	"mapvals": "`bend` over the values, the keys untouched.",

	// -------------------------------------------------------------- sets
	"circle":  "a Circle gathered out of a Thread.",
	"member":  "is that value in the Circle.",
	"insert":  "one value added. On a Circle nothing else can see, the path is written rather than copied.",
	"remove":  "one value taken out.",
	"members": "every value, ascending.",
	"union":   "everything in either.",
	"inter":   "only what is in both.",
	"covers":  "does the first Circle already hold all of the second. `within` asks it of two ranges.",
	"diff":    "what the first has and the second does not.",

	// -------------------------------------------- priority queues and graphs
	"taveren":  "a Taveren gathered out of a Thread. The smallest comes off first, whatever order they went in.",
	"push":     "one value in.",
	"pop":      "the smallest out, twined with what is left, or Stilled when there is nothing.",
	"dijkstra": "the cost of reaching everywhere reachable, given a step function from a place to the (cost, place) that lead out of it. It owns its frontier and its visited set, so asking twice about one graph costs once.",
	"reach":    "everywhere reachable, cost disregarded. A flood fill, a region, a connected area.",
	"route":    "the path rather than the cost, Held, or Stilled when there is none — and there is none through the death gate.",
	"toposort": "the nodes laid so that every edge points forwards, or Stilled when they cannot be.",
	"clumps":   "`reach` from every node at once. Regions of a grid, clusters of a graph, islands.",
	"link":     "every node alone, waiting to be joined. What `clumps` answers all at once, a Link answers as it happens.",
	"bind":     "two circles made one. Threaded through a loop it writes where it lies; held twice it copies eight bytes a node.",
	"bound":    "are these two in one circle. The question `clumps` cannot be asked while the joining is still going on.",
	"clumped":  "the circles a Link has come to, each once. `clumps` for a Link that was built rather than walked.",
	"base":     "an Earth spelled in any base from two to thirty-six. `air` is this at ten.",
	"unbase":   "an Air of 31 and 30 read as the Earth it spells, in any base from two to thirty-six. `earth` is this at ten.",
	"sited":    "where a value sits, which is `cell` asked the other way round. `sites` gives every one.",
	"sites":    "every knot holding that value, in reading order. `sited` stops at the first.",
	"settle":   "apply until a round changes nothing. A steady state, an erosion, a settling.",
	"couples":  "every two strands, each pair once, in the order they lay. `pairs` takes only the neighbours.",
	"index":    "the Web from value to where it sits. `freq` counts them instead; this remembers where.",
	"squeeze":  "a plane too large to draw made drawable: each coordinate a line, each gap one line.",
	"mesh":     "overlapping ranges rolled into the fewest that cover the same ground.",
	"carve":    "`words` with the separators named. Input with punctuation in it, which is most input.",
	"tallies":  "the grid with every cell holding the box above and left of it. Asked once, read many times.",
	"tallied":  "a box out of a tallied grid: one subtraction, whatever its size. The four corners are the verb.",

	// ----------------------------------------------------------- numbers
	"add":   "together.",
	"sub":   "`sub a b` is b less a, because the second argument is the one a pipeline hands over.",
	"mul":   "multiplied.",
	"div":   "divided, truncating towards nothing.",
	"mod":   "what division left over, taking the sign of the divisor.",
	"gcd":   "the largest that divides both.",
	"lcm":   "the smallest both divide.",
	"inc":   "`add 1`, because stepping by one is half the arithmetic there is.",
	"dec":   "`sub 1`.",
	"abs":   "without its sign.",
	"neg":   "with its sign turned. There is no operator for this; there is no operator for anything.",
	"min":   "the smaller of two. `low` is the one over a Thread.",
	"max":   "the larger of two. `high` is the one over a Thread.",
	"even":  "is it divisible by two.",
	"odd":   "is it not.",
	"divBy": "`divBy d n`: does d divide n.",
	"sign":  "one of three answers, and it is an Earth rather than a Spirit because there are three.",
	"sqrt":  "the square root, as a Water.",
	"cbrt":  "the cube root.",
	"ceil":  "up.",
	"floor": "down.",
	"round": "to the nearer, halves away from nothing.",
	"clamp": "held between two bounds.",
	"pow":   "raised to.",
	"bor":   "bitwise, either.",
	"band":  "bitwise, both.",
	"bxor":  "bitwise, one but not both.",
	"bnot":  "every bit turned.",
	"shl":   "the bits moved up.",
	"shr":   "the bits moved down.",
	"pi":    "ο κύκλος's own number.",
	"e":     "the number that is its own rate of change.",
	"inf":   "further than any Water goes.",

	// -------------------------------------------------------- comparison
	"eq":  "the same. Structural, all the way down, for anything with the Eq Talent.",
	"neq": "`eq` inverted.",
	"lt":  "`lt a b` is b below a. The bound comes first so that `sift (lt 10)` reads as it should.",
	"lte": "below or level.",
	"gt":  "`gt a b` is b above a, for the same reason.",
	"gte": "above or level.",

	// ------------------------------------------------------------- logic
	"and":  "both.",
	"or":   "either.",
	"not":  "the other one.",
	"pick": "`pick c a b`: a when c is Light, b otherwise. Only the taken side runs, and both sides are tail position — the ward you write when a ward is too much ceremony.",

	// -------------------------------------------------------- characters
	"isDigit": "is that Fire one of ten.",
	"isAlpha": "is it a letter.",
	"isSpace": "is it space.",
	"ord":     "the Fire's code point.",
	"spark":   "the Fire a code point names. `ord` undone.",
	"digit":   "what that Fire is worth as a figure, Held, or Stilled when it is not one.",
}

// groupGlosses stand in for a verb with none of its own, so that a verb added
// to the prelude cannot leave an empty card here.
var groupGlosses = map[string]string{
	"Input":                      "what arrived.",
	"Sequences":                  "over a Thread.",
	"Ranges":                     "over a Twine read as a range, inclusive at both ends.",
	"Text":                       "over Air.",
	"Absence and failure":        "over a Hold or a Weaving.",
	"Grids":                      "over a Pattern, indexed by Knot.",
	"Maps":                       "over a Web.",
	"Sets":                       "over a Circle.",
	"Priority queues and graphs": "over a Taveren, or over a graph given as a step function.",
	"Numbers":                    "over Earth or Water.",
	"Comparison":                 "against the Ord a value already has.",
	"Logic":                      "over Spirit.",
	"Characters":                 "over Fire.",
}

// glossOf answers for every verb, which is what the test that walks the
// prelude relies on.
func glossOf(name, group string) string {
	if g, ok := glosses[name]; ok {
		return g
	}
	if g, ok := groupGlosses[group]; ok {
		return g
	}
	return "it does what it is called."
}
