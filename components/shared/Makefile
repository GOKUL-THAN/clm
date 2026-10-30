.PHONY: build build-server build-satellite build-cli clean test lint run-server run-satellite docker-build

# Variables
BINARY_DIR=bin
GO=go
GOFLAGS=-ldflags="-s -w"

# Build all binaries
build: build-server build-satellite build-cli

build-server:
	$(GO) build $(GOFLAGS) -o $(BINARY_DIR)/clm-server ./cmd/clm-server

build-satellite:
	$(GO) build $(GOFLAGS) -o $(BINARY_DIR)/clm-satellite ./cmd/clm-satellite

build-cli:
	$(GO) build $(GOFLAGS) -o $(BINARY_DIR)/clm ./cmd/clm-cli

# Run
run-server:
	$(GO) run ./cmd/clm-server

run-satellite:
	$(GO) run ./cmd/clm-satellite

# Test
test:
	$(GO) test ./... -v -cover

test-integration:
	$(GO) test ./tests/integration/... -v -tags=integration

# Lint
lint:
	golangci-lint run ./...

# Clean
clean:
	rm -rf $(BINARY_DIR)

# Docker
docker-build:
	docker-compose -f deploy/docker/docker-compose.yml build

docker-up:
	docker-compose -f deploy/docker/docker-compose.yml up -d

docker-down:
	docker-compose -f deploy/docker/docker-compose.yml down

# Database migrations
migrate-up:
	$(GO) run ./scripts/migrate.go up

migrate-down:
	$(GO) run ./scripts/migrate.go down

# Proto generation
proto:
	protoc --go_out=. --go-grpc_out=. internal/api/grpc/proto/*.proto
