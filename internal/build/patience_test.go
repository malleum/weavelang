package build

import (
	"strings"
	"testing"
)

// TestItemsReadsDownTheFile is the property `weave trace` leans on to say which
// definition ran out of time: the items come back in the order the program
// forces them, which is the order they are written.
func TestItemsReadsDownTheFile(t *testing.T) {
	src := strings.Join([]string{
		"nums is [3 1 4]",
		"",
		"double n is mul 2 n",
		"",
		"nums through sum",
		"",
		"total is nums",
		"  through bend double",
		"  through sum",
		"",
		"total",
		"",
	}, "\n")

	items := Items(src)
	want := []Item{
		{Line: 1, Last: 2, Name: "nums"},
		{Line: 3, Last: 4, Name: "double"},
		{Line: 5, Last: 6},
		{Line: 7, Last: 10, Name: "total"},
		{Line: 11, Last: 12},
	}
	if len(items) != len(want) {
		t.Fatalf("got %d items, want %d: %v", len(items), len(want), items)
	}
	for i, it := range items {
		if it != want[i] {
			t.Errorf("item %d is %+v, want %+v", i, it, want[i])
		}
	}
}

// A definition written over several lines owns all of them, so a record from
// any of its stages counts as that definition reporting.
func TestUnreportedBlamesTheItemThatSaidNothing(t *testing.T) {
	src := strings.Join([]string{
		"nums is [3 1 4]",
		"",
		"total is nums",
		"  through sum",
		"",
		"nums through len",
		"",
	}, "\n")
	items := Items(src)

	// `nums` and the middle of `total` reported; the last line did not.
	got, found := Unreported(items, map[int]bool{1: true, 4: true}, 0)
	if !found || got.Line != 6 {
		t.Errorf("blamed %+v (found %v), want the item on line 6", got, found)
	}

	// Once everything has reported there is nobody left to blame.
	if _, found := Unreported(items, map[int]bool{1: true, 4: true, 6: true}, 0); found {
		t.Errorf("blamed an item when every one of them reported")
	}

	// `after` skips the item already blamed, so a second turn moves on rather
	// than accusing the same line twice.
	got, found = Unreported(items, map[int]bool{1: true}, 3)
	if !found || got.Line != 6 {
		t.Errorf("after line 3 blamed %+v (found %v), want line 6", got, found)
	}
}

// Blank is Salvage's trick with a different reason: the item goes, its lines
// stay, and everything below keeps the number the editor is showing it at.
func TestBlankKeepsLineNumbers(t *testing.T) {
	src := strings.Join([]string{
		"nums is [3 1 4]",
		"",
		"slow is nums",
		"  through sum",
		"",
		"nums through len",
		"",
	}, "\n")

	got := Blank(src, 3)
	lines := strings.Split(got, "\n")
	if len(lines) != len(strings.Split(src, "\n")) {
		t.Fatalf("the line count changed:\n%s", got)
	}
	if strings.Contains(got, "slow") {
		t.Errorf("the blanked item survived:\n%s", got)
	}
	if lines[0] != "nums is [3 1 4]" || lines[5] != "nums through len" {
		t.Errorf("the surviving lines moved:\n%s", got)
	}
	// What is left of the file is what Salvage then has to make compile.
	if trimmed, _ := Salvage("t.weave", got); !compiles("t.weave", trimmed) {
		t.Errorf("what came back does not compile:\n%s", trimmed)
	}
}
