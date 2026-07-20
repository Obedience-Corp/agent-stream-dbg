package client

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/testutil/mockgrpc"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/testutil/mockgrpc/a2apb"
)

func TestConfiguredGRPCTransportUsesA2ADialect(t *testing.T) {
	srv, err := mockgrpc.NewA2A([]*a2apb.StreamResponse{{
		Payload: &a2apb.StreamResponse_StatusUpdate{StatusUpdate: &a2apb.TaskStatusUpdateEvent{
			TaskId:    "task-a2a-001",
			ContextId: "ctx-a2a-001",
			Status:    &a2apb.TaskStatus{State: a2apb.TaskState_TASK_STATE_WORKING},
		}},
	}})
	if err != nil {
		t.Fatalf("mockgrpc.NewA2A: %v", err)
	}
	defer srv.Close()

	preserveCamelCase := false
	cfg := &config.EnhancedConfig{
		Transport: config.TransportConfig{
			Type:               "grpc",
			Target:             srv.Addr(),
			GRPCMethod:         "/a2a.v1.A2AService/SendStreamingMessage",
			Discriminator:      "none",
			PreserveFieldNames: &preserveCamelCase,
			Plaintext:          true,
		},
		Dialect: config.DialectConfig{File: filepath.Join("..", "..", "dialects", "a2a.yaml")},
		Session: config.SessionConfig{ID: "product-a2a-session"},
	}
	c := NewClient(cfg)
	defer c.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Connect(ctx, "hello"); err != nil {
		t.Fatalf("product client Connect: %v", err)
	}

	select {
	case evt := <-c.Events():
		if evt.Kind != events.KindStatusChange {
			t.Fatalf("expected status_change, got %v", evt.Kind)
		}
		if evt.SourceID != "task-a2a-001" || evt.StringField("context_id") != "ctx-a2a-001" {
			t.Fatalf("unexpected A2A event: source=%q fields=%v", evt.SourceID, evt.Fields)
		}
		if evt.StringField("state") != "TASK_STATE_WORKING" {
			t.Fatalf("expected TASK_STATE_WORKING, got %q", evt.StringField("state"))
		}
	case err := <-c.Errors():
		t.Fatalf("unexpected product client error: %v", err)
	case <-ctx.Done():
		t.Fatal("timed out waiting for configured gRPC event")
	}
}
