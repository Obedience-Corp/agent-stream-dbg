# Getting Started with BrainyardV3 Streaming

Quick guide to test BrainyardV3 streaming with the debugger.

## Prerequisites

1. **BrainyardV3 backend running**:
   ```bash
   cd /Users/lancerogers/Dev/AI/Brainyard/BrainyardV3
   just up
   # Wait for backend to be healthy on http://localhost:5003
   ```

2. **Backend health check**:
   ```bash
   curl http://localhost:5003/api/v3/health
   # Should return: {"status":"healthy"}
   ```

## Quick Setup (2 minutes)

### 1. Configure the Debugger

```bash
cd /Users/lancerogers/Dev/AI/Brainyard/stream-debugger

# Copy BrainyardV3 config
cp configs/brainyard-v3.yaml config.yaml

# Create .env file
cat > .env << 'EOF'
BACKEND_URL=http://localhost:5003
API_KEY=dev-test-key
SESSION_ID=debug-session-001
LOG_DIR=./logs
EOF
```

**Note**: Check what API key BrainyardV3 expects. You may need to:
- Look at `BrainyardV3/backend/app/config.py` for auth settings
- Or check if auth is disabled in development
- Update `API_KEY` in `.env` accordingly

### 2. Build (if needed)

```bash
just build
# Creates bin/debugger binary
```

### 3. Test Your First Stream (Real-Time TUI)

```bash
just stream "What is consciousness from multiple philosophical perspectives?"
```

**What happens:**
1. Connects to BrainyardV3 backend
2. Sends your message
3. **Opens a beautiful TUI in your terminal** showing live agent responses
4. Updates in real-time as tokens stream in
5. Logs everything to `logs/` directory

Press **q** when done to exit the TUI and return to your terminal.

## Two Modes: Real-Time TUI vs Timeline Analysis

### Mode 1: Real-Time TUI (During Streaming)

When you run `just stream`, you get an **interactive terminal UI** that updates live:

```
╭────────────────────────────────────────────────╮
│ Stream Debugger - Session: debug-session-001  │
╰────────────────────────────────────────────────╯

╭─────────────────────╮  ╭─────────────────────╮
│ ◉ sam_harris       │  │ ◉ eckhart_tolle    │
│ Tokens: 145        │  │ Tokens: 98         │
│ Seq: 144           │  │ Seq: 97            │
│ Buffered: 0        │  │ Buffered: 0        │
│                    │  │                    │
│ Consciousness is   │  │ The present moment │
│ fundamentally...   │  │ is where true...   │
╰─────────────────────╯  ╰─────────────────────╯

╭────────────────────────────────────────────────╮
│ ◉ Wizard (Synthesis)                          │
│ Tokens: 243 | Seq: 242                        │
│                                                │
│ Integrating both perspectives...              │
╰────────────────────────────────────────────────╯

Events: 487 | Tokens: 486 | Tokens/sec: 24.3 | Errors: 0 | Duration: 20s

[p] pause/resume | [q] quit | [s] save session
```

**Key Points:**
- This is a **live, interactive TUI** using the full terminal window
- Updates in real-time as events stream from the backend
- Press **q** to exit the TUI and return to your normal terminal

**Controls:**
- **p** - Pause/resume display (events still log in background)
- **q** or **Ctrl+C** - Quit TUI and save all logs
- **s** - Save session (future feature)

---

### Mode 2: Timeline Analysis (After Streaming)

After you quit the TUI, use `just timeline` to **visualize what happened** from the saved logs:

```bash
# This renders a visual timeline - NOT a TUI, just printed output
just timeline logs/by-session/session_debug-session-001_*.jsonl
```

This shows a static visualization of parallel execution (see example below in "View Timeline" section).

---

## After Streaming: Analyze Results

Once you've exited the TUI (pressed **q**), all events are saved to logs. Now you can analyze:

### 1. Check What Was Logged

```bash
# See which agents responded
ls logs/by-agent/
# Output: sam_harris.jsonl  eckhart_tolle.jsonl  wizard.jsonl  ...

# Count events by type
wc -l logs/by-event-type/*.jsonl
# Output:
#   10 logs/by-event-type/session_start.jsonl
#  245 logs/by-event-type/agent_content.jsonl
#    5 logs/by-event-type/agent_stream_start.jsonl
#  ...
```

### 2. View Timeline (Parallel Execution)

**Important**: This is NOT a TUI - it prints a visualization to your terminal and exits.

```bash
# Visualize what happened in parallel (prints and exits)
just timeline logs/by-session/session_debug-session-001_*.jsonl
```

**Printed Output:**
```
📊 Timeline View - Parallel Execution Visualization

Duration: 5.234s | Events: 487 | Agents: 3

Time (ms)  sam_harris      eckhart_tolle   marcus_aurelius  wizard
───────────────────────────────────────────────────────────────────
      0    ▶ START         │               │                │
    100    █                ▶ START         │                │
    200    █                █               ▶ START          │
    300    █                █               █                │
    500    █                █               █                │
    700    ■ DONE           █               █                │
    900    │                ■ DONE          █                │
   1100    │                │               ■ DONE           ▶ START
   1200    │                │               │                █
   1500    │                │               │                ■ DONE

Legend: ▶ START  █ Streaming  ■ DONE  ✗ ERROR  │ Idle

🔀 Parallel Execution Summary

sam_harris ran in parallel with: eckhart_tolle, marcus_aurelius
eckhart_tolle ran in parallel with: sam_harris, marcus_aurelius
marcus_aurelius ran in parallel with: sam_harris, eckhart_tolle
wizard ran sequentially after all agents
```

### 3. Read Full Conversation

```bash
# Extract just the content tokens
cat logs/by-session/session_*.jsonl | \
  jq -r 'select(.event.content != null) | .event.content' | \
  tr -d '\n'

# Output: The full conversation text
```

### 4. Find Errors

```bash
# Check if any errors occurred
cat logs/by-event-type/error.jsonl | jq

# See which agent had errors
jq '.event.agent_id' logs/by-event-type/error.jsonl

# See error details
jq '.event | {type: .error_type, message: .message, agent: .agent_id}' \
  logs/by-event-type/error.jsonl
```

### 5. Performance Analysis

```bash
# API call timing
cat logs/api-calls/*.jsonl | jq '.duration_ms'

# Average response time
cat logs/api-calls/*.jsonl | \
  jq -r '.duration_ms' | \
  awk '{sum+=$1; count++} END {print "Average:", sum/count, "ms"}'

# Token throughput
echo "Total tokens: $(wc -l < logs/by-event-type/agent_content.jsonl)"
```

## Common Testing Scenarios

### 1. Test Single Agent

```bash
# Question that should trigger only one agent
just stream "What is the definition of consciousness?"

# Check which agents responded
ls logs/by-agent/
```

### 2. Test Multiple Agents

```bash
# Question that should trigger multiple agents
just stream "How do philosophy, neuroscience, and meditation relate to consciousness?"

# View parallel execution
just timeline logs/by-session/session_*.jsonl
```

### 3. Test Error Handling

```bash
# Stop backend mid-stream
cd ../BrainyardV3
just down

# Try streaming (should error)
cd ../stream-debugger
just stream "test"

# Check error logs
cat logs/by-event-type/error.jsonl | jq
```

### 4. Test Sequence Buffering

If you see out-of-order tokens in logs:

```bash
# Check sequence numbers
cat logs/by-agent/sam_harris.jsonl | \
  jq -r 'select(.event.sequence != null) | .event.sequence'

# Should be: 0, 1, 2, 3, 4... (sequential)
```

## Troubleshooting

### "Failed to connect"

**Problem**: Can't connect to backend

**Solutions**:
```bash
# 1. Check backend is running
curl http://localhost:5003/api/v3/health

# 2. Check correct port in config
cat config.yaml | grep base_url

# 3. Check backend logs
cd ../BrainyardV3
just logs backend
```

### "API_KEY is required"

**Problem**: Missing authentication

**Solutions**:
```bash
# 1. Check .env file exists
cat .env

# 2. Check if BrainyardV3 requires auth
cd ../BrainyardV3
grep -r "require_auth" backend/app/api/

# 3. If no auth needed, set dummy key
echo "API_KEY=dummy" >> .env
```

### "No events received"

**Problem**: Connected but no events streaming

**Debug steps**:
```bash
# 1. Check backend is processing message
cd ../BrainyardV3
just logs backend | grep "stream"

# 2. Check SSE endpoint manually
curl -N -H "Accept: text/event-stream" \
  "http://localhost:5003/api/v3/sessions/test/stream?message=hello"

# 3. Check event types match
cat logs/by-session/*.jsonl | jq '.event.type' | sort -u
```

### Empty Agent Panels

**Problem**: TUI shows no agent activity

**Possible reasons**:
1. Message doesn't trigger any agents
2. Agent selection logic filtered them out
3. Backend not emitting agent events

**Debug**:
```bash
# Check what events were received
cat logs/by-session/*.jsonl | jq '.event.type' | sort | uniq -c

# Try a message that should trigger multiple agents
just stream "Explain consciousness from philosophical, psychological, and spiritual perspectives"
```

## Configuration Tips

### Adjust for Your Setup

Edit `config.yaml` to match your backend:

```yaml
# If using different port
backend:
  base_url: "http://localhost:8080"

# If using different endpoint pattern
stream_endpoint:
  url: "/api/stream"  # Instead of /api/v3/sessions/{session_id}/stream

# If using different event names
events:
  field_mappings:
    type: "event_type"    # If backend uses "event_type" instead of "type"
    agent_id: "source"    # If backend uses "source" instead of "agent_id"
```

### Multiple Configs

Keep different configs for different scenarios:

```bash
# Development
cp configs/brainyard-v3.yaml config-dev.yaml

# Production
cp config-dev.yaml config-prod.yaml
# Edit config-prod.yaml for production URL

# Use specific config
just stream "test" "config-prod.yaml"
```

## Next Steps

### 1. Validate Streaming Works

- [ ] Backend responds to messages
- [ ] Events stream in real-time
- [ ] All expected agents respond
- [ ] Wizard synthesis works
- [ ] Timeline shows parallel execution

### 2. Test Edge Cases

- [ ] Very long messages
- [ ] Special characters in messages
- [ ] Rapid consecutive messages
- [ ] Backend restart mid-stream
- [ ] Network interruption

### 3. Performance Testing

- [ ] 10+ concurrent sessions
- [ ] Long conversations (100+ messages)
- [ ] Large token counts (1000+ tokens per agent)
- [ ] Monitor memory usage
- [ ] Check for token leaks between sessions

## Integration with BrainyardV3 Development

### Use During Development

```bash
# Terminal 1: Backend
cd BrainyardV3
just up
just logs backend

# Terminal 2: Debugger
cd stream-debugger
just stream "Test question"

# Terminal 3: Watch logs
cd stream-debugger
tail -f logs/by-session/*.jsonl | jq
```

### Before Committing Changes

```bash
# Test streaming still works
just stream "What is consciousness?"

# Check no errors
test -f logs/by-event-type/error.jsonl && \
  cat logs/by-event-type/error.jsonl || \
  echo "✅ No errors"

# Verify parallel execution
just timeline logs/by-session/*.jsonl | grep "ran in parallel"
```

## Pro Tips

### Quick Testing Loop

```bash
# Alias for rapid testing
alias test-stream='just stream "What is consciousness from multiple perspectives?"'

# Run and immediately check results
test-stream && just timeline logs/by-session/*.jsonl
```

### Clean Logs Between Tests

```bash
# Clear old logs
rm -rf logs/*

# Or create timestamped log directories
export LOG_DIR="./logs/test-$(date +%Y%m%d-%H%M%S)"
just stream "test message"
```

### Monitor Live

```bash
# Watch events as they come in
watch -n 0.5 'ls -lh logs/by-agent/ | tail -5'

# Watch token count increase
watch -n 0.5 'wc -l logs/by-event-type/agent_content.jsonl'
```

---

**Happy Testing!** 🚀

Questions? Check:
- `README.md` - Full documentation
- `config.yaml.example` - All configuration options
- `QUICKSTART.md` - General quick start guide
