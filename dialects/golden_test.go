package dialects_test

import (
	"context"
	"flag"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lancekrogers/stream-debugger/internal/explain"
	"github.com/lancekrogers/stream-debugger/internal/mapping"
	"github.com/lancekrogers/stream-debugger/internal/transport/replay"
)

// updateGoldens regenerates testdata/goldens/*.explain.txt from the
// current dialects + fixtures. Run via `just goldens`.
var updateGoldens = flag.Bool("update-goldens", false, "regenerate golden explain output files")

// goldenCase pairs a shipped dialect with the fixture and golden file
// that make it a CI-tested artifact: explain over a fixture IS the
// dialect test harness (dialect-spec.md), no network, no mocks per
// dialect.
type goldenCase struct {
	dialect string // relative to this directory: <name>.yaml
	fixture string // relative to this directory: ../testdata/fixtures/<name>.jsonl
	golden  string // relative to this directory: ../testdata/goldens/<name>.explain.txt
}

var goldenCases = []goldenCase{
	{"brainyard.yaml", "../testdata/fixtures/brainyard-session.jsonl", "../testdata/goldens/brainyard.explain.txt"},
	{"openai.yaml", "../testdata/fixtures/openai-chat.jsonl", "../testdata/goldens/openai.explain.txt"},
	{"anthropic.yaml", "../testdata/fixtures/anthropic-messages.jsonl", "../testdata/goldens/anthropic.explain.txt"},
	{"a2a.yaml", "../testdata/fixtures/a2a-session.jsonl", "../testdata/goldens/a2a.explain.txt"},
}

// renderExplain runs every frame in fixturePath through the dialect
// loaded from dialectPath and returns the concatenated explain output.
// The output deliberately carries no timestamps or absolute paths —
// explain.Trace never uses Frame.Timestamp and never echoes the input
// paths — so it's stable to record as a golden byte-for-byte.
func renderExplain(t *testing.T, dialectPath, fixturePath string) string {
	t.Helper()
	engine, err := mapping.LoadFile(dialectPath)
	if err != nil {
		t.Fatalf("failed to load dialect %s: %v", dialectPath, err)
	}

	tr, err := replay.New(fixturePath, 0)
	if err != nil {
		t.Fatalf("failed to open fixture %s: %v", fixturePath, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("failed to start replay: %v", err)
	}
	defer func() { _ = tr.Close() }()

	var b strings.Builder
	index := 0
	for f := range tr.Frames() {
		index++
		if f.Err != nil {
			t.Fatalf("frame %d: malformed: %v", index, f.Err)
		}
		b.WriteString(explain.Trace(engine, f.Name, f.Data, index).Render())
	}
	return b.String()
}

// TestExplainGoldens makes every shipped dialect a CI-tested artifact:
// its explain trace over its own fixture must match a recorded golden.
// A dialect regression (a renamed event, a broken path) changes the
// trace and fails this test — see
// TestExplainGoldens_DetectsRenamedEventRegression for a live proof.
func TestExplainGoldens(t *testing.T) {
	for _, gc := range goldenCases {
		t.Run(gc.dialect, func(t *testing.T) {
			got := renderExplain(t, gc.dialect, gc.fixture)

			if *updateGoldens {
				if err := os.WriteFile(gc.golden, []byte(got), 0o644); err != nil {
					t.Fatalf("failed to write golden %s: %v", gc.golden, err)
				}
				return
			}

			want, err := os.ReadFile(gc.golden)
			if err != nil {
				t.Fatalf("failed to read golden %s (run `just goldens` to generate it): %v", gc.golden, err)
			}
			if got != string(want) {
				t.Errorf("explain output for %s no longer matches its golden.\nRun `just goldens` to update it if this change is intentional.\n\n--- got ---\n%s\n--- want (%s) ---\n%s",
					gc.dialect, got, gc.golden, want)
			}
		})
	}
}

// TestExplainGoldens_DetectsRenamedEventRegression proves the harness
// this task exists to build: a deliberate dialect regression (renaming
// an event brainyard.yaml's fixture actually uses) must fail against the
// recorded golden, not silently pass.
func TestExplainGoldens_DetectsRenamedEventRegression(t *testing.T) {
	original, err := os.ReadFile("brainyard.yaml")
	if err != nil {
		t.Fatalf("failed to read brainyard.yaml: %v", err)
	}
	mutated := strings.Replace(string(original), "event: agent_content", "event: renamed_content", 1)
	if mutated == string(original) {
		t.Fatal("expected to find 'event: agent_content' to mutate — has brainyard.yaml changed?")
	}

	engine, err := mapping.Load([]byte(mutated))
	if err != nil {
		t.Fatalf("failed to load mutated dialect: %v", err)
	}

	tr, err := replay.New("../testdata/fixtures/brainyard-session.jsonl", 0)
	if err != nil {
		t.Fatalf("failed to open fixture: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("failed to start replay: %v", err)
	}
	defer func() { _ = tr.Close() }()

	var b strings.Builder
	index := 0
	for f := range tr.Frames() {
		index++
		b.WriteString(explain.Trace(engine, f.Name, f.Data, index).Render())
	}

	golden, err := os.ReadFile("../testdata/goldens/brainyard.explain.txt")
	if err != nil {
		t.Fatalf("failed to read golden: %v", err)
	}
	if b.String() == string(golden) {
		t.Error("expected the renamed-event regression to change the explain trace, but it matched the golden unchanged")
	}
	if !strings.Contains(b.String(), "no rule matched") {
		t.Error("expected agent_content frames to fall through unmatched after the rename")
	}
}

// TestExplainGoldens_DetectsRenamedEventRegression_A2A is the a2a
// counterpart to TestExplainGoldens_DetectsRenamedEventRegression above: a
// deliberate regression (renaming the field a2a.yaml's statusUpdate rule
// matches on) must fail against the recorded golden, not silently pass.
// a2a.yaml has no wire discriminator (discriminator: auto — see the
// dialect's own header comment), so this is a separate function rather
// than a generalization of the brainyard test: the mutation target here is
// a match: {exists: ...} clause, not an event: string, and generalizing
// would obscure that structural difference behind a shared mutation table.
func TestExplainGoldens_DetectsRenamedEventRegression_A2A(t *testing.T) {
	original, err := os.ReadFile("a2a.yaml")
	if err != nil {
		t.Fatalf("failed to read a2a.yaml: %v", err)
	}
	mutated := strings.Replace(string(original), "exists: statusUpdate", "exists: renamedStatusUpdate", 1)
	if mutated == string(original) {
		t.Fatal("expected to find 'exists: statusUpdate' to mutate — has a2a.yaml changed?")
	}

	engine, err := mapping.Load([]byte(mutated))
	if err != nil {
		t.Fatalf("failed to load mutated dialect: %v", err)
	}

	tr, err := replay.New("../testdata/fixtures/a2a-session.jsonl", 0)
	if err != nil {
		t.Fatalf("failed to open fixture: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		t.Fatalf("failed to start replay: %v", err)
	}
	defer func() { _ = tr.Close() }()

	var b strings.Builder
	index := 0
	for f := range tr.Frames() {
		index++
		b.WriteString(explain.Trace(engine, f.Name, f.Data, index).Render())
	}

	golden, err := os.ReadFile("../testdata/goldens/a2a.explain.txt")
	if err != nil {
		t.Fatalf("failed to read golden: %v", err)
	}
	if b.String() == string(golden) {
		t.Error("expected the renamed-field regression to change the explain trace, but it matched the golden unchanged")
	}
	if !strings.Contains(b.String(), "no rule matched") {
		t.Error("expected statusUpdate frames to fall through unmatched after the rename")
	}
}
