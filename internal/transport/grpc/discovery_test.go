package grpc

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/testutil/mockgrpc"
)

func TestDiscoverStreamingMethodsBuildsUsableOptions(t *testing.T) {
	srv, err := mockgrpc.New(nil)
	if err != nil {
		t.Fatalf("mockgrpc.New: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	methods, err := DiscoverStreamingMethods(ctx, Config{Target: srv.Addr(), Plaintext: true})
	if err != nil {
		t.Fatalf("DiscoverStreamingMethods: %v", err)
	}
	if len(methods) != 4 {
		t.Fatalf("discovered %d methods, want 4: %+v", len(methods), methods)
	}

	byPath := make(map[string]StreamingMethod, len(methods))
	for _, method := range methods {
		byPath[method.Path] = method
		if !method.Supported {
			t.Errorf("%s unexpectedly marked unsupported", method.Path)
		}
		if method.RequestJSON == "" {
			t.Errorf("%s has no request example", method.Path)
		}
	}

	stream := byPath["/agentstream.v1.AgentStream/Stream"]
	if stream.Shape != "server-streaming" {
		t.Errorf("Stream shape = %q, want server-streaming", stream.Shape)
	}
	if stream.InputType != "agentstream.v1.StreamRequest" || stream.OutputType != "agentstream.v1.StreamEvent" {
		t.Errorf("Stream types = %q -> %q", stream.InputType, stream.OutputType)
	}
	if stream.RequestJSON != `{"message":"","session_id":""}` {
		t.Errorf("Stream request example = %s", stream.RequestJSON)
	}
	if stream.Discriminator != "oneof" {
		t.Errorf("Stream discriminator = %q, want oneof", stream.Discriminator)
	}

	chat := byPath["/agentstream.v1.AgentStream/Chat"]
	if chat.Shape != "bidi-streaming" {
		t.Errorf("Chat shape = %q, want bidi-streaming", chat.Shape)
	}
	if chat.RequestJSON != `{"text":""}` {
		t.Errorf("Chat request example = %s", chat.RequestJSON)
	}
}

func TestDiscoverStreamingMethodsExplainsReflectionFallback(t *testing.T) {
	srv, err := mockgrpc.NewWithoutReflection(nil)
	if err != nil {
		t.Fatalf("mockgrpc.NewWithoutReflection: %v", err)
	}
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err = DiscoverStreamingMethods(ctx, Config{Target: srv.Addr(), Plaintext: true})
	if err == nil {
		t.Fatal("expected reflection discovery to fail when reflection is disabled")
	}
	if !strings.Contains(err.Error(), "descriptor-set/--proto-file") {
		t.Fatalf("reflection error did not explain fallback: %v", err)
	}
}
