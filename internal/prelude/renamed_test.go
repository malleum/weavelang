package prelude_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/malleum/weave/internal/prelude"
)

// Every name a verb has ever had, and what it is called now.
//
// The documentation kept drifting in exactly one way: a verb was renamed, the
// code and the tests followed, and the prose did not — so SPEC.md still
// described `seek`, `size` and `product` long after they were gone, and a
// reader following it wrote programs that did not compile. Nothing catches
// that, because stale prose still reads perfectly well.
//
// So the ledger is the check. Once a name is retired it is retired everywhere:
// no document and no program may mention it again. Renaming a verb means
// adding a line here, which then tells you every place left to fix.
var renamed = map[string]string{
	"size":       "len",
	"product":    "prod",
	"find":       "seek",
	"ints":       "earths",
	"chars":      "fires",
	"string":     "air",
	"char":       "spark",
	"grid":       "pattern",
	"parse":      "earth, water or fire",
	"neighbours": "nb4",
	"neighbors":  "nb4",
	"neighbors8": "nb8",
	"tally":      "freq",
	"shed":       "remove",
}

// A retired name only counts when it is written as code — inside backticks in
// Markdown. That keeps English out of it: a document may say it will "parse"
// something or talk about "a string of digits" without tripping this.
func TestRetiredNamesAreGoneFromTheDocumentation(t *testing.T) {
	for _, path := range docFiles(t) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(path)
		for _, span := range inlineCode(string(src)) {
			for _, word := range regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`).FindAllString(span, -1) {
				if now, gone := renamed[word]; gone {
					t.Errorf("%s writes `%s`, which is now `%s`", name, word, now)
				}
			}
		}
	}
}

// The same ledger over the programs that ship with the compiler. These are
// compiled by the test suite, so a retired name would already fail — but the
// message here says what to write instead.
func TestRetiredNamesAreGoneFromThePrograms(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("..", "..", "examples", "*.weave"))
	if err != nil {
		t.Fatal(err)
	}
	more, _ := filepath.Glob(filepath.Join("..", "..", "bench", "weave", "*.weave"))
	deeper, _ := filepath.Glob(filepath.Join("..", "..", "bench", "weave", "raw", "*.weave"))
	paths = append(append(paths, more...), deeper...)

	word := regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)
	for _, path := range paths {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, line := range strings.Split(string(src), "\n") {
			if i := strings.IndexByte(line, '#'); i >= 0 {
				line = line[:i]
			}
			for _, w := range word.FindAllString(line, -1) {
				if now, gone := renamed[w]; gone {
					t.Errorf("%s uses `%s`, which is now `%s`", filepath.Base(path), w, now)
				}
			}
		}
	}
}

// A retired name must not come back as a different verb either, which would
// make every document written before the rename quietly wrong.
func TestRetiredNamesAreNotReused(t *testing.T) {
	for _, e := range prelude.Values {
		if now, gone := renamed[e.Name]; gone {
			t.Errorf("the prelude declares `%s`, which the ledger retires in favour of `%s`",
				e.Name, now)
		}
	}
}

// docFiles is every document that describes the language.
func docFiles(t *testing.T) []string {
	t.Helper()
	root := filepath.Join("..", "..")
	paths, err := filepath.Glob(filepath.Join(root, "docs", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	// TODO.md is deliberately absent: it records what was renamed, so it is
	// the one document that has to be able to write the old names.
	return append(paths,
		filepath.Join(root, "SPEC.md"),
		filepath.Join(root, "README.md"),
	)
}

// uncomment drops a Weave line's trailing `# ...`, which is prose and not
// code. It is what lets a comment say a verb is bound by "it" without the
// ledger reading that as the retired spelling of `this`.
func uncomment(line string) string {
	quoted := false
	for i, r := range line {
		switch r {
		case '"':
			quoted = !quoted
		case '#':
			if !quoted {
				return line[:i]
			}
		}
	}
	return line
}

// cli matches an invocation of the compiler. `weave parse file.weave` is a
// subcommand, not the retired verb, so those lines are not searched.
var cli = regexp.MustCompile(`(\bweave\s+|^\s\s)(run|build|check|fmt|repl|trace|verbs|lsp|version|parse|lex)\b`)

// inlineCode returns the contents of every backticked span and every fenced
// block of Weave, which together are the parts of a document that claim to be
// Weave code. A fence in another language is somebody else's vocabulary — the
// Nix and Lua in docs/nixvim.md have their own words, and a retired Weave verb
// spelled the same way there means nothing.
func inlineCode(doc string) []string {
	var out []string
	lines := strings.Split(doc, "\n")
	inFence, weaveFence := false, false
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if !inFence {
				lang := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "```"))
				weaveFence = lang == "" || lang == "weave"
			}
			inFence = !inFence
			continue
		}
		if inFence {
			if weaveFence && !cli.MatchString(line) {
				out = append(out, uncomment(line))
			}
			continue
		}
		// Inline spans, longest delimiter first so ``…`` is not mistaken for
		// two empty spans.
		for _, m := range regexp.MustCompile("``([^`]*)``|`([^`]+)`").FindAllStringSubmatch(line, -1) {
			span := m[2]
			if m[1] != "" {
				span = m[1]
			}
			if !cli.MatchString(span) {
				out = append(out, span)
			}
		}
	}
	return out
}

// The documentation quotes how many verbs there are, in four places, and the
// number went stale the first time one was added. This is the cheapest way to
// keep a count honest: fail when the prose disagrees with the table.
func TestTheDocumentedVerbCountIsRight(t *testing.T) {
	want := regexp.MustCompile(`\b(\d{2,4}) (?:verbs|built-ins|of them)\b`)
	count := len(prelude.Values)
	for _, path := range docFiles(t) {
		src, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range want.FindAllStringSubmatch(string(src), -1) {
			if m[1] != strconv.Itoa(count) {
				t.Errorf("%s says %s, but the prelude declares %d",
					filepath.Base(path), m[0], count)
			}
		}
	}
}
