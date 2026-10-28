package dialects_test

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/mapping"
)

func TestVoiceTraceCanonicalSemantics(t *testing.T) {
	engine, err := mapping.LoadFile("obey-voice.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"overlap-interrupted.jsonl", "fallback.jsonl", "incomplete.jsonl", "late-attach.jsonl", "unknown.jsonl"} {
		t.Run(name, func(t *testing.T) {
			f, err := os.Open(filepath.Join("../testdata/fixtures/pipeline-trace", name))
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			scanner := bufio.NewScanner(f)
			scanner.Buffer(make([]byte, 4096), 1<<20)
			spans, unknown, zero, missing := 0, 0, false, false
			attempts := map[string]bool{}
			for scanner.Scan() {
				raw := scanner.Bytes()
				e := engine.Decode("", raw)
				if string(e.Raw) != string(raw) || e.Timestamp.Year() != 2026 {
					t.Fatal("source raw/time lost")
				}
				if e.Kind == events.KindUnknown {
					unknown++
				}
				if e.StringField("span_id") != "" {
					spans++
					if e.StringField("runtime_id") == "" || e.StringField("turn_id") == "" {
						t.Fatal("source scope lost")
					}
					if value, ok := e.Fields["start_ms"].(float64); ok && value == 0 {
						zero = true
					}
					if value, ok := e.Fields["end_ms"]; ok && value == nil {
						missing = true
					}
					if id := e.StringField("attempt_id"); id != "" {
						attempts[id] = true
					}
				}
			}
			if err := scanner.Err(); err != nil {
				t.Fatal(err)
			}
			if name == "overlap-interrupted.jsonl" && (spans == 0 || !zero || !missing) {
				t.Fatal("overlap/zero/missing semantics lost")
			}
			if name == "fallback.jsonl" && len(attempts) < 2 {
				t.Fatal("fallback attempt identities collapsed")
			}
			if name == "unknown.jsonl" && unknown == 0 {
				t.Fatal("unknown records not inspectable")
			}
		})
	}
}
func TestVoiceTraceFixtureChecksums(t *testing.T) {
	root := "../testdata/fixtures/pipeline-trace"
	raw, err := os.ReadFile(filepath.Join(root, "checksums.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		SourceRevision string            `json:"source_revision"`
		SHA256         map[string]string `json:"sha256"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.SourceRevision) != 40 {
		t.Fatal("source revision not pinned")
	}
	for name, want := range manifest.SHA256 {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(raw)
		if got := hex.EncodeToString(sum[:]); got != want {
			t.Fatalf("%s canonical checksum changed", name)
		}
	}
}
func TestVoiceTraceLegacyAndUnsupportedRemainDecodable(t *testing.T) {
	engine, err := mapping.LoadFile("obey-voice.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{`{"type":"header","v":1}`, `{"type":"header","v":2}`, `{"type":"header","v":2,"trace_schema":"obey-voice.trace/v99","future":true}`} {
		e := engine.Decode("", []byte(header))
		if e.Kind != events.KindSessionStart || string(e.Raw) != header {
			t.Fatal("legacy/future raw inspection lost")
		}
	}
	e := engine.Decode("", []byte(`{"type":"user","text":"legacy input","seq":2}`))
	if !e.BoolField("local_turn_boundary") || e.StringField("response_id") != "" {
		t.Fatal("legacy input fabricated response identity")
	}
}
