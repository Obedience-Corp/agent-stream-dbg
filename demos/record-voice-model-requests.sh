#!/usr/bin/env bash
# Record model requests from the synthetic contract fixture; no runtime, models or audio.
set -euo pipefail
cd "$(dirname "$0")/.."
go build -o bin/voice-model-requests ./cmd/agent-stream-dbg
env -u NO_COLOR -u CLICOLOR CLICOLOR_FORCE=1 FORCE_COLOR=1 TERM=xterm-256color COLORTERM=truecolor vhs demos/tapes/voice-model-requests.tape
