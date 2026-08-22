.PHONY: all build test coverage lint clean

# Build the binary
build:
	go build -o bin/darna ./cmd/darna

# Run tests
test:
	go test -v -race -cover ./...

# Run tests and write a coverage profile
coverage:
	go test -race -covermode=atomic -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	go tool cover -func=coverage.out | tail -1

# Run linter
lint:
	golangci-lint run ./...

# Clean build artifacts
clean:
	rm -rf bin/
	rm -f coverage.out coverage.html
	go clean

# Run all checks
all: lint test build
