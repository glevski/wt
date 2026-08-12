# Go is not on PATH in this devcontainer, hence the explicit toolchain path.
GO     ?= /usr/local/go/bin/go
PREFIX ?= $(HOME)/.local

.PHONY: build install test vet fmt

build:
	$(GO) build -o worktree .

install:
	$(GO) build -ldflags="-s -w" -o $(PREFIX)/bin/worktree .

test:
	$(GO) test ./...

vet:
	$(GO) vet ./...

fmt:
	$(GO) fmt ./...
