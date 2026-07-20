# Stream Debugger

CLI debugger for SSE and gRPC streams. Point it at a live endpoint (or a
recorded fixture), watch events in a TUI, and analyze parallel multi-agent
execution from logs.

[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

## Install

```bash
go install github.com/lancekrogers/stream-debugger/cmd/stream-debugger@latest
```

From source:

```bash
git clone https://github.com/lancekrogers/stream-debugger.git
cd stream-debugger
just install   # or: go install ./cmd/stream-debugger
```

## Quick start

No backend required — offline demos against bundled fixtures:

```bash
just demo       # multi-agent timeline (bundled fixture)
just demo-acp   # ACP dialect: explain + timeline + replay
```

Connect to a stream with a YAML run config:

```bash
cp config.yaml.example my-config.yaml
# edit transport, dialect, auth
stream-debugger --config my-config.yaml
```

Running without `--config` creates a starter config and opens the TUI
configuration panel. Press `Ctrl+S` to save.

Single-shot stream (useful for scripts):

```bash
stream-debugger stream --config my-config.yaml "your message"
```

## Demos

Recordings under [`docs/assets/`](docs/assets/) show different ways to use the
tool. Offline ones need no API key or live service.

### Multi-agent TUI

Live multi-agent stream with agent lanes (fixture-backed recording):

<p align="center">
  <img src="docs/assets/tui-brainyard-live.gif" alt="Multi-agent stream TUI" width="900">
</p>

### First-run configuration

Launch without a config — setup panel, save with `Ctrl+S`:

<p align="center">
  <img src="docs/assets/tui-first-run-config.gif" alt="First-run configuration panel" width="900">
</p>

### TUI motion chrome

Energy strip, agent pulses, and flow-stage animation during a stream
(`demos/record-stream-anim.sh`):

<p align="center">
  <img src="docs/assets/tui-stream-anim.gif" alt="TUI stream animation and motion chrome" width="900">
</p>

### ACP dialect (explain, timeline, replay)

Decode Agent Client Protocol JSON-RPC sessions offline — `just demo-acp` /
`just record-acp`:

<p align="center">
  <img src="docs/assets/tui-acp-demo.gif" alt="ACP dialect explain, timeline, and TUI demo" width="900">
</p>

### gRPC activity stream

gRPC multi-agent activity view (local daemon; not a public hosted service):

<p align="center">
  <img src="docs/assets/tui-obey-activity.gif" alt="gRPC multi-agent activity TUI" width="900">
</p>

Also available: [`tui-obey-grpc.gif`](docs/assets/tui-obey-grpc.gif) (campaign-state
stream setup) and [`tui-obey-live.gif`](docs/assets/tui-obey-live.gif).
## Commands

| Command | Purpose |
|---------|---------|
| `stream-debugger --config <file>` | Interactive TUI (default) |
| `stream-debugger stream --config <file> "msg"` | One message, then exit |
| `stream-debugger timeline <session.jsonl>` | Parallel execution timeline |
| `stream-debugger replay <session.jsonl>` | Replay a recorded session |
| `stream-debugger explain ...` | Trace dialect matching per frame |
| `stream-debugger init ...` | Draft a dialect from a stream or recording |

Flags for `stream` must come **before** the message argument.

In the TUI: type and Enter to send, `Ctrl+T` toggles RAW (wire) vs PARSED
(agent-organized) views, `c` opens the config panel, `Ctrl+C` quits.

## Configuration

A run config selects transport and dialect. Transports:

- `sse` — HTTP Server-Sent Events
- `grpc` — streaming RPCs (reflection, descriptor set, or `.proto`)
- `replay` — JSONL fixture through the same pipeline

Example:

```yaml
transport:
  type: sse
  base_url: "http://localhost:5003"
  stream_endpoint:
    url: "/api/stream"
    method: "POST"
    auth:
      type: bearer
      token_env: API_KEY

dialect:
  file: openai

logging:
  dir: "./logs"
```

Auth is opt-in: omit `auth:` to send no credential. See
[`config.yaml.example`](config.yaml.example) for the full field set.

### Dialects

| Dialect | Wire | Offline without a live backend |
|---------|------|--------------------------------|
| `openai` | Chat Completions SSE | `testdata/fixtures/openai-chat.jsonl` |
| `anthropic` | Messages SSE | `testdata/fixtures/anthropic-messages.jsonl` |
| `a2a` | Agent2Agent | `testdata/fixtures/a2a-session.jsonl` |
| `acp` | Agent Client Protocol JSON-RPC | `just demo-acp` |
| `brainyard` | Private multi-agent SSE | Fixture only — backend is **not** public |
| `obey` / `obey-activity` | Private Obey gRPC daemon | Needs a local `obey serve` socket |

Dialects are YAML data embedded in the binary. Private backends (Brainyard,
Obey) are **not** published with this repo; their dialects ship so you can
decode recorded streams and so CI can golden-test them. Public use starts
from `openai` / `anthropic` / `a2a` / `acp`, or `stream-debugger init` against
your own recording.

### Example configs

| Path | Notes |
|------|--------|
| `config.yaml.example` | Full field reference |
| `demos/configs/acp-demo.yaml` | Offline ACP fixture SSE demo |
| `configs/brainyard-v3.yaml` | Shape sample for a private multi-agent SSE API |
| `configs/obey-grpc.yaml` | Shape sample for a local Obey daemon (not a public service) |
| `configs/obey-activity-grpc.yaml` | Shape sample for Obey activity streams |

## Logging

Sessions write under `logs/` by event type, agent, session, and API call.
Analyze with `timeline` / `replay`, or inspect the JSONL directly.

## Documentation

- [USAGE.md](USAGE.md) — modes, keyboard controls, troubleshooting
- [Quick start](docs/user-guide/quickstart.md)
- [TUI vs timeline](docs/user-guide/tui-vs-timeline.md)
- [Implementation](docs/development/implementation.md)
- [Testing](docs/development/testing.md)

## Development

```bash
just deps
just build
just test
```

`just --list` shows all recipes (demo, race, lint, multi-platform builds, etc.).

VHS recordings:

```bash
just record-acp                 # docs/assets/tui-acp-demo.gif
bash demos/record-stream-anim.sh  # docs/assets/tui-stream-anim.gif
# Live gRPC/SSE tapes: demos/record-obey-*.sh, vhs.just (need local services)
```

## Contributing

1. Fork and branch
2. Add tests for new behavior
3. Run `just test`
4. Open a pull request

## License

Apache License 2.0 — see [LICENSE](LICENSE).
