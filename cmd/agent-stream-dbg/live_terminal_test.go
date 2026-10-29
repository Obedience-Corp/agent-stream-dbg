package main

import (
	"github.com/Obedience-Corp/agent-stream-dbg/internal/captures"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/home"
	"os"
	"path/filepath"
	"testing"
)

func TestLiveVoiceTerminal(t *testing.T) {
	root := os.Getenv("OBEY_VOICE_LIVE_DEMO_ROOT")
	if root == "" {
		t.Skip("use just record-live-voice <obey-voice checkout>")
	}
	cfg := t.TempDir()
	if err := captures.SaveRoot(cfg, filepath.Join(root, "captures")); err != nil {
		t.Fatal(err)
	}
	if err := runHomeWithOptions(home.Options{Cwd: root, UserConfigDir: cfg}, "", correlationFlags{}); err != nil {
		t.Fatal(err)
	}
}
