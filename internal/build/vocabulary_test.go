package build_test

import (
	"strings"
	"testing"
)

// `base` and `unbase` replaced `bin`, which wrote base two and had no reading
// half at all — so a puzzle that handed you binary could print it and never
// read it back. Neither half is harder for any base than for two.
func TestBaseAndUnbaseAreTwoHalvesOfOneThing(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"binary", `base 2 11`, "1011"},
		{"hexadecimal", `base 16 255`, "ff"},
		{"the largest base", `base 36 1295`, "zz"},
		{"the smallest base", `base 2 5`, "101"},
		{"ten is ordinary", `base 10 1234`, "1234"},
		{"zero", `base 2 0`, "0"},
		{"a negative number keeps its sign", `base 2 (neg 5)`, "-101"},
		{"the smallest Earth does not overflow", `len (base 2 (sub (neg 9223372036854775807) 1))`, "65"},
		{"a base it does not have", `base 1 7`, "0"},
		{"a base above thirty-six", `base 99 7`, "0"},

		{"reading binary back", `unbase 2 "1011" | otherwise 0`, "11"},
		{"reading hexadecimal, either case", `[unbase 16 "ff", unbase 16 "FF"] | glean (h : h) | sum`, "510"},
		{"a round trip", `unbase 7 (base 7 123456) | otherwise 0`, "123456"},
		{"a negative round trip", `unbase 30 (base 30 (neg 98765)) | otherwise 0`, "-98765"},
		{"a leading plus", `unbase 10 "+42" | otherwise 0`, "42"},
		{"leading space", `unbase 10 "  42" | otherwise 0`, "42"},
		{"a digit the base does not have", `holds (unbase 2 "1012")`, "Shadow"},
		{"something that is not a digit", `holds (unbase 16 "ff!")`, "Shadow"},
		{"no digits at all", `holds (unbase 10 "")`, "Shadow"},
		{"only a sign", `holds (unbase 10 "-")`, "Shadow"},
		{"a base it does not have", `holds (unbase 1 "11")`, "Shadow"},
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

// `sited` and `sites` are `cell` asked the other way round: where a value is
// rather than what is at a knot. Both halves, as everywhere else — the first,
// and all of them.
func TestSitedFindsAValueInAGrid(t *testing.T) {
	const grid = "g is \"S.#\\n.E.\\n#.S\" | pattern\n\n"
	cases := []struct{ name, src, want string }{
		{"the first, in reading order", `air (sited g 'S')`, "Held (knot 0 0)"},
		{"one that appears once", `air (sited g 'E')`, "Held (knot 1 1)"},
		{"a value that is not there", `air (sited g 'Z')`, "Stilled"},
		{"every one, in reading order", `air (sites g 'S')`, "[(knot 0 0) (knot 2 2)]"},
		{"none of them", `len (sites g 'Z')`, "0"},
		{"all of them", `len (sites g '.')`, "4"},
		{"it agrees with cell", `cell g (sited g 'E' | otherwise (knot 0 0)) | otherwise ' ' | air`, "E"},
		{"over a grid of Earths", `air (sited ([[1, 2], [3, 2]] | weft 0) 2)`, "Held (knot 0 1)"},
		{"an empty grid", `air (sited ("" | pattern) 'x')`, "Stilled"},
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

// The five the audit found by looking for what the programs kept writing out.
// `span 0 (sub n 1)` alone appeared eight times across four of the twelve
// Advent of Code 2025 solutions.
func TestTheGapVerbs(t *testing.T) {
	cases := []struct{ name, src, want string }{
		// under: the places of n things, rather than a range between two
		// numbers. `span` names both ends because an input that writes one does.
		{"the first five places", `air (under 5)`, "[0 1 2 3 4]"},
		{"one place", `air (under 1)`, "[0]"},
		{"no places", `air (under 0)`, "[]"},
		{"a negative count is no places", `air (under (neg 3))`, "[]"},
		{"it is span 0 (sub n 1)", `eq (under 7) (span 0 6)`, "Light"},
		{"as long as it says", `len (under 1000)`, "1000"},

		// copies: `repeat` for a Thread rather than for text.
		{"three of the same", `air (copies 3 'x')`, "[x x x]"},
		{"none of them", `air (copies 0 1)`, "[]"},
		{"a negative count is none", `air (copies (neg 1) 1)`, "[]"},
		{"of anything at all", `copies 2 [1, 2] | flat | sum`, "6"},
		{"a row of zeroes", `copies 4 0 | sum`, "0"},

		// woven: `holds` for the other half of the pair.
		{"a Woven", `woven (Woven 1)`, "Light"},
		{"a Gentled", `woven (harvest earth ["x"])`, "Shadow"},
		{"a harvest that worked", `woven (harvest earth ["1", "2"])`, "Light"},

		// covers: the containment union, inter and diff were missing.
		{"a Circle holding another", `covers (circle [1, 2, 3]) (circle [1, 3])`, "Light"},
		{"one that does not", `covers (circle [1, 2]) (circle [1, 9])`, "Shadow"},
		{"everything covers nothing", `covers (circle [1]) (circle [])`, "Light"},
		{"nothing covers only nothing", `covers (circle []) (circle [1])`, "Shadow"},
		{"a Circle covers itself", `covers (circle [1, 2]) (circle [2, 1])`, "Light"},
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

// `warp` is the grid constructor `weft` is not: `weft` weaves rows you already
// have, and `warp` is given the shape and what belongs at each knot. Written by
// hand it is a span inside a span inside a weft, plus a fill value that nothing
// will ever read.
func TestWarpLaysOutAGrid(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"each cell from its knot", `air (cells (warp (k gives add (row k) (col k)) 2 3))`, "[0 1 2 1 2 3]"},
		{"the shape it was asked for", `join "x" [air (rows (warp (k gives 0) 4 7)), air (cols (warp (k gives 0) 4 7))]`, "4x7"},
		{"one cell", `air (cells (warp (k gives 9) 1 1))`, "[9]"},
		{"no rows", `len (cells (warp (k gives 0) 0 5))`, "0"},
		{"no columns", `len (cells (warp (k gives 0) 5 0))`, "0"},
		{"a negative shape is empty", `len (cells (warp (k gives 0) (neg 2) 3))`, "0"},
		{"a grid of Fires", `air (cells (warp (k gives pick (eq (row k) (col k)) '#' '.') 2 2))`, "[# . . #]"},
		{"it agrees with cell", `cell (warp (k gives mul (row k) 10) 3 3) (knot 2 0) | otherwise 0`, "20"},
		{"and with knots", `warp (k gives 1) 3 4 | knots | len`, "12"},
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
