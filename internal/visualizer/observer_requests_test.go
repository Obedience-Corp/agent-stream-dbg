package visualizer

import "testing"

func TestRequestUnaccountedTime(t *testing.T) {
	ms := func(value float64) *float64 { return &value }
	for _, tc := range []struct {
		name     string
		interval inspectedInterval
		want     *float64
	}{
		{"no request evidence", inspectedInterval{Start: ms(0), End: ms(100)}, nil},
		{"running", inspectedInterval{Start: ms(0), Request: &requestEvidence{Load: ms(10)}}, nil},
		{"invalid bounds", inspectedInterval{Start: ms(0), End: ms(100), Invalid: true, Request: &requestEvidence{Load: ms(10)}}, nil},
		{"no provider durations", inspectedInterval{Start: ms(0), End: ms(100), Request: &requestEvidence{PromptTokens: ms(40)}}, nil},
		{"null durations are not zero", inspectedInterval{Start: ms(1410), End: ms(3100), Request: &requestEvidence{Prefill: ms(1200), Generation: ms(480)}}, ms(10)},
		{"all durations", inspectedInterval{Start: ms(4564.563584), End: ms(13418.782292), Request: &requestEvidence{Load: ms(5260.404209), Prefill: ms(988.107583), Generation: ms(229.919417)}}, ms(2375.787499)},
		{"measured zero counts", inspectedInterval{Start: ms(0), End: ms(100), Request: &requestEvidence{Load: ms(0)}}, ms(100)},
		{"provider exceeds wall", inspectedInterval{Start: ms(0), End: ms(100), Request: &requestEvidence{Generation: ms(130)}}, ms(-30)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := requestUnaccountedMS(&tc.interval)
			if (got == nil) != (tc.want == nil) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if got != nil && (*got-*tc.want > 1e-6 || *tc.want-*got > 1e-6) {
				t.Fatalf("got %.6f, want %.6f", *got, *tc.want)
			}
		})
	}
}

func TestRequestEvidenceMergeKeepsLatestMeasurement(t *testing.T) {
	start := requestFromFields(map[string]any{"request_provider": "provider", "request_model": "model-a", "load_ms": nil})
	end := requestFromFields(map[string]any{"request_model": "model-a", "prefill_ms": 150.0, "prompt_tokens": 322.0})
	merged := mergeRequestEvidence(start, end)
	if merged.Provider != "provider" || merged.Prefill == nil || *merged.Prefill != 150 || merged.Load != nil || *merged.PromptTokens != 322 {
		t.Fatalf("merge lost evidence: %+v", merged)
	}
	if requestFromFields(map[string]any{"start_ms": 1.0}) != nil {
		t.Fatal("ordinary span given request evidence")
	}
}

func TestRequestDetailsMarkNegativeUnaccountedTime(t *testing.T) {
	ms := func(value float64) *float64 { return &value }
	over := requestDetails(&inspectedInterval{Start: ms(0), End: ms(100), Request: &requestEvidence{Prefill: ms(60), Generation: ms(70)}})
	if got := over["unaccounted_time"]; got != "-30.000ms (provider reported more than wall time)" {
		t.Fatalf("negative unaccounted time %q", got)
	}
	under := requestDetails(&inspectedInterval{Start: ms(0), End: ms(100), Request: &requestEvidence{Prefill: ms(60)}})
	if got := under["unaccounted_time"]; got != "40.000ms" {
		t.Fatalf("positive unaccounted time %q", got)
	}
}
