.PHONY: build install test lint fmt clean docker run-http inspector

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
LDFLAGS := -s -w -X github.com/fbsobreira/gotron-mcp/internal/version.Version=$(VERSION) -X github.com/fbsobreira/gotron-mcp/internal/version.Commit=$(COMMIT)

build:
	go build -ldflags "$(LDFLAGS)" -o bin/gotron-mcp ./cmd/gotron-mcp

install:
	go install -ldflags "$(LDFLAGS)" ./cmd/gotron-mcp

test:
	go test -race -shuffle=on ./...

fmt:
	goimports -w .

lint:
	golangci-lint run

clean:
	rm -rf bin/

docker:
	docker build --build-arg VERSION=$(VERSION) --build-arg COMMIT=$(COMMIT) -t gotron-mcp .

run-http:
	@test -f .env && export $$(grep -v '^#' .env | xargs) 2>/dev/null; \
	go run -ldflags "$(LDFLAGS)" ./cmd/gotron-mcp --transport http --bind 0.0.0.0 --port 8080

NETWORK ?= mainnet

inspector: build
	@echo "Starting MCP Inspector on http://localhost:6274 ..."
	@test -f .env && export $$(grep -v '^#' .env | xargs) 2>/dev/null; \
	npx @modelcontextprotocol/inspector ./bin/gotron-mcp -- --network $(NETWORK)
