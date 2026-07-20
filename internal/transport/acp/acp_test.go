package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/bridge"
	"github.com/lancekrogers/stream-debugger/internal/events"
)

// TestHelperProcess_FakeACPAgent is re-executed as a child process that
// speaks a minimal ACP agent over stdio. Not a real unit test.
func TestHelperProcess_FakeACPAgent(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}
	runFakeACPAgent()
	os.Exit(0)
}

func runFakeACPAgent() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	enc := json.NewEncoder(os.Stdout)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var msg struct {
			ID     json.RawMessage `json:"id"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params"`
		}
		if err := json.Unmarshal(line, &msg); err != nil {
			continue
		}
		switch msg.Method {
		case "initialize":
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(msg.ID),
				"result": map[string]any{
					"protocolVersion": 1,
					"agentInfo":       map[string]any{"name": "fake-acp", "version": "test"},
					"agentCapabilities": map[string]any{
						"loadSession": false,
					},
				},
			})
		case "session/new":
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(msg.ID),
				"result":  map[string]any{"sessionId": "sess_test_1"},
			})
		case "session/prompt":
			// Emit a short turn then complete.
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"method":  "session/update",
				"params": map[string]any{
					"sessionId": "sess_test_1",
					"update": map[string]any{
						"sessionUpdate": "agent_thought_chunk",
						"content":       map[string]any{"type": "text", "text": "thinking…"},
					},
				},
			})
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"method":  "session/update",
				"params": map[string]any{
					"sessionId": "sess_test_1",
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"content":       map[string]any{"type": "text", "text": "hello from fake agent"},
					},
				},
			})
			// Permission request to exercise reverse stub.
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      9001,
				"method":  "session/request_permission",
				"params": map[string]any{
					"sessionId": "sess_test_1",
					"toolCall":  map[string]any{"toolCallId": "tc1", "title": "run"},
				},
			})
			// Wait briefly for the stub reply so the test process does not race exit.
			time.Sleep(50 * time.Millisecond)
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(msg.ID),
				"result":  map[string]any{"stopReason": "end_turn"},
			})
		default:
			if len(msg.ID) > 0 {
				_ = enc.Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      json.RawMessage(msg.ID),
					"error":   map[string]any{"code": -32601, "message": "unknown method " + msg.Method},
				})
			}
		}
	}
}

func helperAgentConfig(prompt string) Config {
	return Config{
		Command:                os.Args[0],
		Args:                   []string{"-test.run=TestHelperProcess_FakeACPAgent", "--"},
		Env:                    []string{"GO_WANT_HELPER_PROCESS=1"},
		Prompt:                 prompt,
		AutoApprovePermissions: true,
		ClientName:             "stream-debugger-test",
	}
}

func TestTransport_ConnectHandshakeAndPrompt(t *testing.T) {
	tr := New(helperAgentConfig("ping"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = tr.Close() }()

	if got := tr.SessionID(); got != "sess_test_1" {
		t.Fatalf("SessionID = %q, want sess_test_1", got)
	}

	var frames []string
	deadline := time.After(5 * time.Second)
	joined := ""
	for !strings.Contains(joined, "hello from fake agent") ||
		!strings.Contains(joined, "request_permission") ||
		!strings.Contains(joined, "end_turn") {
		select {
		case <-deadline:
			t.Fatalf("timeout waiting for turn frames; got %d:\n%s", len(frames), joined)
		case f, ok := <-tr.Frames():
			if !ok {
				t.Fatalf("frames closed early; got:\n%s", joined)
			}
			if f.Err != nil {
				t.Fatalf("frame error: %v", f.Err)
			}
			frames = append(frames, string(f.Data))
			joined = strings.Join(frames, "\n")
		}
	}

	if !strings.Contains(joined, "agent_thought_chunk") {
		t.Errorf("expected thought chunk in frames, got:\n%s", joined)
	}
}

func TestTransport_DialectDecode(t *testing.T) {
	tr := New(helperAgentConfig("decode me"))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = tr.Close() }()

	parser, err := bridge.NewParserFor("../../../dialects/acp.yaml")
	if err != nil {
		// Fall back to embedded name if path fails in module layout.
		parser = bridge.NewParser("acp")
	}

	var kinds []events.Kind
	var content strings.Builder
	deadline := time.After(5 * time.Second)
	for {
		select {
		case <-deadline:
			goto done
		case f, ok := <-tr.Frames():
			if !ok {
				goto done
			}
			if f.Err != nil {
				continue
			}
			evt, _ := parser.Parse(f.Name, f.Data)
			kinds = append(kinds, evt.Kind)
			if evt.Kind == events.KindContent || evt.Kind == events.KindReasoning {
				content.WriteString(evt.Content)
			}
			if evt.Kind == events.KindStreamEnd {
				goto done
			}
		}
	}
done:
	if content.Len() == 0 {
		t.Fatalf("expected content/reasoning from dialect; kinds=%v", kinds)
	}
	if !strings.Contains(content.String(), "hello from fake agent") && !strings.Contains(content.String(), "thinking") {
		t.Errorf("unexpected content %q kinds=%v", content.String(), kinds)
	}
}

func TestTransport_MissingCommand(t *testing.T) {
	tr := New(Config{})
	err := tr.Connect(context.Background())
	if err == nil || !strings.Contains(err.Error(), "command is required") {
		t.Fatalf("expected command required error, got %v", err)
	}
}

func TestTransport_SendAfterConnect(t *testing.T) {
	tr := New(Config{
		Command:       os.Args[0],
		Args:          []string{"-test.run=TestHelperProcess_FakeACPAgent", "--"},
		Env:           []string{"GO_WANT_HELPER_PROCESS=1"},
		SkipHandshake: true,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer func() { _ = tr.Close() }()

	// Drive handshake manually via Send + call would be internal; use Send for a notification.
	payload := []byte(`{"jsonrpc":"2.0","method":"session/cancel","params":{"sessionId":"x"}}`)
	if err := tr.Send(ctx, payload); err != nil {
		t.Fatalf("Send: %v", err)
	}
}
