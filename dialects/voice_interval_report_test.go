package dialects_test

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/visualizer"
	tea "github.com/charmbracelet/bubbletea"
)

func TestVoiceCanonicalMeasuredIntervalReport(t *testing.T) {
	parser, err := bridge.NewParserFor("obey-voice")
	if err != nil {
		t.Fatal(err)
	}
	source := filetransport.New("../testdata/fixtures/pipeline-trace/overlap-interrupted.jsonl", false)
	model := visualizer.NewObserverModel(context.Background(), source, parser, "canonical", false, "")
	defer model.Close()
	_, cmd := model.Update(model.Init()())
	for cmd != nil {
		message := cmd()
		if message == nil {
			break
		}
		_, cmd = model.Update(message)
	}
	model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'['}})
	report := model.IntervalReport()
	assert := func(name string, got, want float64) {
		t.Helper()
		if math.Abs(got-want) > 0.001 {
			t.Fatalf("%s=%f want%f", name, got, want)
		}
	}
	if report.TurnDurationMS == nil || report.FirstOutputDelayMS == nil {
		t.Fatal("source turn/final metric unavailable")
	}
	assert("turn", *report.TurnDurationMS, 2200)
	assert("first software output", *report.FirstOutputDelayMS, 180)
	assert("overlap", report.SegmentOverlapMS, 250)
	assert("gap", report.GapTotalMS, 600)
	assert("waiting text", report.GapByCause["waiting_text"], 250)
	assert("synthesis", report.GapByCause["synthesis"], 350)
	if report.Limited || report.Partial {
		t.Fatal("small canonical fixture unexpectedly limited")
	}
}

func TestVoiceBackgroundJobSelectionKeepsTypedIdentity(t *testing.T) {
	parser, err := bridge.NewParserFor("obey-voice")
	if err != nil {
		t.Fatal(err)
	}
	path := "../testdata/fixtures/pipeline-trace/background-job.jsonl"
	source := filetransport.New(path, false)
	model := visualizer.NewObserverModel(context.Background(), source, parser, path, false, "")
	defer model.Close()
	model.Update(tea.WindowSizeMsg{Width: 120, Height: 100})
	_, command := model.Update(model.Init()())
	for command != nil {
		message := command()
		if message == nil {
			break
		}
		_, command = model.Update(message)
	}
	key := func(value string) { model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(value)}) }
	key("s")
	model.Update(tea.KeyMsg{Type: tea.KeyDown})
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	key("3")
	model.Update(tea.KeyMsg{Type: tea.KeyUp}) // choose interval0 rather than retained Flow row1
	report := model.IntervalReport()
	if !strings.Contains(report.Scope, "/job/typed-job-1") || report.TurnDurationMS != nil || report.FirstOutputDelayMS != nil {
		t.Fatal("job adopted causal foreground turn", report.Scope, report.TurnDurationMS)
	}
	if len(report.Intervals) != 2 {
		t.Fatalf("queue/worker evidence split/lost: %+v", report.Intervals)
	}
	found := false
	for _, interval := range report.Intervals {
		if interval.Stage == "worker" && interval.Start != nil && interval.End != nil && *interval.End-*interval.Start == 500 {
			found = true
		}
	}
	if !found {
		t.Fatal("typed job worker timing unavailable")
	}
	model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	view := model.View()
	if !strings.Contains(view, `"job_id": "typed-job-1"`) || !strings.Contains(view, `"parent_turn_id": "causal-turn"`) || !strings.Contains(view, `"turn_id": null`) {
		t.Fatal("parsed job source context unavailable", view)
	}
	key("r")
	view = model.View()
	if !strings.Contains(view, report.Intervals[0].ID) || !strings.Contains(view, "typed-job-1") {
		t.Fatal("raw evidence lost typed/opaque identity", view)
	}
}
