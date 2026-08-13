package build

import (
	"strings"

	"github.com/malleum/weave/internal/diag"
	"github.com/malleum/weave/internal/parser"
)

// Salvage answers "the file does not compile"; this answers "the file does not
// finish". They are the same problem wearing different clothes: one definition
// is holding the whole file's ghost text hostage, and every other line is
// perfectly reportable. The cure is the same too — take the offender out and
// try again, keeping the surviving lines exactly where they were.
//
// The difference is that a definition which will not finish cannot be found by
// looking at the program; it has to be run. So `weave trace` runs under a time
// limit and reads the records that arrived: the first top-level item that never
// reported is the one still going when the limit ran out.

// Item is one top-level item, in the order `weave trace` reports them.
type Item struct {
	Line int    // 1-based line it starts on, which is where its record goes
	Last int    // last line it covers, so a record can be attributed to it
	Name string // empty for an output expression, matching the record format
}

// Items lists the top-level items of src in the order they are traced, which is
// the order codegen's emitTrace forces them: definitions and output expressions
// interleaved by line, so that reading the records is reading down the file.
//
// It returns nil for a program that does not parse, which cannot be run anyway.
func Items(src string) []Item {
	bag := diag.New("items.weave", src)
	f := parser.Parse(src, bag)
	if !bag.Empty() || f == nil {
		return nil
	}

	var items []Item
	decls, outs := f.Decls, f.Outputs
	for len(decls) > 0 || len(outs) > 0 {
		if len(decls) > 0 && (len(outs) == 0 || decls[0].NamePos.Line <= outs[0].Pos().Line) {
			d := decls[0]
			decls = decls[1:]
			items = append(items, Item{Line: d.NamePos.Line, Name: d.Name})
			continue
		}
		e := outs[0]
		outs = outs[1:]
		items = append(items, Item{Line: e.Pos().Line})
	}

	// An item covers everything up to the next one. A definition reports on its
	// own lines and nowhere else, so that is enough to say which item a record
	// came from.
	last := len(strings.Split(src, "\n"))
	for i := len(items) - 1; i >= 0; i-- {
		items[i].Last = last
		last = items[i].Line - 1
	}
	return items
}

// Unreported returns the first item after the given line that no record was
// seen for — the one still running when the limit ran out.
func Unreported(items []Item, reported map[int]bool, after int) (Item, bool) {
	for _, it := range items {
		if it.Line <= after {
			continue
		}
		heard := false
		for line := it.Line; line <= it.Last && !heard; line++ {
			heard = reported[line]
		}
		if !heard {
			return it, true
		}
	}
	return Item{}, false
}

// Blank takes out the top-level item at the given line, leaving the lines it
// occupied empty so everything below keeps the line number the editor is
// showing it at — the same trick Salvage uses, for the same reason.
func Blank(src string, at int) string {
	lines := strings.Split(src, "\n")
	lo, hi := itemAround(lines, at)
	if lo < 0 {
		return src
	}
	for i := lo; i <= hi && i < len(lines); i++ {
		lines[i] = ""
	}
	return strings.Join(lines, "\n")
}
