Output docs/assets/tui-obey-live.gif

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

# Live Obey daemon at unix:///tmp/obey.sock — no API key.
Hide
Type `export HOME=$(mktemp -d /tmp/sd-obey-home.XXXXXX) TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1 FORCE_COLOR=1 STREAM_DEBUGGER_COLOR_PROFILE=truecolor; unset NO_COLOR; cd "$SD_ROOT"; clear` Enter
Show

# WatchCampaignState against My_Tools (snapshot + live updates).
Type `./bin/stream-debugger -c demos/configs/obey-live-demo.yaml` Enter
Sleep 2s

# Start the long-lived watch RPC (message is retained; daemon supplies frames).
Type `i`
Sleep 300ms
Type `Watch live Obey campaign state for My_Tools`
Sleep 400ms
Enter
Sleep 5s
Escape
Sleep 400ms

# Flow / Events / Timeline with live frames
Type `1`
Sleep 2s
Type `4`
Sleep 2500ms
Type `j` Sleep 400ms Type `j` Sleep 400ms Type `k` Sleep 600ms
Type `3`
Sleep 2s
Type `5`
Sleep 1500ms
Type `2`
Sleep 2s
Type `1`
Sleep 2s
Ctrl+C
Sleep 400ms
