// Package docs builds Weave's reference as a page: every verb, every
// constructor, every keyword, with its type, and a search over the lot.
//
// It is deliberately a poor introduction and a good reference. Everything a
// verb is compared to is another Weave verb, everything a type is explained by
// is another Weave type, and the words are the ones the language is named out
// of. Someone who has not written Weave will not learn it here. Someone who
// has will find the signature they came for in one keystroke, which is what a
// reference is for.
//
// The page is one file with nothing fetched from anywhere, so it works with
// the network off and keeps working when whatever it would have fetched stops
// existing. `weave docs` serves it; `weave docs -o page.html` writes it out.
package docs

import (
	"fmt"
	"html/template"
	"sort"
	"strings"

	_ "embed"

	"github.com/malleum/weave/internal/prelude"
	"github.com/malleum/weave/internal/token"
)

//go:embed page.html
var pageTemplate string

// Verb is one entry as the page needs it.
type Verb struct {
	Name  string
	Sig   string
	Where string
	// Doc is the plain description the compiler already carries, which is what
	// diagnostics quote. Gloss is the other voice; see gloss.go.
	Doc   string
	Gloss string
	Group string
	// Search is everything a query is matched against, lower-cased once here
	// rather than on every keystroke.
	Search string
}

// Group is a heading and the verbs under it.
type Group struct {
	Name  string
	Slug  string
	Verbs []Verb
	// Note says what the group is over, in the same voice as a gloss.
	Note string
}

// Word is a keyword or particle: something with no type, because it is not a
// value.
type Word struct {
	Name    string
	Means   string
	Gloss   string
	Search  string
	Section string
}

// Shape is a type constructor and what builds it.
type Shape struct {
	Name   string
	Kind   string
	Gloss  string
	Ctors  []Ctor
	Search string
}

// Ctor is one way to build a Shape.
type Ctor struct {
	Name, Sig, Doc string
}

// Page is everything the template needs.
type Page struct {
	Title  string
	Count  int
	Groups []Group
	Shapes []Shape
	Words  []Word
	Talent []Word
}

// Build assembles the page model from the prelude and the token table, so that
// a verb cannot exist without appearing here and a keyword cannot be added
// without being listed.
func Build() Page {
	p := Page{Title: "Weave"}

	for _, g := range prelude.Groups() {
		grp := Group{Name: g.Name, Slug: anchor(g.Name), Note: groupGlosses[g.Name]}
		for _, e := range g.Entries {
			v := Verb{
				Name:  e.Name,
				Sig:   e.Sig,
				Where: e.Where,
				Doc:   e.Doc,
				Gloss: glossOf(e.Name, e.Group),
				Group: e.Group,
			}
			v.Search = strings.ToLower(strings.Join(
				[]string{v.Name, v.Sig, v.Where, v.Doc, v.Gloss, v.Group}, " "))
			grp.Verbs = append(grp.Verbs, v)
			p.Count++
		}
		p.Groups = append(p.Groups, grp)
	}

	p.Shapes = shapes()
	p.Words = words()
	p.Talent = talents()
	return p
}

// Render writes the page as one self-contained file.
func Render() (string, error) {
	t, err := template.New("page").Funcs(template.FuncMap{
		"sig":   renderSig,
		"prose": renderProse,
	}).Parse(pageTemplate)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	if err := t.Execute(&b, Build()); err != nil {
		return "", err
	}
	return b.String(), nil
}

// renderSig marks up a signature so the types in it stand out from the arrows
// and the brackets. Anything capitalised is a type; anything else is a
// variable standing for one.
func renderSig(s string) template.HTML {
	var b strings.Builder
	word := strings.Builder{}
	flush := func() {
		w := word.String()
		if w == "" {
			return
		}
		word.Reset()
		if c := w[0]; c >= 'A' && c <= 'Z' {
			fmt.Fprintf(&b, `<span class="t">%s</span>`, template.HTMLEscapeString(w))
			return
		}
		fmt.Fprintf(&b, `<span class="v">%s</span>`, template.HTMLEscapeString(w))
	}
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			word.WriteRune(r)
			continue
		}
		flush()
		if r == '-' || r == '>' {
			b.WriteString(template.HTMLEscapeString(string(r)))
			continue
		}
		b.WriteString(template.HTMLEscapeString(string(r)))
	}
	flush()
	return template.HTML(strings.ReplaceAll(b.String(), "-&gt;", `<span class="a">-&gt;</span>`))
}

// renderProse escapes a description and turns its backticked spans into code,
// which is how every description in the prelude already writes a name.
func renderProse(s string) template.HTML {
	var b strings.Builder
	parts := strings.Split(s, "`")
	for i, part := range parts {
		// Odd pieces sit between a pair of backticks. An unclosed one is
		// prose, not code, so the last piece is only marked up when the count
		// worked out.
		if i%2 == 1 && (i+1 < len(parts) || len(parts)%2 == 1) {
			fmt.Fprintf(&b, "<code>%s</code>", template.HTMLEscapeString(part))
			continue
		}
		b.WriteString(template.HTMLEscapeString(part))
	}
	return template.HTML(b.String())
}

func shapes() []Shape {
	byOwner := map[string][]Ctor{}
	order := []string{}
	for _, c := range prelude.Ctors {
		if _, seen := byOwner[c.Owner]; !seen {
			order = append(order, c.Owner)
		}
		byOwner[c.Owner] = append(byOwner[c.Owner], Ctor{c.Name, c.Sig, c.Doc})
	}

	out := make([]Shape, 0, len(shapeGlosses))
	for _, name := range shapeOrder {
		s := Shape{Name: name, Kind: shapeKinds[name], Gloss: shapeGlosses[name], Ctors: byOwner[name]}
		s.Search = strings.ToLower(s.Name + " " + s.Kind + " " + s.Gloss)
		for _, c := range s.Ctors {
			s.Search += " " + strings.ToLower(c.Name+" "+c.Sig+" "+c.Doc)
		}
		out = append(out, s)
	}
	// A constructor whose owner is not in shapeOrder would vanish, so anything
	// unlisted is appended rather than dropped.
	for _, owner := range order {
		if _, listed := shapeGlosses[owner]; listed {
			continue
		}
		out = append(out, Shape{Name: owner, Ctors: byOwner[owner],
			Search: strings.ToLower(owner)})
	}
	return out
}

func words() []Word {
	var out []Word
	for _, w := range keywords {
		w.Search = strings.ToLower(w.Name + " " + w.Means + " " + w.Gloss)
		out = append(out, w)
	}
	return out
}

func talents() []Word {
	out := append([]Word(nil), talentWords...)
	for i := range out {
		out[i].Search = strings.ToLower(out[i].Name + " " + out[i].Means + " " + out[i].Gloss)
	}
	return out
}

// KeywordNames is every word the lexer reserves, which the sync test compares
// against the page's own list so that a keyword cannot be added without being
// documented.
func KeywordNames() []string {
	var out []string
	for _, w := range keywords {
		if w.Section == "Keywords" || w.Section == "Particles" || w.Section == "Holes" {
			out = append(out, w.Name)
		}
	}
	sort.Strings(out)
	return out
}

// LexerKeywords is the same list read out of the token table.
func LexerKeywords() []string {
	var out []string
	for name := range token.Keywords() {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func anchor(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}
