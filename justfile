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

# ============================================================================
# Cross-Platform Distribution
# ============================================================================

# Target directory for BrainyardV3 distribution
BRAINYARD_TOOLS := parent_directory(justfile_directory()) + "/BrainyardV3/tools/stream-debugger"

# Build binaries for all target platforms
build-all-platforms:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p bin

    echo "🔨 Building for darwin/arm64 (M1/M2/M3 Macs)..."
    GOOS=darwin GOARCH=arm64 go build -o bin/stream-debugger-darwin-arm64 ./cmd/stream-debugger

    echo "🔨 Building for darwin/amd64 (Intel Macs)..."
    GOOS=darwin GOARCH=amd64 go build -o bin/stream-debugger-darwin-amd64 ./cmd/stream-debugger

    echo "🔨 Building for linux/amd64 (Docker/Railway)..."
    GOOS=linux GOARCH=amd64 go build -o bin/stream-debugger-linux-amd64 ./cmd/stream-debugger

    echo "✅ All platforms built:"
    ls -la bin/stream-debugger-*

# Build multi-arch binaries and copy to BrainyardV3
release-to-brainyard: build-all-platforms
    #!/usr/bin/env bash
    set -euo pipefail

    DEST="{{BRAINYARD_TOOLS}}/bin"
    echo "📦 Distributing binaries to BrainyardV3..."

    # Create destination directories
    mkdir -p "$DEST/darwin-arm64"
    mkdir -p "$DEST/darwin-amd64"
    mkdir -p "$DEST/linux-amd64"

    # Copy binaries
    cp bin/stream-debugger-darwin-arm64 "$DEST/darwin-arm64/stream-debugger"
    cp bin/stream-debugger-darwin-amd64 "$DEST/darwin-amd64/stream-debugger"
    cp bin/stream-debugger-linux-amd64 "$DEST/linux-amd64/stream-debugger"

    # Sign macOS binaries (ad-hoc for local use)
    echo "🔏 Signing macOS binaries..."
    codesign --sign - --force --deep "$DEST/darwin-arm64/stream-debugger" 2>/dev/null || true
    codesign --sign - --force --deep "$DEST/darwin-amd64/stream-debugger" 2>/dev/null || true

    # Copy .env.example if source exists
    if [ -f ".env.example" ]; then
        cp .env.example "{{BRAINYARD_TOOLS}}/.env.example"
        echo "📄 Copied .env.example"
    fi

    echo ""
    echo "✅ Release complete!"
    echo ""
    echo "Binaries distributed to:"
    ls -la "$DEST"/*/stream-debugger 2>/dev/null || ls -la "$DEST"
    echo ""
    echo "💡 Non-Go developers can now use stream-debugger from BrainyardV3:"
    echo "   cd BrainyardV3"
    echo "   just sdebug stream \"Hello\""
