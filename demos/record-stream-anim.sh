#!/usr/bin/env bash
# Record docs/assets/tui-stream-anim.gif via VHS against an offline fixture SSE server.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export SD_ROOT="$ROOT"
export TERM="${TERM:-xterm-256color}"
export COLORTERM=truecolor
export CLICOLOR_FORCE=1
export FORCE_COLOR=1
export STREAM_DEBUGGER_COLOR_PROFILE=truecolor
# Agent shells often export NO_COLOR=1 — kill it so VHS/lipgloss paint.
unset NO_COLOR || true
# Ensure motion is on for the demo.
unset STREAM_DEBUGGER_REDUCED_MOTION NO_MOTION || true

echo "→ building binaries"
mkdir -p bin docs/assets
go build -o bin/stream-debugger ./cmd/stream-debugger
go build -o bin/fixture-sse-server ./cmd/fixture-sse-server

echo "→ starting fixture SSE server on :18765"
pkill -f 'fixture-sse-server.*18765' 2>/dev/null || true
./bin/fixture-sse-server \
  -fixture testdata/fixtures/brainyard-session.jsonl \
  -delay 160ms \
  -addr 127.0.0.1:18765 &
SERVER_PID=$!
cleanup() {
  kill "$SERVER_PID" 2>/dev/null || true
}
trap cleanup EXIT

# Wait for port.
for _ in $(seq 1 50); do
  if nc -z 127.0.0.1 18765 2>/dev/null; then
    break
  fi
  sleep 0.1
done
if ! nc -z 127.0.0.1 18765 2>/dev/null; then
  echo "fixture-sse-server failed to listen on 18765" >&2
  exit 1
fi

echo "→ recording VHS tape"
# vhs resolves Output relative to CWD; tape paths assume repo root.
vhs demos/tapes/tui-stream-anim.tape

if [[ ! -s docs/assets/tui-stream-anim.gif ]]; then
  echo "missing docs/assets/tui-stream-anim.gif" >&2
  exit 1
fi

ffprobe -v error -select_streams v:0 \
  -show_entries stream=codec_name,width,height,nb_frames \
  -of default=noprint_wrappers=1 \
  docs/assets/tui-stream-anim.gif

echo "✓ wrote docs/assets/tui-stream-anim.gif"
