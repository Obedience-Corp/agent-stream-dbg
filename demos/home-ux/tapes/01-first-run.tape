Output docs/assets/home-ux-first-run.gif

Set Shell "bash"
Set FontFamily "JetBrainsMonoNL Nerd Font"
Set FontSize 15
Set Width 1100
Set Height 640
Set Padding 16
Set Margin 0
Set BorderRadius 8
Set Framerate 20
Set TypingSpeed 12ms
Set Theme { "name": "stream-debugger", "black": "#111827", "red": "#fb7185", "green": "#86efac", "yellow": "#fde68a", "blue": "#93c5fd", "magenta": "#d8b4fe", "cyan": "#67e8f9", "white": "#f4f7fb", "brightBlack": "#64748b", "brightRed": "#fda4af", "brightGreen": "#bbf7d0", "brightYellow": "#fef3c7", "brightBlue": "#bfdbfe", "brightMagenta": "#e9d5ff", "brightCyan": "#a5f3fc", "brightWhite": "#ffffff", "background": "#10131a", "foreground": "#f4f7fb", "cursor": "#67e8f9", "selection": "#283548" }

Hide
Type `export SD_HOME_UX_CONFIG_DIR=$(mktemp -d /tmp/sd-home-ux.XXXXXX) TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1 FORCE_COLOR=1; unset NO_COLOR SD_HOME_UX_SEED; cd "$SD_ROOT"; clear` Enter
Show

# First launch: empty home hub (no auto-written mystery YAML)
Type `./bin/home-ux` Enter
Sleep 2s

# Move to offline demos to show alternate first path
Type `j`
Sleep 900ms
Type `k`
Sleep 900ms

# Highlight new configuration again
Sleep 1s
Type `q`
Sleep 400ms
