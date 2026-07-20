# Release tasks for Stream Debugger.
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
    git tag -a "{{version}}" -m "Stream Debugger {{version}}"
    git push origin "{{version}}"
    echo "Tagged and pushed {{version}}."

# Create the GitHub release for a pushed tag with generated release notes.
[no-cd]
publish version:
    gh release create "{{version}}" \
        --verify-tag \
        --title "Stream Debugger {{version}}" \
        --generate-notes

# Tag and publish a release in one command. Usage:
# just release create v0.1.0
[no-cd]
create version:
    just release tag "{{version}}"
    just release publish "{{version}}"

# Build binaries for the supported release platforms.
[no-cd]
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
