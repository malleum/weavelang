// Package mdblock finds Weave programs inside a Markdown document.
//
// Two things need this and they used to disagree. The documentation test pulls
// every ```weave block out of docs/ and compiles it, so an example that has
// stopped working fails the build. The language server needs the same blocks
// for a different reason: a Markdown file is not a Weave file, but the code in
// it is, and hovering a verb inside a fence should work the same way it does in
// a .weave file.
//
// So the extraction lives here, once, and both read it.
package mdblock

import "strings"

// Block is one fenced Weave program, and the plain fence that follows it if
// there was one.
type Block struct {
	// Line is the zero-based line of the block's first line of code, which is
	// what turns a position inside the block back into a position in the
	// document.
	Line int
	// Src is the program, newline-terminated.
	Src string
	// Want is the fenced output that followed, when a block was written with
	// one. A block shown without its output is only required to compile.
	Want    string
	HasWant bool
	// Fragment marks a block fenced ```weave-part rather than ```weave: an
	// illustration rather than a program, written with names it never defines.
	// The spec is full of them — `xs | braid (acc x : add acc x) 0` says what a
	// lambda looks like and has no business having an `xs`.
	//
	// A fragment is still parsed, so its *syntax* cannot rot; it is not checked
	// or run, because there is nothing there to run. Editors treat it exactly as
	// they treat any other block.
	Fragment bool
}

// End is the line after the block's last line of code.
func (b Block) End() int { return b.Line + strings.Count(b.Src, "\n") }

// Contains reports whether a zero-based document line falls inside the block.
func (b Block) Contains(line int) bool { return line >= b.Line && line < b.End() }

// Blocks returns every ```weave and ```weave-part block in the document, in
// order.
func Blocks(doc string) []Block {
	var out []Block
	lines := strings.Split(doc, "\n")

	for i := 0; i < len(lines); i++ {
		fence := strings.TrimSpace(lines[i])
		if fence != "```weave" && fence != "```weave-part" {
			continue
		}
		fragment := fence == "```weave-part"
		start := i + 1
		end := start
		for end < len(lines) && strings.TrimSpace(lines[end]) != "```" {
			end++
		}
		if end >= len(lines) {
			break
		}
		b := Block{Line: start, Src: strings.Join(lines[start:end], "\n") + "\n", Fragment: fragment}

		// A bare ``` fence within the next few lines is the expected output.
		for j := end + 1; j < len(lines) && j <= end+3; j++ {
			if strings.TrimSpace(lines[j]) == "" {
				continue
			}
			if strings.TrimSpace(lines[j]) != "```" {
				break
			}
			k := j + 1
			for k < len(lines) && strings.TrimSpace(lines[k]) != "```" {
				k++
			}
			b.Want = strings.Join(lines[j+1:k], "\n")
			b.HasWant = true
			i = k
			break
		}
		out = append(out, b)
		if !b.HasWant {
			i = end
		}
	}
	return out
}

// At returns the block containing a zero-based document line.
func At(blocks []Block, line int) (Block, bool) {
	for _, b := range blocks {
		if b.Contains(line) {
			return b, true
		}
	}
	return Block{}, false
}

// IsMarkdown reports whether a document URI names a Markdown file, which is the
// only thing the server looks at to decide how to read it. A language id would
// do as well, but editors disagree about what they send and the extension does
// not.
func IsMarkdown(uri string) bool {
	for _, ext := range []string{".md", ".markdown", ".mdx"} {
		if strings.HasSuffix(uri, ext) {
			return true
		}
	}
	return false
}
