package visualizer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
)

// Uses an actual owning-TUI capture, replaying its exact bytes incrementally.
// Reopening the same saved file must preserve every normalized source boundary.
func TestObserverOwningCaptureLiveSavedEquivalence(t *testing.T) {
	source, root := os.Getenv("AGENT_STREAM_DBG_INTEGRATION_CAPTURE"), os.Getenv("AGENT_STREAM_DBG_INTEGRATION_ROOT")
	if source == "" || root == "" {
		t.Skip("opt-in actual owning capture integration")
	}
	data, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 8<<20 {
		t.Fatal("explicit integration stimulus exceeds8MiB")
	}
	lines := bytes.SplitAfter(data, []byte{'\n'})
	if len(lines) < 2 || len(lines[len(lines)-1]) != 0 {
		t.Fatal("sealed complete capture required")
	}
	lines = lines[:len(lines)-1]
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Write(lines[0]); err != nil {
		t.Fatal(err)
	}
	create := func(follow bool) *ObserverModel {
		parser, err := bridge.NewParserFor("../../dialects/obey-voice.yaml")
		if err != nil {
			t.Fatal(err)
		}
		m := NewObserverModel(context.Background(), filetransport.New(path, follow), parser, path, follow, "")
		t.Cleanup(func() { m.Close() })
		m.Update(m.Init()())
		return m
	}
	live := create(true)
	writeErr := make(chan error, 1)
	go func() {
		for _, line := range lines[1:] {
			time.Sleep(time.Millisecond)
			if _, err := file.Write(line); err != nil {
				writeErr <- err
				return
			}
		}
		writeErr <- nil
	}()
	ingest := func(m *ObserverModel) {
		for {
			message := m.waitFrame()()
			frame, ok := message.(observerFrame)
			if !ok {
				t.Fatalf("unexpected source message%T", message)
			}
			m.Update(frame)
			_ = m.View()
			if !frame.ok {
				break
			}
			if frame.frame.Terminal {
				t.Fatalf("fatal source: %v", frame.frame.Err)
			}
		}
	}
	ingest(live)
	if err := <-writeErr; err != nil {
		t.Fatal(err)
	}
	saved := create(false)
	ingest(saved)
	reports := func(m *ObserverModel) map[string]ObserverIntervalReport {
		out := map[string]ObserverIntervalReport{}
		for _, group := range m.store.groups() {
			m.selectScope(group.Key, m.store)
			out[group.Key] = m.IntervalReport()
		}
		return out
	}
	a, b := reports(live), reports(saved)
	if !reflect.DeepEqual(a, b) {
		t.Fatal("same owning capture differs between incremental follow and finite saved inspection")
	}
	if live.captureID == "" || live.captureID != saved.captureID || live.runtimeID != saved.runtimeID || !live.sealed || !saved.sealed || len(live.store.records) != len(saved.store.records) {
		t.Fatal("capture identity/seal/raw records changed on reopen")
	}
	report := map[string]any{"source": source, "capture_id": live.captureID, "runtime_id": live.runtimeID, "records": len(live.store.records), "scopes": len(a), "same_normalized_reports": true, "live_sealed": live.sealed, "saved_sealed": saved.sealed, "reports": a}
	encoded, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "equivalence-report.json"), append(encoded, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	t.Log(fmt.Sprintf("live/saved capture%s: %d records, %d scopes agree", live.captureID, len(live.store.records), len(a)))
}
