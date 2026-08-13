package build_test

import (
	"strings"
	"testing"
)

// A Thread of Earths is stored eight bytes to the element rather than sixteen,
// with the tag they all share held once in the header. See the layout note in
// internal/rt/csrc/weave.h.
//
// The layout is meant to be invisible, and these are what say so: every verb
// that reaches past one element at a time, run against a Thread that is packed
// and against the same Thread written out as a literal, which is not. A verb
// that forgets — that indexes the array itself, or hands a window back without
// saying whose storage it is — gives a different answer here, or crashes.

// packing names the two ways of getting the same Thread. `earths` builds a
// packed one; a literal is an array of Values, as everything that is not all
// one immediate Power has to be.
var packings = []struct{ name, three, five string }{
	{"packed", `earths "1 2 3"`, `earths "5 3 1 4 2"`},
	{"boxed", `[1, 2, 3]`, `[5, 3, 1, 4, 2]`},
}

func TestPackedThreadsAnswerLikeOrdinaryOnes(t *testing.T) {
	// Each case is an expression in `xs`, a Thread of three, and `ys`, a Thread
	// of five in no order.
	cases := []struct{ name, expr, want string }{
		{"nth", `nth 1 xs | otherwise 0`, "2"},
		{"len", `len xs`, "3"},
		{"sum", `sum xs`, "6"},
		{"rev", `air (rev xs)`, "[3 2 1]"},
		{"rev of a window", `air (rev (drop 1 xs))`, "[3 2]"},
		{"sort", `air (sort ys)`, "[1 2 3 4 5]"},
		{"sortby", `air (sortby (x : neg x) ys)`, "[5 4 3 2 1]"},
		{"top", `air (top 2 ys)`, "[5 4]"},
		{"bot", `air (bot 2 ys)`, "[1 2]"},
		{"take", `air (take 2 xs)`, "[1 2]"},
		{"drop", `air (drop 1 xs)`, "[2 3]"},
		{"take past the end", `air (take 99 xs)`, "[1 2 3]"},
		{"sever", `sever 1 xs`, "([1], [2 3])"},
		{"weld", `air (weld xs xs)`, "[1 2 3 1 2 3]"},
		{"weld a window", `air (weld (take 1 xs) (drop 2 xs))`, "[3 1]"},
		{"mend", `air (mend 1 9 xs)`, "[1 9 3]"},
		{"wind", `air (wind (neg 1) 9 xs)`, "[1 2 9]"},
		{"turn", `air (turn 1 xs)`, "[2 3 1]"},
		{"wrap", `wrap (neg 1) xs | otherwise 0`, "3"},
		{"repeat", `air (repeat 2 xs)`, "[1 2 3 1 2 3]"},
		{"chunk", `air (chunk 2 xs)`, "[[1 2] [3]]"},
		{"windows", `air (windows 2 xs)`, "[[1 2] [2 3]]"},
		{"flat", `air (flat [xs, xs])`, "[1 2 3 1 2 3]"},
		{"pivot", `air (pivot [xs, xs])`, "[[1 1] [2 2] [3 3]]"},
		{"plait", `air (plait xs xs)`, "[1 1 2 2 3 3]"},
		{"strands", `air (strands (x : gt 1 x) xs)`, "[[1] [2 3]]"},
		{"uniq", `air (uniq (weld xs xs))`, "[1 2 3]"},
		{"takewhile", `air (xs | takewhile (x : lt 3 x))`, "[1 2]"},
		{"dropwhile", `air (xs | dropwhile (x : lt 3 x))`, "[3]"},
		{"zip", `air (zip xs xs | bend ((a, b) : add a b))`, "[2 4 6]"},
		{"equality", `eq xs [1, 2, 3]`, "Light"},
		{"ordering", `lt ys xs`, "Light"},
		{"as a Web key", `get (web [(xs, 7)]) xs | otherwise 0`, "7"},
		{"as a Circle member", `member (circle [xs]) xs`, "Light"},
		{"rendered", `air xs`, "[1 2 3]"},
		{"joined", `xs | bend air | join ","`, "1,2,3"},
		// A packed Thread that is asked for a Text has to stop being packed,
		// and everything above still has to work on it afterwards.
		{"unpacked then read", `sum (mend 1 9 xs)`, "13"},
	}
	for _, tc := range cases {
		for _, p := range packings {
			t.Run(tc.name+"/"+p.name, func(t *testing.T) {
				src := "xs is " + p.three + "\nys is " + p.five + "\n" + tc.expr + "\n"
				got := strings.TrimRight(compileAndRun(t, "packed.weave", src, ""), "\n")
				if got != tc.want {
					t.Errorf("got %q, want %q", got, tc.want)
				}
			})
		}
	}
}

// A fused chain whose result is a Thread of Earths collects payloads rather
// than Values, so this is the other producer of a packed Thread — and the one
// that has to agree with the unfused verbs it replaces.
func TestFusedCollectPacksEarths(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"bend over a span", "span 1 5 | bend (x : mul x x) | air\n", "[1 4 9 16 25]"},
		{"sifted", "span 1 9 | sift (x : eq 0 (mod x 3)) | air\n", "[3 6 9]"},
		{"a window of one", "span 1 5 | bend (x : mul x 2) | drop 3 | air\n", "[8 10]"},
		{"welded to itself", "xs is span 1 3 | bend (x : add x 1)\nweld xs xs | air\n", "[2 3 4 2 3 4]"},
		{"sorted", "span 1 5 | bend (x : mod (mul x 7) 5) | sort | air\n", "[0 1 2 3 4]"},
		{"an empty result", "span 1 5 | sift (x : gt 99 x) | len | air\n", "0"},
		// A `flow` has no length to size against, so its buffer doubles: the
		// packed one has to regrow the same way.
		{"a growing buffer", "flow (x : add x 3) 0 | take 40 | sum | air\n", "2340"},
		// An odd number of elements does not end on the sixteen-byte boundary
		// the allocator rounds to, which is the one thing a buffer of payloads
		// has to get right that a buffer of Values does not.
		{"an odd length", "span 1 7 | bend (x : mul x 2) | sever 3 | air\n", "([2 4 6], [8 10 12 14])"},
		{"an odd length reversed", "span 1 5 | bend (x : mul x 2) | rev | air\n", "[10 8 6 4 2]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(compileAndRun(t, "collect.weave", tc.src, ""), "\n")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
