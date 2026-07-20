# Stream Debugger

TUI debugger for **agent and multi-agent streams**.

Point it at a live backend or a recorded fixture, watch frames in a multi-pane
terminal UI, and decode them through a **dialect** (what the bytes mean).

| Transport | What it is |
|-----------|------------|
| **SSE** | HTTP Server-Sent Events |
| **gRPC** | Streaming RPCs (reflection, descriptor set, or `.proto`) |
| **ACP** | [Agent Client Protocol](https://agentclientprotocol.com/) over stdio |
| **Replay** | JSONL fixture through the same pipeline (offline / CI) |

[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/license-Apache%202.0-blue.svg)](LICENSE)

## Install

```bash
go install github.com/Obedience-Corp/stream-debugger/cmd/stream-debugger@latest
```

From source:

```bash
git clone https://github.com/Obedience-Corp/stream-debugger.git
cd stream-debugger
just install   # or: go install ./cmd/stream-debugger
```

## Quick start

```bash
stream-debugger
```

Bare launch opens the **home hub**:

1. Create or pick a run configuration  
2. Or try an **offline demo** (bundled fixtures, no network)  
3. Open an interactive session  
4. Quit the session → return to the home hub  

<p align="center">
  <img src="docs/assets/home-ux-first-run.gif" alt="Home hub first launch" width="900">
</p>

| Key | Home hub |
|-----|----------|
| `↑` `↓` / `j` `k` | Move |
| `Enter` | Open session (or edit if incomplete) |
| `n` | New configuration wizard |
| `e` | Edit selected config |
| `d` | Delete |
| `q` | Quit |

Jump straight into interactive mode with a YAML file (demos, scripts, CI):

```bash
stream-debugger --config my-config.yaml
# or
stream-debugger -c demos/configs/acp-demo.yaml
```

Single-shot stream (no home hub):

```bash
stream-debugger stream --config my-config.yaml "your message"
```

From a clone, CLI offline recipes without the TUI:

```bash
just demo       # multi-agent timeline (bundled fixture)
just demo-acp   # ACP explain + timeline + replay
```

## Demos

Recordings under [`docs/assets/`](docs/assets/). Offline ones need no API key
or live service.

### Home hub

First run, multi-config select, and new-config wizard:

| Flow | Recording |
|------|-----------|
| Empty first launch | [`home-ux-first-run.gif`](docs/assets/home-ux-first-run.gif) |
| Select → open → return home | [`home-ux-select-open.gif`](docs/assets/home-ux-select-open.gif) |
| New configuration wizard | [`home-ux-new-config.gif`](docs/assets/home-ux-new-config.gif) |

### Multi-agent TUI

Live multi-agent stream with agent lanes (fixture-backed recording):

<p align="center">
  <img src="docs/assets/tui-brainyard-live.gif" alt="Multi-agent stream TUI" width="900">
</p>

### TUI motion chrome

Energy strip, agent pulses, and flow-stage animation
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

### ACP live multi-turn (session reuse)

Interactive TUI over stdio ACP: two prompts, same agent process and
`sessionId` — `just record-acp-live` (uses `cmd/demo-acp-agent`, no network):

<p align="center">
  <img src="docs/assets/tui-acp-live.gif" alt="ACP live multi-turn session reuse" width="900">
</p>

### gRPC activity stream

gRPC multi-agent activity view (local daemon; not a public hosted service):

<p align="center">
  <img src="docs/assets/tui-obey-activity.gif" alt="gRPC multi-agent activity TUI" width="900">
</p>

Also: [`tui-obey-grpc.gif`](docs/assets/tui-obey-grpc.gif),
[`tui-obey-live.gif`](docs/assets/tui-obey-live.gif).

## Commands

| Command | Purpose |
|---------|---------|
| `stream-debugger` | Home hub (default) |
| `stream-debugger -c <file>` | Interactive TUI with a run config |
| `stream-debugger stream -c <file> "msg"` | One message, then exit |
| `stream-debugger timeline <session.jsonl>` | Parallel execution timeline |
| `stream-debugger replay <session.jsonl>` | Replay a recorded session |
| `stream-debugger explain ...` | Trace dialect matching per frame |
| `stream-debugger init ...` | Draft a dialect from a stream or recording |

Flags for `stream` must come **before** the message argument.

### Interactive TUI keys

| Key | Action |
|-----|--------|
| Type + Enter | Send (insert mode: `i`) |
| `1`–`5` | Switch panes (flow, app, timeline, events, …) |
| `Ctrl+T` | Toggle RAW (wire) vs PARSED (agent-organized) |
| `c` | Open config panel |
| `Ctrl+S` | Save config (in panel) |
| `Ctrl+C` / quit | Leave session (returns to home when launched from hub) |

See [USAGE.md](USAGE.md) for the full keyboard map and troubleshooting.

## Configuration

A **run config** is YAML that picks a transport and a dialect. Create one in
the home hub wizard, or copy [`config.yaml.example`](config.yaml.example).

Configs are loaded from:

- The path you pass with `-c` / `--config`
- Well-known names in the working directory (`stream-debugger.yaml`, `config.yaml`, …)
- `configs/*.yaml` in the working directory
- `~/.config/stream-debugger/*.yaml` (user library used by the home hub)

### Transports

| Type | Role |
|------|------|
| `sse` | HTTP Server-Sent Events |
| `grpc` | Streaming RPCs |
| `acp` | ACP agent process over stdio (multi-turn reuses the process) |
| `replay` | JSONL fixture path in `base_url` (offline / demos) |

Example (SSE + OpenAI-style dialect):

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

Auth is opt-in: omit `auth:` to send no credential. Full field set:
[`config.yaml.example`](config.yaml.example).

### Dialects

Dialects are YAML **data** embedded in the binary — they define how frames
map to agents, tools, and content.

| Dialect | Wire | Offline without a live backend |
|---------|------|--------------------------------|
| `openai` | Chat Completions SSE | `testdata/fixtures/openai-chat.jsonl` |
| `anthropic` | Messages SSE | `testdata/fixtures/anthropic-messages.jsonl` |
| `a2a` | Agent2Agent | `testdata/fixtures/a2a-session.jsonl` |
| `acp` | Agent Client Protocol JSON-RPC | Home offline demos; `just demo-acp`; live: `configs/acp-stdio.yaml` |
| `brainyard` | Private multi-agent SSE | Fixture / home offline demo — backend is **not** public |
| `obey` / `obey-activity` | Private Obey gRPC daemon | Needs a local `obey serve` socket |

Private backends (Brainyard, Obey) are **not** published with this repo; their
dialects ship so you can decode recorded streams and so CI can golden-test
them. Public use starts from `openai` / `anthropic` / `a2a` / `acp`, or
`stream-debugger init` against your own recording.

### Example configs

| Path | Notes |
|------|--------|
| `config.yaml.example` | Full field reference |
| `demos/configs/acp-demo.yaml` | Offline ACP fixture SSE demo |
| `configs/acp-stdio.yaml` | Live ACP agent over stdio |
| `configs/brainyard-v3.yaml` | Shape sample for a private multi-agent SSE API |
| `configs/obey-grpc.yaml` | Shape sample for a local Obey daemon |
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

`just --list` shows the top-level recipes. Use `just test`, `just release`, and
`just vhs` to list the recipes in those focused modules.

Release a tagged version with generated GitHub notes after merging the changes
that should ship:

```bash
just release create v0.1.0
```

For a staged release, use `just release tag v0.1.0` followed by
`just release publish v0.1.0`. Build release binaries with
`just release build-all-platforms`. Live TUI recording recipes are grouped
under `just vhs`; the source file is `.justfiles/vhs.just`.

VHS recordings:

```bash
just record-home-ux               # docs/assets/home-ux-*.gif
just record-acp                   # docs/assets/tui-acp-demo.gif
just record-acp-live              # docs/assets/tui-acp-live.gif
bash demos/record-stream-anim.sh  # docs/assets/tui-stream-anim.gif
# Live gRPC/SSE tapes: demos/record-obey-*.sh, .justfiles/vhs.just (need local services)
```

## Contributing

1. Fork and branch  
2. Add tests for new behavior  
3. Run `just test`  
4. Open a pull request  

## License

Apache License 2.0 — see [LICENSE](LICENSE).
