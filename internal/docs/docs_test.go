package docs

import (
	"regexp"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/prelude"
)

// TestEveryVerbIsOnThePage is the anti-rot rule for this file: the page is
// built from the prelude, so a verb that exists must appear, and one that has
// been renamed cannot linger.
func TestEveryVerbIsOnThePage(t *testing.T) {
	page, err := Render()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range prelude.Values {
		if !strings.Contains(page, `id="v-`+e.Name+`"`) {
			t.Errorf("`%s` is in the prelude but not on the page", e.Name)
		}
	}
	for _, c := range prelude.Ctors {
		if !strings.Contains(page, "<b>"+c.Name+"</b>") {
			t.Errorf("the constructor `%s` is not on the page", c.Name)
		}
	}
}

// TestEveryVerbHasAGloss keeps the second voice from thinning out. A verb
// falling through to its group's gloss is allowed — that is what the fallback
// is for — but it should be a deliberate choice rather than a hundred of them.
func TestEveryVerbHasAGloss(t *testing.T) {
	missing := 0
	for _, e := range prelude.Values {
		if _, ok := glosses[e.Name]; !ok {
			t.Logf("`%s` falls back to its group's gloss", e.Name)
			missing++
		}
	}
	if missing > 5 {
		t.Errorf("%d verbs have no gloss of their own; write them", missing)
	}
}

// TestEveryGroupHasANote catches a new prelude group arriving without one,
// which would leave a heading with nothing under it.
func TestEveryGroupHasANote(t *testing.T) {
	for _, g := range prelude.Groups() {
		if groupGlosses[g.Name] == "" {
			t.Errorf("the group %q has no note", g.Name)
		}
	}
}

// TestKeywordsAreDocumented walks the lexer's own table, so a word cannot be
// reserved without turning up on the page and a word cannot be listed here
// after the lexer has stopped reserving it.
func TestKeywordsAreDocumented(t *testing.T) {
	listed := map[string]bool{}
	for _, name := range KeywordNames() {
		listed[name] = true
	}
	for _, name := range LexerKeywords() {
		if !listed[name] {
			t.Errorf("the lexer reserves `%s`, which the page does not mention", name)
		}
	}
	// `_` is a symbol rather than a word, so it is documented without being in
	// the keyword table. Everything else listed must be reserved.
	reserved := map[string]bool{"_": true}
	for _, name := range LexerKeywords() {
		reserved[name] = true
	}
	for name := range listed {
		if !reserved[name] {
			t.Errorf("the page documents `%s`, which the lexer does not reserve", name)
		}
	}
}

// TestThePageFetchesNothing is what makes it work with the network off, and
// keep working when whatever it would have fetched stops existing.
func TestThePageFetchesNothing(t *testing.T) {
	page, err := Render()
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"http://", "https://", "<script src", "<link rel=\"stylesheet\"", "@import"} {
		if strings.Contains(page, bad) {
			t.Errorf("the page reaches outside itself: %q", bad)
		}
	}
}

// TestTheCameosAreEachThereOnce guards eight glosses that say two or three of
// their words in a language that is not English. They are meant to be found by
// someone reading, which means exactly one of each: a second would look like a
// pattern, and none at all would mean somebody tidied.
func TestTheCameosAreEachThereOnce(t *testing.T) {
	cameos := map[string]string{
		"Sømmen synes ikke":                    "weld",
		"unua okazo":                           "uniq",
		"רווח":                                 "strip",
		"ο κύκλος":                             "pi",
		"Ú-veth":                               "flow",
		"ghobe'":                               "none",
		"Crescit ut necesse est":               "perms",
		"an Air of 31 and 30":                  "unbase",
		"there is none through the death gate": "route",
	}
	all := strings.Join(allGlosses(), "\n")
	for phrase, verb := range cameos {
		if n := strings.Count(all, phrase); n != 1 {
			t.Errorf("%q should appear once, on `%s`; it appears %d times", phrase, verb, n)
		}
	}
}

// TestGlossesCompareOnlyToWeave keeps the reference's one rule. Everything a
// verb is explained by has to be something else in this language.
func TestGlossesCompareOnlyToWeave(t *testing.T) {
	elsewhere := regexp.MustCompile(`(?i)\b(haskell|python|clojure|rust|javascript|golang|scala|ocaml|elm|erlang|ruby|lisp|scheme|fold|filter|monad|lambda|tuple|array|hashmap|dictionary)\b`)
	for name, g := range glosses {
		if m := elsewhere.FindString(g); m != "" {
			t.Errorf("`%s` reaches outside the language: %q", name, m)
		}
	}
	for name, g := range shapeGlosses {
		if m := elsewhere.FindString(g); m != "" {
			t.Errorf("the type %s reaches outside the language: %q", name, m)
		}
	}
}

func allGlosses() []string {
	var out []string
	for _, g := range glosses {
		out = append(out, g)
	}
	for _, g := range groupGlosses {
		out = append(out, g)
	}
	for _, g := range shapeGlosses {
		out = append(out, g)
	}
	for _, w := range keywords {
		out = append(out, w.Means, w.Gloss)
	}
	for _, w := range talentWords {
		out = append(out, w.Means, w.Gloss)
	}
	return out
}
