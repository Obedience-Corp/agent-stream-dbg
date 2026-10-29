#!/usr/bin/env bash
# Record the actual loopback source and shared TUI; no providers or audio.
set -euo pipefail
voice_repo="${1:?usage: record-live-voice.sh /path/to/obey-voice}"
voice_repo="$(cd "$voice_repo" && pwd)"
cd "$(dirname "$0")/.."
fixture_root="$(mktemp -d /tmp/live-voice-review.XXXXXX)"
source_pid=""
cleanup() { if [[ -n "$source_pid" ]]; then kill "$source_pid" 2>/dev/null || true; wait "$source_pid" 2>/dev/null || true; fi; rm -rf "$fixture_root"; }
trap cleanup EXIT
export OBEY_VOICE_LIVE_DEMO_ROOT="$fixture_root/config"
(cd "$voice_repo" && go test -c -o "$fixture_root/source" ./internal/tui)
go test -c -o bin/live-voice-terminal ./cmd/agent-stream-dbg
"$fixture_root/source" -test.run '^TestLiveDebuggerTerminalSource$' -test.timeout 90s > "$fixture_root/source.log" 2>&1 &
source_pid=$!
for _ in {1..100}; do
 if compgen -G "$OBEY_VOICE_LIVE_DEMO_ROOT/debug/runtimes/*.json" >/dev/null; then break; fi
 if ! kill -0 "$source_pid" 2>/dev/null; then cat "$fixture_root/source.log"; exit 1; fi
 sleep .1
done
env -u NO_COLOR -u CLICOLOR CLICOLOR_FORCE=1 FORCE_COLOR=1 TERM=xterm-256color COLORTERM=truecolor vhs demos/tapes/live-voice.tape
