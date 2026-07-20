package mapping

import (
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
)

func testEngine(t *testing.T) *Engine {
	t.Helper()
	engine, err := Load([]byte(`
version: 1
name: test
discriminator: event
rules:
  - match: {event: agent_content}
    kind: content
    source: agent_id
    content: content
    seq: sequence
`))
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}
	return engine
}

func TestEngine_Decode_MatchedRule(t *testing.T) {
	engine := testEngine(t)

	evt := engine.Decode("agent_content", []byte(`{"type":"agent_content","agent_id":"a1","content":"hi","sequence":3}`))

	if evt.Kind != events.KindContent {
		t.Errorf("expected kind %v, got %v", events.KindContent, evt.Kind)
	}
	if evt.SourceID != "a1" {
		t.Errorf("expected source 'a1', got %q", evt.SourceID)
	}
	if evt.Content != "hi" {
		t.Errorf("expected content 'hi', got %q", evt.Content)
	}
	if evt.Seq != 3 {
		t.Errorf("expected seq 3, got %d", evt.Seq)
	}
	if evt.Fields["agent_id"] != "a1" {
		t.Errorf("expected fields.agent_id 'a1', got %v", evt.Fields["agent_id"])
	}
}

func TestEngine_Decode_NeverErrorsOnUnknown(t *testing.T) {
	engine := testEngine(t)

	evt := engine.Decode("totally_unrecognized_event", []byte(`{"type":"totally_unrecognized_event","some_field":"some_value"}`))

	if evt == nil {
		t.Fatal("Decode must never return nil — unknown frames are a first-class outcome")
	}
	if evt.Kind != events.KindUnknown {
		t.Errorf("expected kind %v for unmatched frame, got %v", events.KindUnknown, evt.Kind)
	}
	if evt.Name != "totally_unrecognized_event" {
		t.Errorf("expected Name preserved, got %q", evt.Name)
	}
	if len(evt.Raw) == 0 {
		t.Error("expected Raw to be preserved for unknown frame")
	}
	if evt.Fields["some_field"] != "some_value" {
		t.Errorf("expected best-effort Fields parse for unknown frame, got %v", evt.Fields)
	}
}

func TestEngine_Decode_MalformedJSONPassesThrough(t *testing.T) {
	engine := testEngine(t)

	tests := []struct {
		name string
		data string
	}{
		{"not json at all", "this is not json"},
		{"truncated json", `{"type":"agent_content"`},
		{"bare SSE token", "Hello"},
		{"empty body", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			evt := engine.Decode("agent_content", []byte(tt.data))
			if evt == nil {
				t.Fatal("Decode must never return nil, even for malformed JSON")
			}
			if string(evt.Raw) != tt.data {
				t.Errorf("expected Raw preserved exactly, got %q want %q", evt.Raw, tt.data)
			}
			if evt.Fields != nil {
				t.Errorf("expected Fields empty (nil) for malformed JSON, got %v", evt.Fields)
			}
		})
	}
}

func TestEngine_Decode_StatelessGuarantee(t *testing.T) {
	// Same input must always yield the same output, regardless of what was
	// decoded before it or in what order — no cross-frame memory in the DSL.
	engine := testEngine(t)

	frame1 := []byte(`{"type":"agent_content","agent_id":"a1","content":"first","sequence":1}`)
	frame2 := []byte(`{"type":"agent_content","agent_id":"a2","content":"second","sequence":2}`)
	frame3 := []byte(`{"type":"unknown_thing","x":"y"}`)

	// Decode in one order.
	a1 := engine.Decode("agent_content", frame1)
	a2 := engine.Decode("agent_content", frame2)
	a3 := engine.Decode("unknown_thing", frame3)

	// Decode again, interleaved differently, from a fresh engine instance
	// loaded from the same source — must produce identical results.
	engine2 := testEngine(t)
	b3 := engine2.Decode("unknown_thing", frame3)
	b1 := engine2.Decode("agent_content", frame1)
	b2 := engine2.Decode("agent_content", frame2)

	// And decode frame1 again on the ORIGINAL engine, after having already
	// decoded frame2 and frame3 through it — must match the first result.
	a1Again := engine.Decode("agent_content", frame1)

	for _, pair := range []struct {
		name      string
		got, want *events.Event
	}{
		{"frame1 vs repeat on same engine", a1Again, a1},
		{"frame1 across engines/order", b1, a1},
		{"frame2 across engines/order", b2, a2},
		{"frame3 across engines/order", b3, a3},
	} {
		t.Run(pair.name, func(t *testing.T) {
			if pair.got.Kind != pair.want.Kind || pair.got.SourceID != pair.want.SourceID ||
				pair.got.Content != pair.want.Content || pair.got.Seq != pair.want.Seq {
				t.Errorf("expected identical decode result, got %+v want %+v", pair.got, pair.want)
			}
		})
	}
}
