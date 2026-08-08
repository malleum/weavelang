package build

import (
	"strings"

	"github.com/malleum/weave/internal/check"
	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/parser"
)

// A program being edited does not compile most of the time — it stops
// compiling the moment a line is half typed, and starts again a second later.
// `weave trace` reporting nothing for that second means the editor's ghost text
// blinks out on every keystroke, which is worse than useless: it is the values
// disappearing exactly when a change is being made to them.
//
// Salvage answers the question the editor actually has, which is not "does this
// file compile" but "what still holds". It blanks out the top-level item the
// first error is in and tries again, keeping every other line where it was, and
// repeats until what is left compiles or there is nothing left to drop.
//
// Blanking rather than deleting is the whole trick: every record `weave trace`
// then reports still carries the line number it had in the file the editor is
// showing, so the values that survive land beside the code they came from.

// Salvage returns the largest prefix-shaped subset of src that compiles, along
// with how many top-level items it had to drop.
//
// It returns src unchanged when src already compiles, so the normal path costs
// one parse.
func Salvage(path, src string) (string, int) {
	if compiles(path, src) {
		return src, 0
	}

	lines := strings.Split(src, "\n")
	// First the items that do not parse on their own. A top-level item is
	// syntactically self-contained, so one that cannot be read alone is the
	// one that is half typed — and it has to go first, because an unclosed
	// bracket is reported at the *next* item, which is innocent.
	dropped := dropUnparsable(lines)
	// Bounded by the number of top-level items, and every turn blanks at least
	// one line, so this cannot spin.
	for turn := 0; turn < len(lines); turn++ {
		joined := strings.Join(lines, "\n")
		bag := diag.New(path, joined)
		checkOnly(joined, bag)
		if bag.Empty() {
			return joined, dropped
		}
		at := firstErrorLine(bag)
		if at == 0 {
			break
		}
		lo, hi := itemAround(lines, at)
		if lo < 0 {
			break
		}
		for i := lo; i <= hi && i < len(lines); i++ {
			lines[i] = ""
		}
		dropped++
	}
	// Nothing survived, which is a file that is broken all the way down. The
	// caller reports the original errors rather than a mangled program.
	return src, dropped
}

// dropUnparsable blanks every top-level item that will not parse by itself,
// and reports how many it blanked.
func dropUnparsable(lines []string) int {
	dropped := 0
	for lo := 0; lo < len(lines); {
		if !startsItem(lines[lo]) {
			lo++
			continue
		}
		hi := lo
		for hi+1 < len(lines) && !startsItem(lines[hi+1]) {
			hi++
		}
		item := strings.Join(lines[lo:hi+1], "\n") + "\n"
		bag := diag.New("item.weave", item)
		parser.Parse(item, bag)
		if !bag.Empty() {
			for i := lo; i <= hi; i++ {
				lines[i] = ""
			}
			dropped++
		}
		lo = hi + 1
	}
	return dropped
}

func compiles(path, src string) bool {
	bag := diag.New(path, src)
	checkOnly(src, bag)
	return bag.Empty()
}

func firstErrorLine(bag *diag.Bag) int {
	for _, d := range bag.All() {
		if d.Pos.Line > 0 {
			return d.Pos.Line
		}
	}
	return 0
}

// itemAround finds the top-level item the given line belongs to: from the last
// line at column zero at or above it, down to the last line before the next
// one at column zero.
//
// Layout is the only structure needed here, and it is the structure the
// language already has — a top-level item starts in column zero and everything
// under it is indented.
func itemAround(lines []string, at int) (int, int) {
	i := at - 1 // to a 0-based index
	if i < 0 || i >= len(lines) {
		return -1, -1
	}
	lo := i
	for lo > 0 && !startsItem(lines[lo]) {
		lo--
	}
	if !startsItem(lines[lo]) {
		return -1, -1
	}
	hi := lo
	for hi+1 < len(lines) && !startsItem(lines[hi+1]) {
		hi++
	}
	return lo, hi
}

// startsItem reports whether a line begins a top-level item: it has content
// and it begins in column zero.
func startsItem(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}
	return !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t")
}

// checkOnly runs the front end without generating anything, which is all that
// is needed to know whether a program would compile.
func checkOnly(src string, bag *diag.Bag) {
	file := parser.Parse(src, bag)
	if !bag.Empty() {
		return
	}
	check.File(file, bag)
}
