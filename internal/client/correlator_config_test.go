package client

import (
	"strings"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
)

func TestNewCorrelatorFromConfig_InvalidForceErrors(t *testing.T) {
	cfg := &config.EnhancedConfig{
		Correlation: config.CorrelationConfig{
			Propagate:   true,
			Traceparent: "not-a-valid-traceparent",
		},
	}
	_, err := NewCorrelatorFromConfig(cfg)
	if err == nil {
		t.Fatal("expected error for invalid forced traceparent")
	}
	if !strings.Contains(err.Error(), "invalid") {
		t.Errorf("error=%v", err)
	}
}

func TestNewCorrelatorFromConfig_ValidForce(t *testing.T) {
	cfg := &config.EnhancedConfig{
		Correlation: config.CorrelationConfig{
			Propagate:   true,
			Traceparent: "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
		},
	}
	c, err := NewCorrelatorFromConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if c.Current().TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Errorf("trace_id=%q", c.Current().TraceID)
	}
}
