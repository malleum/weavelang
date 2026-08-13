package build_test

import (
	"strings"
	"testing"

	"github.com/malleum/weave/internal/build"
	"github.com/malleum/weave/internal/diag"
)

// The Ply Talent: `take`, `drop`, `sever` and `rev` work on text as readily as
// on a Thread, which is what makes a substring one verb rather than the round
// trip through `fires` and back.
//
// They cut by rune and not by byte, so that `take` agrees with `len` and a
// character never comes apart in the middle.
func TestPlyCutsTextAndThreadsAlike(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"take from text", `take 5 "hello world"`, "hello"},
		{"drop from text", `drop 6 "hello world"`, "world"},
		{"take past the end", `take 99 "short"`, "short"},
		{"drop past the end", `neq "" (drop 99 "short")`, "Shadow"},
		{"take none", `neq "" (take 0 "short")`, "Shadow"},
		{"rev text", `rev "stressed"`, "desserts"},
		// Quoted, because the halves are text and the second one begins with the
		// space: `(hello,  world)` cannot be read back.
		{"sever text", `sever 5 "hello world"`, `("hello", " world")`},

		{"take from a Thread", `air (take 2 [1, 2, 3])`, "[1 2]"},
		{"drop from a Thread", `air (drop 2 [1, 2, 3])`, "[3]"},
		{"rev a Thread", `air (rev [1, 2, 3])`, "[3 2 1]"},
		{"sever a Thread", `sever 1 [1, 2, 3]`, "([1], [2 3])"},

		// Runes, not bytes. Each of these characters is two bytes.
		{"take counts runes", `take 3 "naïve"`, "naï"},
		{"drop counts runes", `drop 3 "naïve"`, "ve"},
		{"rev keeps a rune whole", `rev "naïve"`, "evïan"},
		{"len agrees with take", `len (take 4 "naïve")`, "4"},
		{"a slice still parses", `drop 1 "x42" | earth | otherwise 0`, "42"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(compileAndRun(t, tc.name+".weave", tc.src+"\n", ""), "\n")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// `take` on text must not be read as a fused Thread stage: the loop fuser
// recognises its stages by name, and the name now covers two different things.
func TestTakeOnTextIsNotFused(t *testing.T) {
	const src = "Source | take 3\n"
	if got := strings.TrimRight(compileAndRun(t, "cut.weave", src, "hello world"), "\n"); got != "hel" {
		t.Errorf("got %q, want %q", got, "hel")
	}
}

// `spans` reads the range syntax that `earths` cannot, and `earths` keeps the
// reading it has to keep: a dash before a digit is a sign when there is no
// digit in front of it.
func TestSpansReadsRanges(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"a comma-separated list", `air (spans "11-22,95-115,998-1012")`, "[(11, 22) (95, 115) (998, 1012)]"},
		{"one per line", `air (spans "3-5\n10-14\n")`, "[(3, 5) (10, 14)]"},
		{"spaces around them", `air (spans "  7 - 9  , 11-22")`, "[(11, 22)]"},
		{"a bare number is not a range", `air (spans "5")`, "[]"},
		{"earths still reads a sign", `air (earths "x=-5, y=3")`, "[-5 3]"},
		{"earths still reads a dash as a sign", `air (earths "11-22")`, "[11 -22]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(compileAndRun(t, tc.name+".weave", tc.src+"\n", ""), "\n")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// `priors` is `scan` with the value it started from kept, so it is one longer.
// That is the shape a prefix sum wants: the total over a range is one
// subtraction, and an empty range needs no special case.
func TestPriorsKeepsTheSeed(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"running totals with the start", `air (priors add 0 [1, 2, 3])`, "[0 1 3 6]"},
		{"scan leaves it out", `air (scan add 0 [1, 2, 3])`, "[1 3 6]"},
		{"one longer than the Thread", `len (priors add 0 [1, 2, 3, 4])`, "5"},
		{"the empty Thread keeps the seed", `air (priors add 7 [])`, "[7]"},
		{
			"a range total is one subtraction",
			`t is priors add 0 [5, 1, 9, 2]` + "\n" +
				`total lo hi is sub (nth hi t | otherwise 0) (nth lo t | otherwise 0)` + "\n" +
				`[total 1 3, total 0 4] | bend air | join " "`,
			"10 17",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(compileAndRun(t, tc.name+".weave", tc.src+"\n", ""), "\n")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A Web has a length and no order, so it is Bulk and not Ply. Keeping the two
// Talents apart is the whole reason `take` did not simply join `len`.
func TestPlyRefusesWhatHasNoOrder(t *testing.T) {
	for _, src := range []string{
		"take 1 (web [(1, 2)])\n",
		"rev (circle [1, 2])\n",
	} {
		bag := diag.New("no-order.weave", src)
		if _, err := build.Compile("no-order.weave", src, build.Options{}, bag); err == nil {
			t.Errorf("%q compiled, but a Web and a Circle have no order to cut", src)
		}
	}
}

// `uniq` keeps the first of every value in the order they appeared. It used to
// compare each element against everything already kept, which is quadratic:
// nobody notices on a hand-written Thread, and it was forty seconds of the
// forty-four that Advent of Code 2025 day 2 took. A hundred thousand elements
// is the size at which the difference is unmistakable.
func TestUniqIsNotQuadratic(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"first of each, in order", `air (uniq [3, 1, 3, 2, 1, 4])`, "[3 1 2 4]"},
		{"over text", `air (uniq ["b", "a", "b"])`, `["b" "a"]`},
		{"over Twines", `air (uniq [(1, 2), (1, 2), (3, 4)])`, "[(1, 2) (3, 4)]"},
		{"nothing to drop", `air (uniq [1, 2, 3])`, "[1 2 3]"},
		{"the empty Thread", `air (uniq [])`, "[]"},
		{
			"a hundred thousand elements over seven values",
			`len (uniq (span 1 100000 | bend (n : mod n 7)))`,
			"7",
		},
		{
			"a hundred thousand distinct elements",
			`len (uniq (span 1 100000))`,
			"100000",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(compileAndRun(t, tc.name+".weave", tc.src+"\n", ""), "\n")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A ward's two shapes are told apart by the line, not by the brackets: an
// indented block after the line means the arms are down there and the whole
// line is the subject, and otherwise the arms are the run of bracketed
// `pattern : body` groups the line ends with.
//
// The rule this replaced — the subject stops at the first bracketed group
// holding a `:` — could not tell an arm from a lambda, so a subject was not
// allowed to contain one.
func TestWardTellsAnArmFromALambda(t *testing.T) {
	const decl = "Glow is Bright | Dim\n\n"
	cases := []struct{ name, src, want string }{
		{
			"a lambda in the subject, arms in a block",
			"f xs is\n  ward seek (x : gt 2 x) xs\n    Held v : v\n    Stilled : 0\n\nf [1, 5, 9]\n",
			"5",
		},
		{
			"a lambda in the subject, arms on the line",
			"f xs is ward seek (x : gt 2 x) xs (Held v : v) (Stilled : 0)\n\nf [1, 5, 9]\n",
			"5",
		},
		{
			"arms on the line",
			decl + "g c is ward c (Bright : 1) (Dim : 0)\n\ng Bright\n",
			"1",
		},
		{
			"a ward as an argument",
			decl + "k c is add 1 (ward c (Bright : 2) (Dim : 3))\n\nk Dim\n",
			"4",
		},
		{
			"a ward inside another ward's arm",
			decl + "k c d is ward c (Bright : ward d (Bright : 1) (Dim : 2)) (Dim : 3)\n\nk Bright Dim\n",
			"2",
		},
		{
			"a call in the subject with a block",
			decl + "k xs is\n  ward first xs\n    Held v : v\n    Stilled : 0\n\nk [7]\n",
			"7",
		},
		{
			"a pipeline in the subject",
			"k xs is\n  ward xs | bend (x : mul x 2) | first\n    Held v : v\n    Stilled : 0\n\nk [4]\n",
			"8",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(compileAndRun(t, tc.name+".weave", tc.src, ""), "\n")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
