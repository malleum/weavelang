package prelude

import (
	"fmt"
	"sort"
	"strings"
)

// The built-in vocabulary, rendered. `weave verbs` prints this at a terminal
// and `docs/verbs.md` is the same thing in Markdown, generated rather than
// written so that it cannot fall behind the table above — which it had, by
// about ninety verbs, before this existed.

// Group is one heading of the reference, with the entries filed under it.
type Group struct {
	Name    string
	Entries []Entry
}

// Groups returns the vocabulary in reference order: the groups in the order
// they are declared, and within each the order they were written, which puts
// the verb you reach for first at the top rather than alphabetising `abs` above
// `bend`.
func Groups() []Group {
	var out []Group
	index := map[string]int{}
	for _, e := range Values {
		i, seen := index[e.Group]
		if !seen {
			i = len(out)
			index[e.Group] = i
			out = append(out, Group{Name: e.Group})
		}
		out[i].Entries = append(out[i].Entries, e)
	}
	return out
}

// Signature renders an entry the way it would be written in a program.
func Signature(e Entry) string {
	sig := e.Name + " :: " + e.Sig
	if e.Where != "" {
		sig += "  where " + e.Where
	}
	return sig
}

// Matches reports whether an entry answers a search: its name, its type or its
// description. Searching by type is the useful one — "Knot" finds everything
// that takes or returns a coordinate.
func Matches(e Entry, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	return strings.Contains(strings.ToLower(e.Name), q) ||
		strings.Contains(strings.ToLower(e.Sig), q) ||
		strings.Contains(strings.ToLower(e.Doc), q) ||
		strings.Contains(strings.ToLower(e.Group), q)
}

// Markdown renders the whole reference as a document. The heading and the
// preamble are passed in so the generator owns the prose and this owns the
// table.
func Markdown(title, preamble string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n", title, preamble)

	groups := Groups()

	b.WriteString("\n## Contents\n\n")
	for _, g := range groups {
		fmt.Fprintf(&b, "- [%s](#%s) — %d\n", g.Name, anchor(g.Name), len(g.Entries))
	}
	fmt.Fprintf(&b, "- [Constructors](#constructors) — %d\n", len(Ctors))

	for _, g := range groups {
		fmt.Fprintf(&b, "\n## %s\n\n", g.Name)
		if doc := groupDocs[g.Name]; doc != "" {
			fmt.Fprintf(&b, "%s\n\n", doc)
		}
		b.WriteString("| | |\n|---|---|\n")
		for _, e := range g.Entries {
			fmt.Fprintf(&b, "| `%s` | %s |\n", Signature(e), e.Doc)
		}
	}

	b.WriteString("\n## Constructors\n\nHow a value of a built-in sum type is made, and how a pattern takes it apart.\n\n")
	b.WriteString("| | | |\n|---|---|---|\n")
	for _, c := range Ctors {
		fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", c.Name+" :: "+c.Sig, c.Owner, c.Doc)
	}

	b.WriteString("\n## Special forms\n\n")
	b.WriteString("One name the checker handles itself, because it does not have an ordinary type.\n\n")
	b.WriteString("| | |\n|---|---|\n")
	b.WriteString("| `Source :: Air` | the program's input, read once |\n")

	return b.String()
}

// groupDocs is the one line of prose each heading gets. Keeping it here rather
// than on every entry means it is written once.
var groupDocs = map[string]string{
	"Input":     "Read once, whatever the program does with it.",
	"Sequences": "`Thread a`. A chain of these fuses into a single pass with no intermediate Thread, and one that can answer early — `seek`, `first`, `any`, `all` — stops the whole chain there.",
	"Text":      "`Air`. Text is a `Bulk` type, so `len` counts its characters.",
	"Absence and failure": "`Hold a` is `Held a | Stilled` and `Weaving a e` is `Woven a | Gentled e`. " +
		"There is no null: the compiler makes you handle the empty case.",
	"Grids":                      "`Pattern a`, indexed by `Knot`. A grid threaded through a loop is updated in place rather than copied.",
	"Maps":                       "`Web k v`, a hash array mapped trie. Keyed access takes the collection first, so `w | get _ k` is how it joins a pipeline.",
	"Sets":                       "`Circle a`, sharing the map's trie.",
	"Priority queues and graphs": "`Taveren a` is a leftist heap. Every graph verb here takes the same thing — a function from a place to the steps out of it — which works on an implicit graph, a grid or a state machine, as readily as on one you built. An explicit graph is a `Web k (Thread k)`, which `group` and `mapvals` make in a line.",
	"Numbers":                    "There are no operators anywhere in Weave: arithmetic is these verbs. The `Reckon` Talent is Earth and Water.",
	"Comparison":                 "`Eq` and `Ord` are Talents, so these work on anything that has them — including a type you declared, which derives both.",
	"Logic":                      "`Spirit` is `Light` and `Shadow`. `pick` is the conditional, and evaluates only the branch it takes.",
	"Characters":                 "`Fire`.",
}

func anchor(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	return s
}

// Names lists every built-in name, sorted, for tooling that wants the
// vocabulary and nothing else.
func Names() []string {
	out := make([]string, 0, len(Values)+len(Ctors))
	for _, e := range Values {
		out = append(out, e.Name)
	}
	for _, c := range Ctors {
		out = append(out, c.Name)
	}
	sort.Strings(out)
	return out
}
