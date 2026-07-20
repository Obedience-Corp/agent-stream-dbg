#!/usr/bin/env bash
# Record docs/assets/tui-obey-activity.gif against live Obey WatchAllActivity
# while firing a short discussion turn so AGENT_MESSAGE_DELTA drives the energy strip.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export SD_ROOT="$ROOT"

SOCK="${OBEY_SOCKET:-/tmp/obey.sock}"
CAMPAIGN="${OBEY_CAMPAIGN_ID:-8a57dff6-753a-41c2-be15-47c3d8fc4ca8}" # My_Tools
if [[ ! -S "$SOCK" ]]; then
  echo "Obey socket not found at $SOCK (start: obey serve)" >&2
  exit 1
fi
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

echo "→ create discussion session (ollama)"
CREATE=$(grpcurl -plaintext -d "{
  \"campaign_id\": \"$CAMPAIGN\",
  \"provider\": \"ollama\",
  \"model\": \"llama3.2:latest\",
  \"agent_name\": \"sd-activity-demo\",
  \"working_dir\": \"/Users/lancerogers/Dev/AI/My_Tools\",
  \"mode\": \"discussion\"
}" "unix://$SOCK" local.v1.LocalDaemonService/CreateSession)
SID=$(printf '%s' "$CREATE" | python3 -c 'import json,sys; print(json.load(sys.stdin)["session"]["id"])')
echo "  session=$SID"

cleanup() {
  # Best-effort stop so we don't leave demo sessions running.
  grpcurl -plaintext -d "{\"session_id\":\"$SID\"}" \
    "unix://$SOCK" local.v1.LocalDaemonService/StopSession >/dev/null 2>&1 || true
}
trap cleanup EXIT

echo "→ building agent-stream-dbg"
mkdir -p bin docs/assets
go build -o bin/agent-stream-dbg ./cmd/agent-stream-dbg

# Fire turns only after the VHS tape has opened the activity watch.
# Tape timeline from Show: export/cd ~2s, launch binary ~2s, Sleep 2s,
# type prompt ~3s + Enter, then Sleep 16s. First SendMessage at ~14s so
# the subscription is live before AGENT_MESSAGE_DELTA hits the wire.
(
  sleep 14
  echo "→ SendMessage #1 (content deltas)"
  grpcurl -plaintext -d "{
    \"session_id\": \"$SID\",
    \"campaign_id\": \"$CAMPAIGN\",
    \"message\": \"In one short friendly sentence, say hello and mention you are streaming live.\",
    \"mode\": \"discussion\",
    \"client_ref\": \"vhs-obey-activity\"
  }" "unix://$SOCK" local.v1.LocalDaemonService/SendMessage >/tmp/obey-activity-send.json 2>/tmp/obey-activity-send.err || true
  sleep 4
  echo "→ SendMessage #2"
  grpcurl -plaintext -d "{
    \"session_id\": \"$SID\",
    \"campaign_id\": \"$CAMPAIGN\",
    \"message\": \"One more short sentence about tokens on the wire.\",
    \"mode\": \"discussion\",
    \"client_ref\": \"vhs-obey-activity-2\"
  }" "unix://$SOCK" local.v1.LocalDaemonService/SendMessage >/tmp/obey-activity-send2.json 2>/tmp/obey-activity-send2.err || true
) &
SENDER_PID=$!


echo "→ recording live Obey activity VHS"
vhs demos/tapes/tui-obey-activity.tape

wait "$SENDER_PID" 2>/dev/null || true

test -s docs/assets/tui-obey-activity.gif
ffprobe -v error -select_streams v:0 \
  -show_entries stream=codec_name,width,height,nb_frames \
  -of default=noprint_wrappers=1 \
  docs/assets/tui-obey-activity.gif
echo "✓ wrote docs/assets/tui-obey-activity.gif"
