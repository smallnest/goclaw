# AGENTS.md - GoClaw Development Guide

This file provides development guidelines and commands for working on GoClaw.

## Project Overview

- **Module**: `github.com/smallnest/goclaw`
- **Language**: Go 1.25.5
- **Testing**: Go standard testing with `testify` (`github.com/stretchr/testify`)
- **Logging**: `go.uber.org/zap`
- **Linting**: `golangci-lint`

## Build & Test Commands

### Quick Commands

```bash
# Build the binary
make build

# Run all tests
make test

# Run tests for a specific package
go test ./agent/...

# Run a single test function
go test ./agent/... -run TestRetryManager_ShouldRetry -v

# Run tests with race detector
make test-race

# Run tests with coverage
make test-coverage

# Run tests in short mode (skips long-running tests)
go test -short ./...

# Format code
make fmt

# Check formatting without modifying
make fmt-check

# Run linter
make lint

# Auto-fix lint issues
make lint-fix

# Run all checks (fmt, vet, lint)
make check

# Tidy dependencies
make tidy
```

### Full Development Workflow

```bash
# Install dev tools (golangci-lint)
make install-tools

# Pre-commit checks
make pre-commit

# CI pipeline (deps, check, test-race, test-coverage)
make ci
```

### Docker

```bash
make docker-build      # Build Docker image
make docker-compose-up # Start services
```

### Tauri Desktop App

```bash
make setup-tauri       # Install Tauri CLI
make dev-tauri         # Development mode
make build-tauri       # Build desktop app
```

## Code Style Guidelines

### Imports

Organize imports in three groups separated by blank lines:

1. Standard library
2. External packages (github.com, etc.)
3. Internal packages (`github.com/smallnest/goclaw/...`)

```go
import (
	"errors"
	"fmt"
	"time"

	"github.com/smallnest/goclaw/errors"
	"go.uber.org/zap"
)
```

### Naming Conventions

- **Packages**: lowercase, single word when possible (e.g., `agent`, `config`, `memory`)
- **Interfaces**: camelCase, often with `-er` suffix (e.g., `RetryManager`, `ErrorClassifier`)
- **Structs**: PascalCase (e.g., `RetryManager`, `AppError`)
- **Functions**: PascalCase for exported, camelCase for unexported
- **Variables**: camelCase, short names acceptable for short-lived variables
- **Constants**: PascalCase for exported, camelCase for unexported
- **Error codes**: `ErrCode` prefix with PascalCase (e.g., `ErrCodeInvalidInput`)
- **Error codes (values)**: `ErrCode` prefix with UPPER_SNAKE_CASE constants (e.g., `ErrCodeInvalidInput ErrorCode = "INVALID_INPUT"`)

### Error Handling

Use the structured error types from `github.com/smallnest/goclaw/errors`:

```go
// Create new error
err := errors.New(errors.ErrCodeInvalidInput, "invalid value")

// Wrap existing error
err := errors.Wrap(originalErr, errors.ErrCodeToolExecution, "failed to execute")

// Check error code
if errors.Is(err, errors.ErrCodeNotFound) { ... }

// Check if error is retryable
if errors.IsRetryable(err) { ... }
```

### Logging

Use `go.uber.org/zap` for structured logging:

```go
logger.Info("message",
    zap.String("key", value),
    zap.Int("count", n),
    zap.Error(err),
)
```

Log levels: `Debug`, `Info`, `Warn`, `Error`

### Testing

Follow these conventions for test files:

- Test file naming: `*_test.go`
- Package name matches the package being tested
- Table-driven tests preferred for multiple test cases
- Use `t.Run()` for subtests with descriptive names

```go
func TestRetryManager_ShouldRetry(t *testing.T) {
    tests := []struct {
        name       string
        config     *RetryConfig
        want       bool
    }{
        {name: "disabled should not retry", config: &RetryConfig{Enabled: false}, want: false},
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            rm := NewRetryManager(tt.config, nil)
            got := rm.ShouldRetry(nil)
            if got != tt.want {
                t.Errorf("ShouldRetry() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

For test assertions, use `require` (fail fast) or `assert` (continue testing):

```go
require.NoError(t, err)
assert.Equal(t, expected, actual)
```

### Documentation

- Document exported types and functions with comments
- Comments start with the name of the element being documented
- Internal comments may use Chinese or English

```go
// RetryManager 重试管理器接口
type RetryManager interface {
    ShouldRetry(err error) bool
    // ...
}
```

### Configuration

- Use `spf13/viper` for configuration management
- Support YAML and JSON formats
- Environment variables with `GOSKILLS_*` prefix
- Config file lookup order: `~/.goclaw/config.json`, `./config.json`, env vars

### Project Structure

```
goclaw/
├── agent/          # Agent core logic
├── channels/       # IM platform adapters
├── config/         # Configuration management
├── cron/           # Scheduling
├── errors/         # Error types and utilities
├── gateway/        # WebSocket gateway
├── internal/       # Internal packages
│   ├── logger/     # Logging utilities
│   └── workspace/  # Workspace management
├── memory/         # Memory/vector store
├── providers/      # LLM providers
├── session/        # Session management
├── tools/          # Tool system
└── ui/             # Frontend (separate npm project)
```

### Common Pitfalls

- **Import cycles**: Avoid circular dependencies between packages
- **Context propagation**: Pass `context.Context` explicitly, don't use package-level globals
- **Goroutine leaks**: Ensure spawned goroutines complete or are properly cancelled
- **Error wrapping**: Always wrap with context: `errors.Wrap(err, code, "doing X")`
- **Test isolation**: Each test should be independent; avoid shared mutable state

## CI/CD

GitHub Actions runs on every push to `master` and on PRs:

1. Build: `go build -v ./...`
2. Test: `go test -race -coverprofile=coverage.out -covermode=atomic` (excluding `/examples` and `/showcases`)

## Useful Go Commands

```bash
go mod tidy          # Clean up go.mod
go mod download      # Download dependencies
go list ./...        # List all packages
go doc <package>     # View package docs
go vet ./...         # Check for issues
```
