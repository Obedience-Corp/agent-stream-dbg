package dialects_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/visualizer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const modelRequestFixture = "../testdata/fixtures/pipeline-trace/model-requests.jsonl"

func openVoiceObserver(t *testing.T, path string) (*visualizer.ObserverModel, func(string)) {
	t.Helper()
	return openVoiceObserverAt(t, path, 140)
}

func openVoiceObserverAt(t *testing.T, path string, width int) (*visualizer.ObserverModel, func(string)) {
	t.Helper()
	parser, err := bridge.NewParserFor("obey-voice")
	if err != nil {
		t.Fatal(err)
	}
	model := visualizer.NewObserverModel(context.Background(), filetransport.New(path, false), parser, path, false, "")
	t.Cleanup(func() { model.Close() })
	model.Update(tea.WindowSizeMsg{Width: width, Height: 60})
	_, command := model.Update(model.Init()())
	for command != nil {
		message := command()
		if message == nil {
			break
		}
		_, command = model.Update(message)
	}
	key := func(value string) {
		switch value {
		case "down":
			model.Update(tea.KeyMsg{Type: tea.KeyDown})
		case "enter":
			model.Update(tea.KeyMsg{Type: tea.KeyEnter})
		case "esc":
			model.Update(tea.KeyMsg{Type: tea.KeyEsc})
		default:
			model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)})
		}
	}
	return model, key
}

func viewText(model *visualizer.ObserverModel) string { return ansi.Strip(model.View()) }

func requireInOrder(t *testing.T, view string, parts ...string) {
	t.Helper()
	at := 0
	for _, part := range parts {
		index := strings.Index(view[at:], part)
		if index < 0 {
			t.Fatalf("%q missing or out of order:\n%s", part, view)
		}
		at += index + len(part)
	}
}

// selectSessionLane opens the scope list and chooses the unscoped lane.
func selectSessionLane(t *testing.T, model *visualizer.ObserverModel, key func(string)) {
	t.Helper()
	key("s")
	view := viewText(model)
	_, list, _ := strings.Cut(view, "Choose turn / worker / session")
	for index, row := range strings.Split(list, "\n")[1:] {
		if strings.Contains(row, "session lane") {
			for range index {
				key("down")
			}
			key("enter")
			return
		}
	}
	t.Fatalf("session lane not offered:\n%s", viewText(model))
}

func TestVoiceModelRequestsNestUnderModelStageInStartOrder(t *testing.T) {
	model, key := openVoiceObserver(t, modelRequestFixture)
	key("[")
	report := model.IntervalReport()
	if report.TurnDurationMS == nil || *report.TurnDurationMS != 4300 {
		t.Fatal("turn latency changed", report.TurnDurationMS)
	}
	labels := []string{}
	for _, interval := range report.Intervals {
		if interval.ParentStage == "model" {
			labels = append(labels, interval.Label)
		}
	}
	if strings.Join(labels, ",") != "skill_check,route,route_recovery,reply" {
		t.Fatal("request purposes or start order lost", labels)
	}
	key("1")
	view := viewText(model)
	requireInOrder(t, view,
		"model        · completed [4190ms]",
		"↳ ✓ skill_check    · completed [390ms]",
		"↳ ✓ route          · completed [1690ms]",
		"↳ ✓ route_recovery · completed [490ms]",
		"↳ ✓ reply          · completed [1590ms]",
		"segment_queue",
		"turn         · completed [4300ms]")
	if strings.Contains(view, "model_request") {
		t.Fatal("requests still shown as a separate stage", view)
	}
	key("3")
	requireInOrder(t, viewText(model), "4190.000ms", "model · skill_check", "model · route ", "model · route_recovery", "model · reply")
}

func TestVoiceFailedModelRequestKeepsParentStageDuration(t *testing.T) {
	model, key := openVoiceObserver(t, modelRequestFixture)
	key("1")
	view := viewText(model)
	requireInOrder(t, view, "model        · completed [40ms]", "↳ ✗ route          · failed [40ms]")
	key("3")
	if !strings.Contains(viewText(model), "✗ model · route") {
		t.Fatal("failed request not distinct on the timeline", viewText(model))
	}
	key("1")
	for range 3 {
		key("down")
	}
	key("enter")
	details := viewText(model)
	if !strings.Contains(details, `"label": "route"`) || !strings.Contains(details, `"prompt_tokens": "unavailable"`) || !strings.Contains(details, `"load": "unavailable"`) {
		t.Fatal("null request measurements not shown as unavailable", details)
	}
	if strings.Contains(details, "unaccounted_time") {
		t.Fatal("unaccounted time derived without provider durations", details)
	}
}

func TestVoiceModelRequestDetailsReportUnaccountedTime(t *testing.T) {
	model, key := openVoiceObserver(t, modelRequestFixture)
	key("[")
	key("1")
	for range 4 {
		key("down")
	}
	key("enter")
	details := viewText(model)
	for _, want := range []string{`"label": "route"`, `"provider": "ollama"`, `"model": "fixture-model"`, `"prompt_tokens": 2399`, `"output_tokens": 15`, `"first_token_after_start": "1480.000ms"`, `"wall_time": "1690.000ms"`, `"load": "unavailable"`, `"prefill": "1200.000ms"`, `"generation": "480.000ms"`, `"unaccounted_time": "10.000ms"`} {
		if !strings.Contains(details, want) {
			t.Fatal(want, details)
		}
	}
}

func TestVoiceWarmupRequestStaysOnSessionLane(t *testing.T) {
	model, key := openVoiceObserver(t, modelRequestFixture)
	key("[")
	for _, interval := range model.IntervalReport().Intervals {
		if interval.Label == "warmup" {
			t.Fatal("runtime-scoped warmup adopted a turn")
		}
	}
	selectSessionLane(t, model, key)
	key("1")
	requireInOrder(t, viewText(model), "model", "↳ ✓ warmup         · completed [1495ms]")
	key("down")
	key("down")
	key("down")
	key("enter")
	if !strings.Contains(viewText(model), `"unaccounted_time": "55.000ms"`) {
		t.Fatal("warmup unaccounted time wrong", viewText(model))
	}
}

// fixtureVariant rewrites the canonical fixture: records whose seq is in drop
// are removed, extra records are inserted before the seq they name, and the
// result is renumbered so the capture stays in order.
func fixtureVariant(t *testing.T, drop map[float64]bool, insert map[float64][]map[string]any) string {
	t.Helper()
	raw, err := os.ReadFile(modelRequestFixture)
	if err != nil {
		t.Fatal(err)
	}
	records := []map[string]any{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		seq := record["seq"].(float64)
		records = append(records, insert[seq]...)
		if !drop[seq] {
			records = append(records, record)
		}
	}
	out := []string{}
	for index, record := range records {
		record["seq"] = index + 1
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(encoded))
	}
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func requestRecord(id, state, status, purpose string, start float64, end any) map[string]any {
	return map[string]any{
		"type": "trace_span", "ts": "2026-11-14T12:00:04+00:00",
		"trace": map[string]any{"capture_id": "fixture-model-capture", "runtime_id": "fixture-model-runtime", "session_id": "fixture-model-session", "conversation_epoch": "model-epoch-1", "mono_ms": start, "turn_start_ms": 1000, "turn_id": "model-turn-1", "parent_turn_id": nil, "job_id": nil, "response_id": nil, "persona_id": nil},
		"span": map[string]any{"id": id, "parent_id": "model-turn-1", "stage": "model_request", "lane": "model", "segment_index": nil, "attempt_id": nil, "start_ms": start, "end_ms": end, "status": status, "state": state,
			"request": map[string]any{"purpose": purpose, "provider": "ollama", "model": "fixture-model", "first_token_ms": nil, "prompt_tokens": nil, "output_tokens": nil, "load_ms": nil, "prefill_ms": nil, "generation_ms": nil}},
	}
}

func TestVoiceModelRequestEndWithoutStartAndUnknownPurpose(t *testing.T) {
	path := fixtureVariant(t, map[float64]bool{2: true}, map[float64][]map[string]any{
		14: {
			requestRecord("opaque-request-6", "start", "running", "memory_digest", 4000, nil),
			requestRecord("opaque-request-7", "end", "cancelled", "reply", 4100, 4300),
		},
	})
	model, key := openVoiceObserver(t, path)
	key("[")
	key("1")
	view := viewText(model)
	requireInOrder(t, view,
		"model        · completed [4190ms]",
		"↳ ✓ reply          · completed [1590ms]",
		"↳ ● memory_digest  · running",
		"↳ ■ reply          · cancelled [200ms]",
		"turn         · completed [4300ms]")
	if strings.Contains(view, "running [") {
		t.Fatal("running request given a duration", view)
	}
	key("3")
	requireInOrder(t, viewText(model), "model · memory_digest", "running", "model · reply", "cancelled", "200.000ms")

	selectSessionLane(t, model, key)
	key("1")
	requireInOrder(t, viewText(model), "↳ ✓ warmup         · completed [1495ms]")
	for _, interval := range model.IntervalReport().Intervals {
		if interval.Label == "warmup" && (interval.Start == nil || interval.End == nil || *interval.Start != 5 || *interval.End != 1500) {
			t.Fatal("end-only warmup lost its bounds", interval.Start, interval.End)
		}
	}
}

// purposeVariant renames request purposes in turn 1 of the canonical fixture.
func purposeVariant(t *testing.T, purposes map[string]string) string {
	t.Helper()
	raw, err := os.ReadFile(modelRequestFixture)
	if err != nil {
		t.Fatal(err)
	}
	out := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatal(err)
		}
		if span, ok := record["span"].(map[string]any); ok {
			if request, ok := span["request"].(map[string]any); ok && span["parent_id"] == "model-turn-1" {
				if purpose, ok := purposes[request["purpose"].(string)]; ok {
					request["purpose"] = purpose
				}
			}
		}
		encoded, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(encoded))
	}
	path := filepath.Join(t.TempDir(), "events.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(out, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func requireFitsWidth(t *testing.T, view string, width int) {
	t.Helper()
	for _, line := range strings.Split(view, "\n") {
		if got := lipgloss.Width(line); got > width {
			t.Fatalf("line is %d cells, pane is %d: %q", got, width, line)
		}
	}
}

func TestVoiceLongRequestPurposesKeepStatusAndTimeVisible(t *testing.T) {
	long := strings.Repeat("p", 200)
	wide := strings.Repeat("経路確認", 6)
	path := purposeVariant(t, map[string]string{"route": long, "reply": wide})
	for _, width := range []int{60, 100} {
		model, key := openVoiceObserverAt(t, path, width)
		key("[")
		key("1")
		flow := viewText(model)
		requireFitsWidth(t, flow, width)
		requireInOrder(t, flow, "↳ ✓ skill_check", "· completed [390ms]", "↳ ✓ ppp", "…", "· completed [1690ms]", "↳ ✓ route_recovery", "· completed [490ms]", "↳ ✓ 経路", "…", "· completed [1590ms]")
		columns := map[int]bool{}
		for _, line := range strings.Split(flow, "\n") {
			if before, _, found := strings.Cut(line, " · completed ["); found && strings.Contains(line, "↳") {
				columns[lipgloss.Width(before)] = true
			}
		}
		if len(columns) != 1 {
			t.Fatalf("nested rows not aligned at width %d: %v\n%s", width, columns, flow)
		}
		key("3")
		timeline := viewText(model)
		requireFitsWidth(t, timeline, width)
		requireInOrder(t, timeline, "model · ppp", "…", "completed", "model · 経路", "…", "completed")
	}
}

func TestVoiceAddedRequestPurposesRenderAsSent(t *testing.T) {
	path := purposeVariant(t, map[string]string{"skill_check": "delegation_summary", "route": "discussion", "reply": "announcement"})
	for _, width := range []int{80, 100} {
		model, key := openVoiceObserverAt(t, path, width)
		key("[")
		key("1")
		flow := viewText(model)
		requireFitsWidth(t, flow, width)
		requireInOrder(t, flow,
			"↳ ✓ delegation_summary · completed [390ms]",
			"↳ ✓ discussion         · completed [1690ms]",
			"↳ ✓ route_recovery     · completed [490ms]",
			"↳ ✓ announcement       · completed [1590ms]")
		key("3")
		requireInOrder(t, viewText(model), "model · delegation_sum…", "model · discussion", "model · announcement")
	}
}
