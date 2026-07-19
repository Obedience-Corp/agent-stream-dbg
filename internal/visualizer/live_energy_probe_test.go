//go:build live

package visualizer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/bridge"
	"github.com/lancekrogers/stream-debugger/internal/client"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
)

// TestLiveBrainyardEnergy_TracksRealContentTokens requires a running Brainyard
// on localhost:5003 and API_KEY in the environment. Run:
//
//	API_KEY=... go test -tags=live ./internal/visualizer -run TestLiveBrainyardEnergy -v -count=1 -timeout 3m
func TestLiveBrainyardEnergy_TracksRealContentTokens(t *testing.T) {
	if os.Getenv("API_KEY") == "" {
		t.Skip("API_KEY not set")
	}

	dialectPath := filepath.Join("..", "..", "dialects", "brainyard.yaml")
	if _, err := os.Stat(dialectPath); err != nil {
		// From module root when run as ./internal/visualizer
		dialectPath = filepath.Join("dialects", "brainyard.yaml")
	}
	if _, err := os.Stat(dialectPath); err != nil {
		// Resolve from this file's package dir via cwd.
		wd, _ := os.Getwd()
		candidates := []string{
			filepath.Join(wd, "dialects", "brainyard.yaml"),
			filepath.Join(wd, "..", "..", "dialects", "brainyard.yaml"),
		}
		found := ""
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				found = c
				break
			}
		}
		if found == "" {
			t.Fatalf("cannot find dialects/brainyard.yaml from %s", wd)
		}
		dialectPath = found
	}

	cfg := &config.EnhancedConfig{
		Transport: config.TransportConfig{
			Type:           "sse",
			BaseURL:        envOr("BRAINYARD_URL", "http://localhost:5003"),
			StreamEndpoint: "/api/v3/sessions/{session_id}/stream",
			Method:         "POST",
			Headers:        map[string]string{"Accept": "text/event-stream"},
			Auth:           config.AuthConfig{Type: "bearer", Token: os.Getenv("API_KEY")},
		},
		Session: config.SessionConfig{
			ID:             envOr("SESSION_ID", fmt.Sprintf("live-energy-%d", time.Now().Unix())),
			AutoSetup:      true,
			DefaultAgents:  []string{"sam_harris", "eckhart_tolle", "wizard"},
			SetupEndpoint:  "/api/v3/debug/session",
		},
		Dialect: config.DialectConfig{File: dialectPath},
		LogDir:  t.TempDir(),
	}
	cfg.Normalize()

	parser := bridge.NewParser(dialectPath)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Session setup (same path as interactive TUI).
	vars := config.InterpolationVarsFromConfig(cfg)
	sessionID, err := parser.RunSetup(ctx, vars, cfg.Transport.ResolvedHeaders(), nil)
	if err != nil {
		t.Fatalf("session setup: %v", err)
	}
	if sessionID != "" {
		cfg.Session.ID = sessionID
	}
	t.Logf("session=%s agents=%v", cfg.Session.ID, cfg.Session.DefaultAgents)

	tr, err := client.NewTransport(cfg, parser, "In one short sentence, what is consciousness?")
	if err != nil {
		t.Fatalf("NewTransport: %v", err)
	}
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer tr.Close()

	var energy energyState
	energy = newEnergyState()

	type sample struct {
		at        time.Duration
		content   int
		rate      float64
		level     float64
		maxSample float64
	}
	var (
		start       = time.Now()
		contentN    int
		maxLevel    float64
		maxRate     float64
		snapshots   []sample
		lastTick    = start
		agentsSeen  = map[string]int{}
	)

	tick := func() {
		now := time.Now()
		energy.tick(now, true)
		if energy.Level() > maxLevel {
			maxLevel = energy.Level()
		}
		if energy.Rate() > maxRate {
			maxRate = energy.Rate()
		}
		// Snapshot ~4 times/sec for the report.
		if now.Sub(lastTick) >= 250*time.Millisecond {
			lastTick = now
			maxS := 0.0
			for _, v := range energy.Samples() {
				if v > maxS {
					maxS = v
				}
			}
			snapshots = append(snapshots, sample{
				at:        now.Sub(start),
				content:   contentN,
				rate:      energy.Rate(),
				level:     energy.Level(),
				maxSample: maxS,
			})
		}
	}

	for frame := range tr.Frames() {
		if frame.Err != nil {
			t.Fatalf("frame error: %v", frame.Err)
		}
		evt, err := parser.Parse(frame.Name, frame.Data)
		if err != nil || evt == nil {
			tick()
			continue
		}
		if evt.Kind == events.KindContent {
			contentN++
			if evt.SourceID != "" {
				agentsSeen[evt.SourceID]++
			}
			energy.onTokens(1, time.Now())
		}
		tick()
		if evt.Kind == events.KindSessionEnd || evt.Name == "session_complete" {
			// Keep draining briefly; outer range ends when channel closes.
		}
		if ctx.Err() != nil {
			t.Fatal("timeout waiting for stream")
		}
	}

	// Stream closed — decay.
	for i := 0; i < 15; i++ {
		energy.tick(time.Now(), false)
		time.Sleep(40 * time.Millisecond)
	}
	finalLevel := energy.Level()

	t.Logf("content_tokens=%d agents=%v max_level=%.2f max_rate=%.0f tok/s final_level=%.3f samples=%d",
		contentN, agentsSeen, maxLevel, maxRate, finalLevel, len(energy.Samples()))
	for _, s := range snapshots {
		t.Logf("  t=%5.1fs content=%3d rate=%5.0f level=%.2f hist_peak=%.2f",
			s.at.Seconds(), s.content, s.rate, s.level, s.maxSample)
	}

	if contentN < 5 {
		t.Fatalf("expected real content tokens from inference, got %d", contentN)
	}
	if maxLevel < 0.25 {
		t.Fatalf("energy never rose with content (max_level=%.2f) — meter not tracking tokens", maxLevel)
	}
	if maxRate < 1 {
		t.Fatalf("tok/s never left zero (max_rate=%.1f)", maxRate)
	}
	if finalLevel > maxLevel*0.5 && finalLevel > 0.2 {
		t.Fatalf("energy did not decay after stream end: final=%.2f max=%.2f", finalLevel, maxLevel)
	}
	// Correlation: level should be higher mid-stream than at the quiet start.
	if len(snapshots) >= 3 {
		early := snapshots[0].level
		// Find peak mid snapshots
		peak := early
		for _, s := range snapshots {
			if s.level > peak {
				peak = s.level
			}
		}
		if peak <= early && contentN > 10 {
			t.Fatalf("level never rose above early baseline (early=%.2f peak=%.2f) despite %d content tokens", early, peak, contentN)
		}
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
