package style

import (
	"fmt"
	"strings"
)

// The title, and the help screen it heads.
//
// The block letters are the only decoration the compiler has, and they sit
// above a woven rule — the one place the language gets to look like its name.
// Everything below them is plain, because a help screen is read far more often
// than it is admired.

const letters = `
 ██╗    ██╗███████╗ █████╗ ██╗   ██╗███████╗
 ██║    ██║██╔════╝██╔══██╗██║   ██║██╔════╝
 ██║ █╗ ██║█████╗  ███████║██║   ██║█████╗
 ██║███╗██║██╔══╝  ██╔══██║╚██╗ ██╔╝██╔══╝
 ╚███╔███╔╝███████╗██║  ██║ ╚████╔╝ ███████╗
  ╚══╝╚══╝ ╚══════╝╚═╝  ╚═╝  ╚═══╝  ╚══════╝`

// woven is the rule under the title: warp and weft.
const woven = " ╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲╱╲"

// Banner returns the title block. version is shown beside the rule.
func (s *Style) Banner(version string) string {
	var b strings.Builder
	b.WriteString(s.Accent(letters))
	b.WriteString("\n")
	b.WriteString(s.Dim(woven))
	b.WriteString("\n")
	fmt.Fprintf(&b, " %s %s\n",
		s.Dim("a functional language for Advent of Code, compiled through C  ·"),
		s.Dim(version))
	return b.String()
}

// Command is one line of the help screen: the name, what it takes, and what it
// does.
type Command struct {
	Name string
	Args string
	Doc  string
}

// Help renders the command list under a heading.
func (s *Style) Help(heading string, commands []Command) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n%s\n\n", s.Bold(heading))
	for _, c := range commands {
		if c.Name == "" {
			b.WriteString("\n")
			continue
		}
		fmt.Fprintf(&b, "  %s %s   %s\n",
			pad(s.Accent(c.Name), c.Name, 8),
			pad(s.Dim(c.Args), c.Args, 12),
			c.Doc)
	}
	return b.String()
}

// pad widens a coloured string to a column, measuring the text rather than the
// escape sequences around it.
func pad(coloured, plain string, width int) string {
	if n := width - len([]rune(plain)); n > 0 {
		return coloured + strings.Repeat(" ", n)
	}
	return coloured
}
