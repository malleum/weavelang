# weave — development tasks
GO      ?= go
BIN     ?= bin/weave
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: all build test test-full fmt vet lint check examples fmtcheck grammar docs aoc clean

all: build

build:
	$(GO) build -ldflags "-X main.version=$(VERSION)" -o $(BIN) ./cmd/weave

# Everything but the differential suite, which compiles the whole corpus four
# ways and takes half an hour on a slow machine. That one belongs on a push, not
# between two edits; `make test-full` and continuous integration run it.
test:
	$(GO) test -short -timeout 40m ./...

test-full:
	$(GO) test -timeout 90m ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

# Everything CI should run.
check: fmt vet test examples fmtcheck

# Every example must pass `weave check`, and produce the output its fixtures
# say it should. `weave test` is the same loop a program is written in.
examples: build
	@for f in examples/*.weave; do \
		printf '  %-32s' "$$f"; \
		./$(BIN) check "$$f" >/dev/null || exit 1; \
		echo ok; \
	done
	./$(BIN) test examples/*.weave

# Every example must already be in canonical form, and so must the Advent of
# Code solutions — they are the only programs here written to be read rather
# than to show a feature off, so they are the ones worth holding the canonical
# form to.
fmtcheck: build
	./$(BIN) fmt -check examples/*.weave aoc/*/*.weave

# The Advent of Code solutions, checked against the examples in the puzzle text.
# They are not in `examples/` because they are not teaching material: they are
# the language being used in anger, and what keeps them here is that they break
# when a verb changes under them.
aoc: build
	./$(BIN) test aoc/*/*.weave

# Regenerate the tree-sitter parser after a grammar change. src/parser.c is
# committed, so editors need no CLI; this is what keeps it current.
grammar:
	cd tree-sitter-weave && tree-sitter generate && tree-sitter test
	cp tree-sitter-weave/queries/*.scm weave.nvim/queries/weave/

# The vocabulary reference is generated from the prelude, which the compiler
# parses at start-up, so the two cannot drift. A test fails if this is stale.
docs:
	$(GO) run ./cmd/weave verbs -md > docs/verbs.md

clean:
	rm -rf bin
