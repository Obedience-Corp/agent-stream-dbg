package visualizer

import (
	"bufio"
	"context"
	"fmt"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHistorySustainedPagesAndReplacementBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("sustained retained budget")
	}
	m, _ := newTestObserver(t, "")
	path := filepath.Join(t.TempDir(), "large.jsonl")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	writer := bufio.NewWriter(file)
	fmt.Fprint(writer, historyHeader)
	payload := strings.Repeat("x", 2048)
	for i := 0; i < 12000; i++ {
		fmt.Fprintf(writer, "{\"type\":\"delta\",\"capture_id\":\"capture\",\"runtime_id\":\"runtime\",\"turn_id\":\"long\",\"turn_start_ms\":1,\"padding\":%q}\n", payload)
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	request := historyRequest{Path: path, Parser: m.parser, CaptureID: "capture", Runtime: "runtime", Identity: info, MinSize: info.Size()}
	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	err = historyScan(context.Background(), request, 0, func(event *events.Event, frame transport.Frame, _ string) bool {
		m.store.add(event, frame, true)
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	key := eventScope(&events.Event{Fields: map[string]any{"runtime_id": "runtime", "turn_id": "long"}})
	request.Anchor = observerSelection{Key: key, Offset: int64(len(historyHeader))}
	peak := 0
	request.OwnBytes = func(bytes int) {
		peak = max(peak, m.store.bytes+bytes)
		if bytes > observerPageBytes {
			t.Fatal("in-flight page exceeded cap")
		}
	}
	page, _, err := loadObserverHistory(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	m.page = page
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	liveDelta := int64(after.HeapAlloc) - int64(before.HeapAlloc)
	if peak > 20<<20 || m.store.bytes+m.page.bytes > 20<<20 || len(m.store.records)+len(m.page.records) > 10000 {
		t.Fatal("combined retention cap exceeded")
	}
	if liveDelta > 32<<20 {
		t.Fatalf("live heap delta exceeds total observer budget: %d", liveDelta)
	}
	t.Logf("12000 records: live=%d/%d page=%d/%d combined peak=%d live heap delta=%d", len(m.store.records), m.store.bytes, len(m.page.records), m.page.bytes, peak, liveDelta)
	m.path = path
	m.captureID = "capture"
	m.runtimeID = "runtime"
	m.selectionAnchor = request.Anchor
	command := m.requestHistory(0)
	if m.page != nil {
		t.Fatal("replacement kept old page")
	}
	message := command().(*observerHistoryResult)
	m.historyResult(message)
	if m.store.bytes+m.page.bytes > 20<<20 || m.historyPeak.Load() > observerPageBytes {
		t.Fatal("replacement exceeded shared retained cap")
	}
}

func TestHistoryEscapedIdentityAccounting(t *testing.T) {
	store := newObserverStore()
	runtimeID := strings.Repeat("%", 256)
	for i := 0; i < 10000; i++ {
		turn := fmt.Sprintf("%0256d", i)
		turn = strings.ReplaceAll(turn, "0", "/")
		e := &events.Event{Name: "detail", Seq: i + 1, Fields: map[string]any{"capture_id": strings.Repeat("%", 256), "runtime_id": runtimeID, "turn_id": turn, "turn_start_ms": float64(i)}}
		raw := []byte(fmt.Sprintf(`{"seq":%d}`, i+1))
		store.add(e, transport.Frame{Data: raw, Raw: raw, Offset: int64(i) * 100, Size: int64(len(raw))}, true)
	}
	actual := 0
	for _, record := range store.records {
		minimum := recordBytes(record.Event) + len(record.Scope) + len(record.dedup) + len(record.Issue)
		if record.Bytes < minimum || len(record.Scope) < 1000 {
			t.Fatalf("escaped identity omitted from cost: cost=%d minimum=%d scope=%d", record.Bytes, minimum, len(record.Scope))
		}
		actual += record.Bytes
	}
	if actual != store.bytes || actual > observerPageBytes || !store.evicted {
		t.Fatal("escaped identity budget not enforced", actual, store.bytes)
	}
	t.Logf("escaped identities: records=%d accounted bytes=%d", len(store.records), store.bytes)
}
