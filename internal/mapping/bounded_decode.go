package mapping

import (
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
	"github.com/tidwall/gjson"
	"math"
	"time"
)

// BoundedScalar rejects collections before Value() can expand them. Interval
// metadata is scalar; oversized strings remain available in the original raw.
func (v ValueForm) BoundedScalar(data []byte) (any, bool) {
	scalar := func(r gjson.Result) (any, bool) {
		if r.IsArray() || r.IsObject() {
			return nil, false
		}
		switch r.Type {
		case gjson.Null:
			return nil, r.Exists()
		case gjson.String:
			if len(r.Str) > 256 {
				return nil, false
			}
			return r.Str, true
		case gjson.Number:
			n := r.Float()
			return n, !math.IsNaN(n) && !math.IsInf(n, 0)
		case gjson.True:
			return true, true
		case gjson.False:
			return false, true
		}
		return nil, false
	}
	switch v.kind {
	case valueFormLookup:
		return v.Extract(data)
	case valueFormConst:
		return projectionScalar(v.constVal)
	case valueFormPath:
		return scalar(gjson.GetBytes(data, v.path))
	case valueFormPathDefault:
		r := gjson.GetBytes(data, v.path)
		if r.Exists() {
			return scalar(r)
		}
		return projectionScalar(v.def)
	case valueFormFirst:
		for _, path := range v.first {
			r := gjson.GetBytes(data, path)
			if r.Exists() {
				return scalar(r)
			}
		}
	case valueFormRaw:
		if len(data) <= 256 {
			return string(data), true
		}
	}
	return nil, false
}
func estimateJSON(result gjson.Result, budget, depth int) (int, bool) {
	if depth > 64 || budget < 64 {
		return 0, false
	}
	if result.IsObject() || result.IsArray() {
		used := 128
		ok := true
		result.ForEach(func(key, value gjson.Result) bool {
			cost := 128 + len(key.Str)
			n, valid := estimateJSON(value, budget-used-cost, depth+1)
			used += cost + n
			if !valid || used > budget {
				ok = false
				return false
			}
			return true
		})
		return used, ok
	}
	if result.Type == gjson.String {
		n := 64 + len(result.Str)
		return n, n <= budget
	}
	return 32, true
}
func (v ValueForm) estimate(data []byte, budget int) (int, bool) {
	switch v.kind {
	case valueFormNone:
		return 0, true
	case valueFormRaw:
		n := 64 + len(data)
		return n, n <= budget
	case valueFormLookup:
		if value, ok := v.BoundedScalar(data); ok {
			if text, ok := value.(string); ok {
				return 64 + len(text), 64+len(text) <= budget
			}
		}
		return 32, true
	case valueFormConst:
		if value, ok := projectionScalar(v.constVal); ok {
			if s, ok := value.(string); ok {
				return 64 + len(s), 64+len(s) <= budget
			}
			return 32, true
		}
		return 0, false
	case valueFormPath:
		return estimateJSON(gjson.GetBytes(data, v.path), budget, 0)
	case valueFormPathDefault:
		r := gjson.GetBytes(data, v.path)
		if r.Exists() {
			return estimateJSON(r, budget, 0)
		}
		if value, ok := projectionScalar(v.def); ok {
			if s, ok := value.(string); ok {
				return 64 + len(s), 64+len(s) <= budget
			}
			return 32, true
		}
		return 0, false
	case valueFormFirst:
		for _, path := range v.first {
			r := gjson.GetBytes(data, path)
			if r.Exists() {
				return estimateJSON(r, budget, 0)
			}
		}
		return 0, true
	}
	return 0, false
}

// DecodeBounded preflights retained trees/aliases before ordinary Decode can
// expand JSON collections. Its conservative bound includes the raw buffer and
// the separately capped interval projection. Rejected semantics retain raw and
// bounded immutable scope rather than attempting a partial JSON projection.
func (e *Engine) DecodeBounded(name string, data []byte, budget int) *events.Event {
	used := 1024 + len(data)
	cost, ok := estimateJSON(gjson.ParseBytes(data), budget-used, 0)
	used += cost
	idx := e.Match(e.ResolveName(name, data), data)
	if idx >= 0 && ok {
		rule := e.Rules[idx]
		if len(rule.Fields) > 128 {
			ok = false
		}
		forms := []ValueForm{rule.Source, rule.Content, rule.Seq, rule.Timestamp}
		for _, form := range rule.Fields {
			forms = append(forms, form)
		}
		for _, form := range forms {
			cost, valid := form.estimate(data, budget-used)
			used += cost
			if !valid || used > budget {
				ok = false
				break
			}
		}
		if len(rule.IntervalArrays) > 0 {
			used += maxProjectionBytes
		}
	}
	if ok && used <= budget {
		return e.Decode(name, data)
	}
	event := &events.Event{Name: e.ResolveName(name, data), Kind: events.KindUnknown, Raw: data, Fields: map[string]any{"projection_unavailable": "bounded decode budget exceeded; raw evidence retained"}}
	if idx >= 0 {
		rule := e.Rules[idx]
		for _, key := range []string{"capture_id", "runtime_id", "session_id", "conversation_epoch", "turn_id", "parent_turn_id", "response_id", "persona_id", "job_id", "turn_start_ms", "mono_ms"} {
			if form, ok := rule.Fields[key]; ok {
				if value, ok := form.BoundedScalar(data); ok {
					event.Fields[key] = value
				}
			}
		}
		if value, ok := rule.Seq.BoundedScalar(data); ok {
			if n, ok := projectionNumber(value); ok && n >= 0 && n < 1<<53 {
				event.Seq = int(n)
			}
		}
		if value, ok := rule.Timestamp.BoundedScalar(data); ok {
			if text, ok := value.(string); ok {
				if at, err := time.Parse(time.RFC3339Nano, text); err == nil {
					event.Timestamp = at
					event.SourceTimestampKnown = true
				}
			}
		}
	}
	return event
}
