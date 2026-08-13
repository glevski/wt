# Go is not on PATH in this devcontainer, hence the explicit toolchain path.
GO     ?= /usr/local/go/bin/go
PREFIX ?= $(HOME)/.local

# Stamped into `wt --version`: the latest tag (with -N-g<sha> when ahead of
# it, per git describe) plus the exact commit.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null)
COMMIT  := $(shell git rev-parse --short HEAD 2>/dev/null)
STAMP   := -X wt/internal/cli.version=$(VERSION) -X wt/internal/cli.commit=$(COMMIT)

.PHONY: build install test vet fmt

build:
	$(GO) build -ldflags="$(STAMP)" -o worktree .

install:
	$(GO) build -ldflags="-s -w $(STAMP)" -o $(PREFIX)/bin/worktree .

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...
