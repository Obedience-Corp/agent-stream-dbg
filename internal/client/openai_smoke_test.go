package client

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/Obedience-Corp/stream-debugger/internal/config"
	"github.com/Obedience-Corp/stream-debugger/internal/events"
	"github.com/Obedience-Corp/stream-debugger/internal/testutil"
	"github.com/Obedience-Corp/stream-debugger/internal/transport/sse"
)

// TestOpenAIDialectMockSSESmoke exercises the live SSE frame path with the
// OpenAI fixture and the configured dialect. It intentionally starts the raw
// transport directly because openai.yaml is a watch-only dialect: the smoke
// is proving stream decoding and pane-ready events, not inventing an OpenAI
// request body for the fixture server.
func TestOpenAIDialectMockSSESmoke(t *testing.T) {
	fixture := filepath.Join("..", "..", "testdata", "fixtures", "openai-chat.jsonl")
	srv, err := testutil.NewMockSSEServer(fixture, 0)
	if err != nil {
		t.Fatalf("NewMockSSEServer: %v", err)
	}
	defer srv.Close()

	cfg := &config.EnhancedConfig{
		Dialect: config.DialectConfig{File: filepath.Join("..", "..", "dialects", "openai.yaml")},
	}
	c := NewSSEClient(cfg)
	tr := sse.New("GET", srv.URL(), nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("SSE connect: %v", err)
	}

	c.transport = tr
	c.wg.Add(1)
	go c.readLoop(tr)
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()
	<-done
	c.Close()

	counts := map[events.Kind]int{}
	content := ""
	for evt := range c.Events() {
		counts[evt.Kind]++
		content += evt.Content
	}
	if counts[events.KindContent] != 2 {
		t.Fatalf("content event count = %d, want 2", counts[events.KindContent])
	}
	if counts[events.KindUnknown] != 0 {
		t.Fatalf("unknown event count = %d, want 0 (counts: %v)", counts[events.KindUnknown], counts)
	}
	if content != "The answer is 42." {
		t.Fatalf("decoded content = %q, want %q", content, "The answer is 42.")
	}
	fmt.Printf("OpenAI smoke decode counts: content=%d unknown=%d total=%d\n", counts[events.KindContent], counts[events.KindUnknown], sumEventCounts(counts))
}

func sumEventCounts(counts map[events.Kind]int) int {
	total := 0
	for _, count := range counts {
		total += count
	}
	return total
}
