# Development utilities imported into the root justfile.

# Download dependencies (optional - go build does this automatically)
deps:
    go mod download

# Regenerate Go stubs for the gRPC test service (server-side only — the
# transport client is reflection/descriptor-driven and never imports these).
# Requires: protoc, protoc-gen-go, protoc-gen-go-grpc on PATH.
proto-gen:
    protoc --go_out=. --go_opt=module=github.com/Obedience-Corp/stream-debugger \
        --go-grpc_out=. --go-grpc_opt=module=github.com/Obedience-Corp/stream-debugger \
        --proto_path=testdata/proto testdata/proto/agentstream.proto

# Regenerate Go stubs for the A2A-shaped mock service used by
# dialects/cross_transport_test.go's TestA2ADialect_CrossTransportParity
# — a separate recipe from proto-gen since it's a distinct .proto with its
# own go_package (internal/testutil/mockgrpc/a2apb), not a Brainyard/
# agentstream concern. Requires: protoc, protoc-gen-go, protoc-gen-go-grpc
# on PATH.
proto-gen-a2a:
    protoc --go_out=. --go_opt=module=github.com/Obedience-Corp/stream-debugger \
        --go-grpc_out=. --go-grpc_opt=module=github.com/Obedience-Corp/stream-debugger \
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

# Build outside the source tree and replay a fixture using embedded dialects.
smoke-binary:
    #!/usr/bin/env bash
    set -euo pipefail
    repo_root=$PWD
    tmp_dir=$(mktemp -d)
    trap 'rm -rf "$tmp_dir"' EXIT
    go build -o "$tmp_dir/stream-debugger" ./cmd/stream-debugger
    cd "$tmp_dir"
    "$tmp_dir/stream-debugger" --help >/dev/null
    "$tmp_dir/stream-debugger" replay "$repo_root/testdata/fixtures/brainyard-session.jsonl" >/dev/null

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

# ACP dialect demo: explain + timeline + replay against the bundled fixture.
# No backend, API key, or network. See demos/record-acp.sh for the VHS recording.
demo-acp:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p bin
    test -f bin/stream-debugger || just build
    echo "=== explain ==="
    ./bin/stream-debugger explain --dialect dialects/acp.yaml --from testdata/fixtures/acp-session.jsonl
    echo
    echo "=== timeline ==="
    ./bin/stream-debugger timeline --dialect acp testdata/fixtures/acp-session.jsonl
    echo
    echo "=== replay ==="
    ./bin/stream-debugger replay --dialect acp testdata/fixtures/acp-session.jsonl

# Prototype launch-home TUI (design only — not the product binary).
# See workflow/design/stream-debugger-home-tui/ and demos/home-ux/.
demo-home-ux:
    #!/usr/bin/env bash
    set -euo pipefail
    go build -o bin/home-ux ./demos/home-ux
    export SD_HOME_UX_CONFIG_DIR="${SD_HOME_UX_CONFIG_DIR:-$(mktemp -d /tmp/sd-home-ux.XXXXXX)}"
    ./bin/home-ux

# Record docs/assets/home-ux-*.gif design tapes (requires vhs).
record-home-ux:
    bash demos/home-ux/record.sh all

# Record docs/assets/tui-acp-demo.gif (requires vhs + ffmpeg on PATH).
record-acp:
    bash demos/record-acp.sh

# Record docs/assets/tui-acp-live.gif — multi-turn ACP process/session reuse.
record-acp-live:
    bash demos/record-acp-live.sh

# Run the OpenAI fixture through the mock SSE server and report decode counts.
demo-openai:
    go test ./internal/client -run TestOpenAIDialectMockSSESmoke -count=1 -v

# Run the live TUI recording integration suite. Requires Brainyard, Obey,
# and an external TermCast installation; see .justfiles/vhs.just for configuration.
integration-vhs:
    just vhs integration

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

# Clean build artifacts and logs
clean:
    rm -rf bin/
    rm -rf logs/*

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
