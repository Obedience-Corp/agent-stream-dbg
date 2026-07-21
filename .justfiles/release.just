# Release tasks for agent-stream-dbg.
#
# Production releases match Obedience-Corp/festival:
#   1. just release tag vX.Y.Z     # annotated tag + push
#   2. GitHub Actions runs GoReleaser (binaries, checksums, homebrew-tap Formula)
#      then publishes npm (@obedience-corp/agent-stream-dbg)
#
# Local dry-run: just release snapshot

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

# Create and push an annotated release tag after validation.
# CI on the tag runs GoReleaser + npm publish. Usage: just release tag v0.1.1
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
    echo "Watch: gh run list --workflow=release.yml --repo Obedience-Corp/agent-stream-dbg"

# Tag a release (CI publishes GitHub + Homebrew + npm). Usage: just release create v0.1.1
[no-cd]
create version:
    just release tag "{{version}}"

# Local GoReleaser snapshot (no publish). Requires goreleaser on PATH.
[no-cd]
snapshot:
    goreleaser release --snapshot --clean

# Local GoReleaser dry-run against the current tag state (no publish).
[no-cd]
check-goreleaser:
    goreleaser check

# Cross-compile binaries only (debug helper; production uses GoReleaser).
[no-cd]
build-all-platforms:
    #!/usr/bin/env bash
    set -euo pipefail
    mkdir -p bin
    for pair in darwin/arm64 darwin/amd64 linux/amd64 linux/arm64; do
        goos="${pair%/*}"
        goarch="${pair#*/}"
        echo "🔨 ${goos}/${goarch}"
        CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
            go build -ldflags="-s -w" -o "bin/agent-stream-dbg-${goos}-${goarch}" ./cmd/agent-stream-dbg
    done
    ls -la bin/agent-stream-dbg-*
