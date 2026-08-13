package build_test

import (
	"strings"
	"testing"
)

// Every type that claims Eq has to answer for it.
//
// `w_compare` decides equality for all of them, and its `default` arm answered
// 0 — equal — for anything it had not been taught. A Pattern is Eq, and so two
// grids were the same grid whatever they held: `settle` on a grid stopped after
// one round, a Pattern in a Circle collided with every other Pattern, and
// nothing said so, because a wrong answer here is a plausible answer.
func TestEveryEqTypeReallyCompares(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"two grids that differ", `eq ("ab\ncd" | pattern) ("ab\ncx" | pattern)`, "Shadow"},
		{"two grids that agree", `eq ("ab\ncd" | pattern) ("ab\ncd" | pattern)`, "Light"},
		{"a grid against itself changed", `g is "ab" | pattern` + "\n" + `eq g (set g (knot 0 0) 'z')`, "Shadow"},
		{"a shorter grid is not the longer one", `eq ("ab" | pattern) ("ab\nab" | pattern)`, "Shadow"},
		{"grids as Circle members", `["ab" | pattern, "ab" | pattern, "cd" | pattern] | circle | len`, "2"},

		{"Twines", `eq (1, "a") (1, "a")`, "Light"},
		{"Twines that differ deep", `eq (1, "a") (1, "b")`, "Shadow"},
		{"Webs", `eq (web [(1, 2)]) (web [(1, 2)])`, "Light"},
		{"Webs that differ", `eq (web [(1, 2)]) (web [(1, 3)])`, "Shadow"},
		{"Circles regardless of order", `eq (circle [1, 2]) (circle [2, 1])`, "Light"},
		{"Holds", `eq (Held 1) (Held 1)`, "Light"},
		{"Held against Stilled", `eq (Held 1) (nth 9 [1])`, "Shadow"},
		{"Knots", `eq (knot 1 2) (knot 1 3)`, "Shadow"},
		{"Weavings", `eq (Woven 1) (Gentled 1)`, "Shadow"},
		{"Threads of Threads", `eq [[1], [2]] [[1], [3]]`, "Shadow"},
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

// `settle` applies until a round changes nothing. It is the verb that noticed
// the Pattern hole, since a grid that erodes is exactly what it is for.
func TestSettleStopsWhenNothingChanges(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"counting up to a bound", `settle (x gives pick (gt 100 x) x (add x 1)) 1`, "101"},
		{"already settled", `settle (x gives x) 7`, "7"},
		{"over text", `settle (s gives replace "ab" "b" s) "aaaab"`, "b"},
		{
			// Three rounds, which is the point: a plus loses its four arms, then
			// its middle, then nothing. A `settle` that stopped after one round
			// would answer 1, and that is exactly what it did while two Patterns
			// compared equal whatever they held.
			"over a grid: the rounds run until one takes nothing",
			`g is "..#..\n.###.\n..#.." | pattern` + "\n" +
				`thin p is knots p | braid (q k gives pick (lt 3 (nb4 p k | count (eq '#'))) (set q k '.') q) p` + "\n" +
				`settle thin g | cells | count (eq '#')`,
			"0",
		},
		{
			// Not a fixed point of the function, a fixed point of the value: a
			// round that swaps two things back and forth would never settle,
			// and this one reaches a grid it maps to itself.
			"a Thread that stops shuffling",
			`settle (xs gives xs | sort) [3, 1, 2] | air`,
			"[1 2 3]",
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

// The other four verbs of the same batch.
func TestTheNewSequenceVerbs(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"couples takes every two", `couples [1, 2, 3] | air`, "[(1, 2) (1, 3) (2, 3)]"},
		{"couples of two", `couples [1, 2] | air`, "[(1, 2)]"},
		{"couples of one", `couples [1] | air`, "[]"},
		{"couples of none", `couples [] | air`, "[]"},
		{"couples counts n choose two", `len (couples (span 1 100))`, "4950"},
		{"couples pairs positions, not values", `couples [1, 1] | air`, "[(1, 1)]"},

		{"index says where each value sits", `get (index ["a", "b"]) "b" | otherwise 0`, "1"},
		{"index keeps the first of a repeat", `get (index ["a", "b", "a"]) "a" | otherwise 9`, "0"},
		{"index of nothing", `len (index [])`, "0"},

		{"squeeze keeps a dense axis dense", `squeeze [1, 2, 3] | air`, "[1 2 3]"},
		{"squeeze puts one line in each gap", `squeeze [1, 5] | air`, "[1 2 5]"},
		{"squeeze sorts and drops repeats", `squeeze [5, 1, 5] | air`, "[1 2 5]"},
		{"squeeze of one", `squeeze [7] | air`, "[7]"},
		{"squeeze of none", `squeeze [] | air`, "[]"},
		{
			// The whole point: a run of any length costs one line, so the axis
			// stays small however far apart the coordinates are.
			"a gap of a million is still one line",
			`len (squeeze [0, 1000000])`,
			"3",
		},

		{"mesh rolls overlapping ranges together", `mesh [(1, 3), (2, 5)] | air`, "[(1, 5)]"},
		{"mesh joins ranges that merely touch", `mesh [(1, 3), (4, 6)] | air`, "[(1, 6)]"},
		{"mesh leaves a gap alone", `mesh [(1, 3), (5, 6)] | air`, "[(1, 3) (5, 6)]"},
		{"mesh sorts first", `mesh [(9, 10), (1, 3)] | air`, "[(1, 3) (9, 10)]"},
		{"mesh swallows a range inside another", `mesh [(1, 100), (4, 6)] | air`, "[(1, 100)]"},
		{"mesh drops a range that ends before it begins", `mesh [(5, 1), (7, 9)] | air`, "[(7, 9)]"},
		{"mesh of none", `mesh [] | air`, "[]"},
		{"the covered width is one sum", `mesh [(1, 3), (2, 5), (9, 9)] | bend width | sum`, "6"},
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
