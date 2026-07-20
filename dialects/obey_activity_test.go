package dialects_test

import (
	"testing"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/mapping"
)

func TestObeyActivityDialect_NormalizesNestedActivity(t *testing.T) {
	engine, err := mapping.LoadFile("obey-activity.yaml")
	if err != nil {
		t.Fatalf("mapping.LoadFile: %v", err)
	}

	evt := engine.Decode("local.v1.WatchAllActivityResponse", []byte(`{
  "event": {
    "session_id": "session-1",
    "agent_name": "worker-a",
    "durable_sequence": 12,
    "dropped_count": 0,
    "activity": {
      "type": "AGENT_MESSAGE_DELTA",
      "timestamp": "2026-07-18T12:34:56.123Z",
      "turn_id": "turn-1",
      "message_delta": {"content": "hello"}
    }
  }
}`))

	if evt.Kind != events.KindContent {
		t.Fatalf("expected content, got %v", evt.Kind)
	}
	if evt.SourceID != "worker-a" || evt.Content != "hello" || evt.Seq != 12 {
		t.Fatalf("unexpected normalized event: source=%q content=%q seq=%d", evt.SourceID, evt.Content, evt.Seq)
	}
	wantTime := time.Date(2026, time.July, 18, 12, 34, 56, 123000000, time.UTC)
	if !evt.Timestamp.Equal(wantTime) {
		t.Fatalf("expected timestamp %s, got %s", wantTime, evt.Timestamp)
	}
	if evt.Fields["turn_id"] != "turn-1" {
		t.Errorf("expected turn_id field, got %v", evt.Fields["turn_id"])
	}
}

func TestObeyActivityDialect_PreservesMetaAndFutureActivity(t *testing.T) {
	engine, err := mapping.LoadFile("obey-activity.yaml")
	if err != nil {
		t.Fatalf("mapping.LoadFile: %v", err)
	}

	meta := engine.Decode("", []byte(`{"event":{"meta_event":"META_EVENT_HELLO","session_id":"session-1"}}`))
	if meta.Kind != events.KindDetail || meta.Fields["meta_event"] != "META_EVENT_HELLO" {
		t.Fatalf("expected visible meta detail, got kind=%v fields=%v", meta.Kind, meta.Fields)
	}

	future := engine.Decode("", []byte(`{"event":{"agent_name":"worker-a","activity":{"type":"AGENT_FUTURE_EVENT","turn_id":"turn-2"}}}`))
	if future.Kind != events.KindDetail || future.SourceID != "worker-a" {
		t.Fatalf("expected future activity to remain visible, got kind=%v source=%q", future.Kind, future.SourceID)
	}
}
