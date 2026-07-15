// Package explain traces individual frames through a dialect: which rule
// matched, what each field extracted and from where, and why a frame fell
// through unmatched. A debugger should be able to debug its own config —
// without this, a wrong path is an empty pane with no cause (dialect-spec.md
// 'explain').
package explain

import (
	"fmt"
	"sort"
	"strings"

	"github.com/lancekrogers/stream-debugger/internal/mapping"
)

// Extraction is one field a matched rule pulled (or tried to pull) out of
// a frame.
type Extraction struct {
	Field string // "source", "content", "seq", or a fields.* key
	Path  string // ValueForm.Describe() — how the value was derived
	Value string
	Found bool // false if the declared path had nothing at this frame
	Empty bool // true if Found but the value is the empty string
}

// FrameTrace is the result of running one frame through a dialect.
type FrameTrace struct {
	Index       int
	Name        string
	MatchedIdx  int // -1 if no rule matched
	MatchDesc   string
	Extractions []Extraction
	Hint        string // populated only when MatchedIdx == -1
}

// Unhealthy reports whether this frame's trace should count against
// --explain's exit code: an unmatched frame, or a matched rule that
// extracted an empty string where it declared a path.
func (t FrameTrace) Unhealthy() bool {
	if t.MatchedIdx < 0 {
		return true
	}
	for _, e := range t.Extractions {
		if e.Empty {
			return true
		}
	}
	return false
}

// Trace runs one frame through engine and records what happened.
// transportName is what the transport already knows (an SSE event: line,
// a replay fixture's own dispatch field); Trace resolves the dialect's
// actual dispatch name from it exactly as Engine.Decode would, so a
// {path: ...}-mode dialect is traced correctly rather than assuming the
// transport's name is already the resolved one.
func Trace(engine *mapping.Engine, transportName string, data []byte, index int) FrameTrace {
	name := engine.ResolveName(transportName, data)
	t := FrameTrace{Index: index, Name: name, MatchedIdx: -1}

	idx := engine.Match(name, data)
	if idx < 0 {
		t.Hint = hintFor(engine.Discriminator, name)
		return t
	}

	rule := engine.Rules[idx]
	t.MatchedIdx = idx
	t.MatchDesc = describeMatch(rule.Match)

	if rule.Source.IsSet() {
		t.Extractions = append(t.Extractions, extract("source", rule.Source, data))
	}
	if rule.Content.IsSet() {
		t.Extractions = append(t.Extractions, extract("content", rule.Content, data))
	}
	if rule.Seq.IsSet() {
		t.Extractions = append(t.Extractions, extract("seq", rule.Seq, data))
	}
	fieldKeys := make([]string, 0, len(rule.Fields))
	for k := range rule.Fields {
		fieldKeys = append(fieldKeys, k)
	}
	sort.Strings(fieldKeys)
	for _, k := range fieldKeys {
		t.Extractions = append(t.Extractions, extract(k, rule.Fields[k], data))
	}

	return t
}

func extract(field string, vf mapping.ValueForm, data []byte) Extraction {
	val, ok := vf.Extract(data)
	e := Extraction{Field: field, Path: vf.Describe(), Found: ok}
	if !ok {
		return e
	}
	e.Value = fmt.Sprintf("%v", val)
	e.Empty = e.Value == ""
	return e
}

// describeMatch renders a MatchSpec the way a dialect author wrote it,
// e.g. "{event: agent_content}" or "{path: type, equals: message_start}".
func describeMatch(m mapping.MatchSpec) string {
	var parts []string
	switch len(m.Event) {
	case 0:
	case 1:
		parts = append(parts, fmt.Sprintf("event: %s", m.Event[0]))
	default:
		parts = append(parts, fmt.Sprintf("event: %v", []string(m.Event)))
	}
	if m.Path != "" {
		parts = append(parts, fmt.Sprintf("path: %s, equals: %v", m.Path, m.Equals))
	}
	if m.Exists != "" {
		parts = append(parts, fmt.Sprintf("exists: %s", m.Exists))
	}
	if m.Raw != "" {
		parts = append(parts, fmt.Sprintf("raw: %q", m.Raw))
	}
	return "{" + strings.Join(parts, ", ") + "}"
}

// hintFor suggests a match rule to add, tailored to the dialect's own
// discriminator mode, for a frame that fell through unmatched.
func hintFor(disc mapping.Discriminator, name string) string {
	switch disc.Mode {
	case "path":
		return fmt.Sprintf("add `- match: {event: %s}`   # dispatched via payload field %q", name, disc.Path)
	case "auto":
		return "add a `- match: {exists: <a field unique to this frame's shape>}` rule"
	default: // event
		return fmt.Sprintf("add `- match: {event: %s}`", name)
	}
}

// Render formats a trace the way dialect-spec.md 'explain' shows it.
func (t FrameTrace) Render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "frame %d  event=%s\n", t.Index, t.Name)

	if t.MatchedIdx < 0 {
		b.WriteString("  ✗ no rule matched → kind=unknown (rendered raw)\n")
		fmt.Fprintf(&b, "      hint: %s\n", t.Hint)
		return b.String()
	}

	fmt.Fprintf(&b, "  ✓ rules[%d] matched %s\n", t.MatchedIdx, t.MatchDesc)

	fieldWidth, pathWidth := 0, 0
	for _, e := range t.Extractions {
		fieldWidth = max(fieldWidth, len(e.Field))
		pathWidth = max(pathWidth, len(e.Path))
	}
	for _, e := range t.Extractions {
		field := e.Field + strings.Repeat(" ", fieldWidth-len(e.Field))
		path := e.Path + strings.Repeat(" ", pathWidth-len(e.Path))
		if !e.Found {
			fmt.Fprintf(&b, "      %s ← %s   (not found)\n", field, path)
			continue
		}
		line := fmt.Sprintf("      %s ← %s   = %q", field, path, e.Value)
		if e.Empty {
			line += "        ⚠ empty — path may be wrong"
		}
		b.WriteString(line + "\n")
	}
	return b.String()
}
