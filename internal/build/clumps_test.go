package build_test

import (
	"strings"
	"testing"
)

// `clumps` is `reach` from every node at once: the nodes fall into groups that
// can get to one another, each group given once, in the order its first member
// appears.
//
// It is what a program writes by hand as `reach` inside a fold with a seen-set
// around it, and the seen-set is where the mistake keeps happening — it has to
// survive between the seeds, not only within one of them.
func TestClumpsGroupsWhatCanReachWhat(t *testing.T) {
	const graph = `edges is web [(1, [2]), (2, [1]), (3, []), (4, [5]), (5, [4])]

step v is get edges v else []

`
	cases := []struct{ name, src, want string }{
		{
			"three groups out of five nodes",
			graph + `clumps step [1, 2, 3, 4, 5] as air through join " "`,
			"[1 2] [3] [4 5]",
		},
		{
			"a group is given once, however many of its members were seeds",
			graph + `len (clumps step [1, 2, 3, 4, 5])`,
			"3",
		},
		{
			"groups come in the order their first member appears",
			graph + `clumps step [5, 3, 1] as air through join " "`,
			"[5 4] [3] [1 2]",
		},
		{
			"no nodes, no groups",
			graph + `air (clumps step [])`,
			"[]",
		},
		{
			"one node that goes nowhere",
			graph + `air (clumps step [3] | first | otherwise [])`,
			"[3]",
		},
		{
			// The step function is what says who belongs together; the Thread
			// only says where to start looking. A grid names the filled cells
			// and lets the neighbours drag in the rest.
			"what the step reaches joins the group whether it was named or not",
			`step v is pick (eq 1 v) [2, 3] []

clumps step [1] as air through join " "`,
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

// The shape `clumps` was written for: the filled cells of a grid, grouped into
// the regions that touch. Two blobs and a lone cell between them.
func TestClumpsFindsTheRegionsOfAGrid(t *testing.T) {
	const src = `grid is Source through pattern

filled is knots grid where (k gives cell grid k | otherwise '.' | eq '#') through circle

step p is around4 grid p where (member filled this)

regions is clumps step (members filled)

regions as len through sort through air
`
	const input = "##..#\n#....\n....#\n..###\n"
	got := strings.TrimRight(compileAndRun(t, "regions.weave", src, input), "\n")
	if got != "[1 3 4]" {
		t.Errorf("got %q, want %q", got, "[1 3 4]")
	}
}
