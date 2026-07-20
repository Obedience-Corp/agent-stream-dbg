Output docs/assets/tui-acp-live.gif

Set Shell "bash"
Set FontFamily "JetBrainsMonoNL Nerd Font"
Set FontSize 15
Set Width 1280
Set Height 800
Set Padding 16
Set Margin 0
Set BorderRadius 8
Set Framerate 20
Set TypingSpeed 18ms
Set Theme { "name": "agent-stream-dbg", "black": "#111827", "red": "#fb7185", "green": "#86efac", "yellow": "#fde68a", "blue": "#93c5fd", "magenta": "#d8b4fe", "cyan": "#67e8f9", "white": "#f4f7fb", "brightBlack": "#64748b", "brightRed": "#fda4af", "brightGreen": "#bbf7d0", "brightYellow": "#fef3c7", "brightBlue": "#bfdbfe", "brightMagenta": "#e9d5ff", "brightCyan": "#a5f3fc", "brightWhite": "#ffffff", "background": "#10131a", "foreground": "#f4f7fb", "cursor": "#67e8f9", "selection": "#283548" }

# Isolated env; SD_ROOT set by demos/record-acp-live.sh
Hide
Type `export HOME=$(mktemp -d /tmp/sd-acp-live-home.XXXXXX) TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1 FORCE_COLOR=1; unset NO_COLOR; cd "$SD_ROOT"; clear` Enter
Show

Type `echo "=== ACP live multi-turn (same process + session) ===" && ./bin/agent-stream-dbg --config demos/configs/acp-live-demo.yaml` Enter
Sleep 2s

# Messages pane so agent replies are visible
Escape
Sleep 200ms
Type `5`
Sleep 400ms

# Turn 1
Type `i`
Sleep 300ms
Type `list files in src/`
Sleep 300ms
Enter
Sleep 3500ms

# Turn 2 — same sess_demo_live (process reuse)
Type `i`
Sleep 300ms
Type `now summarize main.go`
Sleep 300ms
Enter
Sleep 3500ms

# Hold on Messages so both turns + agent text are readable
Sleep 2500ms

Ctrl+C
Sleep 500ms
