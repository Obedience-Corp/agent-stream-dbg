# Live TUI integration recordings.
#
# This file intentionally uses the existing content pack and an externally
# installed TermCast. Generated recording output is written to docs/assets/;
# no tapes or recording tools are vendored by this project.

set shell := ["bash", "-euo", "pipefail", "-c"]

root := justfile_directory()
content := root + "/docs/termcast"
assets := root + "/docs/assets"
termcast := env_var_or_default("TERMCAST", "termcast")
brainyard_binary := "/tmp/stream-debugger-brainyard-tui"
obey_binary := "/tmp/stream-debugger-obey-tui"

default:
    @just --list --justfile {{source_file()}}

# Check the external TermCast/VHS/ffmpeg toolchain.
doctor:
    "{{termcast}}" doctor --content "{{content}}"

# Validate the existing TermCast content pack.
validate:
    "{{termcast}}" validate --content "{{content}}"

# Build the binaries referenced by the existing live tapes outside the repo.
build:
    go build -o "{{brainyard_binary}}" ./cmd/stream-debugger
    go build -o "{{obey_binary}}" ./cmd/stream-debugger

# Require both live backends before attempting a recording.
services:
    #!/usr/bin/env bash
    set -euo pipefail
    if ! nc -z 127.0.0.1 5003; then
        echo "Brainyard is not reachable at 127.0.0.1:5003" >&2
        exit 1
    fi
    if ! test -S /tmp/obey-stream-debugger.sock; then
        echo "Obey is not listening on /tmp/obey-stream-debugger.sock" >&2
        exit 1
    fi

# Record and verify the Brainyard integration flow.
record-brainyard: doctor validate build services
    "{{termcast}}" tui brainyard-live --content "{{content}}" --dest "{{assets}}" --width 1500 --fps 30 --colors 256

# Record and verify the Obey gRPC integration flow.
record-obey: doctor validate build services
    "{{termcast}}" tui obey-grpc --content "{{content}}" --dest "{{assets}}" --width 1500 --fps 30 --colors 256

# Run the live recording integration suite.
integration: record-brainyard record-obey verify

# Check that the recordings are usable README assets.
verify:
    #!/usr/bin/env bash
    set -euo pipefail
    for gif in "{{assets}}/tui-brainyard-live.gif" "{{assets}}/tui-obey-grpc.gif"; do
        test -s "$gif"
        metadata=$(ffprobe -v error -select_streams v:0 -show_entries stream=codec_name,width,height -of csv=p=0 "$gif")
        IFS=, read -r codec width height <<< "$metadata"
        test "$codec" = "gif"
        test "$width" -gt 0
        test "$height" -gt 0
        echo "verified $gif ($metadata)"
    done
