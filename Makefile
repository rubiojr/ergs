.PHONY: build clean test install uninstall deps fmt vet lint templ

# Build variables
BINARY_NAME=ergs
BUILD_DIR=bin
VERSION?=$(shell grep 'const Version' pkg/version/version.go | cut -d'"' -f2)
LDFLAGS=-ldflags "-X main.version=$(VERSION)"
BUILD_TAGS=fts5

# CGO Configuration
# Set to 0 for CGO-free builds (default) using ncruces/go-sqlite3
# Set to 1 to enable CGO if needed: make build CGO_ENABLED=1
CGO_ENABLED?=0

# Default target
all: build

# Generate templ templates
templ:
	@echo "Generating templ templates..."
	go install github.com/a-h/templ/cmd/templ@latest
	templ generate

# Build the binary
build: templ
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) go build -tags $(BUILD_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

# Build for multiple platforms
build-all: templ
	@mkdir -p $(BUILD_DIR)
	CGO_ENABLED=$(CGO_ENABLED) GOOS=linux GOARCH=amd64 go build -tags $(BUILD_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin GOARCH=amd64 go build -tags $(BUILD_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	CGO_ENABLED=$(CGO_ENABLED) GOOS=darwin GOARCH=arm64 go build -tags $(BUILD_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	CGO_ENABLED=$(CGO_ENABLED) GOOS=windows GOARCH=amd64 go build -tags $(BUILD_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .

# Install dependencies
deps:
	go mod download
	go mod tidy

# Format code
fmt:
	go fmt ./...

# Vet code
vet:
	go vet ./...

# Run tests
test:
	CGO_ENABLED=$(CGO_ENABLED) go test -tags $(BUILD_TAGS) -v ./...

# Run unit tests only (exclude integration tests)
test-unit:
	CGO_ENABLED=$(CGO_ENABLED) go test -tags $(BUILD_TAGS) -v ./pkg/... ./cmd/...

# Run integration tests
test-integration:
	CGO_ENABLED=$(CGO_ENABLED) go test -tags $(BUILD_TAGS) -v ./integration_tests/...

# Run quick integration tests (for CI/CD)
test-integration-quick:
	CGO_ENABLED=$(CGO_ENABLED) go test -tags $(BUILD_TAGS) -v -run "TestQuick.*|TestDatasourceFactory.*" ./integration_tests/...

# Run tests with coverage
test-coverage:
	CGO_ENABLED=$(CGO_ENABLED) go test -tags $(BUILD_TAGS) -v -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

# Run all tests including integration tests
test-all: test-unit test-integration

# Install binary to ~/.local/bin
install: build
	@mkdir -p ~/.local/bin
	cp $(BUILD_DIR)/$(BINARY_NAME) ~/.local/bin/$(BINARY_NAME).new
	mv ~/.local/bin/$(BINARY_NAME).new ~/.local/bin/$(BINARY_NAME)
	@echo "✓ Installed $(BINARY_NAME) to ~/.local/bin"
	@echo "  Make sure ~/.local/bin is in your PATH"

# Uninstall binary from ~/.local/bin
uninstall:
	@if [ -f ~/.local/bin/$(BINARY_NAME) ]; then \
		rm ~/.local/bin/$(BINARY_NAME); \
		echo "✓ Removed $(BINARY_NAME) from ~/.local/bin"; \
	else \
		echo "$(BINARY_NAME) not found in ~/.local/bin"; \
	fi

# Clean build artifacts
clean:
	rm -rf $(BUILD_DIR)
	rm -f coverage.out coverage.html
	rm -f ergs.db
	@echo "Cleaning generated templ files..."
	find . -name "*_templ.go" -type f -delete

# Development setup
dev-setup: deps
	@echo "Installing development tools..."
	go install golang.org/x/tools/cmd/goimports@latest
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go install github.com/a-h/templ/cmd/templ@latest

# Lint code
lint:
	golangci-lint run

# Run development environment
dev-run:
	CGO_ENABLED=$(CGO_ENABLED) go run -tags $(BUILD_TAGS) . $(ARGS)

# Example commands for testing
example-fetch:
	./$(BUILD_DIR)/$(BINARY_NAME) fetch --github-token $(GITHUB_TOKEN)

example-search:
	./$(BUILD_DIR)/$(BINARY_NAME) search --query golang

# Help
help:
	@echo "Available targets:"
	@echo "  build        - Build the binary (CGO-free by default)"
	@echo "  build-all    - Build for multiple platforms (CGO-free)"
	@echo "  templ        - Generate templ templates"
	@echo "  deps         - Install dependencies"
	@echo "  fmt          - Format code"
	@echo "  vet          - Vet code"
	@echo "  test         - Run tests"
	@echo "  test-unit    - Run unit tests only"
	@echo "  test-integration - Run integration tests"
	@echo "  test-integration-quick - Run quick integration tests"
	@echo "  test-coverage- Run tests with coverage"
	@echo "  test-all     - Run all tests including integration"
	@echo "  install      - Install binary to ~/.local/bin"
	@echo "  uninstall    - Remove binary from ~/.local/bin"
	@echo "  clean        - Clean build artifacts"
	@echo "  dev-setup    - Setup development environment"
	@echo "  lint         - Lint code"
	@echo "  dev-run      - Run in development mode"
	@echo "  help         - Show this help"
	@echo ""
	@echo "CGO Configuration:"
	@echo "  Default: CGO_ENABLED=0 (CGO-free builds using ncruces/go-sqlite3)"
	@echo "  To enable CGO: make build CGO_ENABLED=1"
