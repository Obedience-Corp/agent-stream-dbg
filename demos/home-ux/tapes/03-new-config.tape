Output docs/assets/home-ux-new-config.gif

Set Shell "bash"
Set FontFamily "JetBrainsMonoNL Nerd Font"
Set FontSize 15
Set Width 1100
Set Height 700
Set Padding 16
Set Margin 0
Set BorderRadius 8
Set Framerate 20
Set TypingSpeed 18ms
Set Theme { "name": "agent-stream-dbg", "black": "#111827", "red": "#fb7185", "green": "#86efac", "yellow": "#fde68a", "blue": "#93c5fd", "magenta": "#d8b4fe", "cyan": "#67e8f9", "white": "#f4f7fb", "brightBlack": "#64748b", "brightRed": "#fda4af", "brightGreen": "#bbf7d0", "brightYellow": "#fef3c7", "brightBlue": "#bfdbfe", "brightMagenta": "#e9d5ff", "brightCyan": "#a5f3fc", "brightWhite": "#ffffff", "background": "#10131a", "foreground": "#f4f7fb", "cursor": "#67e8f9", "selection": "#283548" }

Hide
Type `export SD_HOME_UX_CONFIG_DIR=$(mktemp -d /tmp/sd-home-ux.XXXXXX) TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1 FORCE_COLOR=1; unset NO_COLOR SD_HOME_UX_SEED; cd "$SD_ROOT"; clear` Enter
Show

Type `./bin/home-ux` Enter
Sleep 1500ms

# New configuration from empty home
Enter
Sleep 900ms

# Template: stay on SSE
Enter
Sleep 800ms

# Connection fields
Type `http://localhost:8080`
Sleep 300ms
Tab
Sleep 200ms
# endpoint already has default; tab to auth
Tab
Sleep 200ms
Type `API_KEY`
Sleep 300ms
Enter
Sleep 800ms

# Dialect: move to brainyard
Type `j`
Sleep 600ms
Enter
Sleep 800ms

# Name
Type `local-dev`
Sleep 400ms
Tab
Sleep 400ms
# Save and return to home (second choice)
Type `j`
Sleep 500ms
Enter
Sleep 2s

# Show new row in list
Sleep 1500ms
Type `q`
Sleep 400ms
