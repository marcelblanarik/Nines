VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: build test vet lint fmt golden fuzz wasm web clean

build:
	CGO_ENABLED=0 go build -ldflags "$(LDFLAGS)" -o bin/nines ./cmd/nines

vet:
	go vet ./...
	GOOS=js GOARCH=wasm go vet ./cmd/wasm

test: vet
	go test -race ./...

lint:
	golangci-lint run

fmt:
	gofmt -w cmd internal

# Rewrite testdata/golden after an intended output change, then review the diff.
golden:
	go test ./cmd/nines -update

fuzz:
	go test ./internal/model -run=^$$ -fuzz=FuzzParse -fuzztime=30s

# The WASM core and the files the web app loads next to it.
wasm:
	mkdir -p web/public
	CGO_ENABLED=0 GOOS=js GOARCH=wasm go build -ldflags "-s -w" -o web/public/nines.wasm ./cmd/wasm
	cp "$$(go env GOROOT)/lib/wasm/wasm_exec.js" web/public/wasm_exec.js

web: wasm
	cd web && npm ci && npm run build

clean:
	rm -rf bin dist web/dist web/public/nines.wasm web/public/wasm_exec.js
