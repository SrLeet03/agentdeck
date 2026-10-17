PREFIX ?= $(HOME)/.local/bin

.PHONY: build install test clean

build:
	go build -o deck ./cmd/deck

install:
	go build -o $(PREFIX)/deck ./cmd/deck
	@echo "installed $(PREFIX)/deck"

test:
	go vet ./...
	go test ./...

clean:
	rm -f deck
