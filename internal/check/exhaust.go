package check

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/malleum/weave/internal/ast"
	"github.com/malleum/weave/internal/token"
	"github.com/malleum/weave/internal/types"
)

// Exhaustiveness checking follows Maranget, "Warnings for pattern matching"
// (JFP 2007). Patterns are arranged into a matrix, one row per arm, and two
// questions are asked of it:
//
//   - Is a fresh wildcard row still useful? If so the match is missing a case,
//     and the algorithm hands back a witness value that nothing matches.
//   - Is row i useful against rows 0..i-1? If not, that arm can never run.
//
// Both reduce to the same recursion over specialised matrices, so one set of
// helpers answers both.

// tag identifies what a pattern tests for: a constructor, a tuple shape, or a
// literal value. Literals are treated as nullary constructors drawn from an
// unbounded set, which is why a match on Earth always needs a catch-all.
type tag struct {
	name  string
	arity int
	lit   bool
	// thread marks a tag that tests a Thread's length. arity is the number of
	// fixed elements; open means the pattern ended in a rest, so it matches
	// any length at or above that.
	thread bool
	open   bool
}

func (t tag) equal(o tag) bool { return t.name == o.name && t.arity == o.arity }

// covers reports whether a row tested by t also matches a value of shape o.
// For everything but a Thread that is equality; a Thread pattern with a rest
// matches every longer shape too, which is what makes `[]` and `[x ..rest]`
// between them cover the type.
func (t tag) covers(o tag) bool {
	if !t.thread || !o.thread {
		return t.equal(o)
	}
	if t.open {
		return o.arity >= t.arity
	}
	return !o.open && o.arity == t.arity
}

const tupleTag = "(,)"

// threadTag names the test a Thread pattern performs: a length, or a minimum
// length when the pattern ends in a rest.
//
// It is marked lit, like an integer literal, because a Thread's length is not
// drawn from a finite set — so a list of Thread patterns is never exhaustive
// and always needs a `_`, which is the truth. `[..rest]` is the exception: it
// fixes no length at all, so it is a wildcard and tagOf reports nothing for it.
func threadTag(p *ast.PThread) tag {
	if p.Rest != nil {
		return tag{name: fmt.Sprintf("[%d+]", len(p.Elems)), arity: len(p.Elems),
			thread: true, open: true}
	}
	return tag{name: fmt.Sprintf("[%d]", len(p.Elems)), arity: len(p.Elems), thread: true}
}

// isThreadTag reports whether a tag came from a Thread pattern.
func isThreadTag(t tag) bool { return t.thread }

// threadSignature is the constructor list for a Thread column.
//
// A Thread has no finite set of shapes, so there is no signature that depends
// on the type alone — but there is one that depends on the patterns present.
// Split the lengths at the first one no pattern pins down exactly: every
// length below the split is its own shape, and everything at or above it is
// one open shape. `[]` and `[x ..rest]` split at 1 and so between them name
// both shapes, which is why that pair is exhaustive without a `_`.
func threadSignature(present []tag) []tag {
	split := 0
	for _, t := range present {
		if !t.thread {
			return nil
		}
		if !t.open && t.arity >= split {
			split = t.arity + 1
		}
		if t.open && t.arity > split {
			split = t.arity
		}
	}
	out := make([]tag, 0, split+1)
	for n := 0; n < split; n++ {
		out = append(out, tag{name: fmt.Sprintf("[%d]", n), arity: n, thread: true})
	}
	return append(out, tag{name: fmt.Sprintf("[%d+]", split), arity: split,
		thread: true, open: true})
}

// tagOf returns the tag a pattern tests for. Wildcards and variables test for
// nothing, so they report false.
func tagOf(p ast.Pattern) (tag, bool) {
	switch p := p.(type) {
	case *ast.PCtor:
		return tag{name: p.Name, arity: len(p.Args)}, true
	case *ast.PTwine:
		return tag{name: tupleTag, arity: len(p.Elems)}, true
	case *ast.PThread:
		if len(p.Elems) == 0 && p.Rest != nil {
			// `[..rest]` fixes nothing, so it matches every Thread.
			return tag{}, false
		}
		return threadTag(p), true
	case *ast.PInt:
		return tag{name: strconv.FormatInt(p.Value, 10), lit: true}, true
	case *ast.PFloat:
		return tag{name: strconv.FormatFloat(p.Value, 'g', -1, 64), lit: true}, true
	case *ast.PChar:
		return tag{name: "'" + string(p.Value) + "'", lit: true}, true
	case *ast.PText:
		return tag{name: strconv.Quote(p.Value), lit: true}, true
	}
	return tag{}, false
}

// subPatterns returns the patterns nested inside a constructor pattern.
func subPatterns(p ast.Pattern) []ast.Pattern {
	switch p := p.(type) {
	case *ast.PCtor:
		return p.Args
	case *ast.PTwine:
		return p.Elems
	case *ast.PThread:
		// The rest is a binding, not a test: it always matches whatever is
		// left, so it contributes no column.
		return p.Elems
	}
	return nil
}

// wildcards builds a row of n wildcard patterns.
func wildcards(n int) []ast.Pattern {
	out := make([]ast.Pattern, n)
	for i := range out {
		out[i] = &ast.PWild{}
	}
	return out
}

// specialize keeps the rows that can match tg, replacing each row's first
// pattern with that constructor's fields.
func specialize(tg tag, matrix [][]ast.Pattern) [][]ast.Pattern {
	var out [][]ast.Pattern
	for _, row := range matrix {
		if len(row) == 0 {
			continue
		}
		head, rest := row[0], row[1:]
		rowTag, isCtor := tagOf(head)
		switch {
		case !isCtor: // wildcard matches anything
			out = append(out, append(wildcards(tg.arity), rest...))
		case rowTag.covers(tg):
			// `[x ..r]` against a three-element shape is `[x _ _]`: the rest
			// absorbs the columns the pattern did not name.
			fields := append([]ast.Pattern{}, subPatterns(head)...)
			for len(fields) < tg.arity {
				fields = append(fields, &ast.PWild{})
			}
			out = append(out, append(fields[:tg.arity:tg.arity], rest...))
		}
	}
	return out
}

// defaultMatrix keeps only the rows whose first pattern is a wildcard, with
// that column dropped.
func defaultMatrix(matrix [][]ast.Pattern) [][]ast.Pattern {
	var out [][]ast.Pattern
	for _, row := range matrix {
		if len(row) == 0 {
			continue
		}
		if _, isCtor := tagOf(row[0]); !isCtor {
			out = append(out, row[1:])
		}
	}
	return out
}

// columnTags collects the distinct tags tested in the first column.
func columnTags(matrix [][]ast.Pattern) []tag {
	var out []tag
	for _, row := range matrix {
		if len(row) == 0 {
			continue
		}
		tg, isCtor := tagOf(row[0])
		if !isCtor {
			continue
		}
		found := false
		for _, seen := range out {
			if seen.equal(tg) {
				found = true
				break
			}
		}
		if !found {
			out = append(out, tg)
		}
	}
	return out
}

// signature returns every tag a value of type t can have, and whether that set
// is finite. Types with unbounded value sets (Earth, Air, Thread, an
// unresolved variable) report false, so they always need a catch-all.
func (c *checker) signature(t types.Type) ([]tag, bool) {
	con, ok := types.Resolve(t).(*types.Con)
	if !ok {
		return nil, false
	}
	if con.Name == types.TwineCon {
		return []tag{{name: tupleTag, arity: len(con.Args)}}, true
	}
	names, ok := c.typeCtors[con.Name]
	if !ok {
		return nil, false
	}
	out := make([]tag, 0, len(names))
	for _, name := range names {
		info, ok := c.ctors[name]
		if !ok {
			return nil, false
		}
		out = append(out, tag{name: name, arity: info.Arity})
	}
	return out, true
}

// fieldTypes returns the types of the values a tag carries, given the type of
// the column it appears in.
func (c *checker) fieldTypes(tg tag, colType types.Type) []types.Type {
	if isThreadTag(tg) {
		// Every element of a Thread has the same type.
		if con, ok := types.Resolve(colType).(*types.Con); ok &&
			con.Name == types.ThreadCon && len(con.Args) == 1 {
			out := make([]types.Type, tg.arity)
			for i := range out {
				out[i] = con.Args[0]
			}
			return out
		}
		return freshTypes(c, tg.arity)
	}
	if tg.name == tupleTag {
		if con, ok := types.Resolve(colType).(*types.Con); ok && con.Name == types.TwineCon {
			return con.Args
		}
		return freshTypes(c, tg.arity)
	}
	info, ok := c.ctors[tg.name]
	if !ok || tg.arity == 0 {
		return nil
	}
	// Instantiate the constructor and unify its result with the column type,
	// which resolves its field types: matching `Held n` against `Hold Earth`
	// tells us `n` is an Earth.
	t := c.alloc.Instantiate(info.Scheme, c.level)
	fields := make([]types.Type, 0, tg.arity)
	for i := 0; i < tg.arity; i++ {
		fn, ok := types.Resolve(t).(*types.Fn)
		if !ok {
			break
		}
		fields = append(fields, fn.From)
		t = fn.To
	}
	_ = types.Unify(t, colType) // best effort: only shapes the witness text

	// A pattern whose arity disagrees with its constructor has already been
	// reported. Pad here so the matrix stays rectangular and the analysis can
	// finish instead of panicking on a ragged row.
	if len(fields) < tg.arity {
		fields = append(fields, freshTypes(c, tg.arity-len(fields))...)
	}
	return fields
}

func freshTypes(c *checker, n int) []types.Type {
	out := make([]types.Type, n)
	for i := range out {
		out[i] = c.alloc.Fresh(c.level)
	}
	return out
}

// ------------------------------------------------------------- missing cases

// missingCase looks for a value that no row of the matrix matches. The witness
// is returned as one rendered pattern per column.
func (c *checker) missingCase(matrix [][]ast.Pattern, colTypes []types.Type) ([]string, bool) {
	if len(colTypes) == 0 {
		// An empty matrix matches nothing, so the empty value is a witness.
		return nil, len(matrix) == 0
	}

	present := columnTags(matrix)
	all, finite := c.signature(colTypes[0])
	if len(present) > 0 && present[0].thread {
		all, finite = threadSignature(present), true
	}

	complete := finite && len(present) > 0
	if complete {
		for _, want := range all {
			found := false
			for _, have := range present {
				if have.covers(want) {
					found = true
					break
				}
			}
			if !found {
				complete = false
				break
			}
		}
	}

	if complete {
		// Every constructor is tested, so any gap must be inside one of them.
		for _, tg := range all {
			fields := c.fieldTypes(tg, colTypes[0])
			cols := append(append([]types.Type{}, fields...), colTypes[1:]...)
			witness, ok := c.missingCase(specialize(tg, matrix), cols)
			if !ok {
				continue
			}
			return append([]string{renderTag(tg, witness[:tg.arity])}, witness[tg.arity:]...), true
		}
		return nil, false
	}

	// The column is open: recurse on the rows that ignore it.
	witness, ok := c.missingCase(defaultMatrix(matrix), colTypes[1:])
	if !ok {
		return nil, false
	}
	return append([]string{c.uncoveredTag(colTypes[0], present, all, finite)}, witness...), true
}

// uncoveredTag names a value of the column type that the matrix does not test
// for, used to describe the missing case.
func (c *checker) uncoveredTag(t types.Type, present, all []tag, finite bool) string {
	if finite {
		for _, want := range all {
			found := false
			for _, have := range present {
				if have.equal(want) {
					found = true
					break
				}
			}
			if !found {
				return renderTag(want, wildcardStrings(want.arity))
			}
		}
	}
	return "_"
}

func wildcardStrings(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "_"
	}
	return out
}

func renderTag(tg tag, args []string) string {
	if tg.name == tupleTag {
		return "(" + strings.Join(args, ", ") + ")"
	}
	if isThreadTag(tg) {
		if strings.HasSuffix(tg.name, "+]") {
			return "[" + strings.Join(append(args, ".."), " ") + "]"
		}
		return "[" + strings.Join(args, " ") + "]"
	}
	if len(args) == 0 {
		return tg.name
	}
	return tg.name + " " + strings.Join(args, " ")
}

// ---------------------------------------------------------------- usefulness

// useful reports whether row can match a value that no row of matrix matches.
// A `ward` arm that is not useful can never run.
func (c *checker) useful(matrix [][]ast.Pattern, row []ast.Pattern, colTypes []types.Type) bool {
	if len(colTypes) == 0 || len(row) == 0 {
		return len(matrix) == 0
	}

	head := row[0]
	if tg, isCtor := tagOf(head); isCtor {
		fields := c.fieldTypes(tg, colTypes[0])
		next := append(append([]ast.Pattern{}, subPatterns(head)...), row[1:]...)
		return c.useful(specialize(tg, matrix),
			next, append(append([]types.Type{}, fields...), colTypes[1:]...))
	}

	present := columnTags(matrix)
	all, finite := c.signature(colTypes[0])
	if len(present) > 0 && present[0].thread {
		all, finite = threadSignature(present), true
	}
	complete := finite && len(present) >= len(all)
	if complete {
		for _, want := range all {
			found := false
			for _, have := range present {
				if have.covers(want) {
					found = true
					break
				}
			}
			if !found {
				complete = false
				break
			}
		}
	}

	if complete {
		for _, tg := range all {
			fields := c.fieldTypes(tg, colTypes[0])
			next := append(wildcards(tg.arity), row[1:]...)
			if c.useful(specialize(tg, matrix), next,
				append(append([]types.Type{}, fields...), colTypes[1:]...)) {
				return true
			}
		}
		return false
	}
	return c.useful(defaultMatrix(matrix), row[1:], colTypes[1:])
}

// ------------------------------------------------------------------ reports

// checkWardCoverage reports missing cases and unreachable arms for a ward.
func (c *checker) checkWardCoverage(w *ast.Ward, subject types.Type) {
	var matrix [][]ast.Pattern
	for _, arm := range w.Arms {
		row := []ast.Pattern{arm.Pat}
		if !c.useful(matrix, row, []types.Type{subject}) {
			c.bag.AddHint(arm.Pos(), "an earlier arm already covers it",
				"this arm can never match")
		}
		matrix = append(matrix, row)
	}

	witness, missing := c.missingCase(matrix, []types.Type{subject})
	if !missing {
		return
	}
	c.reportMissing(w.P, "ward", witness)
}

// checkClauseCoverage applies the same analysis to a multi-clause definition,
// whose parameter patterns form the matrix.
func (c *checker) checkClauseCoverage(d *ast.Decl, self types.Type) {
	colTypes := paramTypes(self, d.Arity())
	if len(colTypes) != d.Arity() {
		return // an earlier error left the type unknown
	}

	var matrix [][]ast.Pattern
	for _, cl := range d.Clauses {
		row := append([]ast.Pattern{}, cl.Params...)
		if !c.useful(matrix, row, colTypes) {
			c.bag.AddHint(cl.Pos(), "an earlier clause already covers it",
				"this clause of `%s` can never match", d.Name)
		}
		matrix = append(matrix, row)
	}

	witness, missing := c.missingCase(matrix, colTypes)
	if !missing {
		return
	}
	c.reportMissing(d.NamePos, "`"+d.Name+"`", witness)
}

// A missing case is reported softly: the generated code traps on it rather
// than doing something undefined, so the compiler could proceed. It does not,
// for a program — a program that stops halfway is not what anyone wanted. The
// REPL is the exception, since a definition being entered a clause at a time
// is incomplete in between. See diag.Diagnostic.Soft.
func (c *checker) reportMissing(pos token.Pos, what string, witness []string) {
	rendered := strings.Join(witness, " ")
	if strings.Trim(rendered, "_ ") == "" {
		c.bag.AddSoft(pos, "add a `_` arm to cover the rest",
			"%s does not handle every case", what)
		return
	}
	c.bag.AddSoft(pos, fmt.Sprintf("add an arm for `%s`", rendered),
		"%s does not handle every case: `%s` is unmatched", what, rendered)
}

// paramTypes peels n argument types off a function type.
func paramTypes(t types.Type, n int) []types.Type {
	out := make([]types.Type, 0, n)
	for i := 0; i < n; i++ {
		fn, ok := types.Resolve(t).(*types.Fn)
		if !ok {
			return out
		}
		out = append(out, fn.From)
		t = fn.To
	}
	return out
}
