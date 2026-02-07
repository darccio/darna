.PHONY: all build test lint clean

# Build the binary
build:
	go build -o bin/darna ./cmd/darna

# Run tests
test:
	go test -v -race -cover ./...

# Run linter
lint:
	golangci-lint run ./...

# Clean build artifacts
clean:
	rm -rf bin/
	go clean

# Run all checks
all: lint test build
