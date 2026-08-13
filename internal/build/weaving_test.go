package build_test

import (
	"strings"
	"testing"
)

// `Weaving a e` is Weave's Result: `Woven a | Gentled e`. It is compiled on the
// same representation as a declared sum type, so these also check that the two
// paths agree.
func TestWeavingEndToEnd(t *testing.T) {
	cases := []struct {
		name, src, in, want string
	}{
		{
			name: "constructing and matching",
			src: `divide _ 0 is Gentled "divide by zero"
divide a b is Woven (div a b)

say w is
  ward w
    Woven n : air n
    Gentled e : e

[divide 10 2, divide 1 0, divide 9 3] | bend say | join ", "
`,
			want: "5, divide by zero, 3",
		},
		{
			name: "rescue takes the default on a failure",
			src: `safe 0 is Gentled "no"
safe n is Woven n

[safe 3, safe 0] | bend (rescue 99)
`,
			want: "3\n99",
		},
		{
			name: "a Weaving prints as it was written",
			src: `[Woven 1, Gentled "oh"]
`,
			want: "Woven 1\nGentled \"oh\"",
		},
		{
			name: "equality is structural",
			src: `[eq (Woven 5) (Woven 5), eq (Woven 5) (Woven 6), eq (Gentled "a") (Woven 1)]
`,
			want: "Light\nShadow\nShadow",
		},
		{
			name: "success sorts before failure",
			src: `[Gentled "b", Woven 1, Gentled "a", Woven 0] | sort
`,
			want: "Woven 0\nWoven 1\nGentled \"a\"\nGentled \"b\"",
		},
		{
			name: "a Weaving works as a Web key",
			src: `[Woven 1, Gentled "x", Woven 1] | freq | len
`,
			want: "2",
		},
		{
			name: "exhaustiveness covers both cases",
			src: `only w is
  ward w
    Woven n : n
    Gentled _ : 0

only (Gentled "e")
`,
			want: "0",
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

// `dijkstra` takes a step function and answers with the cost of reaching every
// node, so a graph never has to be built.
func TestDijkstra(t *testing.T) {
	cases := []struct {
		name, src, in, want string
	}{
		{
			name: "weighted steps along a line",
			src: `steps n is pick (gte 4 n) [] [(mul n 2, add n 1)]

dijkstra steps 1 | items | sort
`,
			// 1 -> 2 costs 2, 2 -> 3 costs 4, 3 -> 4 costs 6.
			want: "(1, 0)\n(2, 2)\n(3, 6)\n(4, 12)",
		},
		{
			name: "the cheaper of two roads wins",
			src: `steps 'a' is [(1, 'b'), (10, 'd')]
steps 'b' is [(1, 'c')]
steps 'c' is [(1, 'd')]
steps _ is []

get (dijkstra steps 'a') 'd' | otherwise (neg 1)
`,
			want: "3",
		},
		{
			name: "an unreachable node is absent",
			src: `steps 1 is [(1, 2)]
steps _ is []

[known (dijkstra steps 1) 2, known (dijkstra steps 1) 9]
`,
			want: "Light\nShadow",
		},
		{
			name: "a cycle terminates",
			src: `steps n is [(1, mod (add n 1) 5)]

dijkstra steps 0 | len
`,
			want: "5",
		},
		{
			name: "over a declared type",
			src: `Room is Hall | Study | Cellar | Attic

steps Hall is [(1, Study), (7, Attic)]
steps Study is [(2, Cellar)]
steps Cellar is [(1, Attic)]
steps Attic is []

get (dijkstra steps Hall) Attic | otherwise (neg 1)
`,
			want: "4",
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

// The maze example uses `dijkstra` now, so the hand-written form it replaced —
// a recursive walk over a Taveren — is kept here rather than lost.
func TestTaverenByHand(t *testing.T) {
	src := `walk queue seen is
  ward pop queue
    Stilled : neg 1
    Held ((d, n), rest) :
      pick (member seen n) (walk rest seen)
        (pick (eq 12 n) d
          (walk (braid push rest [(add d 1, add n 1), (add d 3, mul n 2)]) (insert seen n)))

walk (taveren [(0, 1)]) (circle [])
`
	got := strings.TrimRight(compileAndRun(t, "taveren.weave", src, ""), "\n")
	// 1 -> 2 -> 3 -> 6 -> 12 costs 1 + 1 + 3 + 3 = 8; going up one at a time
	// from 1 to 12 costs 11.
	if got != "8" {
		t.Errorf("got %q, want 8", got)
	}
}

// dijkstra owns its frontier and its distance map outright, and updates both
// in place. These are the shapes that would go wrong if that ownership were
// ever untrue — a map handed back and then added to, and two searches over the
// same graph.
func TestDijkstraOwnershipIsSound(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"the result survives being added to",
			`steps n is pick (gte 5 n) [] [(1, add n 1)]

d is dijkstra steps 1
e is put d 99 99

(len d, len e, get d 99 | otherwise (neg 1))
`,
			"(5, 6, -1)",
		},
		{
			"two searches over the same graph agree",
			`steps n is pick (gte 20 n) [] [(2, add n 1)]

a is dijkstra steps 1
b is dijkstra steps 1

(len a, len b, eq (items a) (items b))
`,
			"(20, 20, Light)",
		},
		{
			"a result used as a key in another map",
			`steps n is pick (gte 3 n) [] [(1, add n 1)]

d is dijkstra steps 1
w is put (web []) (items d | sort) 1

(len d, len w)
`,
			"(3, 1)",
		},
		{
			"a large frontier settles every node once",
			`side is 60

inb2 k is and (gt (neg 1) (row k)) (and (gt (neg 1) (col k)) (and (lt side (row k)) (lt side (col k))))

steps k is
  [knot (row k) (add (col k) 1), knot (add (row k) 1) (col k)]
    | sift inb2
    | bend (n : (1, n))

d is dijkstra steps (knot 0 0)

(len d, get d (knot 59 59) | otherwise (neg 1))
`,
			"(3600, 118)",
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
