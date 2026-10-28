package visualizer

import (
	"crypto/sha256"
	"fmt"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport"
	"sort"
	"strings"
)

// Retained event trees/raw buffers share a 24 MiB budget; 8 MiB is reserved for
// bounded summaries, dedup/index metadata, current frame, and visible panes.
// Live and on-demand pages each use half; together at most 10,000 events.
const observerPageEvents = 5000
const observerPageBytes = 12 << 20
const observerMaxGroups = 100
const observerMaxIndex = 4096

type observedRecord struct {
	Event  *events.Event
	Scope  string
	Offset int64
	Bytes  int
	Issue  string
	hash   [32]byte
	dedup  string
}
type sourceIndexEntry struct {
	Offset  int64
	Ordinal int64
	Seq     int
	Scope   string
}
type observedGroup struct {
	StartMS                                           *float64
	Key, Label, Runtime, Turn, Response, Job, Persona string
	Offset                                            int64
	Count                                             int
}
type observerStore struct {
	records    []*observedRecord
	bytes      int
	seen       map[string][32]byte
	index      []sourceIndexEntry
	spacing    int64
	ordinal    int64
	evicted    bool
	lastSeq    int
	legacyTurn int
	legacy     bool
	issues     []string
}

func newObserverStore() *observerStore {
	return &observerStore{seen: make(map[string][32]byte), spacing: 256}
}
func boundedIdentity(value string) string {
	if len(value) > 256 {
		return ""
	}
	return value
}
func eventScope(e *events.Event) string {
	runtime := boundedIdentity(e.StringField("runtime_id"))
	session := boundedIdentity(e.StringField("session_id"))
	if turn := boundedIdentity(e.StringField("turn_id")); turn != "" {
		return runtime + "/turn/" + turn
	}
	if job := boundedIdentity(e.StringField("job_id")); job != "" {
		return runtime + "/job/" + job
	}
	if response := boundedIdentity(e.StringField("response_id")); response != "" {
		return runtime + "/response/" + response
	}
	return runtime + "/session/" + session
}
func (s *observerStore) issue(message string) {
	if len(message) > 512 {
		message = message[:512]
	}
	if len(s.issues) == 16 {
		s.issues = s.issues[1:]
	}
	s.issues = append(s.issues, message)
}
func retainedValueBytes(value any, depth int) int {
	if depth > 64 {
		return observerPageBytes + 1
	}
	switch v := value.(type) {
	case string:
		return 32 + len(v)
	case []byte:
		return 32 + cap(v)
	case map[string]any:
		n := 128
		for k, item := range v {
			n += 128 + len(k) + retainedValueBytes(item, depth+1)
			if n > observerPageBytes {
				return n
			}
		}
		return n
	case []any:
		n := 32 + 32*cap(v)
		for _, item := range v {
			n += retainedValueBytes(item, depth+1)
			if n > observerPageBytes {
				return n
			}
		}
		return n
	default:
		return 32
	}
}
func recordBytes(e *events.Event) int {
	return 1024 + cap(e.Raw) + len(e.Name) + len(e.SourceID) + len(e.Content) + retainedValueBytes(e.Fields, 0)
}
func (s *observerStore) add(e *events.Event, frame transport.Frame, index bool) bool {
	scope := eventScope(e)
	if s.legacy && e.StringField("turn_id") == "" && e.StringField("job_id") == "" && e.Kind != events.KindSessionStart && e.Kind != events.KindSessionEnd {
		if e.BoolField("local_turn_boundary") {
			s.legacyTurn++
		}
		if s.legacyTurn > 0 {
			scope = fmt.Sprintf("legacy-local/%d", s.legacyTurn)
		}
	}
	s.ordinal++
	if index && (s.ordinal == 1 || (s.ordinal-1)%s.spacing == 0) {
		s.index = append(s.index, sourceIndexEntry{frame.Offset, s.ordinal, e.Seq, scope})
		if len(s.index) > observerMaxIndex {
			kept := s.index[:0]
			for i, entry := range s.index {
				if i%2 == 0 {
					kept = append(kept, entry)
				}
			}
			s.index = kept
			s.spacing *= 2
		}
	}
	raw := frame.Raw
	if raw == nil {
		raw = frame.Data
	}
	e.Raw = raw
	hash := sha256.Sum256(frame.Data)
	capture := boundedIdentity(e.StringField("capture_id"))
	key := ""
	issue := ""
	if capture != "" && e.Seq > 0 {
		key = fmt.Sprintf("%s/%d", capture, e.Seq)
		if prior, ok := s.seen[key]; ok {
			if prior == hash {
				return false
			}
			issue = "Capture conflict: same capture/sequence has different content"
			s.issue(issue)
		} else if e.Seq <= s.lastSeq {
			issue = "Earlier duplicate requires on-disk verification"
			s.issue(issue)
		}
		if e.Seq > s.lastSeq+1 && s.lastSeq > 0 {
			s.issue(fmt.Sprintf("Missing source sequences %d–%d", s.lastSeq+1, e.Seq-1))
		}
		s.lastSeq = max(s.lastSeq, e.Seq)
	}
	size := recordBytes(e)
	if size > observerPageBytes {
		s.issue("Record too large for retained decoded budget; raw evidence remains on disk")
		e = &events.Event{Name: e.Name, Kind: events.KindDetail, Timestamp: e.Timestamp, Seq: e.Seq, Raw: raw, Fields: map[string]any{"projection_unavailable": "decoded budget exceeded"}}
		size = recordBytes(e)
		key = ""
	}
	r := &observedRecord{Event: e, Scope: scope, Offset: frame.Offset, Bytes: size, Issue: issue, hash: hash, dedup: key}
	for len(s.records) >= observerPageEvents || s.bytes+size > observerPageBytes {
		s.evict()
	}
	s.records = append(s.records, r)
	s.bytes += size
	if key != "" {
		s.seen[key] = hash
	}
	return true
}
func (s *observerStore) evict() {
	if len(s.records) == 0 {
		return
	}
	r := s.records[0]
	s.records[0] = nil
	s.records = s.records[1:]
	s.bytes -= r.Bytes
	s.evicted = true
	if r.dedup != "" {
		if hash, ok := s.seen[r.dedup]; ok && hash == r.hash {
			delete(s.seen, r.dedup)
		}
	}
}
func (s *observerStore) groups() []observedGroup {
	groups := make([]observedGroup, 0, observerMaxGroups)
	positions := make(map[string]int, observerMaxGroups)
	for i := len(s.records) - 1; i >= 0; i-- {
		r := s.records[i]
		position, ok := positions[r.Scope]
		if !ok {
			if len(groups) >= observerMaxGroups {
				continue
			}
			position = len(groups)
			positions[r.Scope] = position
			e := r.Event
			turn := boundedIdentity(e.StringField("turn_id"))
			job := boundedIdentity(e.StringField("job_id"))
			response := boundedIdentity(e.StringField("response_id"))
			label := turn
			if label == "" {
				label = job
			}
			if label == "" {
				label = response
			}
			if label == "" {
				label = "session lane"
			}
			if strings.HasPrefix(r.Scope, "legacy-local/") {
				label = r.Scope + " (uncorrelated)"
			}
			groups = append(groups, observedGroup{Key: r.Scope, Label: label, Runtime: boundedIdentity(e.StringField("runtime_id")), Turn: turn, Job: job, Response: response, Persona: boundedIdentity(e.StringField("persona_id")), Offset: r.Offset})
		}
		g := &groups[position]
		g.Count++
		if g.StartMS == nil {
			if value, ok := finiteNumber(r.Event.Fields["turn_start_ms"]); ok {
				g.StartMS = &value
			}
		}
		g.Offset = min(g.Offset, r.Offset)
		if g.Response == "" {
			g.Response = boundedIdentity(r.Event.StringField("response_id"))
		}
		if g.Persona == "" {
			g.Persona = boundedIdentity(r.Event.StringField("persona_id"))
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		a, b := groups[i], groups[j]
		if a.Runtime != b.Runtime {
			return a.Runtime < b.Runtime
		}
		if a.StartMS == nil && b.StartMS != nil {
			return true
		}
		if a.StartMS != nil && b.StartMS == nil {
			return false
		}
		if a.StartMS != nil && b.StartMS != nil && *a.StartMS != *b.StartMS {
			return *a.StartMS < *b.StartMS
		}
		return a.Offset < b.Offset
	})
	return groups
}
func (s *observerStore) selected(key string) []*observedRecord {
	out := make([]*observedRecord, 0)
	for _, r := range s.records {
		if r.Scope == key {
			out = append(out, r)
		}
	}
	return out
}

func latestObservedGroup(groups []observedGroup, runtime string) string {
	best := -1
	for i, g := range groups {
		if g.Turn == "" && !strings.HasPrefix(g.Key, "legacy-local/") {
			continue
		}
		if runtime != "" && g.Runtime != "" && g.Runtime != runtime {
			continue
		}
		if best < 0 || (g.StartMS != nil && (groups[best].StartMS == nil || *g.StartMS >= *groups[best].StartMS)) {
			best = i
		} else if g.StartMS == nil && groups[best].StartMS == nil && g.Offset > groups[best].Offset {
			best = i
		}
	}
	if best >= 0 {
		return groups[best].Key
	}
	if len(groups) > 0 {
		return groups[len(groups)-1].Key
	}
	return ""
}
