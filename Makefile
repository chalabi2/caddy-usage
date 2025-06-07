# Go parameters
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOLINT=golangci-lint

# Binary name
BINARY_NAME=caddy-usage
BINARY_UNIX=$(BINARY_NAME)_unix

# Build info
VERSION?=$(shell git describe --tags --always --dirty)
COMMIT?=$(shell git rev-parse --short HEAD)
BUILD_TIME?=$(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Ldflags
LDFLAGS=-ldflags "-X main.Version=$(VERSION) -X main.Commit=$(COMMIT) -X main.BuildTime=$(BUILD_TIME)"

.PHONY: all build clean test coverage race lint fmt vet deps help install-tools

## Build
all: test build

build: ## Build the binary
	$(GOBUILD) -o $(BINARY_NAME) -v $(LDFLAGS) ./...

build-linux: ## Build the binary for Linux
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GOBUILD) -o $(BINARY_UNIX) -v $(LDFLAGS) ./...

clean: ## Remove build artifacts
	$(GOCLEAN)
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_UNIX)
	rm -f coverage.out
	rm -f coverage.html

## Testing
test: ## Run unit tests
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-unit: ## Run unit tests only (excluding integration)
	$(GOTEST) -v -race -coverprofile=coverage.out -tags="!integration" ./...

test-all: ## Run all tests including integration
	$(GOTEST) -v -race -coverprofile=coverage.out ./...

test-short: ## Run tests in short mode
	$(GOTEST) -v -short ./...

coverage: test ## Generate coverage report
	$(GOCMD) tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

race: ## Run tests with race detection
	$(GOTEST) -race -short ./...

benchmark: ## Run benchmarks
	$(GOTEST) -bench=. -benchmem ./...

## Code Quality
lint: ## Run linter
	$(GOLINT) run

fmt: ## Format code
	$(GOFMT) -s -w .

vet: ## Run go vet
	$(GOCMD) vet ./...

check: fmt vet lint ## Run all code quality checks

## Dependencies
deps: ## Download dependencies
	$(GOMOD) download

tidy: ## Tidy dependencies
	$(GOMOD) tidy

verify: ## Verify dependencies
	$(GOMOD) verify

update: ## Update dependencies
	$(GOMOD) get -u ./...
	$(GOMOD) tidy

## Caddy Integration
xcaddy-build: ## Build Caddy with this plugin using xcaddy
	xcaddy build --with github.com/chalabi/caddy-usage=.

xcaddy-run: xcaddy-build ## Build and run Caddy with this plugin
	./caddy run --config ./example-configs/Caddyfile

## CI/CD
ci: deps verify check test ## Run all CI checks locally

install-tools: ## Install development tools
	bash -c "go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"
	bash -c "go install github.com/caddyserver/xcaddy/cmd/xcaddy@latest"

## Docker
docker-build: ## Build Docker image
	docker build -t $(BINARY_NAME):$(VERSION) .

docker-run: docker-build ## Build and run Docker container
	docker run --rm -p 8080:8080 $(BINARY_NAME):$(VERSION)

## Release
release-dry-run: ## Test release process
	goreleaser release --snapshot --rm-dist

release: ## Create release
	goreleaser release --rm-dist

## Documentation
docs: ## Generate documentation
	$(GOCMD) doc -all

## Help
help: ## Display this help
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help 