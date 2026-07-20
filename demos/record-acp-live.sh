#!/usr/bin/env bash
# Record docs/assets/tui-acp-live.gif — multi-turn ACP session reuse via VHS.
# Offline: uses cmd/demo-acp-agent (no network, no API keys).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export SD_ROOT="$ROOT"
export TERM="${TERM:-xterm-256color}"
export COLORTERM=truecolor
export CLICOLOR_FORCE=1
export FORCE_COLOR=1
unset NO_COLOR || true

echo "→ building binaries"
mkdir -p bin docs/assets
go build -o bin/stream-debugger ./cmd/stream-debugger
go build -o bin/demo-acp-agent ./cmd/demo-acp-agent

# Sanity: multi-turn ACP transport (no TTY / no LLM required).
echo "→ smoke multi-turn ACP transport tests"
go test ./internal/transport/acp/ -run 'MultiTurn|ConnectHandshake' -count=1


echo "→ recording VHS tape"
vhs demos/tapes/tui-acp-live.tape

if [[ ! -s docs/assets/tui-acp-live.gif ]]; then
  echo "missing docs/assets/tui-acp-live.gif" >&2
  exit 1
fi

ffprobe -v error -select_streams v:0 \
  -show_entries stream=codec_name,width,height,nb_frames \
  -of default=noprint_wrappers=1 \
  docs/assets/tui-acp-live.gif

echo "✓ wrote docs/assets/tui-acp-live.gif"
