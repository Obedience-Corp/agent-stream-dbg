Output out/tui-first-run-config.gif

Set Shell "bash"
Set FontFamily "JetBrainsMono Nerd Font"
Set FontSize 18
Set Width 1500
Set Height 700
Set Padding 22
Set Margin 0
Set BorderRadius 0
Set Framerate 30
Set TypingSpeed 55ms
Source out/_theme.tape

# Keep the recording isolated from the operator's real config and credentials.
Hide
Type "export HOME=$(mktemp -d /tmp/stream-debugger-termcast-home.XXXXXX); export TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1; clear" Enter
Show

# First launch opens the setup panel automatically.
Type "/tmp/stream-debugger-config-ux" Sleep 500ms Enter
Sleep 2s
Type "openai" Sleep 150ms
Tab
Tab
Tab
Type "https://api.example.com" Sleep 150ms
Tab
Type "/v1/stream" Sleep 150ms
Tab
Type "sam_harris,eckhart_tolle,wizard" Sleep 150ms
Tab
Type "region=demo" Sleep 500ms
Ctrl+S
Sleep 2s

# Reopen, inspect, cancel, then exercise every main pane.
Type "c" Sleep 150ms
Sleep 2s
Escape
Sleep 1s
Type "2" Sleep 1200ms
Type "3" Sleep 1200ms
  Type "4" Sleep 1200ms
  Type "5" Sleep 1200ms
  Sleep 2s
