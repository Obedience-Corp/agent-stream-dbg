# TUI vs Timeline: What's the Difference?

Quick clarification on the two visualization modes.

## Mode 1: Real-Time TUI (Interactive)

**Command**: `just stream "your message"`

**What it is:**
- **Full-screen Terminal UI** (like `htop` or `vim`)
- **Interactive** - respond to keyboard input
- **Live updates** - changes as events stream in
- **Bubbletea-based** - beautiful, reactive interface

**When to use:**
- During active streaming sessions
- When you want to watch agents respond in real-time
- When debugging live streaming issues

**What you see:**
```
┌─────────────────────────────────────────┐
│ Stream Debugger - Session: xxx         │  ← Full terminal window
└─────────────────────────────────────────┘

┌──────────────┐  ┌──────────────┐
│ ◉ Agent 1   │  │ ◉ Agent 2   │  ← Live panels
│ Tokens: 42  │  │ Tokens: 38  │     updating
└──────────────┘  └──────────────┘

[p] pause | [q] quit                       ← Keyboard controls
```

**How it works:**
1. Press Enter to run command
2. **Terminal switches to TUI mode** (takes over full screen)
3. Watch live streaming
4. Press **q** to exit TUI
5. **Terminal returns to normal** (command prompt back)

**Technical:**
- Uses Bubbletea (Elm architecture for Go)
- Alt-screen buffer (doesn't scroll your terminal history)
- Event-driven updates (60 FPS)

---

## Mode 2: Timeline Analysis (Static Output)

**Command**: `just timeline logs/by-session/session_*.jsonl`

**What it is:**
- **Printed output** to your terminal (like `cat` or `ls`)
- **Static** - just prints and exits
- **Post-analysis** - reads from saved log files
- **Text-based visualization** - box drawing characters

**When to use:**
- After streaming session is done
- When analyzing what happened
- When you want to see parallel execution patterns

**What you see:**
```
$ just timeline logs/by-session/session_*.jsonl

📊 Timeline View - Parallel Execution Visualization

Duration: 5.234s | Events: 487 | Agents: 3

Time (ms)  sam_harris      eckhart_tolle   wizard
─────────────────────────────────────────────────────────
      0    ▶ START         │               │
    100    █                ▶ START         │
    200    █                █               │
...

$ _  ← Back to normal prompt
```

**How it works:**
1. Press Enter to run command
2. **Prints visualization to terminal** (like any normal command)
3. **Exits immediately** after printing
4. Command prompt returns

**Technical:**
- Reads .jsonl log files
- Parses events
- Renders static text visualization
- Prints to stdout and exits

---

## Quick Comparison

| Feature | Real-Time TUI | Timeline Analysis |
|---------|---------------|-------------------|
| **Interactive?** | ✅ Yes (press keys) | ❌ No (just prints) |
| **Live?** | ✅ Updates in real-time | ❌ Reads saved logs |
| **Full screen?** | ✅ Takes over terminal | ❌ Prints like normal |
| **When?** | During streaming | After streaming |
| **Exit** | Press 'q' | Auto-exits |
| **Data source** | Live SSE events | Log files |
| **Use case** | Watch it happen | Analyze what happened |

---

## Examples

### Example 1: Watch Streaming (TUI)

```bash
# Start streaming - TUI will take over your terminal
$ just stream "What is consciousness?"

# ┌────────────────────────────────────┐
# │ Stream Debugger                    │ ← TUI mode
# │                                    │   (full screen)
# │ ◉ sam_harris streaming...         │
# │ ◉ eckhart_tolle streaming...      │
# │                                    │
# │ [q] quit                          │
# └────────────────────────────────────┘

# Press 'q' to exit TUI

$ _  ← Back to normal terminal
```

### Example 2: Analyze Results (Timeline)

```bash
# View what happened - just prints output
$ just timeline logs/by-session/session_*.jsonl

📊 Timeline View - Parallel Execution
[... prints visualization ...]
Legend: ▶ START  █ Streaming  ■ DONE

$ _  ← Back to normal terminal immediately
```

---

## Which Should I Use?

### Use Real-Time TUI when:
- ✅ Testing if streaming works
- ✅ Watching agents respond live
- ✅ Debugging connection issues
- ✅ Checking if all agents activate
- ✅ Seeing real-time token counts

### Use Timeline Analysis when:
- ✅ Understanding what happened
- ✅ Checking if agents ran in parallel
- ✅ Finding bottlenecks
- ✅ Generating reports
- ✅ Debugging from saved logs

---

## Common Confusion

❌ **"The timeline command doesn't show a TUI"**
- Correct! Timeline is NOT a TUI, it just prints output

❌ **"How do I exit the timeline?"**
- It exits automatically after printing

✅ **"Stream mode takes over my terminal"**
- Yes! That's the TUI. Press 'q' to exit.

---

## Pro Tip: Use Both Together

**Best workflow:**

1. **Stream in real-time (TUI)**:
   ```bash
   just stream "your question"
   # Watch it happen live
   # Press 'q' when done
   ```

2. **Analyze timeline (printed output)**:
   ```bash
   just timeline logs/by-session/session_*.jsonl
   # See parallel execution patterns
   ```

This gives you:
- ✅ Live feedback during streaming (TUI)
- ✅ Detailed analysis after (Timeline)
- ✅ Best of both worlds!

---

**TL;DR:**
- `just stream` = **Interactive TUI** (like vim, takes over terminal, press 'q' to exit)
- `just timeline` = **Static output** (like cat, prints and exits)
