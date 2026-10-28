package visualizer

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
)

// Measures the real file transport, poll, decoder, Update, and View. The end is
// the View string containing the appended marker, not a physical screen scan.
func TestObserverFileToViewPopulation(t *testing.T) {
	root := os.Getenv("AGENT_STREAM_DBG_FRESHNESS_ROOT")
	if root == "" {
		t.Skip("opt-in file-to-view population")
	}
	if !filepath.IsAbs(root) {
		t.Fatal("absolute evidence root required")
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "events.jsonl")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := fmt.Fprintln(file, `{"type":"header","seq":1,"trace_schema":"obey-voice.trace/v1","trace":{"capture_id":"freshness","runtime_id":"runtime","session_id":"session"}}`); err != nil {
		t.Fatal(err)
	}
	parser, err := bridge.NewParserFor("../../dialects/obey-voice.yaml")
	if err != nil {
		t.Fatal(err)
	}
	tr := filetransport.New(path, true)
	model := NewObserverModel(context.Background(), tr, parser, path, true, "")
	defer model.Close()
	model.pane = PaneMessages
	if connected := model.Init()(); connected.(observerConnected).err != nil {
		t.Fatal(connected)
	} else {
		model.Update(connected)
	}
	header := model.waitFrame()()
	model.Update(header)
	const n = 500
	starts := make(chan time.Time, n)
	writeErr := make(chan error, 1)
	go func() {
		tick := time.NewTicker(10 * time.Millisecond)
		defer tick.Stop()
		for i := 0; i < n; i++ {
			<-tick.C
			start := time.Now()
			starts <- start
			_, err := fmt.Fprintf(file, "{\"type\":\"user\",\"seq\":%d,\"text\":\"freshness-marker-%04d\",\"trace\":{\"capture_id\":\"freshness\",\"runtime_id\":\"runtime\",\"session_id\":\"session\",\"turn_id\":\"turn-%04d\",\"turn_start_ms\":%d}}\n", i+2, i, i, i*10)
			if err != nil {
				writeErr <- err
				return
			}
		}
		writeErr <- nil
	}()
	durations := make([]int64, 0, n)
	for i := 0; i < n; i++ {
		message := model.waitFrame()()
		frame, ok := message.(observerFrame)
		if !ok || !frame.ok || frame.frame.Err != nil {
			t.Fatalf("frame%d: %+v", i, message)
		}
		model.Update(frame)
		view := model.View()
		if !strings.Contains(view, fmt.Sprintf("freshness-marker-%04d", i)) {
			t.Fatalf("View lacks source marker%d: %s", i, view)
		}
		durations = append(durations, time.Since(<-starts).Nanoseconds())
	}
	if err := <-writeErr; err != nil {
		t.Fatal(err)
	}
	sorted := append([]int64(nil), durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	report := map[string]any{"measurement": "before append Write to completed ObserverModel.View containing matching source marker; actual follow poll/framing/bounded decode/Update/View included; no physical screen claim", "n": n, "append_rate_hz": 100, "poll_interval_ms": 50, "sample_ns": durations, "p50_ns": sorted[249], "p95_ns": sorted[474], "max_ns": sorted[499], "budget_p95_ns": 150000000}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "freshness-report.json"), append(data, '\n'), 0600); err != nil {
		t.Fatal(err)
	}
	if sorted[474] > 150000000 {
		t.Fatalf("file-to-view p95=%dns exceeds150ms; raw population retained", sorted[474])
	}
	t.Logf("file-to-view p95=%dns n=%d", sorted[474], n)
}
