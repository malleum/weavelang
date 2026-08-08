package build

import (
	"strings"
	"testing"
)

// TestSalvageLeavesAWorkingFileAlone is the property that keeps the normal
// path cheap and honest: a file that compiles is handed back byte for byte.
func TestSalvageLeavesAWorkingFileAlone(t *testing.T) {
	src := "nums is [1 2 3]\n\ntotal is nums through sum\n\ntotal\n"
	got, dropped := Salvage("t.weave", src)
	if got != src || dropped != 0 {
		t.Errorf("a working file was changed: dropped %d\n%s", dropped, got)
	}
}

// TestSalvageKeepsLineNumbers is what makes the result useful to an editor:
// every definition that survives has to still be on the line it was on, or the
// ghost text lands beside the wrong code.
func TestSalvageKeepsLineNumbers(t *testing.T) {
	src := strings.Join([]string{
		"nums is [3 1 4]",
		"",
		"bad is nums through frobnicate",
		"",
		"total is nums through sum",
		"",
		"total",
		"",
	}, "\n")
	got, dropped := Salvage("t.weave", src)
	if dropped != 1 {
		t.Fatalf("expected one definition dropped, got %d:\n%s", dropped, got)
	}
	lines := strings.Split(got, "\n")
	if len(lines) != len(strings.Split(src, "\n")) {
		t.Fatalf("the line count changed:\n%s", got)
	}
	if lines[0] != "nums is [3 1 4]" || lines[4] != "total is nums through sum" {
		t.Errorf("the surviving definitions moved:\n%s", got)
	}
	if strings.Contains(got, "frobnicate") {
		t.Errorf("the broken definition survived:\n%s", got)
	}
	if !compiles("t.weave", got) {
		t.Errorf("what came back does not compile:\n%s", got)
	}
}

// TestSalvageBlamesTheHalfTypedLine is the case this exists for. An unclosed
// bracket is reported at the *next* top-level item, which is innocent — so the
// item that will not parse on its own goes first, and the innocent one stays.
func TestSalvageBlamesTheHalfTypedLine(t *testing.T) {
	src := strings.Join([]string{
		"nums is [3 1 4]",
		"",
		"half is nums through bend (",
		"",
		"total is nums through sum",
		"",
		"total",
		"",
	}, "\n")
	got, dropped := Salvage("t.weave", src)
	if dropped != 1 {
		t.Errorf("expected one definition dropped, got %d:\n%s", dropped, got)
	}
	if !strings.Contains(got, "total is nums through sum") {
		t.Errorf("the innocent definition was dropped:\n%s", got)
	}
	if !compiles("t.weave", got) {
		t.Errorf("what came back does not compile:\n%s", got)
	}
}

// TestSalvageDropsWhatDependsOnTheBrokenThing, since a definition standing on
// one that has gone has no value either.
func TestSalvageDropsWhatDependsOnTheBrokenThing(t *testing.T) {
	src := "nums is [3 1 4]\nbad is nums through frobnicate\nuses is bad through sum\nuses\n"
	got, _ := Salvage("t.weave", src)
	if strings.Contains(got, "uses is bad") {
		t.Errorf("a definition standing on the broken one survived:\n%s", got)
	}
	if !strings.Contains(got, "nums is [3 1 4]") {
		t.Errorf("the definition that did not depend on it was dropped:\n%s", got)
	}
	if !compiles("t.weave", got) {
		t.Errorf("what came back does not compile:\n%s", got)
	}
}

// TestSalvageOfSomethingBrokenAllTheWayDown has to terminate and hand back
// something that compiles, even when that is nothing at all.
func TestSalvageOfSomethingBrokenAllTheWayDown(t *testing.T) {
	got, _ := Salvage("t.weave", "a is frobnicate 1\nb is a through frobnicate\nb\n")
	if !compiles("t.weave", got) {
		t.Errorf("what came back does not compile:\n%s", got)
	}
	if strings.Contains(got, "frobnicate") {
		t.Errorf("something broken survived:\n%s", got)
	}
}

// TestSalvageKeepsAMultiClauseDefinitionTogether: dropping one clause of a
// definition would leave the rest non-exhaustive, which is a different error
// rather than fewer of them.
func TestSalvageKeepsAMultiClauseDefinitionWorking(t *testing.T) {
	src := strings.Join([]string{
		"fib 0 is 0",
		"fib 1 is 1",
		"fib n is add (fib (sub n 1)) (fib (sub n 2))",
		"",
		"bad is frobnicate 2",
		"",
		"fib 10",
		"",
	}, "\n")
	got, _ := Salvage("t.weave", src)
	if !compiles("t.weave", got) {
		t.Errorf("what came back does not compile:\n%s", got)
	}
	if !strings.Contains(got, "fib 10") {
		t.Errorf("the output expression was dropped:\n%s", got)
	}
}
