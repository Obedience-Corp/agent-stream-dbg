# Quick Start Guide

## Setup

1. **Configure environment**:
   ```bash
   cp .env.example .env
   ```

2. **Edit .env** with your settings:
   ```env
   BACKEND_URL=http://localhost:5003
   API_KEY=your-api-key-here
   SESSION_ID=test-session-001
   ```

3. **Install dependencies** (if not already done):
   ```bash
   just deps
   ```

4. **Build** (if binary not present):
   ```bash
   just build
   ```

## Usage

### Stream Mode (Primary Use Case)

Start the real-time TUI debugger:

```bash
just stream "What is consciousness?"
```

Or directly:

```bash
./bin/debugger stream "What is consciousness?"
```

### What You'll See

The TUI will display:

- **Header**: Session ID and connection status
- **Agent Panels**: Each active agent's stream (side-by-side)
  - Status indicator (● inactive, ◉ active)
  - Agent name (color-coded)
  - Token count and sequence number
  - Buffer status (if tokens are buffered)
  - Content preview (last 200 characters)
- **Wizard Panel**: Synthesis stream (full width)
  - Status indicator
  - Token count and sequence
  - Content preview (last 400 characters)
- **Stats Bar**: Real-time metrics
  - Total events received
  - Total tokens streamed
  - Tokens per second
  - Error count
  - Session duration
- **Controls**: Keyboard shortcuts

### Keyboard Controls

- **p**: Pause/resume streaming display
- **q** or **Ctrl+C**: Quit and save logs
- **s**: Save session (future feature)

## Log Files

All events are automatically logged to four dimensions:

### 1. By Event Type
```
logs/by-event-type/
├── session_start.jsonl
├── agent_content.jsonl
├── wizard_content.jsonl
└── error.jsonl
```

### 2. By Agent
```
logs/by-agent/
├── sam_harris.jsonl
├── tony_robbins.jsonl
├── wizard.jsonl
└── ...
```

### 3. By Session (Timeline)
```
logs/by-session/
└── session_test-session-001_20250128_143022.jsonl
```

### 4. API Calls
```
logs/api-calls/
└── http_20250128_143022.jsonl
```

## Inspecting Logs

All logs are in JSON Lines format (`.jsonl`):

```bash
# View all agent content events
cat logs/by-event-type/agent_content.jsonl | jq

# View specific agent's activity
cat logs/by-agent/sam_harris.jsonl | jq

# View session timeline
cat logs/by-session/session_*.jsonl | jq

# Count events by type
wc -l logs/by-event-type/*.jsonl
```

## Troubleshooting

### "API_KEY is required but not set"

Make sure you have a `.env` file with `API_KEY=your-key` set.

### "failed to connect to SSE endpoint"

1. Check that BrainyardV3 backend is running: `http://localhost:5003`
2. Verify `BACKEND_URL` in `.env`
3. Check API key is valid

### Connection hangs

The SSE client waits for events. If no events arrive, check:
1. Backend is processing the message
2. Backend SSE emitter is working
3. Check backend logs for errors

### Empty agent panels

Some agents may not be triggered for certain messages. Try a message that activates multiple agents:

```bash
just stream "What can philosophy teach us about mental toughness and consciousness?"
```

## Next Steps

- **Load Testing**: Spawn multiple concurrent sessions (future feature)
- **Replay**: Replay sessions from log files (future feature)
- **Custom Analysis**: Write scripts to analyze `.jsonl` logs

## Advanced Usage

### Custom Session ID

```bash
SESSION_ID=my-custom-session just stream "Test message"
```

### Different Backend

```bash
BACKEND_URL=http://production-server:5003 just stream "Test message"
```

### Verbose Logging

```bash
LOG_LEVEL=debug just stream "Test message"
```
