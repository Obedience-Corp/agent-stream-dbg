Output docs/assets/tui-obey-activity.gif

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

# Live Obey WatchAllActivity — record script fires SendMessage after watch starts.
Hide
Type `export HOME=$(mktemp -d /tmp/sd-obey-act-home.XXXXXX) TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1 FORCE_COLOR=1 STREAM_DEBUGGER_COLOR_PROFILE=truecolor; unset NO_COLOR; cd "$SD_ROOT"; clear` Enter
Show

Type `./bin/stream-debugger -c demos/configs/obey-activity-live.yaml` Enter
Sleep 2s

# Open the long-lived activity watch.
Type `i`
Sleep 300ms
Type `Watch live Obey agent activity for My_Tools`
Sleep 400ms
Enter
# Stay on Flow while background SendMessage fires content deltas (script ~14s / ~18s).
# Energy strip should spike on KindContent; hold long enough for both turns.
Sleep 18s
Escape
Sleep 400ms

# Events with tokens ON so KindContent deltas are visible in the list.
Type `4`
Sleep 500ms
Type `t`
Sleep 4s
Type `G`
Sleep 1500ms
Type `j` Sleep 300ms Type `k` Sleep 600ms

# App pane — agent mini-bars from token rates
Type `2`
Sleep 2500ms

# Hold LIVE chrome on Flow
Type `1`
Sleep 2s
Ctrl+C
Sleep 400ms
