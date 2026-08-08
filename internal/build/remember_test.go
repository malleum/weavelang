package build_test

import (
	"strings"
	"testing"
	"time"

	"github.com/malleum/weave/internal/build"
	"github.com/malleum/weave/internal/diag"
)

// `remember` keeps a definition's results, keyed on its arguments.
func TestRememberEndToEnd(t *testing.T) {
	cases := []struct {
		name, src, in, want string
	}{
		{
			name: "a remembered definition answers the same as a plain one",
			src: `remember fib 0 is 0
fib 1 is 1
fib n is add (fib (sub n 1)) (fib (sub n 2))

fib 25
`,
			want: "75025",
		},
		{
			name: "the marker may be written on any clause",
			src: `slow 0 is 0
remember slow n is add 1 (slow (sub n 1))

slow 100
`,
			want: "100",
		},
		{
			name: "remembered on two arguments",
			src: `remember paths 0 _ is 1
paths _ 0 is 1
paths r c is add (paths (sub r 1) c) (paths r (sub c 1))

paths 14 14
`,
			want: "40116600",
		},
		{
			name: "remembered on a Knot",
			src: `remember reach k is
  ward k
    knot 0 _ : 1
    knot _ 0 : 1
    knot r c : add (reach (knot (sub r 1) c)) (reach (knot r (sub c 1)))

reach (knot 12 12)
`,
			want: "2704156",
		},
		{
			name: "remembered on a declared type",
			src: `Coin is Penny | Nickel | Dime

remember worth Penny is 1
worth Nickel is 5
worth Dime is 10

[Penny, Dime, Penny, Nickel] | bend worth | sum
`,
			want: "17",
		},
		{
			name: "remembered on text",
			src: `remember size s is len s

Source | lines | bend size | sum
`,
			in:   "ab\ncde\nab\n",
			want: "7",
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

// The point of the marker is the running time, so this checks that: fib 34
// unmemoised is about eleven million calls, and finishes in well under a
// second either way — but only one of the two would still be going at fib 60.
func TestRememberActuallyMemoises(t *testing.T) {
	requireCC(t)

	src := `remember fib 0 is 0
fib 1 is 1
fib n is add (fib (sub n 1)) (fib (sub n 2))

fib 60
`
	done := make(chan string, 1)
	go func() { done <- compileAndRun(t, "fib60.weave", src, "") }()

	select {
	case got := <-done:
		if strings.TrimRight(got, "\n") != "1548008755920" {
			t.Errorf("got %q, want 1548008755920", got)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("fib 60 did not finish, so `remember` is not memoising")
	}
}

func TestRememberIsChecked(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{
			"nothing to remember by",
			"remember answer is 42\n\nanswer\n",
			"takes no arguments",
		},
		{
			"an argument that cannot be compared",
			"remember twice f x is f (f x)\n\ntwice (add 1) 2\n",
			"a function has no Eq Talent",
		},
		{
			"remember with no name after it",
			"remember is 1\n\n1\n",
			"a name must follow it",
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
