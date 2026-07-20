Output out/tui-brainyard-live.gif

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 18
Set Width 1500
Set Height 700
Set Padding 22
Set Margin 0
Set BorderRadius 0
Set Framerate 30
Set TypingSpeed 45ms
Source out/_theme.tape

# The caller supplies API_KEY; this tape never contains or prints the secret.
# Keep generated config, logs, and Ctrl+S exports outside the repository.
Hide
Type "cd /tmp; export HOME=$(mktemp -d /tmp/agent-stream-dbg-brainyard-termcast-home.XXXXXX); export SESSION_ID=termcast-brainyard-live; export TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1; clear" Enter
Show

# First-run setup against the real local BrainyardV3 service.
Type "/tmp/agent-stream-dbg-brainyard-tui" Sleep 500ms Enter
Sleep 2s
Type "brainyard" Sleep 150ms
Tab
Tab
Tab
Type "http://localhost:5003" Sleep 150ms
Tab
Type "/api/v3/sessions/{session_id}/stream" Sleep 150ms
Tab
Type "sam_harris,eckhart_tolle,wizard" Sleep 150ms
Tab
Type "region=live" Sleep 500ms
Ctrl+S
Sleep 2s

# Two real turns verify setup, auth, multi-agent streaming, and history.
Type "i"
Sleep 500ms
Type "What is the clearest way to explain a multi-agent stream?"
Sleep 1s
Enter
Sleep 30s
Escape
Sleep 1s
Type "i"
Sleep 500ms
Type "Summarize the tradeoffs in one sentence."
Sleep 1s
Enter
Sleep 30s
Escape
Sleep 1s

# Visit every pane and exercise its primary navigation/toggle commands.
Type "1" Sleep 1s
Type "j" Type "j" Type "k" Type "p" Type "a" Type "[" Type "]" Sleep 1s
Enter Sleep 1s
Type "o" Type "f" Sleep 1s
Type "2" Sleep 1s
Type "f" Type "j" Type "k" Sleep 1s
Type "3" Sleep 1s
Type "j" Type "k" Type "g" Type "G" Sleep 1s
Type "4" Sleep 1s
Type "j" Type "j" Type "k" Type "Enter" Type "t" Type "W" Sleep 1s
Type "5" Sleep 1s
Ctrl+T
Sleep 1s
Ctrl+T
Type "w" Type "h" Type "l" Type "H" Sleep 1s

# Reopen and cancel config to verify the saved Brainyard flow is discoverable.
Type "c" Sleep 2s
Escape
Ctrl+S
Sleep 2s
Ctrl+C
