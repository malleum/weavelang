package build_test

import (
	"strings"
	"testing"

	"github.com/malleum/weave/internal/build"
	"github.com/malleum/weave/internal/diag"
)

// The hole words answer two different questions, and the spellings say which:
// `_` and `this` are the first argument, `that` is the second, and the rest are
// components of the first.
//
// A component word carries its *width* as well as its position — `former` and
// `latter` are the halves of a Twine of two, `fore`, `mid` and `aft` the parts
// of one of three — so one word on its own says both, and a group holding one
// can be read where it stands rather than after its type is known. That is what
// makes these behave the same in a pipeline stage, in brackets, and as a
// function's argument. Two earlier spellings named a position relative to a
// width instead, and each worked in some of those three places and not others,
// which is the whole reason for the third attempt.
func TestHoleWords(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"a hole is the argument", "[1 2 3] | bend (mul _ _) | air\n", "[1 4 9]"},
		{"this is the same word", "[1 2 3] | bend (mul this this) | air\n", "[1 4 9]"},
		{"that is the second", "[1 2 3] | braid (add this that) 0\n", "6"},
		{"a stage binds the hole", `web [(1, 'a')] | get _ 1 | otherwise 'z'` + "\n", "a"},

		// A Twine of two.
		{"the halves of a pair", "(1, 5) | add former latter\n", "6"},
		{"former alone", "[(1, 5), (3, 2)] as (former) | sum\n", "4"},
		{"latter alone", "[(1, 5), (3, 2)] as (latter) | sum\n", "7"},
		{"a pair through a stage", "[(1, 5), (3, 2)] as add former latter | sum\n", "11"},

		// A Twine of three, which is what `dupe` answers with. Every one of
		// these is somewhere an earlier spelling failed.
		{"fore of a triple", "(1, 5, 9) | fore\n", "1"},
		{"mid of a triple", "(1, 5, 9) | mid\n", "5"},
		{"aft of a triple", "(1, 5, 9) | aft\n", "9"},
		{"bracketed, on a triple", "(1, 5, 9) | (fore)\n", "1"},
		{"as a function's argument", "[(1, 5, 9)] as (aft) | sum\n", "9"},
		{"two of the three", "(1, 5, 9) | sub aft fore\n", "8"},
		{"a triple through a stage", "[(1, 2, 3), (4, 5, 6)] as add fore mid | sum\n", "12"},
		{"where a dupe found the repeat", "[3 1 2 1] | dupe else (0, 0, 0) | fore\n", "3"},
		{"what a dupe repeated", "[3 1 2 1] | dupe else (0, 0, 0) | (aft)\n", "1"},
		{"the cycle a dupe found", "[3 1 2 1] | dupe else (0, 0, 0) | sub fore mid\n", "2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := strings.TrimRight(compileAndRun(t, "holes.weave", tc.src, ""), "\n")
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// Each word says how wide the Twine is, so there are three ways to be wrong: a
// word against the wrong width, two words that disagree with each other, and a
// word with nothing to be part of. Nothing names a component of a Twine of
// four, which is deliberate — at that width a pattern says it better.
func TestHoleWidthIsCheckedAgainstTheTwine(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"a pair's words against a triple", "(1, 5, 9) | add former latter\n", "(a, b, c)"},
		{"a triple's words against a pair", "(1, 5) | add fore mid\n", "(a, b)"},
		{"one of a triple's words against a pair", "(1, 5) | (mid)\n", "(a, b)"},
		{"nothing names a component of four", "(1, 5, 9, 11) | aft\n", "(a, b, c, d)"},
		{"words that disagree", "(1, 5) | add former aft\n", "disagree about how wide"},
		{"disagreeing inside brackets", "[(1, 5)] as (add former aft)\n", "disagree about how wide"},
		{"a stray part", "add former 1\n", "no Twine to be part of"},
		{"a stray second argument", "[1 2 3] | bend that | sum\n", "no second argument"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bag := diag.New("holes.weave", tc.src)
			if _, err := build.Compile("holes.weave", tc.src, build.Options{}, bag); err == nil {
				t.Fatalf("expected a compile error for:\n%s", tc.src)
			}
			if out := bag.String(); !strings.Contains(out, tc.want) {
				t.Errorf("expected an error mentioning %q, got:\n%s", tc.want, out)
			}
		})
	}
}
