#!/usr/bin/env bash
# Exercise the real home/observer loop with disposable fixture recordings.
set -euo pipefail
cd "$(dirname "$0")/.."
go test -c -o bin/voice-launcher-terminal ./cmd/agent-stream-dbg
vhs demos/tapes/voice-launcher.tape
