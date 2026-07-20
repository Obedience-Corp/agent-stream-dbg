# agent-stream-dbg Usage Guide

## Two Modes of Operation

agent-stream-dbg has **two distinct modes** for different use cases:

---

## 🎮 Interactive Mode (Multi-Turn Chat)

**Best for:** Development, testing multi-turn conversations, exploring agent interactions

### Command

```bash
agent-stream-dbg --config config.yaml
```

### Features

✅ **Multi-turn conversations** - Type messages and get responses indefinitely
✅ **View toggle (Ctrl+T)** - Switch between RAW and PARSED views
✅ **Pretty-printed JSON** - Expanded, readable JSON in raw view
✅ **Full scrolling** - Arrow keys, PgUp/PgDn, Home/End
✅ **All message history** - See your entire conversation

### Keyboard Controls

| Key | Action |
|-----|--------|
| **Type & Enter** | Send message to agents |
| **Ctrl+T** | Toggle between RAW (SSE) ↔ PARSED (agent-organized) views |
| **↑ ↓** | Scroll line by line |
| **PgUp / PgDn** | Scroll page by page |
| **Home / End** | Jump to top/bottom |
| **Ctrl+C** | Quit and save logs |

### Views

**RAW View (default):**
```
event: agent_content
data:
  {
    "type": "agent_content",
    "agent_id": "sam_harris",
    "message_id": "msg_abc123",
    "content": "Consciousness is...",
    "sequence": 1,
    "timestamp": "2025-10-31T16:08:08.760689+00:00"
  }
```

**PARSED View (Ctrl+T):**
```
sam_harris (145 tokens) ✓
Consciousness is a complex and multifaceted concept that has been
studied by philosophers, neuroscientists, and psychologists...

eckhart_tolle (98 tokens) ✓
The present moment is where true awareness resides. When we observe
our thoughts without judgment...

wizard (243 tokens) ✓
Integrating both perspectives, consciousness can be understood as
both a neurological phenomenon and a subjective experience...
```

### Example Session

```bash
# 1. Start interactive mode
agent-stream-dbg --config tools/agent-stream-dbg/config.yaml

# 2. Type your message and press Enter
> What is consciousness?

# 3. Watch agents respond in real-time

# 4. Press Ctrl+T to toggle views
# 5. Use arrows to scroll through responses
# 6. Type another message to continue the conversation
# 7. Press Ctrl+C when done
```

---

## ⚡ Stream Mode (Single Message)

**Best for:** CI/CD, scripting, automation, quick one-off tests

### Command

```bash
agent-stream-dbg stream "your message" --config config.yaml
```

**⚠️ IMPORTANT:** The message must come BEFORE the `--config` flag!

### Features

✅ **Single message** - Send one message and exit when complete
✅ **Real-time visualization** - Watch agents respond in a TUI
✅ **Agent state tracking** - Token counts, sequences, timing
✅ **Auto-exit** - Closes when response completes

### Example

```bash
# Correct syntax (message first, then --config)
agent-stream-dbg stream "What is consciousness?" --config config.yaml

# ❌ WRONG - will fail
agent-stream-dbg stream --config config.yaml "What is consciousness?"
```

### Use Cases

```bash
# Quick test
agent-stream-dbg stream "Hello" --config config.yaml

# CI/CD pipeline
./agent-stream-dbg stream "$TEST_MESSAGE" --config ci-config.yaml

# Scripting
for msg in "test1" "test2" "test3"; do
  agent-stream-dbg stream "$msg" --config config.yaml
done
```

---

## 📊 Timeline Mode (Log Analysis)

**Best for:** Understanding parallel execution, performance analysis

### Command

```bash
agent-stream-dbg timeline logs/by-session/session_*.jsonl
```

### Features

- Visual timeline of agent execution
- Shows parallel vs sequential execution
- Identifies performance bottlenecks
- Token counts and timing statistics

### Example Output

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
```

---

## 🔧 Configuration Setup

### Quick Setup

```bash
# Navigate to your project directory (wherever config.yaml lives)
cd /path/to/your/project

# Ensure .env has your API key
cat .env
# API_KEY=your-api-key-here
# SESSION_ID=debug-session-id

# Run interactive mode
agent-stream-dbg --config config.yaml
```

### Config File Location

The config file path can be:
- Relative: `--config config.yaml` (looks in current directory)
- Absolute: `--config /full/path/to/config.yaml`
- From BrainyardV3: `--config tools/agent-stream-dbg/config.yaml`

---

## 🚀 BrainyardV3 Integration

### Using Just Commands (Recommended)

```bash
# Navigate to your backend's root directory
cd /path/to/your/backend

# Run manually with the backend's config
agent-stream-dbg --config tools/agent-stream-dbg/config.yaml
```

### Session Setup

The BrainyardV3 config has `auto_setup: true`, which automatically:
1. Creates or retrieves a debug session
2. Initializes agents (sam_harris, eckhart_tolle, wizard)
3. Persists the session ID for continued use

---

## 📝 Log Files

All modes save logs to the configured directory:

```
logs/
├── by-event-type/       # Events grouped by type
│   ├── agent_content.jsonl
│   ├── wizard_content.jsonl
│   └── error.jsonl
├── by-agent/            # Events grouped by agent
│   ├── sam_harris.jsonl
│   ├── eckhart_tolle.jsonl
│   └── wizard.jsonl
├── by-session/          # Complete session timelines
│   └── session_xxx.jsonl
└── api-calls/           # HTTP requests/responses
    └── http_xxx.jsonl
```

### Analyzing Logs

```bash
# View all agent responses
cat logs/by-agent/sam_harris.jsonl | jq -r '.event.content'

# Count events by type
wc -l logs/by-event-type/*.jsonl

# Timeline visualization
agent-stream-dbg timeline logs/by-session/session_*.jsonl
```

---

## ❓ Common Issues

### "No such file or directory: config.yaml"

**Problem:** Config file not found
**Solution:** Use absolute path or navigate to config directory first

```bash
# Option 1: Use absolute path
agent-stream-dbg --config /full/path/to/config.yaml

# Option 2: Navigate first
cd /path/to/config/directory
agent-stream-dbg --config config.yaml
```

### "API_KEY environment variable not set"

**Problem:** Missing API key in environment
**Solution:** Create `.env` file in the same directory as config

```bash
echo 'API_KEY=your-api-key-here' > .env
```

### "No active agents in session"

**Problem:** Session doesn't have agents configured
**Solution:** Use the debug API to create session with agents

```bash
curl -X POST http://localhost:5003/api/v3/debug/session \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"agents": ["sam_harris", "eckhart_tolle", "wizard"]}'
```

---

## 🎯 Choosing the Right Mode

| Use Case | Mode | Command |
|----------|------|---------|
| **Development & exploration** | Interactive | `agent-stream-dbg --config config.yaml` |
| **Multi-turn conversations** | Interactive | `agent-stream-dbg --config config.yaml` |
| **Testing agent interactions** | Interactive | `agent-stream-dbg --config config.yaml` |
| **CI/CD testing** | Stream | `agent-stream-dbg stream "test" --config config.yaml` |
| **Automation/scripting** | Stream | `agent-stream-dbg stream "$MSG" --config config.yaml` |
| **Quick one-off test** | Stream | `agent-stream-dbg stream "hello" --config config.yaml` |
| **Performance analysis** | Timeline | `agent-stream-dbg timeline logs/session_*.jsonl` |
| **Understanding execution** | Timeline | `agent-stream-dbg timeline logs/session_*.jsonl` |

---

## 📚 More Information

- **README.md** - Project overview and installation
- **config.yaml.example** - Full configuration reference
- **docs/user-guide/** - Additional user guides
- **docs/development/** - Development and testing guides

---

**Questions?** File an issue at https://github.com/Obedience-Corp/agent-stream-dbg/issues
