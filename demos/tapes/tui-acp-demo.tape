Output docs/assets/tui-acp-demo.gif

Set Shell "bash"
Set FontFamily "JetBrainsMonoNL Nerd Font"
Set FontSize 15
Set Width 1280
Set Height 800
Set Padding 16
Set Margin 0
Set BorderRadius 8
Set Framerate 20
Set TypingSpeed 12ms
Set Theme { "name": "agent-stream-dbg", "black": "#111827", "red": "#fb7185", "green": "#86efac", "yellow": "#fde68a", "blue": "#93c5fd", "magenta": "#d8b4fe", "cyan": "#67e8f9", "white": "#f4f7fb", "brightBlack": "#64748b", "brightRed": "#fda4af", "brightGreen": "#bbf7d0", "brightYellow": "#fef3c7", "brightBlue": "#bfdbfe", "brightMagenta": "#e9d5ff", "brightCyan": "#a5f3fc", "brightWhite": "#ffffff", "background": "#10131a", "foreground": "#f4f7fb", "cursor": "#67e8f9", "selection": "#283548" }

# Isolated env; SD_ROOT is set by demos/record-acp.sh
Hide
Type `export HOME=$(mktemp -d /tmp/sd-acp-home.XXXXXX) TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1 FORCE_COLOR=1; unset NO_COLOR; cd "$SD_ROOT"; clear` Enter
Show

# --- 1) Explain: every ACP JSON-RPC frame → dialect rule ---------------
Type `echo "=== ACP dialect: explain ===" && ./bin/agent-stream-dbg explain --dialect dialects/acp.yaml --from testdata/fixtures/acp-session.jsonl | head -45` Enter
Sleep 6s

# --- 2) Timeline: parent session, tools, child subagent lanes ----------
Type `clear; echo "=== ACP dialect: timeline ===" && ./bin/agent-stream-dbg timeline --dialect acp testdata/fixtures/acp-session.jsonl` Enter
Sleep 6s

# --- 3) Detailed replay log -------------------------------------------
Type `clear; echo "=== ACP dialect: replay ===" && ./bin/agent-stream-dbg replay --dialect acp testdata/fixtures/acp-session.jsonl` Enter
Sleep 5s

# --- 4) Live TUI against fixture SSE replaying ACP NDJSON -------------
Type `clear; echo "=== ACP dialect: TUI stream (fixture SSE) ===" && ./bin/agent-stream-dbg stream --config demos/configs/acp-demo.yaml "List files and summarize main.go"` Enter
Sleep 8s
Ctrl+C
Sleep 600ms
