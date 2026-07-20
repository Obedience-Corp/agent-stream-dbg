#!/usr/bin/env bash
# Record docs/assets/tui-obey-live.gif against a live local Obey daemon.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export SD_ROOT="$ROOT"

SOCK="${OBEY_SOCKET:-/tmp/obey.sock}"
if [[ ! -S "$SOCK" ]]; then
  echo "Obey socket not found at $SOCK (start: obey serve)" >&2
  exit 1
fi

# Quick connectivity check
if ! grpcurl -plaintext "unix://$SOCK" list >/dev/null 2>&1; then
  echo "Cannot list gRPC services on unix://$SOCK" >&2
  exit 1
fi

export TERM="${TERM:-xterm-256color}"
export COLORTERM=truecolor
export CLICOLOR_FORCE=1
export FORCE_COLOR=1
export STREAM_DEBUGGER_COLOR_PROFILE=truecolor
unset NO_COLOR STREAM_DEBUGGER_REDUCED_MOTION NO_MOTION || true

echo "→ probe live Obey stream"
go run demos/obey_live_probe.go demos/configs/obey-live-demo.yaml

echo "→ building agent-stream-dbg"
mkdir -p bin docs/assets
go build -o bin/agent-stream-dbg ./cmd/agent-stream-dbg

echo "→ recording live Obey VHS (socket=$SOCK)"
vhs demos/tapes/tui-obey-live.tape

test -s docs/assets/tui-obey-live.gif
ffprobe -v error -select_streams v:0 \
  -show_entries stream=codec_name,width,height,nb_frames \
  -of default=noprint_wrappers=1 \
  docs/assets/tui-obey-live.gif
echo "✓ wrote docs/assets/tui-obey-live.gif"
