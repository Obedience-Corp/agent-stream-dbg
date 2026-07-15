package mapping

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"
)

// valueFormKind identifies which of the DSL's five value forms a ValueForm holds.
type valueFormKind int

const (
	valueFormNone valueFormKind = iota
	valueFormPath
	valueFormConst
	valueFormPathDefault
	valueFormFirst
	valueFormRaw
)

// ValueForm extracts a value from a matched frame's data for
// source/content/seq/fields.*, per one of the DSL's five forms:
// a bare gjson path, {const: v}, {path: p, default: v}, {first: [p1, p2]},
// or {raw: true} (the entire frame body as a string).
type ValueForm struct {
	kind     valueFormKind
	path     string
	def      any
	first    []string
	constVal any
}

// UnmarshalYAML implements yaml.Unmarshaler.
func (v *ValueForm) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var s string
		if err := node.Decode(&s); err != nil {
			return err
		}
		v.kind = valueFormPath
		v.path = s
		return nil
	case yaml.MappingNode:
		var constNode, pathNode, defaultNode, firstNode, rawNode *yaml.Node
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, valNode := node.Content[i].Value, node.Content[i+1]
			switch key {
			case "const":
				constNode = valNode
			case "path":
				pathNode = valNode
			case "default":
				defaultNode = valNode
			case "first":
				firstNode = valNode
			case "raw":
				rawNode = valNode
			default:
				return fmt.Errorf("invalid value form: unknown key %q (must be one of: const, path, default, first, raw)", key)
			}
		}
		switch {
		case constNode != nil:
			var cv any
			if err := constNode.Decode(&cv); err != nil {
				return err
			}
			v.kind = valueFormConst
			v.constVal = cv
		case pathNode != nil:
			var p string
			if err := pathNode.Decode(&p); err != nil {
				return err
			}
			v.kind = valueFormPathDefault
			v.path = p
			if defaultNode != nil {
				var dv any
				if err := defaultNode.Decode(&dv); err != nil {
					return err
				}
				v.def = dv
			}
		case firstNode != nil:
			var list []string
			if err := firstNode.Decode(&list); err != nil {
				return err
			}
			v.kind = valueFormFirst
			v.first = list
		case rawNode != nil:
			var b bool
			if err := rawNode.Decode(&b); err != nil {
				return err
			}
			if !b {
				return fmt.Errorf("value form must be one of: const, path, first, raw")
			}
			v.kind = valueFormRaw
		default:
			return fmt.Errorf("value form must be one of: const, path, first, raw")
		}
		return nil
	default:
		return fmt.Errorf("value form must be a string or a mapping ({const:}, {path:}, {first:}, {raw:})")
	}
}

// IsSet reports whether this value form was declared at all (an omitted
// YAML field leaves the zero ValueForm, which extracts nothing).
func (v ValueForm) IsSet() bool {
	return v.kind != valueFormNone
}

// Extract evaluates this value form against a matched frame's data,
// returning the extracted value and whether anything was found. Extraction
// never errors — a missing path is a soft failure (zero value, ok=false),
// never a reason to drop the event.
func (v ValueForm) Extract(data []byte) (any, bool) {
	switch v.kind {
	case valueFormPath:
		r := gjson.GetBytes(data, v.path)
		if !r.Exists() {
			return nil, false
		}
		return r.Value(), true
	case valueFormConst:
		return v.constVal, true
	case valueFormPathDefault:
		r := gjson.GetBytes(data, v.path)
		if r.Exists() {
			return r.Value(), true
		}
		if v.def != nil {
			return v.def, true
		}
		return nil, false
	case valueFormFirst:
		for _, p := range v.first {
			if r := gjson.GetBytes(data, p); r.Exists() {
				return r.Value(), true
			}
		}
		return nil, false
	case valueFormRaw:
		return strings.TrimSpace(string(data)), true
	default: // valueFormNone
		return nil, false
	}
}

// String extracts this value form as a string, or "" if unset/missing.
func (v ValueForm) String(data []byte) string {
	val, ok := v.Extract(data)
	if !ok {
		return ""
	}
	if s, ok := val.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", val)
}

// Int extracts this value form as an int, or 0 if unset/missing/non-numeric.
// JSON numbers decode as float64, so that form is accepted too.
func (v ValueForm) Int(data []byte) int {
	val, ok := v.Extract(data)
	if !ok {
		return 0
	}
	switch x := val.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case string:
		if n, err := strconv.Atoi(x); err == nil {
			return n
		}
	}
	return 0
}
