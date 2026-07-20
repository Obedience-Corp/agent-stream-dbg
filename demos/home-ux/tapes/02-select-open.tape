Output docs/assets/home-ux-select-open.gif

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
Type `export SD_HOME_UX_CONFIG_DIR=$(mktemp -d /tmp/sd-home-ux.XXXXXX) SD_HOME_UX_SEED=multi TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1 FORCE_COLOR=1; unset NO_COLOR; cd "$SD_ROOT"; clear` Enter
Show

# Returning user: multi-config home
Type `./bin/home-ux` Enter
Sleep 2s

# Browse list
Type `j`
Sleep 700ms
Type `j`
Sleep 900ms
Type `k`
Sleep 700ms
Type `k`
Sleep 700ms

# Open ready config → mock session (product would open real TUI)
Enter
Sleep 2s

# Return to home hub (key product goal: quit session ≠ quit app)
Type `q`
Sleep 1500ms

# Incomplete config → edit instead of open
Type `j`
Sleep 400ms
Type `j`
Sleep 600ms
Enter
Sleep 1500ms
Type `Escape`
Sleep 800ms
Type `q`
Sleep 400ms
