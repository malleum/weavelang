package build_test

import (
	"strings"
	"testing"
)

// Sum types are exercised end to end: declaring, constructing, matching,
// comparing, hashing and printing values of a type the program invented.
func TestSumTypesEndToEnd(t *testing.T) {
	cases := []struct {
		name, src, in, want string
	}{
		{
			name: "nullary constructors round-trip",
			src: `Direction is North | South | East | West

turn North is East
turn East is South
turn South is West
turn West is North

turn (turn North)
`,
			want: "South",
		},
		{
			name: "constructors carry fields",
			src: `Move is Step Earth Earth | Rest

dist (Step a b) is add (abs a) (abs b)
dist Rest is 0

dist (Step 3 (neg 4))
`,
			want: "7",
		},
		{
			name: "ward dispatches on the constructor",
			src: `Shape is Square Earth | Rect Earth Earth | Dot

area s is
  ward s
    Square n : mul n n
    Rect w h : mul w h
    Dot : 0

[Square 3, Rect 2 5, Dot] | bend area | sum
`,
			want: "19",
		},
		{
			name: "a recursive type folds",
			src: `Tree a is Leaf | Node (Tree a) a (Tree a)

total Leaf is 0
total (Node l v r) is add v (add (total l) (total r))

total (Node (Node Leaf 1 Leaf) 2 (Node Leaf 3 Leaf))
`,
			want: "6",
		},
		{
			name: "values print as they were written",
			src: `Tree a is Leaf | Node (Tree a) a (Tree a)

Node (Node Leaf 1 Leaf) 2 Leaf
`,
			want: "Node (Node Leaf 1 Leaf) 2 Leaf",
		},
		{
			name: "equality is structural",
			src: `Move is Step Earth Earth | Rest

[ eq (Step 1 2) (Step 1 2)
, eq (Step 1 2) (Step 1 3)
, eq Rest Rest
, eq Rest (Step 0 0)
]
`,
			want: "Light\nShadow\nLight\nShadow",
		},
		{
			name: "ordering follows the declaration",
			src: `Rank is Low | Mid | High

[High, Low, Mid] | sort
`,
			want: "Low\nMid\nHigh",
		},
		{
			name: "fields break ties in the ordering",
			src: `Card is Num Earth | Face Air

[Face "K", Num 9, Num 2, Face "A"] | sort
`,
			want: "Num 2\nNum 9\nFace A\nFace K",
		},
		{
			name: "values are usable as Web keys",
			src: `Colour is Red | Green | Blue

counts is [Red, Blue, Red, Green, Red] | freq

[get counts Red, get counts Blue, get counts Green]
`,
			want: "Held 3\nHeld 1\nHeld 1",
		},
		{
			name: "values are usable as Circle members",
			src: `Colour is Red | Green | Blue

seen is circle [Red, Blue]

[member seen Red, member seen Green]
`,
			want: "Light\nShadow",
		},
		{
			name: "a constructor is a function",
			src: `Box is Wrap Earth

[1 2 3] | bend Wrap | len
`,
			want: "3",
		},
		{
			name: "a constructor applies partially",
			src: `Pair is Both Earth Earth

add10 p is
  ward p
    Both a b : add a b

[1 2 3] | bend (Both 10) | bend add10 | sum
`,
			want: "36",
		},
		{
			name: "a sum type carries a collection",
			src: `Row is Row Air (Thread Earth)

width (Row _ ns) is len ns

Row "a" [1 2 3] | width
`,
			want: "3",
		},
		{
			name: "nested patterns destructure",
			src: `Tree a is Leaf | Node (Tree a) a (Tree a)

leftmost (Node Leaf v _) is v
leftmost (Node l _ _) is leftmost l
leftmost Leaf is 0

leftmost (Node (Node (Node Leaf 5 Leaf) 4 Leaf) 3 Leaf)
`,
			want: "5",
		},
		{
			name: "sum types work over the input",
			src: `Line is Blank | Text Air

classify "" is Blank
classify s is Text s

count Blank is 0
count (Text s) is len s

Source | lines | bend classify | bend count | sum
`,
			in:   "ab\n\ncde\n",
			want: "5",
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

// The pairwise and one-level-deeper verbs, and the prefix scans.
func TestPairwiseVerbs(t *testing.T) {
	cases := []struct {
		name, src, in, want string
	}{
		{
			name: "zipwith stops at the shorter",
			src:  "zipwith add [1 2 3 4] [10 20 30]\n",
			want: "11\n22\n33",
		},
		{
			name: "bendr goes one level in",
			src:  "bendr (mul _ 10) [[1 2] [3 4]]\n",
			want: "[10 20]\n[30 40]",
		},
		{
			name: "siftr filters each inner Thread",
			src:  "siftr even [[1 2 3] [4 5]]\n",
			want: "[2]\n[4]",
		},
		{
			name: "zipr combines two Threads of Threads",
			src:  "zipr add [[1 2] [3 4]] [[10 20] [30 40]]\n",
			want: "[11 22]\n[33 44]",
		},
		{
			name: "sums and prods are running totals",
			src:  "[sums [1 2 3 4], prods [1 2 3 4]]\n",
			want: "[1 3 6 10]\n[1 2 6 24]",
		},
		{
			name: "a running total over an empty Thread",
			src:  "sums [] | len\n",
			want: "0",
		},
		{
			name: "cellwise keeps the pattern's shape",
			src:  "\"abc\\ndef\" | pattern | cellwise (c : spark (add 1 (ord c)))\n",
			want: "bcd\nefg",
		},
		{
			name: "cellwise can change the cell type",
			src:  "\"12\\n34\" | pattern | cellwise (c : sub (ord c) (ord '0')) | cells | sum\n",
			want: "10",
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

// A value that renders as a name and arguments has to be bracketed wherever
// values sit side by side, or `[knot 1 2, knot 3 4]` reads as one long call.
func TestNestedValuesAreBracketed(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"knots in a Thread", "[[knot 1 2, knot 3 4]]", "[(knot 1 2) (knot 3 4)]"},
		{"Holds in a Thread", "[[Held 1, Stilled, Held 2]]", "[(Held 1) Stilled (Held 2)]"},
		{"a Hold of a Knot", "Held (knot 1 2)", "Held (knot 1 2)"},
		{"a Hold of a Hold", "Held (Held 5)", "Held (Held 5)"},
		{"a Circle of Knots", "circle [(knot 0 0)]", "{(knot 0 0)}"},
		{
			"declared values in a Thread",
			"Tree a is Leaf | Node (Tree a) a (Tree a)\n\n[[Node Leaf 1 Leaf, Leaf]]",
			"[(Node Leaf 1 Leaf) Leaf]",
		},
		// Anything already delimited stays as it is.
		{"nested Threads", "[[[1 2] [3 4]]]", "[[1 2] [3 4]]"},
		{"tuples", "[[(1, 2), (3, 4)]]", "[(1, 2) (3, 4)]"},
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
