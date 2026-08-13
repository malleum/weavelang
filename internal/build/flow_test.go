package build_test

import (
	"strings"
	"testing"

	"github.com/malleum/weave/internal/build"
	"github.com/malleum/weave/internal/diag"
)

// `flow` is an endless Thread, so it exists only as a fused loop: the compiler
// has to see where it is created and where it stops in the same pipeline.
func TestFlowEndToEnd(t *testing.T) {
	cases := []struct {
		name, src, in, want string
	}{
		{
			name: "take bounds a flow",
			src:  "flow (add 10) 0 | take 5\n",
			want: "0\n10\n20\n30\n40",
		},
		{
			name: "takewhile bounds a flow",
			src:  "flow (mul 3) 1 | takewhile (lt 500)\n",
			want: "1\n3\n9\n27\n81\n243",
		},
		{
			name: "seek stops a flow",
			src:  "flow (mul 2) 1 | seek (gt 1000)\n",
			want: "Held 1024",
		},
		{
			name: "first stops a flow after a filter",
			src:  "flow (add 1) 1 | sift (x : eq 0 (mod x 7)) | first\n",
			want: "Held 7",
		},
		{
			name: "a flow past its first buffer",
			src:  "flow (add 1) 1 | take 100 | len\n",
			want: "100",
		},
		{
			name: "the Collatz chain from 27",
			src: `next n is pick (even n) (div n 2) (add 1 (mul 3 n))

flow next 27 | takewhile (neq 1) | len
`,
			want: "111",
		},
		{
			name: "a flow of pairs",
			src: `fst p is
  ward p
    (a, _) : a

flow ((a, b) : (b, add a b)) (0, 1) | bend fst | take 10
`,
			want: "0\n1\n1\n2\n3\n5\n8\n13\n21\n34",
		},
		// Every verb that answers from one element and stops is allowed to stop
		// an endless one. Leaving one out of that list is not a missed
		// optimisation: the compiler refuses to build the program.
		{
			name: "idx stops a flow",
			src:  "flow (mul 2) 1 | idx 1024\n",
			want: "Held 10",
		},
		{
			name: "seekidx stops a flow",
			src:  "flow (mul 2) 1 | seekidx (gt 1000)\n",
			want: "Held 10",
		},
		{
			name: "has stops a flow",
			src:  "flow (mul 2) 1 | has 1024\n",
			want: "Light",
		},
		{
			name: "none stops a flow",
			src:  "flow (mul 2) 1 | none (gt 1000)\n",
			want: "Shadow",
		},
		{
			name: "nth stops a flow",
			src:  "flow (mul 2) 1 | nth 5\n",
			want: "Held 32",
		},
		{
			name: "second stops a flow",
			src:  "flow (mul 2) 1 | second\n",
			want: "Held 2",
		},
		{
			name: "a position counts what got past the stages",
			src:  "flow (add 1) 1 | sift even | seekidx (gt 10)\n",
			want: "Held 5",
		},
		{
			name: "idx after a stage",
			src:  "flow (add 1) 1 | bend (mul 3) | idx 12\n",
			want: "Held 3",
		},
		{
			name: "a flow over declared values",
			src: `Dir is North | East | South | West

turn North is East
turn East is South
turn South is West
turn West is North

flow turn North | take 6
`,
			want: "North\nEast\nSouth\nWest\nNorth\nEast",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(compileAndRun(t, tc.name+".weave", tc.src, tc.in), "\n")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// A flow that nothing stops is a program that never finishes, and the compiler
// is the only thing that can catch it.
func TestUnboundedFlowIsRejected(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"no bound at all",
			"flow (add 1) 1 | bend (mul _ 2) | sum\n",
			"would never finish",
		},
		{
			"collected without a bound",
			"flow (add 1) 1 | bend (mul _ 2)\n",
			"would never finish",
		},
		{
			"bound to a name",
			"xs is flow (add 1) 1\n\nxs | take 3\n",
			"endless",
		},
		{
			"passed to a verb that is not a stage",
			"flow (add 1) 1 | sort | take 3\n",
			"endless",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bag := diag.New(tc.name+".weave", tc.src)
			if _, err := build.Compile(tc.name+".weave", tc.src, build.Options{}, bag); err == nil {
				t.Fatalf("expected a compile error for:\n%s", tc.src)
			}
			if !strings.Contains(bag.String(), tc.want) {
				t.Errorf("expected an error mentioning %q, got:\n%s", tc.want, bag)
			}
		})
	}
}
