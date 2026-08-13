package build_test

import (
	"strings"
	"testing"
)

// `solve` reads a Pattern of Earths as an augmented matrix and answers with a
// Weaving, because a system of equations has two different kinds of answer and
// only one of them is a point.
//
// The cases below are the four things that can happen, and the reason the
// answer is a Weaving at all is that the last two used to be indistinguishable
// from the first without reading the elimination back out by hand.
func TestSolve(t *testing.T) {
	cases := []struct{ name, src, want string }{
		// Pinned down, and whole.
		{"a small system", "weft 0 [[1, 1, 5], [1, neg 1, 1]] | solve\n", "Woven [3 2]"},
		{"one unknown", "weft 0 [[3, 12]] | solve\n", "Woven [4]"},
		// Advent of Code 2024 day 13, which is this and nothing else.
		{"two buttons and a prize",
			"weft 0 [[94, 22, 8400], [34, 67, 5400]] | solve\n", "Woven [80 40]"},
		{"the order of the equations does not matter",
			"weft 0 [[34, 67, 5400], [94, 22, 8400]] | solve\n", "Woven [80 40]"},
		{"more equations than unknowns, agreeing",
			"weft 0 [[1, 1, 5], [1, neg 1, 1], [2, 0, 6]] | solve\n", "Woven [3 2]"},

		// Pinned down, but not to whole numbers. No point and no directions:
		// there is nothing here to hand back.
		{"a fractional solution", "weft 0 [[2, 3, 8], [1, neg 1, 1]] | solve\n", "Gentled ([], [])"},
		{"equations that disagree", "weft 0 [[1, 1, 1], [1, 1, 2]] | solve\n", "Gentled ([], [])"},
		{"one unknown, no whole answer", "weft 0 [[3, 11]] | solve\n", "Gentled ([], [])"},

		// Not pinned down: a point and the directions the answers run in.
		{"one unknown too few",
			"weft 0 [[1, 1, 1, 6], [1, neg 1, 0, 0]] | solve\n",
			"Gentled ([3 3 0], [[-1 -1 2]])"},
		{"two unknowns too few",
			"weft 0 [[1, 1, 1, 1, 10]] | solve\n",
			"Gentled ([10 0 0 0], [[-1 1 0 0] [-1 0 1 0] [-1 0 0 1]])"},
		{"nothing said at all",
			"weft 0 [[0, 0, 0]] | solve\n", "Gentled ([0 0], [[1 0] [0 1]])"},
		// Solutions over the rationals, none of them whole where every free
		// unknown is zero. The empty point says so; the directions are still
		// worth having, which is why this is not the same answer as a system
		// that has no solutions at all.
		{"a solution set with no whole point at the origin",
			"weft 0 [[2, 2, 3]] | solve\n", "Gentled ([], [[-1 1]])"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(compileAndRun(t, "solve.weave", tc.src, ""), "\n")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// The point and the directions are only useful together: every combination of
// them has to satisfy the equations too, or the answer is not the solution set
// it claims to be. This checks that rather than the shape of it.
func TestSolveDirectionsStayOnTheSolutionSet(t *testing.T) {
	const src = `m is weft 0 [[1, 1, 1, 6], [1, neg 1, 0, 0]]

holds is
  weave answer is solve m
  weave (point, dirs) is answer failing ([], [])
  weave at is (k gives zipwith add point (bend (mul k) (nth 0 dirs else [])))
  span (neg 3) 3
    | bend at
    | all (v : and (eq 6 (sum v)) (eq 0 (sub (nth 0 v else 0) (nth 1 v else 0))))

holds
`
	if got := strings.TrimRight(compileAndRun(t, "walk.weave", src, ""), "\n"); got != "Light" {
		t.Errorf("got %q, want %q", got, "Light")
	}
}

// `rescue` is how the common case reads, and the whole argument for a Weaving
// is that ignoring the other side has to be something you said rather than
// something you did by accident.
func TestSolveReadsAsAPlainAnswer(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"rescue takes the one answer",
			"weft 0 [[1, 1, 5], [1, neg 1, 1]] | solve | rescue [] | air\n", "[3 2]"},
		{"rescue falls back when there is more than one",
			"weft 0 [[1, 1, 1, 6], [1, neg 1, 0, 0]] | solve | rescue [] | air\n", "[]"},
		{"woven asks which happened",
			"weft 0 [[3, 11]] | solve | woven\n", "Shadow"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(compileAndRun(t, "rescue.weave", tc.src, ""), "\n")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
