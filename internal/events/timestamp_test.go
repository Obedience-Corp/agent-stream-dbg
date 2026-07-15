package events

import (
	"testing"
	"time"
)

func TestParseTimestamp_GarbageYieldsReceiptTime(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty", ``},
		{"null", `null`},
		{"garbage string", `"not-a-date"`},
		{"garbage object", `{"foo":"bar"}`},
		{"zero protobuf timestamp", `{"seconds":0,"nanos":0}`},
		{"tiny number below epoch heuristic", `5`},
		{"malformed json", `{`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := time.Now()
			got := ParseTimestamp([]byte(tt.raw))
			after := time.Now()

			if got.Before(before) || got.After(after) {
				t.Errorf("expected local receipt time between %v and %v, got %v", before, after, got)
			}
		})
	}
}

func TestParseTimestamp_AcceptedFormats(t *testing.T) {
	wantSec := int64(1706432400) // 2024-01-28T09:00:00Z

	tests := []struct {
		name string
		raw  string
	}{
		{"RFC3339", `"2024-01-28T09:00:00Z"`},
		{"RFC3339Nano", `"2024-01-28T09:00:00.123456789Z"`},
		{"RFC3339 with offset", `"2024-01-28T04:00:00-05:00"`},
		{"epoch seconds bare number", `1706432400`},
		{"epoch seconds as string", `"1706432400"`},
		{"epoch millis bare number", `1706432400000`},
		{"protobuf timestamp form", `{"seconds":1706432400,"nanos":0}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseTimestamp([]byte(tt.raw))
			if got.Unix() != wantSec {
				t.Errorf("expected unix time %d, got %d (%v)", wantSec, got.Unix(), got)
			}
		})
	}
}

func TestParseTimestamp_RoundTripsRFC3339Nano(t *testing.T) {
	want := time.Date(2026, 7, 15, 4, 8, 0, 123456000, time.UTC)
	raw := []byte(`"` + want.Format(time.RFC3339Nano) + `"`)

	got := ParseTimestamp(raw)
	if !got.Equal(want) {
		t.Errorf("expected %v, got %v", want, got)
	}
}
