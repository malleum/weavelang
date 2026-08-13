package build_test

import (
	"strings"
	"testing"
)

// A Link is who is joined to whom, and it is the one question `clumps` cannot
// answer. `clumps` is asked once, of a finished graph; a Link is asked *while*
// the joining happens — after this connection, are these two together yet, and
// how big has each circle grown?
func TestALinkJoinsCirclesAsItGoes(t *testing.T) {
	const five = "l is link [1, 2, 3, 4, 5]\n\n"
	cases := []struct{ name, src, want string }{
		{"every node starts alone", `clumped l | bend air | join " "`, "[1] [2] [3] [4] [5]"},
		{"binding two joins their circles", `clumped (bind l 1 2) | bend air | join " "`, "[1 2] [3] [4] [5]"},
		{"binding is transitive", `clumped (bind (bind l 1 2) 2 3) | bend air | join " "`, "[1 2 3] [4] [5]"},
		{"two separate circles", `clumped (bind (bind l 1 2) 4 5) | bend air | join " "`, "[1 2] [3] [4 5]"},
		{"binding what is already bound does nothing", `clumped (bind (bind l 1 2) 2 1) | bend air | join " "`, "[1 2] [3] [4] [5]"},
		{"a node bound to itself stays alone", `clumped (bind l 3 3) | bend air | join " "`, "[1] [2] [3] [4] [5]"},

		{"bound says yes", `bound (bind l 1 2) 1 2`, "Light"},
		{"bound says no", `bound (bind l 1 2) 1 3`, "Shadow"},
		{"bound is transitive too", `bound (bind (bind l 1 2) 2 3) 1 3`, "Light"},
		{"a node is bound to itself", `bound l 4 4`, "Light"},
		{"nothing is bound before anything is", `bound l 1 2`, "Shadow"},

		// The circles come in the order their first member was given, and each
		// circle holds its own members in that order too, which is what makes
		// the answer reproducible rather than merely correct.
		{"order follows the Thread the Link was built from", `clumped (bind (link [5, 4, 3, 2, 1]) 1 5) | bend air | join " "`, "[5 1] [4] [3] [2]"},

		{"the sizes add up to the nodes", `clumped (bind l 1 2) | bend len | sum`, "5"},
		{"a Link renders as its circles", `air (bind l 1 2)`, "<link [1 2] [3] [4] [5]>"},

		{"a Link over text", `clumped (bind (link ["a", "b", "c"]) "a" "c") | bend air | join " "`, `["a" "c"] ["b"]`},
		{"a Link over Twines", `clumped (bind (link [(1, 2), (3, 4)]) (1, 2) (3, 4)) | bend len | sum`, "2"},

		{"no nodes at all", `len (clumped (link []))`, "0"},
		{"one node", `clumped (link [7]) | bend air | join " "`, "[7]"},
		{"a value given twice is one node", `len (clumped (link [1, 1, 2]))`, "2"},

		// A node the Link was never given does not exist, so it cannot be
		// joined to anything — and equals only itself.
		{"binding a node it does not know does nothing", `len (clumped (bind l 9 1))`, "5"},
		{"an unknown node is bound only to itself", `bound l 9 9`, "Light"},
		{"two unknown nodes are not bound", `bound l 9 8`, "Shadow"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(compileAndRun(t, tc.name+".weave", five+tc.src+"\n", ""), "\n")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Binding hands back a new Link and leaves the old one as it was, which is the
// whole difference between this and a mutable structure. The in-place path
// exists to make that free when nobody is looking, and must not make it false
// when somebody is.
func TestBindingLeavesTheOldLinkAlone(t *testing.T) {
	const src = `l is link [1, 2, 3]

joined is bind l 1 2

# ` + "`l`" + ` is read after ` + "`joined`" + ` was made from it, so the copy has to have
# happened: an in-place bind here would answer 2 twice.
[len (clumped joined), len (clumped l)] | bend air | join " "
`
	if got := strings.TrimRight(compileAndRun(t, "persist.weave", src, ""), "\n"); got != "2 3" {
		t.Errorf("got %q, want %q — the original Link was written through", got, "2 3")
	}
}

// Kruskal's algorithm, which is what a Link is for: walk the pairs in order,
// join the ones not already together, stop when everything is in one circle.
// The loop threads the Link through a tail call and skips the update on a pair
// already joined — the shape that used to lose the in-place path entirely.
func TestALinkThreadsThroughALoop(t *testing.T) {
	const src = `edges is [(0, 1), (2, 3), (1, 2), (0, 3), (4, 5)]

walk es l made is
  ward es
    [] : (l, made)
    [(a, b) ..rest] :
      pick (bound l a b) (walk rest l made) (walk rest (bind l a b) (add made 1))

done is walk edges (link (span 0 5)) 0

ward done
  (l, made) : join " " [air made, air (clumped l | bend len | sort | rev)]
`
	if got := strings.TrimRight(compileAndRun(t, "kruskal.weave", src, ""), "\n"); got != "4 [4 2]" {
		t.Errorf("got %q, want %q", got, "4 [4 2]")
	}
}
