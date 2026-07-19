Output docs/assets/tui-stream-anim.gif

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 16
Set Width 1280
Set Height 720
Set Padding 18
Set Margin 0
Set BorderRadius 8
Set Framerate 24
Set TypingSpeed 40ms
Set Theme { "name": "stream-debugger", "black": "#111827", "red": "#fb7185", "green": "#86efac", "yellow": "#fde68a", "blue": "#93c5fd", "magenta": "#d8b4fe", "cyan": "#67e8f9", "white": "#f4f7fb", "brightBlack": "#64748b", "brightRed": "#fda4af", "brightGreen": "#bbf7d0", "brightYellow": "#fef3c7", "brightBlue": "#bfdbfe", "brightMagenta": "#e9d5ff", "brightCyan": "#a5f3fc", "brightWhite": "#ffffff", "background": "#10131a", "foreground": "#f4f7fb", "cursor": "#67e8f9", "selection": "#283548" }

# Isolated home + forced color so the energy strip paints under VHS.
# Explicitly clear NO_COLOR — parent agent shells often export it.
Hide
Type `export HOME=$(mktemp -d /tmp/sd-anim-home.XXXXXX) TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1 FORCE_COLOR=1 STREAM_DEBUGGER_COLOR_PROFILE=truecolor; unset NO_COLOR; cd "$SD_ROOT"; clear` Enter
Show

# Launch interactive TUI against fixture-sse-server (started by record script).
Type `./bin/stream-debugger -c demos/configs/anim-demo.yaml` Enter
Sleep 2s

# Send a turn — fixture streams with delay so meters/pulses animate.
Type `i`
Sleep 400ms
Type `What is consciousness?`
Sleep 500ms
Enter
# Hold in streaming state so HeaderMeter shows LIVE + tok/s.
Sleep 6s
# Leave insert mode before pane keys (otherwise they type into the box).
Escape
Sleep 400ms

# Flow pane: completed checks after the turn.
Type `1`
Sleep 1800ms
Type `j`
Sleep 500ms
Type `k`
Sleep 800ms

# App pane: agent glyphs + mini bars.
Type `2`
Sleep 2500ms
Type `j`
Sleep 700ms
Type `k`
Sleep 900ms

# Timeline + Events briefly.
Type `3`
Sleep 1400ms
Type `4`
Sleep 1400ms

# Back to Flow for a final beat of the energy strip.
Type `1`
Sleep 2500ms
Ctrl+C
Sleep 400ms
