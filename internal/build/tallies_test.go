package build_test

import (
	"strings"
	"testing"
)

// `tallies` and `tallied` are the summed-area table: ask once, and every "how
// much is inside this box" afterwards is one subtraction whatever the box's
// size.
//
// The four corners are the whole reason it is a verb rather than a page of
// arithmetic. Three of them sit one row or one column *before* the box, which
// is what a hand-written version has to pad for, and getting the sign of the
// fourth wrong gives an answer that is right everywhere except at the edges —
// so the edges are what these check.
func TestTalliesAnswersABoxInOneSubtraction(t *testing.T) {
	const grid = "g is [[1, 2, 3], [4, 5, 6], [7, 8, 9]] | weft 0\n\nt is tallies g\n\n"
	cases := []struct{ name, src, want string }{
		{"the whole grid", `tallied t (knot 0 0) (knot 2 2)`, "45"},
		{"one cell", `tallied t (knot 1 1) (knot 1 1)`, "5"},
		{"the top left cell, where three corners are missing", `tallied t (knot 0 0) (knot 0 0)`, "1"},
		{"a box away from every edge", `tallied t (knot 1 1) (knot 2 2)`, "28"},
		{"a box touching the top edge", `tallied t (knot 0 1) (knot 1 2)`, "16"},
		{"a box touching the left edge", `tallied t (knot 1 0) (knot 2 1)`, "24"},
		{"one whole row", `tallied t (knot 1 0) (knot 1 2)`, "15"},
		{"one whole column", `tallied t (knot 0 2) (knot 2 2)`, "18"},
		{"the knots may be given either way round", `tallied t (knot 2 2) (knot 1 1)`, "28"},
		{"knots crossed on one axis only", `tallied t (knot 2 1) (knot 1 2)`, "28"},
		{"a box running off the far edge is clipped", `tallied t (knot 1 1) (knot 99 99)`, "28"},
		{"a box starting before the grid is clipped", `tallied t (knot (neg 9) (neg 9)) (knot 0 0)`, "1"},
		{"a box wholly off the grid holds nothing", `tallied t (knot 9 9) (knot 9 9)`, "0"},

		// The table itself, read directly: every cell is the box above and left
		// of it, so the last one is the total.
		{"the table's last cell is the total", `cell t (knot 2 2) | otherwise 0`, "45"},
		{"the table's first cell is the first cell", `cell t (knot 0 0) | otherwise 0`, "1"},
		{"the table keeps the grid's shape", `join "x" [air (rows t), air (cols t)]`, "3x3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(compileAndRun(t, tc.name+".weave", grid+tc.src+"\n", ""), "\n")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Every box, checked against walking it. A table that is subtly wrong agrees
// with the walk in the middle of the grid and disagrees at an edge, so this
// asks for all 400 of them on a grid whose values are all different.
func TestTalliedAgreesWithWalkingTheBox(t *testing.T) {
	const src = `g is span 0 4 as (r gives span 0 4 as (c gives add (mul r 7) c)) | weft 0

t is tallies g

walked r1 c1 r2 c2 is
  span r1 r2 | mapcat (r gives span c1 c2 as (c gives cell g (knot r c) | otherwise 0)) | sum

boxes is couples (span 0 4) | mapcat ((a, b) gives couples (span 0 4) as ((c, d) gives (a, b, c, d)))

wrong is boxes | count ((r1, r2, c1, c2) gives neq (walked r1 c1 r2 c2) (tallied t (knot r1 c1) (knot r2 c2)))

wrong
`
	if got := strings.TrimRight(compileAndRun(t, "boxes.weave", src, ""), "\n"); got != "0" {
		t.Errorf("%s boxes disagree with walking them", got)
	}
}

// `tallies` is Reckon, not Earth, so it has to hold for Water too.
func TestTalliesOverWater(t *testing.T) {
	const src = `g is [[1.5, 2.5], [3.5, 4.5]] | weft 0.0

tallied (tallies g) (knot 0 0) (knot 1 1)
`
	if got := strings.TrimRight(compileAndRun(t, "water.weave", src, ""), "\n"); got != "12.0" {
		t.Errorf("got %q, want %q", got, "12.0")
	}
}

// `carve` is `words` with the separators named. Weave had `split`, which takes
// one separator, and `words`, which takes whitespace and nothing else — so a
// line with punctuation in it meant a chain of `replace` calls first.
func TestCarveSplitsOnAnyOfTheseCharacters(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"brackets and commas at once", `carve "[](){}, " "Machine [1,2] (34) {5}" | join "|"`, "Machine|1|2|34|5"},
		{"empty runs are dropped, as words drops them", `carve "," "a,,b," | join "|"`, "a|b"},
		{"no separators leaves the text whole", `carve "" "abc" | join "|"`, "abc"},
		{"nothing but separators", `len (carve "," ",,,")`, "0"},
		{"empty text", `len (carve "," "")`, "0"},
		{"leading and trailing separators", `carve "," ",a,b," | join "|"`, "a|b"},
		{"a separator given twice is still one separator", `carve ",,," "a,b" | join "|"`, "a|b"},
		{"it reads a line of numbers", `carve ": ," "reg: 1, 2, 3" | glean earth | sum`, "6"},
		{"a rune above ASCII is never a separator", `carve "," "naïve,café" | join "|"`, "naïve|café"},
		{"whitespace has to be asked for", `carve "," "a b,c" | join "|"`, "a b|c"},
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
