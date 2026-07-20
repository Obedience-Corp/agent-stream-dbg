#!/usr/bin/env bash
# Record docs/assets/tui-acp-demo.gif — offline ACP dialect demo via VHS.
# No network, no API keys: explain + timeline + replay + fixture SSE TUI.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

export SD_ROOT="$ROOT"
export TERM="${TERM:-xterm-256color}"
export COLORTERM=truecolor
export CLICOLOR_FORCE=1
export FORCE_COLOR=1
unset NO_COLOR || true

PORT=18767
ADDR="127.0.0.1:${PORT}"

echo "→ building binaries"
mkdir -p bin docs/assets
go build -o bin/agent-stream-dbg ./cmd/agent-stream-dbg
go build -o bin/fixture-sse-server ./cmd/fixture-sse-server

echo "→ starting ACP fixture SSE server on :${PORT}"
pkill -f "fixture-sse-server.*${PORT}" 2>/dev/null || true
./bin/fixture-sse-server \
  -fixture testdata/fixtures/acp-session.jsonl \
  -delay 90ms \
  -addr "${ADDR}" &
SERVER_PID=$!
cleanup() {
  kill "$SERVER_PID" 2>/dev/null || true
}
trap cleanup EXIT

for _ in $(seq 1 50); do
  if nc -z 127.0.0.1 "${PORT}" 2>/dev/null; then
    break
  fi
  sleep 0.1
done
if ! nc -z 127.0.0.1 "${PORT}" 2>/dev/null; then
  echo "fixture-sse-server failed to listen on ${PORT}" >&2
  exit 1
fi

# Sanity: dialect explain must exit 0 before we burn VHS time.
./bin/agent-stream-dbg explain \
  --dialect dialects/acp.yaml \
  --from testdata/fixtures/acp-session.jsonl \
  >/dev/null

echo "→ recording VHS tape"
vhs demos/tapes/tui-acp-demo.tape

if [[ ! -s docs/assets/tui-acp-demo.gif ]]; then
  echo "missing docs/assets/tui-acp-demo.gif" >&2
  exit 1
fi

ffprobe -v error -select_streams v:0 \
  -show_entries stream=codec_name,width,height,nb_frames \
  -of default=noprint_wrappers=1 \
  docs/assets/tui-acp-demo.gif

echo "✓ wrote docs/assets/tui-acp-demo.gif"
