# Chaperone Authentication Gateway - Justfile
# A Justfile provides a modern alternative to Makefiles with better ergonomics

# Configuration
BINARY := "chaperone"
BUILD_DIR := "."
MAIN_PACKAGE := "github.com/bmf/chaperone"
CMD_DIR := "cmd/chaperone"
GO_VERSION := "1.25"

# Build the binary
build:
    go build -o {{BUILD_DIR}}/{{BINARY}} ./{{CMD_DIR}}

# Alternative build path
build-alt:
    go build -o {{BUILD_DIR}}/{{BINARY}} ./cmd/chaperone

# Build with version info
build-v:
    @echo "Building {{BINARY}} with version info..."
    go build -ldflags "-X {{MAIN_PACKAGE}}/cmd.Version=dev" -o {{BUILD_DIR}}/{{BINARY}} ./{{CMD_DIR}}

# Install dependencies
deps:
    go mod download
    go mod verify

# Tidy dependencies
mod-tidy:
    go mod tidy

# Run all tests
test:
    go test -v ./...

# Run tests with coverage
test-coverage:
    go test -v ./... -coverprofile=coverage.out -coverpkg=./...
    @echo "Coverage report generated. Open coverage.out in browser:"
    @echo "  go tool cover -html=coverage.out"

# Run tests with race detector
test-race:
    go test -race -v ./...

# Run tests with timeout
test-timeout:
    go test -timeout=5m -v ./...

# Lint with golangci-lint
lint:
    golangci-lint run

# Lint with timeout
lint-fast:
    golangci-lint run --timeout=3m

# Format all Go source files
fmt:
    gofmt -w .
    goimports -w .

# Check formatting without changes
fmt-check:
    @if [ "$(shell gofmt -s -l . | wc -l)" -gt 0 ]; then \
        echo "The following files are not formatted:"; \
        gofmt -s -l .; \
        exit 1; \
    fi

# Vet code
vet:
    go vet ./...

# Security scan with gosec
sec:
    @command -v gosec >/dev/null 2>&1 || { echo "gosec not installed. Install with: go install github.com/securecodewarrior/gosec/v2/cmd/gosec@latest"; exit 1; }
    gosec ./...

# Run all checks (lint, fmt, vet, test)
check: fmt-check lint vet test
    @echo "✓ All checks passed!"

# Watch mode for development (requires goworld or similar)
watch:
    @command -v gow >/dev/null 2>&1 || { echo "gow not installed. Install with: go install github.com/mitranim/gow@latest"; exit 1; }
    gow -d . -r

# Clean build artifacts
clean:
    go clean
    rm -f {{BUILD_DIR}}/{{BINARY}}
    rm -f coverage.out

# Install the binary to GOBIN or GOPATH/bin
install:
    go install ./{{CMD_DIR}}

# Run the application
run config="chaperone.toml":
    ./{{BINARY}} run --config {{config}}

# Initialize configuration
init service="openai":
    ./{{BINARY}} init {{service}}

# Setup system proxy (macOS)
setup-proxy:
    ./{{BINARY}} setup proxy

# Show version
version:
    ./{{BINARY}} --version

# Benchmark tests
bench:
    go test -bench=. -benchmem ./...

# Generate documentation
docs:
    @command -v godoc >/dev/null 2>&1 || { echo "godoc not available. Run: go install golang.org/x/tools/cmd/godoc@latest"; exit 1; }
    godoc -http=:6060
    @echo "Documentation available at http://localhost:6060"

# Build for multiple platforms
release:
    @echo "Building releases for multiple platforms..."
    mkdir -p dist
    GOOS=linux GOARCH=amd64 go build -o dist/{{BINARY}}-linux-amd64 ./{{CMD_DIR}}
    GOOS=darwin GOARCH=amd64 go build -o dist/{{BINARY}}-darwin-amd64 ./{{CMD_DIR}}
    GOOS=darwin GOARCH=arm64 go build -o dist/{{BINARY}}-darwin-arm64 ./{{CMD_DIR}}
    GOOS=windows GOARCH=amd64 go build -o dist/{{BINARY}}-windows-amd64.exe ./{{CMD_DIR}}
    @echo "Releases built in dist/ directory"

# Run with custom config and verbose logging
dev config="chaperone.toml":
    RUST_LOG=debug ./{{BINARY}} run --config {{config}}

# Update Go tools
tools:
    go install golang.org/x/tools/cmd/goimports@latest
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
    @echo "✓ Go tools updated"

# Show help
help:
    @echo "Chaperone Authentication Gateway - Available Commands:"
    @echo ""
    @echo "Building:"
    @echo "  build          Build the binary"
    @echo "  build-v        Build with version info"
    @echo "  clean          Clean build artifacts"
    @echo "  release        Build for multiple platforms"
    @echo ""
    @echo "Testing:"
    @echo "  test           Run all tests"
    @echo "  test-coverage  Run tests with coverage report"
    @echo "  test-race      Run tests with race detector"
    @echo "  test-timeout   Run tests with 5min timeout"
    @echo "  bench          Run benchmark tests"
    @echo ""
    @echo "Code Quality:"
    @echo "  fmt            Format Go source files"
    @echo "  fmt-check      Check formatting without changes"
    @echo "  lint           Run golangci-lint"
    @echo "  lint-fast      Run linter with 3min timeout"
    @echo "  vet            Run go vet"
    @echo "  sec            Run security scan with gosec"
    @echo "  check          Run all checks (fmt, lint, vet, test)"
    @echo ""
    @echo "Dependencies:"
    @echo "  deps           Download dependencies"
    @echo "  mod-tidy       Tidy dependencies"
    @echo "  tools          Update Go development tools"
    @echo ""
    @echo "Development:"
    @echo "  watch          Watch for changes and rebuild (requires gow)"
    @echo "  run            Run the application"
    @echo "  dev            Run with debug logging"
    @echo "  init           Initialize configuration"
    @echo "  setup-proxy    Setup system proxy (macOS)"
    @echo ""
    @echo "Utilities:"
    @echo "  version        Show binary version"
    @echo "  docs           Generate and serve documentation"
    @echo "  help           Show this help message"
    @echo ""
    @echo "Examples:"
    @echo "  just build-v && just run"
    @echo "  just check"
    @echo "  just test-coverage"
    @echo "  just release"

# Default recipe
@default:
    @just --list
