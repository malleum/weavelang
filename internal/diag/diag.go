// Package diag collects and renders compiler diagnostics with source context.
package diag

import (
	"fmt"
	"sort"
	"strings"

	"github.com/malleum/weave/internal/style"
	"github.com/malleum/weave/internal/token"
)

// Diagnostic is a single compiler error, optionally carrying a hint that
// suggests a fix.
type Diagnostic struct {
	Pos  token.Pos
	Msg  string
	Hint string
	// Soft marks a diagnostic the compiler could proceed past. There is
	// exactly one kind: a definition that does not cover every case, which
	// compiles to a runtime trap on the missing one. It is an error in a
	// program, because a program that runs at all should run to the end. In
	// the REPL it is not, because a definition being written a clause at a
	// time is incomplete in between, and refusing it makes the REPL unusable
	// for the thing it is for.
	Soft bool
}

func (d Diagnostic) Error() string { return fmt.Sprintf("%s: %s", d.Pos, d.Msg) }

// Bag accumulates diagnostics for one source file. The zero value is not
// usable; construct one with New.
type Bag struct {
	File  string
	lines []string
	diags []Diagnostic
	// Lenient makes soft diagnostics advisory: they are still recorded and
	// still rendered, but they no longer make the bag non-empty, so the
	// compilation proceeds. The REPL sets it; nothing else does.
	Lenient bool
}

// New returns a Bag that renders diagnostics against src, reported as file.
func New(file, src string) *Bag {
	return &Bag{File: file, lines: strings.Split(src, "\n")}
}

// Add records a diagnostic at pos.
func (b *Bag) Add(pos token.Pos, format string, args ...any) {
	b.diags = append(b.diags, Diagnostic{Pos: pos, Msg: fmt.Sprintf(format, args...)})
}

// AddHint records a diagnostic at pos along with a suggested fix.
func (b *Bag) AddHint(pos token.Pos, hint, format string, args ...any) {
	b.diags = append(b.diags, Diagnostic{Pos: pos, Msg: fmt.Sprintf(format, args...), Hint: hint})
}

// AddSoft records a diagnostic the compiler could proceed past. See
// Diagnostic.Soft.
func (b *Bag) AddSoft(pos token.Pos, hint, format string, args ...any) {
	b.diags = append(b.diags, Diagnostic{
		Pos: pos, Msg: fmt.Sprintf(format, args...), Hint: hint, Soft: true,
	})
}

// Empty reports whether nothing has been recorded that should stop the
// compilation.
func (b *Bag) Empty() bool { return b.Len() == 0 }

// Len returns the number of recorded diagnostics that should stop the
// compilation.
func (b *Bag) Len() int {
	n := 0
	for _, d := range b.diags {
		if !d.Soft || !b.Lenient {
			n++
		}
	}
	return n
}

// Notes returns the soft diagnostics, which are only separated out when the
// bag is lenient — otherwise they are ordinary errors and appear in All.
func (b *Bag) Notes() []Diagnostic {
	if !b.Lenient {
		return nil
	}
	var out []Diagnostic
	for _, d := range b.All() {
		if d.Soft {
			out = append(out, d)
		}
	}
	return out
}

// All returns the recorded diagnostics in source order.
func (b *Bag) All() []Diagnostic {
	sort.SliceStable(b.diags, func(i, j int) bool {
		a, c := b.diags[i].Pos, b.diags[j].Pos
		if a.Line != c.Line {
			return a.Line < c.Line
		}
		return a.Col < c.Col
	})
	return b.diags
}

// Err returns an error summarising the bag, or nil if it is empty.
func (b *Bag) Err() error {
	if b.Empty() {
		return nil
	}
	return fmt.Errorf("%s", b.String())
}

// String renders every diagnostic with a source excerpt and caret, in the
// shape:
//
//	file.weave:3:5: cannot find `foo`
//	  3 | bar is foo 1
//	    |        ^
//	  hint: did you mean `food`?
//
// It never colours, so a diagnostic embedded in a test or a log stays
// readable. Rendered goes through a style.
func (b *Bag) String() string { return b.Rendered(style.Plain()) }

// Rendered is String with colour: the location bold, the message plain, the
// caret red under the offending column, and the hint dim. Nothing else, since
// a diagnostic is read in a hurry.
func (b *Bag) Rendered(st *style.Style) string {
	var sb strings.Builder
	for i, d := range b.All() {
		if i > 0 {
			sb.WriteByte('\n')
		}
		fmt.Fprintf(&sb, "%s %s\n",
			st.Bold(fmt.Sprintf("%s:%s:", b.File, d.Pos)), d.Msg)
		if line, ok := b.line(d.Pos.Line); ok {
			num := fmt.Sprintf("%d", d.Pos.Line)
			gutter := strings.Repeat(" ", len(num))
			fmt.Fprintf(&sb, " %s %s\n", st.Dim(num+" |"), line)
			fmt.Fprintf(&sb, " %s %s%s\n", st.Dim(gutter+" |"),
				caretPad(line, d.Pos.Col), st.Red("^"))
		}
		if d.Hint != "" {
			fmt.Fprintf(&sb, " %s %s\n", st.Yellow("hint:"), st.Dim(d.Hint))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (b *Bag) line(n int) (string, bool) {
	if n < 1 || n > len(b.lines) {
		return "", false
	}
	return strings.TrimRight(b.lines[n-1], "\r"), true
}

// caretPad builds the run of whitespace that places a caret under column col,
// preserving tabs so the caret stays aligned in the reader's terminal.
func caretPad(line string, col int) string {
	var sb strings.Builder
	for i := 0; i < col-1 && i < len(line); i++ {
		if line[i] == '\t' {
			sb.WriteByte('\t')
		} else {
			sb.WriteByte(' ')
		}
	}
	for i := len(line); i < col-1; i++ {
		sb.WriteByte(' ')
	}
	return sb.String()
}
