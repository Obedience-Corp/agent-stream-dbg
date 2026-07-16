# Stream Debugger - Development Commands

# Display available commands
default:
    @just --list --justfile {{source_file()}}

# Download dependencies (optional - go build does this automatically)
deps:
    go mod download

# Regenerate Go stubs for the gRPC test service (server-side only — the
# transport client is reflection/descriptor-driven and never imports these).
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc on PATH.
proto-gen:
    protoc --go_out=. --go_opt=module=github.com/lancekrogers/stream-debugger \
        --go-grpc_out=. --go-grpc_opt=module=github.com/lancekrogers/stream-debugger \
        --proto_path=testdata/proto testdata/proto/agentstream.proto

# Regenerate Go stubs for the A2A-shaped mock service used by
# dialects/cross_transport_test.go's TestA2ADialect_CrossTransportParity
# — a separate recipe from proto-gen since it's a distinct .proto with its
# own go_package (internal/testutil/mockgrpc/a2apb), not a Brainyard/
# agentstream concern. Requires: protoc, protoc-gen-go, protoc-gen-go-grpc
# on PATH.
proto-gen-a2a:
    protoc --go_out=. --go_opt=module=github.com/lancekrogers/stream-debugger \
        --go-grpc_out=. --go-grpc_opt=module=github.com/lancekrogers/stream-debugger \
        --proto_path=testdata/proto testdata/proto/a2a.proto

# Regenerate the compiled FileDescriptorSet used to test the gRPC
# transport's descriptor-set fallback tier (reflection-disabled servers).
# --include_imports is required: agentstream.proto imports
# google/protobuf/any.proto, and protodesc.NewFiles needs that dependency
# present in the set to resolve it, not just agentstream.proto itself.
testdata-descriptorset:
    mkdir -p testdata/descriptorsets
    protoc --proto_path=testdata/proto \
        --descriptor_set_out=testdata/descriptorsets/agentstream.binpb \
        --include_imports \
        testdata/proto/agentstream.proto

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

# Run the timeline demo against a bundled fixture - no backend, key, or network needed
demo:
    @mkdir -p bin
    @test -f bin/stream-debugger || just build
    ./bin/stream-debugger timeline testdata/fixtures/brainyard-session.jsonl

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

# Regenerate testdata/goldens/*.explain.txt — review the diff before committing, an unreviewed one is a silent regression
goldens:
    go test ./dialects/... -run TestExplainGoldens -update-goldens -v

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

# Run the test suite under the race detector (offline, no backend needed)
race:
    go test -race ./...

# Run the compiled binary under the race detector against a live backend
race-live message="Test message":
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

# Show current configuration (variable names only, never values)
config:
    @echo "=== Stream Debugger Configuration ==="
    @test -f .env && grep -o '^[A-Z_]*=' .env || echo "No .env file found. Copy .env.example to .env"

# Initialize project (first time setup)
init:
    cp .env.example .env
    @echo "✅ Created .env file"
    @echo "📝 Please edit .env with your API key and backend URL"
    @echo "🚀 Ready to run: just build (dependencies will be downloaded automatically)"

# ============================================================================
# Cross-Platform Distribution
# ============================================================================

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
