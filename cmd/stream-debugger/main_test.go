package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/lancekrogers/stream-debugger/dialects"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
)

func TestReplayPathUsesConfiguredOpenAIDialect(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "fixtures", "openai-chat.jsonl")
	dialect := filepath.Join("..", "..", "dialects", "openai.yaml")

	evts, err := loadEventsFromFile(fixture, dialect)
	if err != nil {
		t.Fatalf("loadEventsFromFile: %v", err)
	}

	var content []string
	for _, evt := range evts {
		if evt.Kind == events.KindContent {
			content = append(content, evt.Content)
		}
	}
	if got, want := strings.Join(content, ""), "The answer is 42."; got != want {
		t.Fatalf("decoded OpenAI content = %q, want %q", got, want)
	}
}

func TestApplyDialectOverride(t *testing.T) {
	cfg := &config.EnhancedConfig{Dialect: config.DialectConfig{File: dialects.DefaultName}}
	applyDialectOverride(cfg, "openai")
	if got, want := cfg.Dialect.File, "openai"; got != want {
		t.Fatalf("dialect override = %q, want %q", got, want)
	}

	applyDialectOverride(cfg, " ")
	if got, want := cfg.Dialect.File, "openai"; got != want {
		t.Fatalf("blank override changed dialect to %q, want %q", got, want)
	}
}
