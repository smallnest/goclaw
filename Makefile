.PHONY: help all build test test-race test-coverage test-verbose lint fmt fmt-check vet clean deps tidy check install-tools benchmark

# Variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOCLEAN=$(GOCMD) clean
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=gofmt
GOVET=$(GOCMD) vet
BINARY_NAME=goclaw
BUILD_DIR=.
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
DOCKER_IMAGE=goclaw
DOCKER_TAG=$(VERSION)
COVERAGE_FILE=coverage.out
COVERAGE_HTML=coverage.html

# Colors for terminal output
COLOR_RESET=\033[0m
COLOR_BOLD=\033[1m
COLOR_GREEN=\033[32m
COLOR_YELLOW=\033[33m
COLOR_BLUE=\033[34m

# Default target
all: clean fmt lint test build

## help: Display this help message
help:
	@echo "$(COLOR_BOLD)GoClaw - Makefile Commands$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)Usage:$(COLOR_RESET)"
	@echo "  make $(COLOR_GREEN)<target>$(COLOR_RESET)"
	@echo ""
	@echo "$(COLOR_BOLD)Available targets:$(COLOR_RESET)"
	@grep -E '^## ' $(MAKEFILE_LIST) | sed 's/## /  $(COLOR_GREEN)/' | sed 's/:/ $(COLOR_RESET)-/'
	@echo ""

## build: Build the project
build:
	@echo "$(COLOR_BLUE)Building $(BINARY_NAME)...$(COLOR_RESET)"
	@mkdir -p $(BUILD_DIR)
	$(GOBUILD) -ldflags="-X 'main.Version=$(VERSION)'" -o $(BUILD_DIR)/$(BINARY_NAME) .

## test: Run all tests
test:
	@echo "$(COLOR_BLUE)Running tests...$(COLOR_RESET)"
	$(GOTEST) -v ./...

## test-short: Run tests in short mode
test-short:
	@echo "$(COLOR_BLUE)Running tests (short mode)...$(COLOR_RESET)"
	$(GOTEST) -short ./...

## test-race: Run tests with race detector
test-race:
	@echo "$(COLOR_BLUE)Running tests with race detector...$(COLOR_RESET)"
	$(GOTEST) -race ./...

## test-coverage: Run tests with coverage report
test-coverage:
	@echo "$(COLOR_BLUE)Running tests with coverage...$(COLOR_RESET)"
	$(GOTEST) -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	@echo "$(COLOR_GREEN)Coverage report generated: $(COVERAGE_FILE)$(COLOR_RESET)"
	$(GOCMD) tool cover -html=$(COVERAGE_FILE) -o $(COVERAGE_HTML)
	@echo "$(COLOR_GREEN)HTML coverage report: $(COVERAGE_HTML)$(COLOR_RESET)"

## test-verbose: Run tests with verbose output
test-verbose:
	@echo "$(COLOR_BLUE)Running tests (verbose)...$(COLOR_RESET)"
	$(GOTEST) -v -count=1 ./...

## benchmark: Run benchmarks
benchmark:
	@echo "$(COLOR_BLUE)Running benchmarks...$(COLOR_RESET)"
	$(GOTEST) -bench=. -benchmem ./...

## lint: Run golangci-lint
lint:
	@echo "$(COLOR_BLUE)Running linter...$(COLOR_RESET)"
	@which golangci-lint > /dev/null || (echo "$(COLOR_YELLOW)golangci-lint not found. Run 'make install-tools'$(COLOR_RESET)" && exit 1)
	golangci-lint run ./...

## lint-fix: Auto-fix lint issues
lint-fix:
	@echo "$(COLOR_BLUE)Auto-fixing lint issues...$(COLOR_RESET)"
	@which golangci-lint > /dev/null || (echo "$(COLOR_YELLOW)golangci-lint not found. Run 'make install-tools'$(COLOR_RESET)" && exit 1)
	golangci-lint run --fix ./...
	@echo "$(COLOR_GREEN)Lint fixes applied$(COLOR_RESET)"

## fmt: Format all Go files
fmt:
	@echo "$(COLOR_BLUE)Formatting code...$(COLOR_RESET)"
	$(GOFMT) -s -w .
	@echo "$(COLOR_GREEN)Code formatted successfully$(COLOR_RESET)"

## fmt-check: Check if code is formatted
fmt-check:
	@echo "$(COLOR_BLUE)Checking code formatting...$(COLOR_RESET)"
	@test -z "$$($(GOFMT) -l .)" || (echo "$(COLOR_YELLOW)The following files need formatting:$(COLOR_RESET)" && $(GOFMT) -l . && exit 1)
	@echo "$(COLOR_GREEN)All files are properly formatted$(COLOR_RESET)"

## vet: Run go vet
vet:
	@echo "$(COLOR_BLUE)Running go vet...$(COLOR_RESET)"
	$(GOVET) ./...

## check: Run fmt-check, vet, and lint
check: fmt-check vet lint
	@echo "$(COLOR_GREEN)All checks passed!$(COLOR_RESET)"

## clean: Clean build artifacts and test cache
clean:
	@echo "$(COLOR_BLUE)Cleaning...$(COLOR_RESET)"
	$(GOCLEAN)
	rm -f $(COVERAGE_FILE) $(COVERAGE_HTML)
	rm -f $(BUILD_DIR)/$(BINARY_NAME)
	@echo "$(COLOR_GREEN)Clean complete$(COLOR_RESET)"

## deps: Download dependencies
deps:
	@echo "$(COLOR_BLUE)Downloading dependencies...$(COLOR_RESET)"
	$(GOMOD) download
	@echo "$(COLOR_GREEN)Dependencies downloaded$(COLOR_RESET)"

## tidy: Tidy and verify dependencies
tidy:
	@echo "$(COLOR_BLUE)Tidying dependencies...$(COLOR_RESET)"
	$(GOMOD) tidy
	$(GOMOD) verify
	@echo "$(COLOR_GREEN)Dependencies tidied$(COLOR_RESET)"

## install-tools: Install development tools
install-tools:
	@echo "$(COLOR_BLUE)Installing development tools...$(COLOR_RESET)"
	@which golangci-lint > /dev/null || (echo "Installing golangci-lint..." && \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
	@echo "$(COLOR_GREEN)Tools installed$(COLOR_RESET)"

## run: Run the application
run:
	@echo "$(COLOR_BLUE)Running $(BINARY_NAME)...$(COLOR_RESET)"
	$(GOCMD) run .

## install: Install the binary to GOPATH/bin
install:
	@echo "$(COLOR_BLUE)Installing $(BINARY_NAME)...$(COLOR_RESET)"
	$(GOCMD) install

## docs: Generate documentation
docs:
	@echo "$(COLOR_BLUE)Generating documentation...$(COLOR_RESET)"
	@echo "Open http://localhost:6060/pkg/github.com/smallnest/dogclaw/goclaw/ in your browser"
	godoc -http=:6060

## ci: Run continuous integration checks
ci: deps check test-race test-coverage
	@echo "$(COLOR_GREEN)CI checks passed!$(COLOR_RESET)"

## pre-commit: Run pre-commit checks (fmt, vet, lint, test)
pre-commit: fmt vet lint test
	@echo "$(COLOR_GREEN)Pre-commit checks passed!$(COLOR_RESET)"

## update-deps: Update all dependencies to latest versions
update-deps:
	@echo "$(COLOR_BLUE)Updating dependencies...$(COLOR_RESET)"
	$(GOGET) -u ./...
	$(GOMOD) tidy
	@echo "$(COLOR_GREEN)Dependencies updated$(COLOR_RESET)"

## version: Display Go version
version:
	@$(GOCMD) version

## info: Display project information
info:
	@echo "$(COLOR_BOLD)Project Information$(COLOR_RESET)"
	@echo "  Name: GoClaw"
	@echo "  Module: github.com/smallnest/dogclaw/goclaw"
	@echo "  Go Version: $$($(GOCMD) version | cut -d' ' -f3)"
	@echo "  Version: $(VERSION)"
	@echo "  Packages: $$(find . -name '*.go' -not -path './vendor/*' | xargs dirname | sort -u | wc -l | tr -d ' ')"
	@echo "  Lines of Code: $$(find . -name '*.go' -not -path './vendor/*' | xargs wc -l | tail -1 | awk '{print $$1}')"

## setup: Setup development environment
setup:
	@echo "$(COLOR_BLUE)Setting up development environment...$(COLOR_RESET)"
	@mkdir -p .goclaw/workspace .goclaw/sessions
	@cp .env.example .env 2>/dev/null || echo "Please copy .env.example to .env and configure"
	@echo "$(COLOR_GREEN)Setup complete. Edit .env with your configuration.$(COLOR_RESET)"

# Docker targets
## docker-build: Build Docker image
docker-build:
	@echo "$(COLOR_BLUE)Building Docker image...$(COLOR_RESET)"
	docker build -t $(DOCKER_IMAGE):$(DOCKER_TAG) .
	docker tag $(DOCKER_IMAGE):$(DOCKER_TAG) $(DOCKER_IMAGE):latest
	@echo "$(COLOR_GREEN)Docker image built: $(DOCKER_IMAGE):$(DOCKER_TAG)$(COLOR_RESET)"

## docker-run: Run Docker container
docker-run:
	@echo "$(COLOR_BLUE)Running Docker container...$(COLOR_RESET)"
	docker run --rm -it \
		-p 8080:8080 \
		-v $(PWD)/config.json:/home/goclaw/.goclaw/config.json:ro \
		$(DOCKER_IMAGE):latest

## docker-compose-up: Start services with docker-compose
docker-compose-up:
	@echo "$(COLOR_BLUE)Starting services with docker-compose...$(COLOR_RESET)"
	docker-compose up -d

## docker-compose-down: Stop services
docker-compose-down:
	@echo "$(COLOR_BLUE)Stopping services...$(COLOR_RESET)"
	docker-compose down

## docker-compose-logs: Show logs from services
docker-compose-logs:
	@echo "$(COLOR_BLUE)Showing logs...$(COLOR_RESET)"
	docker-compose logs -f

## docker-compose-ps: Show running services
docker-compose-ps:
	@echo "$(COLOR_BLUE)Showing running services...$(COLOR_RESET)"
	docker-compose ps

## docker-shell: Open shell in container
docker-shell:
	@echo "$(COLOR_BLUE)Opening shell in container...$(COLOR_RESET)"
	docker-compose exec goclaw sh

## dev: Start development environment
dev: docker-compose-up docker-compose-logs
