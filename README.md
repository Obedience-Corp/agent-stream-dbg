# Stream Debugger

A **configuration-driven** real-time Terminal UI (TUI) debugger for visualizing Server-Sent Events (SSE) streaming from any API. Built for BrainyardV3 but designed to work with any SSE streaming system.

## 🎯 Key Features

### 1. Configuration-Driven

- **Pluggable configs** for different SSE APIs
- Define how to connect, authenticate, and parse events
- No code changes needed for new APIs

### 2. Real-Time Visualization

- Beautiful TUI showing parallel agent streams
- Live stats: tokens/sec, latency, event counts
- Color-coded agents with sequence tracking

### 3. Timeline Analysis

- **Visualize parallel execution** from log files
- See exactly when agents ran simultaneously
- Three views: Timeline, Detailed Log, Parallel Summary

### 4. Multi-Dimensional Logging

- Events logged to 4 categories simultaneously:
  - By event type
  - By agent
  - By session (full timeline)
  - API calls

## 🚀 Quick Start

### 1. Setup

```bash
cd stream-debugger
cp config.yaml.example config.yaml
# Edit config.yaml with your API details

# Or use a pre-configured setup
cp configs/brainyard-v3.yaml config.yaml
```

### 2. Stream (Real-Time)

```bash
# Using default config
just stream "What is consciousness?"

# Using specific config
just stream "Your question" "configs/brainyard-v3.yaml"

# Or directly
./bin/debugger stream --config configs/brainyard-v3.yaml "Your question"
```

### 3. Analyze (From Logs)

```bash
# Timeline visualization (parallel execution)
just timeline logs/by-session/session_*.jsonl

# Detailed sequential log
./bin/debugger replay logs/by-session/session_*.jsonl
```

## 📋 Configuration

### Configuration Files

```
stream-debugger/
├── config.yaml.example     # Full template with all options
├── configs/
│   ├── brainyard-v3.yaml  # BrainyardV3 multi-agent system
│   └── generic-sse.yaml   # Generic SSE API template
```

### Key Configuration Sections

#### Backend Configuration

```yaml
backend:
  base_url: "http://localhost:5003"

  stream_endpoint:
    url: "/api/v3/sessions/{session_id}/stream"
    method: "GET" # or POST

    message_format:
      type: "query_param" # or json_body, form_data
      param_name: "message"
```

#### Authentication

```yaml
auth:
  type: "bearer" # or api_key, basic, none
  token_env: "API_KEY"
```

#### Event Configuration

```yaml
events:
  types: # Which events to track
    - "session_start"
    - "agent_content"
    - "error"

  field_mappings: # Map API fields to standard names
    type: "type"
    agent_id: "agent_id"
    content: "content"
```

See `config.yaml.example` for full documentation.

## 🎨 Real-Time TUI

When streaming, you'll see:

```
╭─────────────────────────────────────────────╮
│ Stream Debugger - Session: test-session-001│
╰─────────────────────────────────────────────╯

╭───────────────────╮  ╭───────────────────╮
│ ◉ sam_harris     │  │ ◉ eckhart_tolle  │
│ Tokens: 145 | Seq: 144                   │
│ Buffered: 0      │  │ Buffered: 0      │
│                  │  │                  │
│ ...consciousness │  │ ...present moment│
╰───────────────────╯  ╰───────────────────╯

╭────────────────────────────────────────────╮
│ ◉ Wizard (Synthesis)                      │
│ Tokens: 243 | Seq: 242                    │
│                                            │
│ ...integrating both perspectives...       │
╰────────────────────────────────────────────╯

Events: 487 | Tokens: 486 | Tokens/sec: 24.3 | Errors: 0 | Duration: 20s

[p] pause/resume | [q] quit | [s] save session
```

## 📊 Timeline Visualization

**NEW**: Visualize what happened in parallel from log files!

```bash
just timeline logs/by-session/session_test_20250128.jsonl
```

### Timeline View (Parallel Execution)

```
📊 Timeline View - Parallel Execution Visualization

Duration: 5.234s | Events: 487 | Agents: 3

Time (ms)  sam_harris      eckhart_tolle   wizard
─────────────────────────────────────────────────────────
      0    ▶ START         │               │
    100    █                ▶ START         │
    200    █                █               │
    300    █                █               │
    400    █                █               │
    500    ■ DONE           █               │
    600    │                ■ DONE          ▶ START
    700    │                │               █
    800    │                │               █
    900    │                │               ■ DONE

Legend: ▶ START  █ Streaming  ■ DONE  ✗ ERROR  │ Idle
```

### Parallel Execution Summary

```
🔀 Parallel Execution Summary

sam_harris ran in parallel with: eckhart_tolle
eckhart_tolle ran in parallel with: sam_harris
wizard ran sequentially after all agents
```

### Detailed Event Log

```
📋 Detailed Event Log

[+  0.000s] sam_harris      | agent_stream_start   |
[+  0.012s] sam_harris      | agent_content        | "Consciousness"
[+  0.024s] eckhart_tolle   | agent_stream_start   |
[+  0.036s] sam_harris      | agent_content        | " is"
[+  0.048s] eckhart_tolle   | agent_content        | "The present"
...
```

## 📝 Log Files

All events are logged to 4 dimensions simultaneously:

```
logs/
├── by-event-type/
│   ├── session_start.jsonl
│   ├── agent_content.jsonl
│   ├── wizard_content.jsonl
│   └── error.jsonl
├── by-agent/
│   ├── sam_harris.jsonl
│   ├── wizard.jsonl
│   └── tony_robbins.jsonl
├── by-session/
│   └── session_test-session-001_20250128_143022.jsonl
└── api-calls/
    └── http_20250128_143022.jsonl
```

### Analyzing Logs

```bash
# See which agents were active
ls logs/by-agent/

# Read full conversation
cat logs/by-session/session_*.jsonl | jq -r '.event.content' | tr -d '\n'

# Count events by type
wc -l logs/by-event-type/*.jsonl

# Find errors
cat logs/by-event-type/error.jsonl | jq

# Analyze API performance
cat logs/api-calls/*.jsonl | jq '.duration_ms'
```

## 🔧 Usage Examples

### BrainyardV3 Multi-Agent System

```bash
# Stream with multiple agents
just stream "What can philosophy teach us about consciousness and mental toughness?"

# Analyze the session
just timeline logs/by-session/session_*.jsonl
```

### Generic SSE API

```bash
# Create custom config
cp configs/generic-sse.yaml my-api.yaml
# Edit my-api.yaml with your API details

# Stream
./bin/debugger stream --config my-api.yaml "Your prompt"
```

### OpenAI-Compatible Streaming

```yaml
# configs/openai-streaming.yaml
backend:
  base_url: "https://api.openai.com"
  stream_endpoint:
    url: "/v1/chat/completions"
    method: "POST"
    message_format:
      type: "json_body"
      body_template: |
        {
          "model": "gpt-4",
          "messages": [{"role": "user", "content": "{message}"}],
          "stream": true
        }
    auth:
      type: "bearer"
      token_env: "OPENAI_API_KEY"
```

## 🎯 Why Configuration-Driven?

### Before (Hardcoded)

- Only worked with BrainyardV3
- Required code changes for new APIs
- Couldn't adapt to different event structures

### After (Config-Driven)

- Works with **any SSE API**
- **No code changes** needed
- Define endpoint, auth, events in YAML
- Reusable across projects

## 📦 Commands

```bash
# Development
just deps      # Install dependencies
just build     # Build binary
just clean     # Remove artifacts

# Streaming
just stream "message" ["config.yaml"]  # Real-time streaming
just timeline "session.jsonl"          # Timeline visualization
just replay "session.jsonl"            # Detailed log view

# Testing
just test      # Run tests
just race      # Race detector
```

## 🛠️ Architecture

```
stream-debugger/
├── bin/                  # Binaries
├── cmd/debugger/         # CLI entry point
├── internal/
│   ├── client/          # SSE client (config-driven)
│   ├── config/          # YAML config loading
│   ├── events/          # Event type definitions
│   ├── logger/          # Multi-dimensional logging
│   └── visualizer/      # TUI + Timeline views
├── configs/             # Pre-configured setups
├── logs/                # Generated logs
└── config.yaml          # Your configuration
```

## 🔑 Configuration Examples

See `configs/` directory for examples:

- `brainyard-v3.yaml` - BrainyardV3 multi-agent system
- `generic-sse.yaml` - Generic template for any SSE API

## 🚦 Workflow

1. **Configure**: Choose or create a config file for your API
2. **Stream**: Watch real-time events in beautiful TUI
3. **Analyze**: Visualize parallel execution from logs
4. **Iterate**: Tune your system based on insights

## 📚 Documentation

- `config.yaml.example` - Full configuration reference
- `QUICKSTART.md` - Step-by-step guide
- `IMPLEMENTATION_COMPLETE.md` - Technical details

## 🎓 Use Cases

- **Multi-Agent Debugging**: See which agents run in parallel
- **Performance Analysis**: Identify bottlenecks and slowdowns
- **API Integration Testing**: Validate SSE event flows
- **Production Monitoring**: Real-time streaming observability
- **Log Analysis**: Understand complex multi-agent conversations

## 🤝 Contributing

This tool is designed to be generic and reusable. To add support for a new API:

1. Create a config file in `configs/your-api.yaml`
2. Define endpoint, auth, and event mappings
3. Test with `just stream "test" "configs/your-api.yaml"`
4. Share your config!

## 📄 License

MIT

---

**Made for debugging complex SSE streaming systems** 🚀

Visualize parallel execution • Real-time monitoring • Configuration-driven • Log analysis
