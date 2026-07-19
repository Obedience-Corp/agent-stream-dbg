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
Set TypingSpeed 35ms
Set Theme { "name": "stream-debugger", "black": "#111827", "red": "#fb7185", "green": "#86efac", "yellow": "#fde68a", "blue": "#93c5fd", "magenta": "#d8b4fe", "cyan": "#67e8f9", "white": "#f4f7fb", "brightBlack": "#64748b", "brightRed": "#fda4af", "brightGreen": "#bbf7d0", "brightYellow": "#fef3c7", "brightBlue": "#bfdbfe", "brightMagenta": "#e9d5ff", "brightCyan": "#a5f3fc", "brightWhite": "#ffffff", "background": "#10131a", "foreground": "#f4f7fb", "cursor": "#67e8f9", "selection": "#283548" }

# Real Brainyard inference — API_KEY must already be in the environment.
Hide
Type `export HOME=$(mktemp -d /tmp/sd-live-home.XXXXXX) TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1 FORCE_COLOR=1 STREAM_DEBUGGER_COLOR_PROFILE=truecolor SESSION_ID=vhs-live-$$; unset NO_COLOR; cd "$SD_ROOT"; clear` Enter
Show

Type `./bin/stream-debugger -c configs/brainyard-v3.yaml` Enter
Sleep 2500ms

Type `i`
Sleep 300ms
Type `In two short sentences, what is consciousness?`
Sleep 400ms
Enter
# Real multi-agent stream — give tokens time to drive the meter.
Sleep 12s
Escape
Sleep 400ms

# Flow while turn is still settling / complete
Type `1`
Sleep 2s
Type `j` Sleep 400ms Type `k` Sleep 600ms

# App pane — agent bars from real token rates
Type `2`
Sleep 3s

Type `1`
Sleep 2s
Ctrl+C
Sleep 400ms
