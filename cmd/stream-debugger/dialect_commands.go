package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Obedience-Corp/stream-debugger/internal/bridge"
	"github.com/Obedience-Corp/stream-debugger/internal/config"
	"github.com/Obedience-Corp/stream-debugger/internal/dialectinit"
	"github.com/Obedience-Corp/stream-debugger/internal/events"
	"github.com/Obedience-Corp/stream-debugger/internal/explain"
	"github.com/Obedience-Corp/stream-debugger/internal/mapping"
	"github.com/Obedience-Corp/stream-debugger/internal/transport"
	"github.com/Obedience-Corp/stream-debugger/internal/transport/replay"
	"github.com/Obedience-Corp/stream-debugger/internal/transport/sse"
)

// loadEventsFromFile reads a JSONL session log through the replay transport.
func loadEventsFromFile(filePath, dialectSource string) ([]*events.Event, error) {
	tr, err := replay.New(filePath, 0)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to start replay: %w", err)
	}
	defer func() { _ = tr.Close() }()
	parser, err := bridge.NewParserFor(dialectSource)
	if err != nil {
		return nil, fmt.Errorf("failed to load dialect: %w", err)
	}
	var result []*events.Event
	for frame := range tr.Frames() {
		if frame.Err != nil {
			fmt.Fprintf(os.Stderr, "Warning: malformed frame: %v\n", frame.Err)
			continue
		}
		evt, _ := parser.Parse(frame.Name, frame.Data)
		result = append(result, evt)
	}
	return result, nil
}

func applyDialectOverride(cfg *config.EnhancedConfig, override string) {
	if strings.TrimSpace(override) != "" {
		cfg.Dialect.File = override
	}
}

func runInit(fixturePath, sourceLabel string) error {
	tr, err := replay.New(fixturePath, 0)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		return fmt.Errorf("failed to start replay: %w", err)
	}
	defer func() { _ = tr.Close() }()
	samples := collectSamples(tr.Frames())
	if len(samples) == 0 {
		return fmt.Errorf("no frames observed in %s", fixturePath)
	}
	fmt.Print(dialectinit.Infer(samples, sourceLabel).Render())
	return nil
}

func runInitLive(requestURL, sourceLabel string) error {
	tr := sse.New(http.MethodGet, requestURL, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() { _ = tr.Close() }()
	samples := collectSamples(tr.Frames())
	if len(samples) == 0 {
		return fmt.Errorf("no frames observed from %s", requestURL)
	}
	fmt.Print(dialectinit.Infer(samples, sourceLabel).Render())
	return nil
}

func collectSamples(frames <-chan transport.Frame) []dialectinit.Sample {
	var samples []dialectinit.Sample
	for f := range frames {
		if f.Err != nil {
			fmt.Fprintf(os.Stderr, "Warning: malformed frame skipped: %v\n", f.Err)
			continue
		}
		samples = append(samples, dialectinit.Sample{Name: f.Name, Data: f.Data})
	}
	return samples
}

func runExplain(engine *mapping.Engine, fixturePath string) error {
	tr, err := replay.New(fixturePath, 0)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		return fmt.Errorf("failed to start replay: %w", err)
	}
	defer func() { _ = tr.Close() }()
	return renderExplainTraces(engine, tr.Frames())
}

func runExplainLive(engine *mapping.Engine, requestURL string) error {
	tr := sse.New(http.MethodGet, requestURL, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	defer func() { _ = tr.Close() }()
	return renderExplainTraces(engine, tr.Frames())
}

func renderExplainTraces(engine *mapping.Engine, frames <-chan transport.Frame) error {
	index := 0
	unhealthy := false
	for f := range frames {
		index++
		if f.Err != nil {
			fmt.Printf("frame %d  ✗ malformed frame: %v\n", index, f.Err)
			unhealthy = true
			continue
		}
		trace := explain.Trace(engine, f.Name, f.Data, index)
		fmt.Print(trace.Render())
		if trace.Unhealthy() {
			unhealthy = true
		}
	}
	if index == 0 {
		return fmt.Errorf("no frames observed")
	}
	if unhealthy {
		os.Exit(1)
	}
	return nil
}
