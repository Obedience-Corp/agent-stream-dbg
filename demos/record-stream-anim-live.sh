#!/usr/bin/env bash
# Record docs/assets/tui-stream-anim.gif against live Brainyard inference.
# Requires: Brainyard on localhost:5003 and API_KEY (or a sourced .env).
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export SD_ROOT="$ROOT"

# Prefer Brainyard tools env if present (never printed).
if [[ -z "${API_KEY:-}" ]]; then
  for f in \
    "$HOME/Dev/AI/Brainyard/projects/BrainyardV3/tools/stream-debugger/.env" \
    "$ROOT/.env" \
    "$(dirname "$ROOT")/stream-debugger/.env"; do
    if [[ -f "$f" ]]; then
      # shellcheck disable=SC1090
      set -a; source "$f"; set +a
      break
    fi
  done
fi
if [[ -z "${API_KEY:-}" ]]; then
  echo "API_KEY required for live recording" >&2
  exit 1
fi
if ! nc -z 127.0.0.1 5003 2>/dev/null; then
  echo "Brainyard not reachable at 127.0.0.1:5003" >&2
  exit 1
fi

export TERM="${TERM:-xterm-256color}"
export COLORTERM=truecolor
export CLICOLOR_FORCE=1
export FORCE_COLOR=1
export STREAM_DEBUGGER_COLOR_PROFILE=truecolor
unset NO_COLOR STREAM_DEBUGGER_REDUCED_MOTION NO_MOTION || true

echo "→ building stream-debugger"
mkdir -p bin docs/assets
go build -o bin/stream-debugger ./cmd/stream-debugger

echo "→ recording live Brainyard VHS (API_KEY present, not printed)"
vhs demos/tapes/tui-stream-anim-live.tape

test -s docs/assets/tui-stream-anim.gif
ffprobe -v error -select_streams v:0 \
  -show_entries stream=codec_name,width,height,nb_frames \
  -of default=noprint_wrappers=1 \
  docs/assets/tui-stream-anim.gif
echo "✓ wrote docs/assets/tui-stream-anim.gif (live inference)"
