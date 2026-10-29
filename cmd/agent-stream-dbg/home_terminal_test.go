package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/captures"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/home"
)

// TestVoiceLauncherTerminal runs the product home/observer loop in a real PTY.
// It is opt-in because it accepts interactive keyboard input. All config and
// captures are disposable; no backend, microphone, speaker, or user state is used.
func TestVoiceLauncherTerminal(t *testing.T) {
	if os.Getenv("AGENT_STREAM_DBG_TERMINAL_DEMO") != "1" {
		t.Skip("terminal harness; use just record-voice-launcher")
	}
	fixture, err := os.ReadFile("../../testdata/fixtures/pipeline-trace/overlap-interrupted.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp("/tmp", "voice-launcher-demo-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	captureRoot := filepath.Join(root, "captures")
	if err := os.Mkdir(captureRoot, 0700); err != nil {
		t.Fatal(err)
	}
	configDir := filepath.Join(root, "config")
	if err := captures.SaveRoot(configDir, captureRoot); err != nil {
		t.Fatal(err)
	}
	publish := func(name, status string, stamp time.Time, content []byte) error {
		dir := filepath.Join(captureRoot, name)
		if err := os.Mkdir(dir, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "events.jsonl"), content, 0600); err != nil {
			return err
		}
		raw, err := json.Marshal(captures.Entry{Schema: "obey-voice.capture/v1", CaptureID: "fixture-capture", SessionID: name, Status: status, Mode: "talk", StartedAt: stamp})
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "manifest.json"), raw, 0600)
	}
	done := make(chan struct{})
	defer close(done)
	published := make(chan error, 1)
	go func() {
		select {
		case <-done:
			return
		case <-time.After(4 * time.Second):
		}
		started := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
		if err := publish("saved-conversation", "sealed", started, fixture); err != nil {
			published <- err
			return
		}
		lines := strings.Split(strings.TrimSpace(string(fixture)), "\n")
		published <- publish("current-conversation", "open", started.Add(time.Minute), []byte(strings.Join(lines[:len(lines)-1], "\n")+"\n"))
	}()
	if err := runHomeWithOptions(home.Options{Cwd: root, UserConfigDir: configDir}, "", correlationFlags{}); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-published:
		if err != nil {
			t.Fatal(err)
		}
	default:
	}
}
