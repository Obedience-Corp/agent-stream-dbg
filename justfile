# Stream Debugger - Development Commands

# Display available commands
default:
    @just --list --justfile {{source_file()}}

# Download dependencies (optional - go build does this automatically)
deps:
    go mod download

# Build the binary
build:
    mkdir -p bin
    go build -o bin/stream-debugger ./cmd/stream-debugger

# Install the binary to $GOPATH/bin
install:
    @echo "📦 Installing stream-debugger to Go bin..."
    go install ./cmd/stream-debugger
    @echo "✅ Installed successfully to $(go env GOPATH)/bin/stream-debugger"
    @echo "💡 Make sure $(go env GOPATH)/bin is in your PATH"

# Run the debugger with a test message
stream message="What is consciousness?" config="configs/brainyard-v3.yaml":
    @mkdir -p bin
    @test -f bin/stream-debugger || just build
    ./bin/stream-debugger stream --config "{{config}}" "{{message}}"

# Replay a session from logs with timeline visualization
replay session_file:
    @mkdir -p bin
    @test -f bin/stream-debugger || just build
    ./bin/stream-debugger replay "{{session_file}}"

# Visualize timeline from session log
timeline session_file:
    @mkdir -p bin
    @test -f bin/stream-debugger || just build
    ./bin/stream-debugger timeline "{{session_file}}"

# Run tests
test:
    go test -v ./...

# Format code
fmt:
    go fmt ./...

# Check for suspicious code (built-in Go tool)
vet:
    go vet ./...

# Lint code (requires golangci-lint)
lint:
    golangci-lint run

# Run all code quality checks
check:
    @echo "🔍 Running code quality checks..."
    go fmt ./...
    go vet ./...
    @echo "✅ All checks passed!"

# Clean build artifacts and logs
clean:
    rm -rf bin/
    rm -rf logs/*

# Run with race detector
race message="Test message":
    mkdir -p bin
    go run -race ./cmd/stream-debugger stream "{{message}}"

# Sign the binary for macOS (prevents security warnings)
sign-macos:
    @echo "🔏 Signing binary for macOS..."
    codesign --sign - --force --deep bin/stream-debugger
    @echo "✅ Binary signed (ad-hoc signature for local use)"
    @echo "💡 For distribution, use: codesign --sign \"Developer ID\" bin/stream-debugger"

# Build and sign in one step (macOS only)
build-signed:
    just build
    just sign-macos

# Show current configuration
config:
    @echo "=== Stream Debugger Configuration ==="
    @cat .env 2>/dev/null || echo "No .env file found. Copy .env.example to .env"

# Initialize project (first time setup)
init:
    cp .env.example .env
    @echo "✅ Created .env file"
    @echo "📝 Please edit .env with your API key and backend URL"
    @echo "🚀 Ready to run: just build (dependencies will be downloaded automatically)"
