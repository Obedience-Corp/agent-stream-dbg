package mapping

import (
	"runtime"
	"strings"
	"testing"
)

const intervalTestDialect = `version: 1
name: measured-array
discriminator: {path: type}
rules:
  - match: {event: metrics}
    kind: detail
    fields: {turn_id: turn_id}
    interval_arrays:
      - path: items
        anchor: origin
        fields: {segment_index: index, metadata: meta}
        spans:
          - {stage: compute, start: a, end: b, scale: 1000}
`

func TestIntervalArrayProjectionClockNullAndShape(t *testing.T) {
	engine, err := Load([]byte(intervalTestDialect))
	if err != nil {
		t.Fatal(err)
	}
	e := engine.DecodeBounded("", []byte(`{"type":"metrics","origin":10,"items":[{"index":0,"a":0,"b":0.01},{"index":1,"a":null,"b":0.02},{"index":2,"a":{},"b":0.03,"meta":{"untrusted":[]}}]}`), 4<<20)
	rows := e.Fields["interval_evidence"].([]any)
	zero := rows[0].(map[string]any)
	missing := rows[1].(map[string]any)
	bad := rows[2].(map[string]any)
	if zero["start_ms"] != 10.0 || zero["end_ms"] != 20.0 || zero["relative"] != false || missing["start_ms"] != nil || bad["invalid_timing"] != true || bad["metadata"] != nil {
		t.Fatalf("projection changed missing/shape: %+v", rows)
	}
}
func TestBoundedDecodeRejectsExpansionAndRetainsRaw(t *testing.T) {
	engine, err := Load([]byte(intervalTestDialect))
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"type":"metrics","turn_id":"old","items":[` + strings.Repeat(`{},`, 120000) + `{}]}`)
	runtime.GC()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	e := engine.DecodeBounded("", raw, 4<<20)
	runtime.ReadMemStats(&after)
	if e.StringField("projection_unavailable") == "" || string(e.Raw) != string(raw) || e.StringField("turn_id") != "old" || e.Fields["interval_evidence"] != nil {
		t.Fatal("oversized collections expanded or raw/scope lost")
	}
	if after.TotalAlloc-before.TotalAlloc > 8<<20 {
		t.Fatalf("bounded current-frame reserve exceeded: %d allocated bytes", after.TotalAlloc-before.TotalAlloc)
	}
	e = engine.DecodeBounded("", []byte(`{"type":"unknown","future":{"a":[1,2,3]}}`), 16)
	if e.StringField("projection_unavailable") == "" || len(e.Raw) == 0 {
		t.Fatal("budget exhaustion lost raw")
	}
}
func TestIntervalProjectionOutputIsBounded(t *testing.T) {
	engine, err := Load([]byte(intervalTestDialect))
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"type":"metrics","items":[` + strings.Repeat(`{"index":0,"a":0,"b":1},`, 300) + `{"index":1,"a":0,"b":1}]}`)
	e := engine.DecodeBounded("", raw, 4<<20)
	rows, _ := e.Fields["interval_evidence"].([]any)
	if len(rows) > maxProjectionItems || e.Fields["projection_limited"] != true {
		t.Fatal("unbounded/hidden projection truncation")
	}
}

func TestBoundedMalformedAndUnknownRawInspection(t *testing.T) {
	engine, err := Load([]byte(intervalTestDialect))
	if err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{`{broken`, `[]`, `null`, `{"type":"future","unknown":{"new":[1,2]}}`} {
		e := engine.DecodeBounded("", []byte(raw), 4<<20)
		if string(e.Raw) != raw || e.Kind != "unknown" {
			t.Fatalf("unknown/malformed raw lost: %q", raw)
		}
	}
}

func TestLookupNormalizesBoundedStageEnums(t *testing.T) {
	engine, err := Load([]byte(`version: 1
name: generic-enum
discriminator: {path: type}
rules:
  - match: {event: '*'}
    kind: detail
    fields:
      span_role: {lookup: operation, values: {alpha: input, beta: output}}
`))
	if err != nil {
		t.Fatal(err)
	}
	e := engine.DecodeBounded("", []byte(`{"type":"sample","operation":"alpha"}`), 4<<20)
	if e.StringField("span_role") != "input" {
		t.Fatal("enum lookup lost declared semantics")
	}
	e = engine.DecodeBounded("", []byte(`{"type":"sample","operation":{"untrusted":[]}}`), 4<<20)
	if e.Fields["span_role"] != nil {
		t.Fatal("malformed enum selector expanded")
	}
}
