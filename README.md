# Stream Debugger

**A configuration-driven CLI tool for visualizing and debugging Server-Sent Events (SSE) streaming from any API.**

Real-time TUI for watching streams + timeline analysis for understanding parallel execution patterns.

[![Go Version](https://img.shields.io/badge/go-1.25+-blue.svg)](https://golang.org/dl/)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

---

## Features

🎬 **Real-Time TUI** - Watch SSE streams live in your terminal
📊 **Timeline Visualization** - Analyze parallel execution from logs
⚙️ **Configuration-Driven** - Works with any SSE API via YAML config
📝 **Multi-Dimensional Logging** - Events logged by type, agent, session, and API calls
🎯 **9 Event Types** - Full SSE lifecycle support
🚀 **Performance Tracking** - Tokens/sec, latency, sequence numbers

---

## Installation

### Via Go Install (Recommended)

```bash
go install github.com/lancekrogers/stream-debugger/cmd/stream-debugger@latest
```

### From Source

```bash
git clone https://github.com/lancekrogers/stream-debugger.git
cd stream-debugger
go build -o bin/stream-debugger ./cmd/stream-debugger

# Optional: Install to $GOPATH/bin
go install ./cmd/stream-debugger
```

### From Binary Release

Download the latest release from [GitHub Releases](https://github.com/lancekrogers/stream-debugger/releases).

---

## Quick Start

### 1. Configure

```bash
# Copy example config
cp configs/brainyard-v3.yaml my-config.yaml

# Or create .env for simple setup
cat > .env << EOF
BACKEND_URL=http://localhost:5003
API_KEY=your-api-key
SESSION_ID=debug-session
EOF
```

### 2. Stream (Real-Time TUI)

```bash
stream-debugger stream "What is consciousness?"
```

This opens an **interactive TUI** in your terminal showing live agent responses. Press `q` to exit.

### 3. Analyze (Timeline from Logs)

```bash
stream-debugger timeline logs/by-session/session_*.jsonl
```

This prints a visual timeline showing which agents ran in parallel.

---

## Usage

```bash
# Real-time streaming with TUI
stream-debugger stream "your message"

# With custom config
stream-debugger stream --config my-config.yaml "your message"

# Timeline visualization from logs
stream-debugger timeline logs/by-session/session_*.jsonl

# Detailed event log
stream-debugger replay logs/by-session/session_*.jsonl

# Help
stream-debugger --help
```

---

## Real-Time TUI

When you run `stream stream`, you get a **full-screen interactive terminal UI**:

```
╭────────────────────────────────────────────────╮
│ Stream Debugger - Session: debug-session-001  │
╰────────────────────────────────────────────────╯

╭─────────────────────╮  ╭─────────────────────╮
│ ◉ sam_harris       │  │ ◉ eckhart_tolle    │
│ Tokens: 145        │  │ Tokens: 98         │
│ Seq: 144 | Buf: 0  │  │ Seq: 97 | Buf: 0   │
│                    │  │                    │
│ Consciousness is   │  │ The present moment │
│ fundamentally...   │  │ is where...        │
╰─────────────────────╯  ╰─────────────────────╯

╭────────────────────────────────────────────────╮
│ ◉ Wizard (Synthesis)                          │
│ Tokens: 243 | Seq: 242                        │
│                                                │
│ Integrating both perspectives...              │
╰────────────────────────────────────────────────╯

Events: 487 | Tokens: 486 | Tokens/sec: 24.3 | Errors: 0 | Duration: 20s

[p] pause/resume | [q] quit | [s] save
```

**Controls:**
- `p` - Pause/resume display
- `q` or `Ctrl+C` - Quit and save logs
- `s` - Save session (future)

---

## Timeline Analysis

After streaming, analyze what happened:

```bash
stream-debugger timeline logs/by-session/session_*.jsonl
```

**Output:**

```
📊 Timeline View - Parallel Execution Visualization

Duration: 5.234s | Events: 487 | Agents: 3

Time (ms)  sam_harris      eckhart_tolle   wizard
─────────────────────────────────────────────────────────
      0    ▶ START         │               │
    100    █                ▶ START         │
    200    █                █               │
    500    ■ DONE           █               │
    700    │                ■ DONE          ▶ START
   1000    │                │               ■ DONE

Legend: ▶ START  █ Streaming  ■ DONE  ✗ ERROR  │ Idle

🔀 Parallel Execution Summary

sam_harris ran in parallel with: eckhart_tolle
eckhart_tolle ran in parallel with: sam_harris
wizard ran sequentially after all agents
```

---

## Configuration

Stream Debugger uses YAML configuration files to work with any SSE API.

### Example Config

```yaml
# configs/my-api.yaml
backend:
  base_url: "http://localhost:5003"

  stream_endpoint:
    url: "/api/stream"
    method: "POST"

    message_format:
      type: "json_body"
      body_template: '{"prompt": "{message}", "stream": true}'

    auth:
      type: "bearer"
      token_env: "API_KEY"

events:
  types:
    - "session_start"
    - "agent_content"
    - "error"

logging:
  dir: "./logs"
  dimensions:
    by_event_type: true
    by_agent: true
    by_session: true
    api_calls: true
```

See [`config.yaml.example`](config.yaml.example) for all options.

### Pre-configured Examples

- `configs/brainyard-v3.yaml` - BrainyardV3 multi-agent system
- `configs/generic-sse.yaml` - Generic SSE API template

---

## Log Files

All events are automatically logged to 4 categories:

```
logs/
├── by-event-type/       # All events of one type
│   ├── session_start.jsonl
│   ├── agent_content.jsonl
│   └── error.jsonl
├── by-agent/            # All events from one agent
│   ├── sam_harris.jsonl
│   └── wizard.jsonl
├── by-session/          # Complete session timelines
│   └── session_xxx_20250128.jsonl
└── api-calls/           # HTTP requests and responses
    └── http_20250128.jsonl
```

### Analyzing Logs

```bash
# See which agents responded
ls logs/by-agent/

# Read full conversation
cat logs/by-session/session_*.jsonl | jq -r '.event.content' | tr -d '\n'

# Count events by type
wc -l logs/by-event-type/*.jsonl

# Find errors
cat logs/by-event-type/error.jsonl | jq
```

---

## Documentation

- **User Guides**
  - [Quick Start](docs/user-guide/quickstart.md)
  - [Getting Started with BrainyardV3](docs/user-guide/getting-started-brainyard.md)
  - [TUI vs Timeline Mode](docs/user-guide/tui-vs-timeline.md)

- **Development**
  - [Implementation Details](docs/development/implementation.md)
  - [Testing Guide](docs/development/testing.md)

- **Configuration**
  - [`config.yaml.example`](config.yaml.example) - Full config reference
  - [`configs/`](configs/) - Example configurations

---

## Development

For contributors, we use `just` for development convenience:

```bash
# Install dependencies
just deps

# Build binary
just build              # Creates bin/stream-debugger

# Build and sign for macOS
just build-signed       # Includes code signing

# Run tests
just test

# Development shortcuts (calls the binary)
just stream "test message"        # Quick test
just timeline logs/session_*.jsonl
```

### Code Signing (macOS)

```bash
# Ad-hoc signing for local use
just sign-macos

# Or manually with Developer ID for distribution
codesign --sign "Developer ID Application: Your Name" bin/stream-debugger
```

---

## Use Cases

- **Multi-Agent Debugging** - See which agents run in parallel
- **Performance Analysis** - Identify bottlenecks and slowdowns
- **API Integration Testing** - Validate SSE event flows
- **Production Monitoring** - Real-time streaming observability
- **Log Analysis** - Understand complex multi-agent conversations

---

## Architecture

```
stream-debugger/
├── cmd/stream-debugger/  # CLI entry point
├── internal/
│   ├── client/          # SSE client (config-driven)
│   ├── config/          # YAML config loading
│   ├── events/          # Event type definitions
│   ├── logger/          # Multi-dimensional logging
│   └── visualizer/      # TUI + Timeline views
├── configs/             # Pre-configured setups
├── docs/                # Documentation
└── logs/                # Generated logs (gitignored)
```

---

## Contributing

Contributions welcome! Please:

1. Fork the repository
2. Create a feature branch
3. Add tests for new functionality
4. Run `just test` to ensure all tests pass
5. Submit a pull request

---

## License

MIT License - see [LICENSE](LICENSE) for details.

---

## Credits

Built for debugging complex SSE streaming systems. Originally created for [BrainyardV3](https://github.com/lancekrogers/BrainyardV3) but designed to be generic and reusable.

**Technologies:**
- [Bubbletea](https://github.com/charmbracelet/bubbletea) - Terminal UI framework
- [Lipgloss](https://github.com/charmbracelet/lipgloss) - Terminal styling
- [Zerolog](https://github.com/rs/zerolog) - Fast structured logging
- [r3labs/sse](https://github.com/r3labs/sse) - SSE client library

---

**Made for debugging SSE streaming systems** 🚀

Visualize parallel execution • Real-time monitoring • Configuration-driven • Log analysis
