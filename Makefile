.PHONY: build clean test install uninstall deps fmt vet lint templ

# Build variables
BINARY_NAME=ergs
BUILD_DIR=bin
VERSION?=1.3.0
LDFLAGS=-ldflags "-X main.version=$(VERSION)"
BUILD_TAGS=fts5

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
	go build -tags $(BUILD_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME) .

# Build for multiple platforms
build-all: templ
	@mkdir -p $(BUILD_DIR)
	GOOS=linux GOARCH=amd64 go build -tags $(BUILD_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-linux-amd64 .
	GOOS=darwin GOARCH=amd64 go build -tags $(BUILD_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-amd64 .
	GOOS=darwin GOARCH=arm64 go build -tags $(BUILD_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-darwin-arm64 .
	GOOS=windows GOARCH=amd64 go build -tags $(BUILD_TAGS) $(LDFLAGS) -o $(BUILD_DIR)/$(BINARY_NAME)-windows-amd64.exe .

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
	go test -tags $(BUILD_TAGS) -v ./...

# Run unit tests only (exclude integration tests)
test-unit:
	go test -tags $(BUILD_TAGS) -v -short ./...

# Run integration tests
test-integration:
	go test -tags $(BUILD_TAGS) -v ./integration_tests/...

# Run quick integration tests (for CI/CD)
test-integration-quick:
	go test -tags $(BUILD_TAGS) -v -run "TestQuick.*|TestDatasourceFactory.*" ./integration_tests/...

# Run tests with coverage
test-coverage:
	go test -tags $(BUILD_TAGS) -v -coverprofile=coverage.out ./...
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
	go run -tags $(BUILD_TAGS) . $(ARGS)

# Example commands for testing
example-fetch:
	./$(BUILD_DIR)/$(BINARY_NAME) fetch --github-token $(GITHUB_TOKEN)

example-search:
	./$(BUILD_DIR)/$(BINARY_NAME) search --query golang

# Help
help:
	@echo "Available targets:"
	@echo "  build        - Build the binary"
	@echo "  build-all    - Build for multiple platforms"
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
