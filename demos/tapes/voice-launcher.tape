Output docs/assets/voice-launcher.gif
Set Shell "bash"
Set FontSize 16
Set Width 1280
Set Height 820
Set Padding 12
Set Framerate 12
Set Theme "Catppuccin Mocha"

Hide
Type `cd cmd/agent-stream-dbg && AGENT_STREAM_DBG_TERMINAL_DEMO=1 ../../bin/voice-launcher-terminal -test.run '^TestVoiceLauncherTerminal$' -test.timeout 90s`
Enter
Sleep 1s
Show
Sleep 1s
Enter
Sleep 1s
Screenshot docs/assets/voice-launcher-empty.png
Sleep 4s
Screenshot docs/assets/voice-launcher-picker.png
Sleep 1s
Enter
Sleep 2s
Type "["
Sleep 2s
Screenshot docs/assets/voice-launcher-timeline.png
Sleep 2s
Type "q"
Sleep 1s
Down
Enter
Sleep 2s
Type "q"
Sleep 1s
Escape
Sleep 1s
Type "q"
Sleep 500ms
