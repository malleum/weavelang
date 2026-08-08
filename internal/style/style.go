// Package style renders terminal colour, and knows when not to.
//
// Colour is switched off when the stream is not a terminal, when NO_COLOR is
// set to anything at all (https://no-color.org), when TERM says the terminal
// cannot do it, or when WEAVE_COLOR says so explicitly. That last one is what
// the tests use: a golden file with escape sequences in it is a golden file
// nobody can read.
package style

import (
	"fmt"
	"io"
	"os"
	"strings"
)

// Colour codes. Deliberately the eight basic colours plus bold and dim, so
// this looks right on a light background as well as a dark one and does not
// fight whatever palette the terminal already has.
const (
	reset     = "\x1b[0m"
	boldSeq   = "\x1b[1m"
	dimSeq    = "\x1b[2m"
	redSeq    = "\x1b[31m"
	greenSeq  = "\x1b[32m"
	yellowSeq = "\x1b[33m"
	blueSeq   = "\x1b[34m"
	magenta   = "\x1b[35m"
	cyanSeq   = "\x1b[36m"
)

// Style renders text for one stream.
type Style struct{ on bool }

// For returns the style appropriate to a writer.
func For(w io.Writer) *Style { return &Style{on: enabled(w)} }

// Plain returns a style that never colours anything.
func Plain() *Style { return &Style{} }

// On reports whether this style is colouring.
func (s *Style) On() bool { return s != nil && s.on }

func (s *Style) wrap(seq, text string) string {
	if !s.On() || text == "" {
		return text
	}
	return seq + text + reset
}

func (s *Style) Bold(text string) string    { return s.wrap(boldSeq, text) }
func (s *Style) Dim(text string) string     { return s.wrap(dimSeq, text) }
func (s *Style) Red(text string) string     { return s.wrap(redSeq, text) }
func (s *Style) Green(text string) string   { return s.wrap(greenSeq, text) }
func (s *Style) Yellow(text string) string  { return s.wrap(yellowSeq, text) }
func (s *Style) Blue(text string) string    { return s.wrap(blueSeq, text) }
func (s *Style) Magenta(text string) string { return s.wrap(magenta, text) }
func (s *Style) Cyan(text string) string    { return s.wrap(cyanSeq, text) }

// Accent is the language's own colour, used for the banner, headings and
// anything that should read as Weave speaking rather than reporting.
func (s *Style) Accent(text string) string { return s.wrap(cyanSeq+boldSeq, text) }

// Sprintf formats and then applies a colour, which saves a nesting at most call
// sites.
func (s *Style) Sprintf(colour func(string) string, format string, args ...any) string {
	return colour(fmt.Sprintf(format, args...))
}

// enabled decides whether a stream should be coloured.
func enabled(w io.Writer) bool {
	switch strings.ToLower(os.Getenv("WEAVE_COLOR")) {
	case "always", "1", "true", "yes":
		return true
	case "never", "0", "false", "no":
		return false
	}
	if _, set := os.LookupEnv("NO_COLOR"); set {
		return false
	}
	if term := os.Getenv("TERM"); term == "" || term == "dumb" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
