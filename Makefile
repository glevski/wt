# Go is not on PATH in this devcontainer, hence the explicit toolchain path.
GO     ?= /usr/local/go/bin/go
PREFIX ?= $(HOME)/.local

# Stamped into `wt --version`: the latest tag (with -N-g<sha> when ahead of
# it, per git describe) plus the exact commit. `?=` so a caller building
# outside a checkout — e.g. from a `git archive` tarball, which carries no
# .git — can supply them instead, via the environment or the make command
# line: make install VERSION=0.0.11 COMMIT=7af004a
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null)
STAMP   := -X wt/internal/cli.version=$(VERSION) -X wt/internal/cli.commit=$(COMMIT)

# Go's own vcs.revision fallback is unavailable for the same reason git is, so
# an unstamped build silently degrades to `wt dev`. Say so rather than ship it
# quietly.
ifeq ($(strip $(VERSION)$(COMMIT)),)
$(warning no git metadata in $(CURDIR) — building unstamped, `wt --version` will report "dev"; pass VERSION=… COMMIT=… to stamp it)
endif

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
