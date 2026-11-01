BINARY_NAME := lazyssh
BUILD_DIR := bin
CMD_PATH := ./cmd/lazyssh
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
DATE ?= $(shell date -u +"%Y-%m-%dT%H:%M:%SZ")

LDFLAGS := -s -w \
	-X 'main.version=$(VERSION)' \
	-X 'main.commit=$(COMMIT)' \
	-X 'main.date=$(DATE)'

.PHONY: all build clean run test test-coverage fmt lint check install help

all: build

help: ## Show this help message
	@echo "Usage: make [target]"
	@echo ""
	@echo "Targets:"
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the lazyssh binary
	@mkdir -p $(BUILD_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BUILD_DIR)/$(BINARY_NAME) $(CMD_PATH)
	@echo "✅ Built $(BUILD_DIR)/$(BINARY_NAME)"

run: ## Run lazyssh locally
	go run $(CMD_PATH)

test: ## Run unit tests with race detection
	go test -v -race ./...

test-coverage: ## Run tests and generate HTML code coverage report
	@mkdir -p $(BUILD_DIR)
	go test -v -race -coverprofile=$(BUILD_DIR)/coverage.out ./...
	go tool cover -html=$(BUILD_DIR)/coverage.out -o $(BUILD_DIR)/coverage.html
	@echo "✅ Coverage report generated at $(BUILD_DIR)/coverage.html"

fmt: ## Format all Go source files
	go fmt ./...

lint: ## Run golangci-lint
	@which golangci-lint > /dev/null 2>&1 && golangci-lint run -c .github/.golangci.yml ./... || echo "⚠️ golangci-lint is not installed. Install via: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"

check: fmt lint test ## Run formatting, linting, and tests (pre-merge check)
	@echo "✅ All checks passed!"

install: build ## Install binary to $GOPATH/bin
	install -m 0755 $(BUILD_DIR)/$(BINARY_NAME) $(shell go env GOPATH)/bin/$(BINARY_NAME)
	@echo "✅ Installed $(BINARY_NAME) to $(shell go env GOPATH)/bin/$(BINARY_NAME)"

clean: ## Remove build artifacts
	rm -rf $(BUILD_DIR)
