# Release tasks for agent-stream-dbg.
#
# Releases are tag-based: the tag is pushed to GitHub, then GitHub creates the
# release with generated notes. Run `just release create v0.1.0` after merging
# the changes that should ship.

[private]
default:
    @just --list --unsorted --justfile {{source_file()}}

# Validate the worktree and run the checks required before tagging.
[no-cd]
check:
    #!/usr/bin/env bash
    set -euo pipefail
    [ -z "$(git status --porcelain)" ] || { echo "working tree is dirty — commit or stash first" >&2; exit 1; }
    just test
    just test vet

# Create and push an annotated release tag after validation. Usage:
# just release tag v0.1.0
[no-cd]
tag version: check
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ ! "{{version}}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
        echo "version must look like vX.Y.Z (received: {{version}})" >&2
        exit 1
    fi
    if git rev-parse --verify --quiet "refs/tags/{{version}}" >/dev/null; then
        echo "tag already exists: {{version}}" >&2
        exit 1
    fi
    git tag -a "{{version}}" -m "agent-stream-dbg {{version}}"
    git push origin "{{version}}"
    echo "Tagged and pushed {{version}}."

# Create the GitHub release for a pushed tag with generated release notes.
[no-cd]
publish version:
    gh release create "{{version}}" \
        --verify-tag \
        --title "agent-stream-dbg {{version}}" \
        --generate-notes

# Tag and publish a release in one command. Usage:
# just release create v0.1.0
[no-cd]
create version:
    just release tag "{{version}}"
    just release publish "{{version}}"
    just release package-binaries "{{version}}"

# Build binaries for the supported release platforms.
[no-cd]
build-all-platforms:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p bin

    echo "🔨 Building for darwin/arm64 (M1/M2/M3 Macs)..."
    CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o bin/agent-stream-dbg-darwin-arm64 ./cmd/agent-stream-dbg

    echo "🔨 Building for darwin/amd64 (Intel Macs)..."
    CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o bin/agent-stream-dbg-darwin-amd64 ./cmd/agent-stream-dbg

    echo "🔨 Building for linux/amd64..."
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o bin/agent-stream-dbg-linux-amd64 ./cmd/agent-stream-dbg

    echo "🔨 Building for linux/arm64..."
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o bin/agent-stream-dbg-linux-arm64 ./cmd/agent-stream-dbg

    echo "✅ All platforms built:"
    ls -la bin/agent-stream-dbg-*

# Package platform binaries as release assets (npm install downloads these).
# Asset names: agent-stream-dbg_<tag>_<os>_<arch>.tar.gz
# Usage: just release package-binaries v0.1.0
[no-cd]
package-binaries version:
    #!/usr/bin/env bash
    set -euo pipefail
    if [[ ! "{{version}}" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$ ]]; then
        echo "version must look like vX.Y.Z (received: {{version}})" >&2
        exit 1
    fi
    just release build-all-platforms
    mkdir -p dist
    package() {
        local goos="$1" goarch="$2" bin="bin/agent-stream-dbg-${1}-${2}"
        local asset="agent-stream-dbg_{{version}}_${goos}_${goarch}.tar.gz"
        local stage
        stage=$(mktemp -d)
        cp "$bin" "${stage}/agent-stream-dbg"
        chmod +x "${stage}/agent-stream-dbg"
        tar -C "$stage" -czf "dist/${asset}" agent-stream-dbg
        rm -rf "$stage"
        echo "packed dist/${asset}"
    }
    package darwin arm64
    package darwin amd64
    package linux amd64
    package linux arm64
    gh release upload "{{version}}" dist/agent-stream-dbg_{{version}}_*.tar.gz --clobber
    echo "✅ Uploaded platform assets to {{version}}"
    ls -la dist/agent-stream-dbg_{{version}}_*.tar.gz

# Print the Homebrew source tarball sha256 for a tag (update homebrew/agent-stream-dbg.rb).
[no-cd]
homebrew-sha version:
    #!/usr/bin/env bash
    set -euo pipefail
    url="https://github.com/Obedience-Corp/agent-stream-dbg/archive/refs/tags/{{version}}.tar.gz"
    echo "url: ${url}"
    curl -sL "$url" | shasum -a 256
