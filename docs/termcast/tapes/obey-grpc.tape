Output out/tui-obey-grpc.gif

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

# The caller supplies a live Obey daemon at the temporary Unix socket. This
# tape keeps its generated config and logs under an isolated HOME directory.
Hide
Type "cd /tmp; export HOME=$(mktemp -d /tmp/agent-stream-dbg-obey-termcast-home.XXXXXX); export TERM=xterm-256color COLORTERM=truecolor CLICOLOR_FORCE=1; clear" Enter
Show

# First-run setup through the actual config panel. The daemon is plaintext
# gRPC over a Unix socket, and WatchCampaignState requires the initial snapshot
# request to produce the first frame immediately.
Type "/tmp/agent-stream-dbg-obey-tui" Sleep 500ms Enter
Sleep 2s
Type "obey" Sleep 150ms
Tab
Type "grpc" Sleep 150ms
Tab
Tab
Type "unix:///tmp/obey-agent-stream-dbg.sock" Sleep 150ms
Tab
Type "plaintext" Sleep 150ms
Tab
Type "/local.v1.LocalDaemonService/WatchCampaignState" Sleep 150ms
Tab
Type '{"include_initial_snapshot":true}' Sleep 150ms
Tab
Type "oneof" Sleep 150ms
Tab
Tab
Tab
Type "region=obey" Sleep 500ms
Ctrl+S
Sleep 2s

# Start the configured live stream. WatchCampaignState is a watch RPC, so the
# message is retained in history while the daemon stream supplies its frames.
Type "i"
Sleep 500ms
Type "Inspect the live Obey campaign state stream."
Sleep 1s
Enter
Sleep 8s
Escape
Sleep 1s

# Exercise pane switching, list navigation, raw/parsed toggle, and config
# reopen/cancel after the real stream has produced frames.
Type "1" Sleep 1s
Type "j" Type "j" Type "k" Type "p" Type "a" Type "[" Type "]" Sleep 1s
Type "2" Sleep 1s
Type "f" Type "j" Type "k" Sleep 1s
Type "3" Sleep 1s
Type "j" Type "k" Type "g" Type "G" Sleep 1s
Type "4" Sleep 1s
Type "j" Type "k" Type "Enter" Type "t" Type "W" Sleep 1s
Type "5" Sleep 1s
Ctrl+T
Sleep 1s
Ctrl+T
Type "c" Sleep 2s
Escape
Sleep 1s
Ctrl+C
