BINARY := quests

.PHONY: build install run clean check

build:
	go build -o $(BINARY) ./cmd/quests

install:
	go install ./cmd/quests

run: build
	./$(BINARY)

check:
	gofmt -l . && go vet ./... && go build ./...

clean:
	rm -f $(BINARY)
