// Package acp implements a Transport over an Agent Client Protocol (ACP)
// agent process: JSON-RPC 2.0 NDJSON on the child's stdin/stdout.
//
// Connect spawns the configured command, performs a minimal client
// handshake (initialize → session/new → session/prompt), and emits every
// agent→client line as a Frame for the dialect engine. Reverse requests
// from the agent (permissions, fs, terminal) are stubbed so a real agent
// does not hang waiting for a full IDE host.
//
// This is a debugger transport, not a product ACP host: reverse RPC is
// intentionally minimal (auto-approve permissions; error other methods).
package acp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/transport"
)

// Config configures an ACP stdio agent process.
type Config struct {
	// Command is the agent binary (required), e.g. "npx" or "grok".
	Command string
	// Args are passed after Command, e.g. ["agent", "stdio"].
	Args []string
	// Cwd is the child working directory; empty means inherit.
	Cwd string
	// Env is extra KEY=VALUE entries merged on top of the process environment.
	Env []string
	// Prompt is the first session/prompt text sent after session/new.
	// Empty skips session/prompt (watch-only after handshake).
	Prompt string
	// ProtocolVersion is the ACP protocol version offered in initialize.
	// Zero defaults to 1.
	ProtocolVersion int
	// ClientName is reported in initialize clientInfo; empty defaults to
	// "stream-debugger".
	ClientName string
	// AutoApprovePermissions answers session/request_permission with allow.
	// When false, requests are cancelled.
	AutoApprovePermissions bool
	// SkipHandshake disables initialize/session/new/prompt — only spawn and
	// read stdout. Useful for tests that drive the wire themselves via Send.
	SkipHandshake bool
}

// Transport is a long-lived ACP stdio session.
type Transport struct {
	cfg Config

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	frames chan transport.Frame
	closed chan struct{}

	mu        sync.Mutex
	writeMu   sync.Mutex
	nextID    atomic.Int64
	pending   map[int64]chan rpcOutcome
	sessionID string

	wg           sync.WaitGroup
	closeOnce    sync.Once
	framesOnce   sync.Once
	streamCancel context.CancelFunc
}

type rpcOutcome struct {
	result json.RawMessage
	err    error
}

// New builds an ACP transport. Connect starts the child process.
func New(cfg Config) *Transport {
	if cfg.ProtocolVersion == 0 {
		cfg.ProtocolVersion = 1
	}
	if cfg.ClientName == "" {
		cfg.ClientName = "stream-debugger"
	}
	return &Transport{
		cfg:     cfg,
		frames:  make(chan transport.Frame, 64),
		closed:  make(chan struct{}),
		pending: make(map[int64]chan rpcOutcome),
	}
}

// Name identifies this transport kind.
func (t *Transport) Name() string { return "acp" }

// SessionID returns the id assigned by session/new, empty until handshake.
func (t *Transport) SessionID() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.sessionID
}

// Connect spawns the agent and optionally completes the ACP handshake.
// The dialing context only bounds setup; after success the stream is
// governed by Close (same contract as SSE/gRPC transports).
func (t *Transport) Connect(ctx context.Context) (err error) {
	defer func() {
		if err != nil {
			t.cleanupPartial()
			t.closeFrames()
		}
	}()

	if strings.TrimSpace(t.cfg.Command) == "" {
		return fmt.Errorf("acp: command is required")
	}

	streamCtx, streamCancel := context.WithCancel(context.Background())
	stopDialCancel := context.AfterFunc(ctx, streamCancel)
	defer stopDialCancel()
	defer func() {
		if err != nil {
			streamCancel()
		}
	}()

	cmd := exec.CommandContext(streamCtx, t.cfg.Command, t.cfg.Args...)
	if t.cfg.Cwd != "" {
		cmd.Dir = t.cfg.Cwd
	}
	cmd.Env = append(os.Environ(), t.cfg.Env...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("acp: stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("acp: stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return fmt.Errorf("acp: stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		_ = stderr.Close()
		return fmt.Errorf("acp: start %q: %w", t.cfg.Command, err)
	}

	if !stopDialCancel() && ctx.Err() != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return fmt.Errorf("acp: connect: %w", ctx.Err())
	}

	t.cmd = cmd
	t.stdin = stdin
	t.stdout = stdout
	t.stderr = stderr
	t.streamCancel = streamCancel

	t.wg.Add(2)
	go t.readStdout(streamCtx)
	go t.drainStderr()

	if !t.cfg.SkipHandshake {
		if err := t.handshake(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (t *Transport) handshake(ctx context.Context) error {
	initParams := map[string]any{
		"protocolVersion": t.cfg.ProtocolVersion,
		"clientInfo": map[string]any{
			"name":    t.cfg.ClientName,
			"version": "0.1.0",
		},
		"clientCapabilities": map[string]any{
			"fs": map[string]any{
				"readTextFile":  false,
				"writeTextFile": false,
			},
		},
	}
	if _, err := t.call(ctx, "initialize", initParams); err != nil {
		return fmt.Errorf("acp: initialize: %w", err)
	}

	cwd := t.cfg.Cwd
	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	newParams := map[string]any{
		"cwd":        cwd,
		"mcpServers": []any{},
	}
	raw, err := t.call(ctx, "session/new", newParams)
	if err != nil {
		return fmt.Errorf("acp: session/new: %w", err)
	}
	var newResult struct {
		SessionID string `json:"sessionId"`
	}
	if err := json.Unmarshal(raw, &newResult); err != nil {
		return fmt.Errorf("acp: session/new decode: %w", err)
	}
	if newResult.SessionID == "" {
		return fmt.Errorf("acp: session/new returned empty sessionId")
	}
	t.mu.Lock()
	t.sessionID = newResult.SessionID
	t.mu.Unlock()

	prompt := strings.TrimSpace(t.cfg.Prompt)
	if prompt == "" {
		return nil
	}
	promptParams := map[string]any{
		"sessionId": newResult.SessionID,
		"prompt": []any{
			map[string]any{"type": "text", "text": prompt},
		},
	}
	// Fire-and-forget wait: the prompt response may arrive after many
	// session/update notifications. Wait in the background so Connect can
	// return once the turn is started; frames already stream via readStdout.
	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		// Long timeout: agent turns can take minutes.
		pctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		_, _ = t.call(pctx, "session/prompt", promptParams)
	}()
	return nil
}

// call sends a JSON-RPC request and waits for the matching response.
func (t *Transport) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := t.nextID.Add(1)
	ch := make(chan rpcOutcome, 1)
	t.mu.Lock()
	t.pending[id] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
	}()

	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}
	if err := t.writeJSON(msg); err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-t.closed:
		return nil, fmt.Errorf("acp: transport closed")
	case out := <-ch:
		if out.err != nil {
			return nil, out.err
		}
		return out.result, nil
	}
}

// Send writes a raw JSON-RPC line to the agent (advanced / multi-turn).
// Payload must be a complete JSON object; a trailing newline is added.
func (t *Transport) Send(ctx context.Context, payload []byte) error {
	select {
	case <-t.closed:
		return fmt.Errorf("acp: transport closed")
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	if t.stdin == nil {
		return fmt.Errorf("acp: not connected")
	}
	line := bytesTrimSpace(payload)
	if len(line) == 0 {
		return fmt.Errorf("acp: empty payload")
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if _, err := t.stdin.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("acp: write: %w", err)
	}
	return nil
}

func (t *Transport) writeJSON(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if t.stdin == nil {
		return fmt.Errorf("acp: not connected")
	}
	if _, err := t.stdin.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("acp: write: %w", err)
	}
	return nil
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

// Frames returns agent→client NDJSON lines as Frames.
func (t *Transport) Frames() <-chan transport.Frame { return t.frames }

func (t *Transport) readStdout(ctx context.Context) {
	defer t.wg.Done()
	defer t.closeFrames()

	sc := bufio.NewScanner(t.stdout)
	// ACP frames can carry large tool payloads.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 16*1024*1024)

	for sc.Scan() {
		select {
		case <-t.closed:
			return
		case <-ctx.Done():
			return
		default:
		}
		line := append([]byte(nil), sc.Bytes()...)
		if len(line) == 0 {
			continue
		}
		t.handleLine(line)
	}
	if err := sc.Err(); err != nil {
		select {
		case <-t.closed:
		default:
			t.emitFrame(transport.Frame{
				Data:      nil,
				Raw:       []byte(err.Error()),
				Timestamp: time.Now(),
				Err:       fmt.Errorf("acp: stdout: %w", err),
			})
		}
	}
}

func (t *Transport) handleLine(line []byte) {
	// Always surface the wire line as a frame for the dialect engine.
	frame := transport.Frame{
		Data:      line,
		Raw:       line,
		Timestamp: time.Now(),
	}
	if !json.Valid(line) {
		frame.Err = fmt.Errorf("acp: non-JSON line")
	}
	t.emitFrame(frame)

	if frame.Err != nil {
		return
	}

	var envelope struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(line, &envelope); err != nil {
		return
	}

	// Response to one of our requests.
	if len(envelope.ID) > 0 && envelope.Method == "" {
		var id int64
		if err := json.Unmarshal(envelope.ID, &id); err != nil {
			return
		}
		t.mu.Lock()
		ch := t.pending[id]
		t.mu.Unlock()
		if ch == nil {
			return
		}
		if envelope.Error != nil {
			ch <- rpcOutcome{err: fmt.Errorf("jsonrpc %d: %s", envelope.Error.Code, envelope.Error.Message)}
			return
		}
		ch <- rpcOutcome{result: envelope.Result}
		return
	}

	// Agent→client request: stub reverse RPC so the agent keeps moving.
	if envelope.Method != "" && len(envelope.ID) > 0 {
		t.replyReverse(envelope.ID, envelope.Method, envelope.Params)
	}
}

func (t *Transport) replyReverse(id json.RawMessage, method string, params json.RawMessage) {
	var result any
	var rpcErr *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}

	switch method {
	case "session/request_permission":
		if t.cfg.AutoApprovePermissions {
			result = map[string]any{
				"outcome": map[string]any{"outcome": "selected", "optionId": "allow-once"},
			}
		} else {
			result = map[string]any{
				"outcome": map[string]any{"outcome": "cancelled"},
			}
		}
	default:
		// fs/*, terminal/*, unknown: fail closed so the agent does not hang.
		rpcErr = &struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}{Code: -32601, Message: fmt.Sprintf("stream-debugger acp stub: method %q not implemented", method)}
	}

	resp := map[string]any{
		"jsonrpc": "2.0",
		"id":      json.RawMessage(id),
	}
	if rpcErr != nil {
		resp["error"] = rpcErr
	} else {
		resp["result"] = result
	}
	_ = t.writeJSON(resp)
}

func (t *Transport) emitFrame(f transport.Frame) {
	select {
	case <-t.closed:
		return
	case t.frames <- f:
	}
}

func (t *Transport) drainStderr() {
	defer t.wg.Done()
	sc := bufio.NewScanner(t.stderr)
	for sc.Scan() {
		// Stderr is diagnostic; do not treat as frames. Best-effort discard.
		_ = sc.Text()
	}
}

func (t *Transport) closeFrames() {
	t.framesOnce.Do(func() { close(t.frames) })
}

func (t *Transport) cleanupPartial() {
	if t.stdin != nil {
		_ = t.stdin.Close()
	}
	if t.stdout != nil {
		_ = t.stdout.Close()
	}
	if t.stderr != nil {
		_ = t.stderr.Close()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		_, _ = t.cmd.Process.Wait()
	}
}

// Close terminates the agent process and stops frame delivery.
func (t *Transport) Close() error {
	t.closeOnce.Do(func() {
		close(t.closed)
		if t.streamCancel != nil {
			t.streamCancel()
		}
		if t.stdin != nil {
			_ = t.stdin.Close()
		}
		if t.cmd != nil && t.cmd.Process != nil {
			_ = t.cmd.Process.Kill()
		}
		if t.stdout != nil {
			_ = t.stdout.Close()
		}
		if t.stderr != nil {
			_ = t.stderr.Close()
		}
		if t.cmd != nil {
			_, _ = t.cmd.Process.Wait()
		}
		// Unblock any pending RPC waiters.
		t.mu.Lock()
		for id, ch := range t.pending {
			ch <- rpcOutcome{err: fmt.Errorf("acp: closed")}
			delete(t.pending, id)
		}
		t.mu.Unlock()
		t.wg.Wait()
		t.closeFrames()
	})
	return nil
}
