package mapping

import (
	"testing"
	"time"
)

func TestDecode_ExtractsNestedTimestamp(t *testing.T) {
	engine, err := Load([]byte(`
version: 1
name: nested-time
discriminator: event
rules:
  - match: {event: activity}
    kind: detail
    timestamp: envelope.observed_at
`))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	evt := engine.Decode("activity", []byte(`{"envelope":{"observed_at":"2026-07-18T12:34:56.123Z"}}`))
	want := time.Date(2026, time.July, 18, 12, 34, 56, 123000000, time.UTC)
	if !evt.Timestamp.Equal(want) {
		t.Fatalf("expected nested timestamp %s, got %s", want, evt.Timestamp)
	}
}
