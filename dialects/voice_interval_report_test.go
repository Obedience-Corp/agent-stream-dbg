package dialects_test

import (
	"context"
	"math"
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
