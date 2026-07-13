BINARY := quests

# Version stamped into the binary (see cmd/quests/main.go). Prefer the exact
# git tag, falling back to a `describe` string, then "dev" outside a repo.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build install dev run clean check

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/quests

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/quests

# `make dev` installs the current source as a `quests-dev` command on your PATH
# (Homebrew's bin dir is already on PATH), kept separate from the released
# `quests` so a dev build never shadows or clobbers the brew install. Anyone
# working on Quests can `make dev` then run `quests-dev` from anywhere; re-run
# `make dev` after changes. `quests-dev --version` shows the git describe stamp
# so you can tell it apart from the released `quests`.
DEV_NAME := quests-dev
DEV_DIR := $(shell brew --prefix 2>/dev/null)/bin

dev:
	@test -d "$(DEV_DIR)" || { echo "brew bin dir not found ($(DEV_DIR)); set DEV_DIR=..."; exit 1; }
	go build -ldflags "$(LDFLAGS)" -o "$(DEV_DIR)/$(DEV_NAME)" ./cmd/quests
	@echo "dev build → $(DEV_DIR)/$(DEV_NAME)   (run it anywhere with: $(DEV_NAME))"

run: build
	./$(BINARY)

check:
	gofmt -l . && go vet ./... && go build ./...

clean:
	rm -f $(BINARY)
