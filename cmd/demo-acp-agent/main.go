// Command demo-acp-agent is a minimal multi-turn ACP agent for offline demos
// and VHS recordings. It speaks JSON-RPC 2.0 NDJSON on stdio:
// initialize → session/new → session/prompt (repeatable).
//
// Not a product agent — only enough wire surface for stream-debugger's
// transport.type: acp path.
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)

	var turn atomic.Int64
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
					"agentInfo": map[string]any{
						"name":    "demo-acp-agent",
						"version": "0.1.0",
					},
					"agentCapabilities": map[string]any{"loadSession": false},
				},
			})
		case "session/new":
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(msg.ID),
				"result":  map[string]any{"sessionId": "sess_demo_live"},
			})
		case "session/prompt":
			n := turn.Add(1)
			userText := extractPromptText(msg.Params)
			// Slight delay so the TUI has time to paint streaming state.
			time.Sleep(120 * time.Millisecond)
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"method":  "session/update",
				"params": map[string]any{
					"sessionId": "sess_demo_live",
					"update": map[string]any{
						"sessionUpdate": "agent_thought_chunk",
						"content": map[string]any{
							"type": "text",
							"text": fmt.Sprintf("turn %d: planning response…", n),
						},
					},
				},
			})
			time.Sleep(80 * time.Millisecond)
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"method":  "session/update",
				"params": map[string]any{
					"sessionId": "sess_demo_live",
					"update": map[string]any{
						"sessionUpdate": "agent_message_chunk",
						"content": map[string]any{
							"type": "text",
							"text": fmt.Sprintf(
								"[sess_demo_live turn %d] You said: %q. Same process + session reused.",
								n, userText,
							),
						},
					},
				},
			})
			time.Sleep(60 * time.Millisecond)
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(msg.ID),
				"result":  map[string]any{"stopReason": "end_turn"},
			})
		case "session/cancel":
			// notification — no response
		default:
			if len(msg.ID) > 0 {
				_ = enc.Encode(map[string]any{
					"jsonrpc": "2.0",
					"id":      json.RawMessage(msg.ID),
					"error": map[string]any{
						"code":    -32601,
						"message": "demo-acp-agent: method not implemented: " + msg.Method,
					},
				})
			}
		}
	}
}

func extractPromptText(params json.RawMessage) string {
	var p struct {
		Prompt []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"prompt"`
	}
	if err := json.Unmarshal(params, &p); err != nil || len(p.Prompt) == 0 {
		return ""
	}
	return p.Prompt[0].Text
}
