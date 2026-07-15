package dialectinit

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/transport/replay"
)

func samplesFromFixture(t *testing.T, path string) []Sample {
	t.Helper()
	tr, err := replay.New(path, 0)
	if err != nil {
		t.Fatalf("replay.New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = tr.Close() }()

	var samples []Sample
	for f := range tr.Frames() {
		samples = append(samples, Sample{Name: f.Name, Data: f.Data})
	}
	return samples
}

func ruleByGroupKey(d *Draft, key string) *RuleGuess {
	for i := range d.Rules {
		if d.Rules[i].GroupKey == key {
			return &d.Rules[i]
		}
	}
	return nil
}

// TestInfer_OpenAIFixture_DetectsAutoDiscriminator satisfies this task's
// explicit Done-When: init on the openai fixture correctly detects
// discriminator: auto — no event names, no common payload discriminator
// field, purely structural.
func TestInfer_OpenAIFixture_DetectsAutoDiscriminator(t *testing.T) {
	samples := samplesFromFixture(t, "../../testdata/fixtures/openai-chat.jsonl")
	if len(samples) == 0 {
		t.Fatal("expected samples from openai fixture")
	}

	draft := Infer(samples, "testdata/fixtures/openai-chat.jsonl")

	if draft.Discriminator.Mode != "auto" {
		t.Fatalf("expected discriminator mode 'auto', got %q (note: %s)", draft.Discriminator.Mode, draft.Discriminator.Note)
	}
	if len(draft.Rules) == 0 {
		t.Fatal("expected at least one inferred rule")
	}

	rendered := draft.Render()
	if !strings.Contains(rendered, "discriminator: auto") {
		t.Errorf("expected rendered draft to declare discriminator: auto, got:\n%s", rendered)
	}
}

// TestInfer_BrainyardFixture_RecoversMostHandWrittenRules satisfies this
// task's explicit Done-When: init --from <the 002 fixture> recovers >=70%
// of hand-written brainyard.yaml rules (event coverage, correct source/
// content paths). Expectations below are hand-derived from
// dialects/brainyard.yaml itself — wizard's {const: wizard} source is
// deliberately not checked, since no field-based heuristic could ever
// discover a constant (the design's own acknowledged inference gap).
func TestInfer_BrainyardFixture_RecoversMostHandWrittenRules(t *testing.T) {
	samples := samplesFromFixture(t, "../../testdata/fixtures/brainyard-session.jsonl")
	if len(samples) == 0 {
		t.Fatal("expected samples from brainyard fixture")
	}

	draft := Infer(samples, "testdata/fixtures/brainyard-session.jsonl")
	if draft.Discriminator.Mode != "event" {
		t.Fatalf("expected discriminator mode 'event', got %q", draft.Discriminator.Mode)
	}

	type expectation struct {
		event       string
		wantSource  string
		wantContent string
		wantSeq     string
	}
	groundTruth := []expectation{
		{event: "session_start"},
		{event: "session_complete"},
		{event: "agent_stream_start", wantSource: "agent_id"},
		{event: "agent_content", wantSource: "agent_id", wantContent: "content", wantSeq: "sequence"},
		{event: "agent_stream_complete", wantSource: "agent_id"},
		{event: "wizard_stream_start"}, // source is {const: wizard} — unguessable, not checked
		{event: "wizard_content", wantContent: "content", wantSeq: "sequence"},
		{event: "wizard_stream_complete"},
		{event: "flow_step_start"},
		{event: "flow_step_end"},
		{event: "flow_step_detail"},
		{event: "error"},
	}

	total, passed := 0, 0
	for _, exp := range groundTruth {
		total++ // event coverage check
		rule := ruleByGroupKey(draft, exp.event)
		if rule == nil {
			t.Errorf("event %q: no rule inferred (coverage miss)", exp.event)
			continue
		}
		passed++

		if exp.wantSource != "" {
			total++
			if rule.Source == exp.wantSource {
				passed++
			} else {
				t.Errorf("event %q: expected source %q, got %q", exp.event, exp.wantSource, rule.Source)
			}
		}
		if exp.wantContent != "" {
			total++
			if rule.Content == exp.wantContent {
				passed++
			} else {
				t.Errorf("event %q: expected content %q, got %q", exp.event, exp.wantContent, rule.Content)
			}
		}
		if exp.wantSeq != "" {
			total++
			if rule.Seq == exp.wantSeq {
				passed++
			} else {
				t.Errorf("event %q: expected seq %q, got %q", exp.event, exp.wantSeq, rule.Seq)
			}
		}
	}

	recovery := float64(passed) / float64(total)
	t.Logf("recovery: %d/%d = %.1f%%", passed, total, recovery*100)
	if recovery < 0.70 {
		t.Errorf("expected >=70%% recovery of hand-written brainyard.yaml rules, got %.1f%%", recovery*100)
	}
}

func TestInfer_EmptySamples(t *testing.T) {
	draft := Infer(nil, "nowhere")
	if draft.FrameCount != 0 {
		t.Errorf("expected FrameCount 0, got %d", draft.FrameCount)
	}
	if len(draft.Rules) != 0 {
		t.Errorf("expected no rules, got %d", len(draft.Rules))
	}
	rendered := draft.Render()
	if !strings.Contains(rendered, "no frames observed") {
		t.Errorf("expected empty-draft note in render, got:\n%s", rendered)
	}
}

func TestInfer_AnthropicFixture_DetectsPathOrEventDiscriminator(t *testing.T) {
	samples := samplesFromFixture(t, "../../testdata/fixtures/anthropic-messages.jsonl")
	draft := Infer(samples, "testdata/fixtures/anthropic-messages.jsonl")

	// The fixture's flat JSONL always carries a "type" field, so the
	// heuristic reasonably infers "event" mode from Frame.Name (the
	// replay transport already reads that same field as the dispatch
	// name) — a defensible draft even though the hand-written dialect
	// chose {path: type} for its own stylistic reasons.
	if draft.Discriminator.Mode != "event" && draft.Discriminator.Mode != "path" {
		t.Errorf("expected discriminator mode 'event' or 'path', got %q", draft.Discriminator.Mode)
	}
	if ruleByGroupKey(draft, "message_start") == nil {
		t.Error("expected a rule inferred for message_start")
	}
	if ruleByGroupKey(draft, "message_stop") == nil {
		t.Error("expected a rule inferred for message_stop")
	}
}

// TestInfer_AutoMode_AvoidsCommonFieldAsMatch reproduces a real bug caught
// during development: a field present in every shape (like a "created"
// timestamp on every chunk) must never be chosen as the distinguishing
// match, since it would match every group, not just this one.
func TestInfer_AutoMode_AvoidsCommonFieldAsMatch(t *testing.T) {
	samples := []Sample{
		{Data: []byte(`{"created":1,"kind_a":"x"}`)},
		{Data: []byte(`{"created":1,"kind_a":"x"}`)},
		{Data: []byte(`{"created":1,"kind_b":"y"}`)},
	}
	draft := Infer(samples, "test")
	if len(draft.Rules) != 2 {
		t.Fatalf("expected 2 groups (kind_a, kind_b), got %d", len(draft.Rules))
	}
	for _, r := range draft.Rules {
		if strings.Contains(r.MatchComment, "exists: created") {
			t.Errorf("expected a field unique to its group, not the universally-present 'created', got %q", r.MatchComment)
		}
	}
}

// TestInfer_AutoMode_SubsetGroupFlagsOverlap covers the genuinely
// ambiguous case: one group's fields are a strict subset of another's, so
// no field is truly unique — the guess must still pick something and
// honestly flag the overlap rather than claim false confidence.
func TestInfer_AutoMode_SubsetGroupFlagsOverlap(t *testing.T) {
	samples := []Sample{
		{Data: []byte(`{"a":"1","b":"2"}`)}, // superset shape
		{Data: []byte(`{"a":"3"}`)},         // subset shape
	}
	draft := Infer(samples, "test")
	if len(draft.Rules) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(draft.Rules))
	}
	sawOverlapWarning := false
	for _, r := range draft.Rules {
		if strings.Contains(r.MatchComment, "review manually") {
			sawOverlapWarning = true
		}
	}
	if !sawOverlapWarning {
		t.Error("expected at least one rule to flag the overlap between subset/superset shapes")
	}
}

func TestDraft_Render_UnclassifiedFrameShowsSample(t *testing.T) {
	samples := []Sample{
		{Name: "mystery_thing", Data: []byte(`{"foo":"bar","n":3}`)},
		{Name: "mystery_thing", Data: []byte(`{"foo":"baz","n":4}`)},
	}
	draft := Infer(samples, "test")
	rendered := draft.Render()

	if !strings.Contains(rendered, "UNCLASSIFIED") {
		t.Errorf("expected UNCLASSIFIED marker in render, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "kind: unknown") {
		t.Errorf("expected kind: unknown in render, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "# sample:") {
		t.Errorf("expected a sample comment in render, got:\n%s", rendered)
	}
}
