#!/usr/bin/env bash
# Record home-ux design prototype VHS tapes → docs/assets/home-ux-*.gif
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

export SD_ROOT="$ROOT"
export TERM="${TERM:-xterm-256color}"
export COLORTERM=truecolor
export CLICOLOR_FORCE=1
export FORCE_COLOR=1
unset NO_COLOR || true

mkdir -p bin docs/assets

echo "→ building home-ux prototype"
go build -o bin/home-ux ./demos/home-ux

which vhs >/dev/null || {
  echo "vhs not found on PATH" >&2
  exit 1
}

run_tape() {
  local tape="$1"
  echo "→ recording $(basename "$tape")"
  vhs "$tape"
}

case "${1:-all}" in
  first-run|01)
    run_tape demos/home-ux/tapes/01-first-run.tape
    ;;
  select-open|02)
    run_tape demos/home-ux/tapes/02-select-open.tape
    ;;
  new-config|03)
    run_tape demos/home-ux/tapes/03-new-config.tape
    ;;
  all)
    run_tape demos/home-ux/tapes/01-first-run.tape
    run_tape demos/home-ux/tapes/02-select-open.tape
    run_tape demos/home-ux/tapes/03-new-config.tape
    ;;
  *)
    echo "usage: $0 [all|first-run|select-open|new-config]" >&2
    exit 2
    ;;
esac

echo "✓ gifs:"
ls -la docs/assets/home-ux-*.gif 2>/dev/null || true
