# Stream Debugger - Development Commands

# Display available commands
default:
    @just --list --justfile {{source_file()}}

# Install dependencies
deps:
    go get github.com/charmbracelet/bubbletea
    go get github.com/charmbracelet/lipgloss
    go get github.com/rs/zerolog
    go get github.com/joho/godotenv
    go get github.com/r3labs/sse/v2
    go mod tidy

# Build the binary
build:
    mkdir -p bin
    go build -o bin/stream-debugger ./cmd/stream-debugger

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

# Lint code
lint:
    golangci-lint run

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
    just deps
    @echo "✅ Dependencies installed"
    @echo "🚀 Ready to run: just stream \"your message\""
