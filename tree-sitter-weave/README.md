# tree-sitter-weave

The [Weave](../README.md) grammar for tree-sitter: syntax highlighting,
structural selection, folding, and ` ```weave ` blocks inside Markdown — which
is the one thing the language server cannot do, since it only ever sees whole
`.weave` files.

## What is here

```
grammar.js            the grammar
src/scanner.c         the layout algorithm: newline, indent, dedent
src/parser.c          generated; committed so the CLI is not needed to build
queries/              highlights and injections
test/corpus/          parse tests, run by `tree-sitter test`
grammar_test.go       `go test` runs the corpus and every example
```

## Layout

Weave is indentation-sensitive in an unusual way: **indentation opens a block
only after something that wants one** — `is`, a ward arm's `:`, or the `ward`
line itself. Everywhere else a deeper line continues the line above, which is
what lets an application span lines:

```
answer is
  pick (member seen k)
    (walk rest seen)
    (walk (push rest k) (insert seen k))
```

The compiler's lexer decides this by remembering the last token of the previous
line. The scanner here does not have to: the grammar answers it directly, since
a block can begin exactly where `_indent` is a valid symbol. That leaves no
state to drift out of step with the parser, and it is why `src/scanner.c` is
about a hundred lines rather than a reimplementation of `internal/lexer`.

A line opening with `|`, `where`, `as` or `through` continues the line above
too, so a long pipeline can breathe. The scanner looks ahead for those before
deciding to end a line.

## One deliberate difference from the compiler

`[1 2 3]` is three elements and `[Step North 3, Rest]` is two: the compiler
decides on whether a comma appears at the top level of the brackets. An LR
automaton cannot make that distinction — a run of atoms and an application are
the same item set — so the grammar admits both readings and lets tree-sitter's
dynamic precedence prefer elements. The consequence is that `[a b]` may be
reported as an application of `a` to `b`; nothing else observes it.

## Working on it

```sh
tree-sitter generate      # after editing grammar.js or scanner.c
tree-sitter test          # the corpus in test/corpus
tree-sitter parse FILE    # the tree for one program
tree-sitter highlight FILE
```

Regenerate and commit `src/parser.c` with any grammar change: it is the
artefact editors actually build.
