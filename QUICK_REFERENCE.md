# Stream Debugger - Quick Reference

## 🚀 The Two Commands You Need

### 1. Interactive Mode (Chat Interface)
```bash
stream-debugger --config config.yaml
```
- Multi-turn conversations
- Press **Ctrl+T** to toggle views (RAW ↔ PARSED)
- Arrow keys to scroll
- **Ctrl+C** to quit

### 2. Stream Mode (Single Message)
```bash
stream-debugger stream "your message" --config config.yaml
```
⚠️ **Message MUST come before `--config`**

---

## 📍 BrainyardV3 Integration

```bash
# From BrainyardV3 root directory:
cd /Users/lancerogers/Dev/AI/Brainyard/BrainyardV3

# Interactive mode (recommended)
just debug-stream

# Or manually:
stream-debugger --config tools/stream-debugger/config.yaml
```

---

## ⌨️ Keyboard Controls (Interactive Mode)

| Key | Action |
|-----|--------|
| **Type & Enter** | Send message |
| **Ctrl+T** | Toggle RAW (SSE) ↔ PARSED (agent responses) |
| **↑ ↓** | Scroll line by line |
| **PgUp / PgDn** | Scroll page by page |
| **Home / End** | Jump to top/bottom |
| **Ctrl+C** | Quit |

---

## 🔍 Views Explained

### RAW View (Default)
Shows complete SSE stream with **pretty-printed JSON**:
```
event: agent_content
data:
  {
    "type": "agent_content",
    "agent_id": "sam_harris",
    "content": "Consciousness is...",
    "sequence": 1
  }
```

### PARSED View (Press Ctrl+T)
Shows agent-organized, **color-coded responses**:
```
sam_harris (145 tokens) ✓
Consciousness is a complex and multifaceted concept...

eckhart_tolle (98 tokens) ✓
The present moment is where true awareness resides...
```

---

## 🐛 Troubleshooting

**"Config file not found"**
```bash
# Use absolute path
stream-debugger --config /full/path/to/config.yaml
```

**"API_KEY not set"**
```bash
# Create .env file in same directory as config
echo 'API_KEY=your-api-key-here' > .env
```

**"No active agents"**
```bash
# Create session with agents via debug API
curl -X POST http://localhost:5003/api/v3/debug/session \
  -H "Authorization: Bearer $API_KEY" \
  -d '{"agents": ["sam_harris", "eckhart_tolle", "wizard"]}'
```

---

## 📝 Common Mistakes

❌ **WRONG:** `stream-debugger stream --config config.yaml "message"`
✅ **RIGHT:** `stream-debugger stream "message" --config config.yaml`

❌ **WRONG:** `stream-debugger config.yaml` (missing --config flag)
✅ **RIGHT:** `stream-debugger --config config.yaml`

---

## 📚 More Info

- **Full Documentation:** [USAGE.md](USAGE.md)
- **Project Overview:** [README.md](README.md)
- **Configuration:** `config.yaml.example`

---

**Quick start:**
```bash
cd /Users/lancerogers/Dev/AI/Brainyard/BrainyardV3
just debug-stream
```
