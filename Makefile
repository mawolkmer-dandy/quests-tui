BINARY := quests

# Version stamped into the binary (see cmd/quests/main.go). Prefer the exact
# git tag, falling back to a `describe` string, then "dev" outside a repo.
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build install run clean check

build:
	go build -ldflags "$(LDFLAGS)" -o $(BINARY) ./cmd/quests

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/quests

run: build
	./$(BINARY)

check:
	gofmt -l . && go vet ./... && go build ./...

clean:
	rm -f $(BINARY)
