// Package dialectinit infers a draft dialect from observed wire frames.
// Heuristics are deliberately dumb and legible (workflow/design/
// stream-debugger-open-source/dialect-spec.md 'init'): a wrong guess is an
// edit, not a bug. The output is always a labeled draft — never presented
// as a finished dialect.
package dialectinit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/tidwall/gjson"
)

// Sample is one observed frame, reduced to what inference needs.
type Sample struct {
	Name string // transport-provided dispatch name, "" if none
	Data []byte // frame body (JSON, or raw bytes for a non-JSON sentinel)
}

// sourceFieldCandidates and the others below are checked in this priority
// order — first candidate present wins. Order matches the design's table.
var (
	sourceFieldCandidates  = []string{"agent_id", "agent", "source", "actor", "node", "name"}
	contentFieldCandidates = []string{"content", "text", "delta", "chunk", "token"}
	seqFieldCandidates     = []string{"seq", "sequence", "index"}
	discriminatorFields    = []string{"type", "event", "kind"}
)

// namePattern maps a glob-ish substring match on the event name to a kind
// guess. Checked in order; first match wins.
var namePatterns = []struct {
	substr string
	kind   string
}{
	{"tool", "tool_call"},
	{"error", "error"},
	{"fail", "error"},
	{"handoff", "handoff"},
	{"delegate", "handoff"},
	{"transfer", "handoff"},
	{"start", "stream_start"},
	{"end", "stream_end"},
	{"complete", "stream_end"},
}

// Draft is an inferred dialect, still labeled as a draft.
type Draft struct {
	SourceLabel   string // where the samples came from, for the header comment
	FrameCount    int
	Discriminator DiscriminatorGuess
	Rules         []RuleGuess
}

// DiscriminatorGuess is the inferred discriminator plus the evidence for it.
type DiscriminatorGuess struct {
	Mode string // "event", "path", or "auto"
	Path string // set only for "path"
	Note string // human-readable justification, rendered as a comment
}

// RuleGuess is one inferred rule, plus the evidence behind each guess.
type RuleGuess struct {
	GroupKey     string // the raw dispatch key this rule matched on (event name, payload value, or structural signature)
	MatchComment string // e.g. `{event: agent_content}` or `{exists: choices.0.delta.content}`
	SeenCount    int
	Kind         string
	KindNote     string
	Source       string // field path, "" if not guessed
	SourceNote   string
	Content      string
	ContentNote  string
	Seq          string
	SeqNote      string
	Unclassified bool
	Sample       string // raw sample text, shown only when Unclassified
}

// Infer groups samples by an inferred discriminator and guesses a rule per
// group. It never errors: an empty sample set yields an empty Draft.
func Infer(samples []Sample, sourceLabel string) *Draft {
	d := &Draft{SourceLabel: sourceLabel, FrameCount: len(samples)}
	d.Discriminator = inferDiscriminator(samples)

	groups := groupSamples(samples, d.Discriminator)
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// For auto mode, precompute how many distinct groups each leaf path
	// appears in, so a rule's match field can be chosen for being unique
	// to its own group rather than merely present in it (a field common
	// to every group would match everything, defeating the point).
	var pathGroupCount map[string]int
	if d.Discriminator.Mode == "auto" {
		pathGroupCount = make(map[string]int)
		for _, key := range keys {
			for _, p := range leafPaths(key) {
				pathGroupCount[p]++
			}
		}
	}

	for _, key := range keys {
		d.Rules = append(d.Rules, guessRule(key, groups[key], d.Discriminator, pathGroupCount))
	}
	return d
}

// inferDiscriminator decides whether frame names come from the transport
// ("event"), a payload field ("path"), or neither ("auto").
func inferDiscriminator(samples []Sample) DiscriminatorGuess {
	nameCounts := map[string]int{}
	for _, s := range samples {
		if s.Name != "" {
			nameCounts[s.Name]++
		}
	}
	if len(nameCounts) > 0 {
		return DiscriminatorGuess{
			Mode: "event",
			Note: fmt.Sprintf("%d distinct event names seen", len(nameCounts)),
		}
	}

	for _, field := range discriminatorFields {
		counts := map[string]int{}
		seenIn := 0
		for _, s := range samples {
			v := gjson.GetBytes(s.Data, field)
			if v.Exists() {
				seenIn++
				counts[v.String()]++
			}
		}
		if seenIn > 0 && len(counts) >= 2 {
			return DiscriminatorGuess{
				Mode: "path",
				Path: field,
				Note: fmt.Sprintf("%d distinct values seen in payload field %q", len(counts), field),
			}
		}
	}

	return DiscriminatorGuess{
		Mode: "auto",
		Note: "no distinguishing event name or payload field found; matching structurally",
	}
}

// groupSamples buckets samples by dispatch name (event/path mode) or by
// structural signature (auto mode).
func groupSamples(samples []Sample, disc DiscriminatorGuess) map[string][]Sample {
	groups := make(map[string][]Sample)
	for _, s := range samples {
		key := dispatchKey(s, disc)
		groups[key] = append(groups[key], s)
	}
	return groups
}

func dispatchKey(s Sample, disc DiscriminatorGuess) string {
	switch disc.Mode {
	case "event":
		return s.Name
	case "path":
		return gjson.GetBytes(s.Data, disc.Path).String()
	default: // auto
		return structuralSignature(s.Data)
	}
}

// structuralSignature fingerprints a frame by its sorted set of leaf JSON
// paths (array elements collapse to index 0), so frames with the same
// shape group together regardless of field values.
func structuralSignature(data []byte) string {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return "raw:" + strings.TrimSpace(string(data))
	}
	var paths []string
	var walk func(prefix string, node any)
	walk = func(prefix string, node any) {
		switch n := node.(type) {
		case map[string]any:
			if len(n) == 0 {
				paths = append(paths, prefix+"{}")
				return
			}
			for k, val := range n {
				p := k
				if prefix != "" {
					p = prefix + "." + k
				}
				walk(p, val)
			}
		case []any:
			if len(n) == 0 {
				paths = append(paths, prefix+"[]")
				return
			}
			walk(prefix+".0", n[0])
		default:
			paths = append(paths, prefix)
		}
	}
	walk("", v)
	sort.Strings(paths)
	return strings.Join(paths, ",")
}

// guessRule infers kind/source/content/seq for one group of same-shape
// samples.
func guessRule(key string, group []Sample, disc DiscriminatorGuess, pathGroupCount map[string]int) RuleGuess {
	rg := RuleGuess{GroupKey: key, SeenCount: len(group)}

	switch disc.Mode {
	case "event":
		rg.MatchComment = fmt.Sprintf("{event: %s}", key)
	case "path":
		rg.MatchComment = fmt.Sprintf("{event: %s}   # via payload field %q", key, disc.Path)
	default:
		rg.MatchComment = structuralMatchComment(key, pathGroupCount)
	}

	sourceField, sourcePresent := pickField(group, sourceFieldCandidates, isPresent)
	contentField, contentPresent := pickLongestStringField(group, contentFieldCandidates)
	seqField, seqPresent := pickField(group, seqFieldCandidates, isNumeric)

	rg.Kind = guessKind(key, contentPresent)
	rg.KindNote = kindNote(key, rg.Kind, contentPresent)

	if sourcePresent {
		rg.Source = sourceField
		rg.SourceNote = fmt.Sprintf("guessed: %d/%d present", fieldPresenceCount(group, sourceField), len(group))
	}
	if contentPresent {
		rg.Content = contentField
		rg.ContentNote = "guessed: longest string field"
	}
	if seqPresent {
		rg.Seq = seqField
		rg.SeqNote = "guessed: numeric field"
	}

	if rg.Kind == "unknown" {
		rg.Unclassified = true
		rg.Sample = strings.TrimSpace(string(group[0].Data))
	}

	return rg
}

// leafPaths splits a structural signature back into its component leaf
// paths, or nil for a non-JSON ("raw:"-prefixed) signature.
func leafPaths(signature string) []string {
	if strings.HasPrefix(signature, "raw:") {
		return nil
	}
	paths := strings.Split(signature, ",")
	if len(paths) == 1 && paths[0] == "" {
		return nil
	}
	return paths
}

// structuralMatchComment picks a match for this group's structural
// signature: a raw match for a non-JSON sentinel like "[DONE]", or —
// among this group's own leaf paths — the one appearing in the fewest
// distinct groups (via pathGroupCount), preferring a real field over a
// bare "{}"/"[]" presence marker. A field present in every group would
// match everything, so uniqueness is what makes the guess useful.
func structuralMatchComment(signature string, pathGroupCount map[string]int) string {
	if raw, ok := strings.CutPrefix(signature, "raw:"); ok {
		return fmt.Sprintf("{raw: %q}", raw)
	}
	paths := leafPaths(signature)
	if len(paths) == 0 {
		return "{exists: \"\"}  # ⚠ no fields found — review manually"
	}

	best := ""
	bestCount := -1
	for _, p := range paths {
		if strings.HasSuffix(p, "{}") || strings.HasSuffix(p, "[]") {
			continue // presence marker, not a real field — last resort only
		}
		count := pathGroupCount[p]
		if bestCount == -1 || count < bestCount || (count == bestCount && p < best) {
			best, bestCount = p, count
		}
	}
	if best == "" {
		// Every path was a bare presence marker; fall back to the first one.
		best, bestCount = paths[0], pathGroupCount[paths[0]]
	}

	if bestCount > 1 {
		return fmt.Sprintf("{exists: %s}  # ⚠ also present in %d other shape(s) — review manually", best, bestCount-1)
	}
	return fmt.Sprintf("{exists: %s}", best)
}

func guessKind(name string, hasContent bool) string {
	lower := strings.ToLower(name)
	for _, p := range namePatterns {
		if strings.Contains(lower, p.substr) {
			return p.kind
		}
	}
	if hasContent {
		return "content"
	}
	return "unknown"
}

func kindNote(name, kind string, hasContent bool) string {
	if kind == "unknown" {
		return "UNCLASSIFIED, please set `kind:`"
	}
	lower := strings.ToLower(name)
	for _, p := range namePatterns {
		if strings.Contains(lower, p.substr) {
			return fmt.Sprintf("guessed: event name matches *%s*", p.substr)
		}
	}
	if hasContent {
		return "guessed: has a text-ish field"
	}
	return ""
}

func isPresent(v gjson.Result) bool { return v.Exists() }
func isNumeric(v gjson.Result) bool { return v.Exists() && v.Type == gjson.Number }

// pickField returns the first candidate field (by priority) satisfying
// pred in a majority of the group's samples.
func pickField(group []Sample, candidates []string, pred func(gjson.Result) bool) (string, bool) {
	for _, field := range candidates {
		count := 0
		for _, s := range group {
			if pred(gjson.GetBytes(s.Data, field)) {
				count++
			}
		}
		if count*2 >= len(group) && count > 0 {
			return field, true
		}
	}
	return "", false
}

// pickLongestStringField returns the candidate string field with the
// highest average length across the group, among fields present at least
// once — a proxy for "the growing content field".
func pickLongestStringField(group []Sample, candidates []string) (string, bool) {
	best := ""
	bestAvg := 0.0
	found := false
	for _, field := range candidates {
		total, count := 0, 0
		for _, s := range group {
			v := gjson.GetBytes(s.Data, field)
			if v.Exists() && v.Type == gjson.String {
				total += len(v.String())
				count++
			}
		}
		if count == 0 {
			continue
		}
		avg := float64(total) / float64(count)
		if !found || avg > bestAvg {
			found = true
			best = field
			bestAvg = avg
		}
	}
	return best, found
}

func fieldPresenceCount(group []Sample, field string) int {
	n := 0
	for _, s := range group {
		if gjson.GetBytes(s.Data, field).Exists() {
			n++
		}
	}
	return n
}

// Render produces the commented draft YAML text, matching the shape
// documented in dialect-spec.md 'init'.
func (d *Draft) Render() string {
	var b bytes.Buffer

	fmt.Fprintf(&b, "# Inferred from %d frames at %s — REVIEW BEFORE USE.\n", d.FrameCount, d.SourceLabel)

	switch d.Discriminator.Mode {
	case "event":
		fmt.Fprintf(&b, "discriminator: event    # %s\n", d.Discriminator.Note)
	case "path":
		fmt.Fprintf(&b, "discriminator: {path: %s}    # %s\n", d.Discriminator.Path, d.Discriminator.Note)
	default:
		fmt.Fprintf(&b, "discriminator: auto    # %s\n", d.Discriminator.Note)
	}

	if len(d.Rules) == 0 {
		b.WriteString("rules: []  # no frames observed\n")
		return b.String()
	}

	b.WriteString("rules:\n")
	for i, r := range d.Rules {
		if r.Unclassified {
			fmt.Fprintf(&b, "  - match: %s   # seen %dx — UNCLASSIFIED, please set `kind:`\n", r.MatchComment, r.SeenCount)
			b.WriteString("    kind: unknown\n")
			fmt.Fprintf(&b, "    # sample: %s\n", truncate(r.Sample, 120))
		} else {
			fmt.Fprintf(&b, "  - match: %s   # seen %dx\n", r.MatchComment, r.SeenCount)
			fmt.Fprintf(&b, "    kind: %s", r.Kind)
			if r.KindNote != "" {
				fmt.Fprintf(&b, "                   # %s", r.KindNote)
			}
			b.WriteString("\n")
			if r.Source != "" {
				fmt.Fprintf(&b, "    source: %s                # %s\n", r.Source, r.SourceNote)
			}
			if r.Content != "" {
				fmt.Fprintf(&b, "    content: %s               # %s\n", r.Content, r.ContentNote)
			}
			if r.Seq != "" {
				fmt.Fprintf(&b, "    seq: %s                   # %s\n", r.Seq, r.SeqNote)
			}
		}
		if i != len(d.Rules)-1 {
			b.WriteString("\n")
		}
	}

	return b.String()
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
