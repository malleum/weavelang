# weave — development tasks
GO      ?= go
BIN     ?= bin/weave
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)

.PHONY: all build test fmt vet lint check examples fmtcheck grammar docs clean

all: build

build:
	$(GO) build -ldflags "-X main.version=$(VERSION)" -o $(BIN) ./cmd/weave

test:
	$(GO) test ./...

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

# Every example must already be in canonical form.
fmtcheck: build
	./$(BIN) fmt -check examples/*.weave

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
