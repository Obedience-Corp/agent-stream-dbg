package mapping

import (
	"fmt"

	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"
)

// Discriminator tells frames apart, producing the name match rules dispatch
// on. It has three modes:
//   - "event": the transport already provides the name (SSE event: line,
//     gRPC oneof case) — used as-is.
//   - "path": the name lives in the payload; Path is a gjson path to it.
//   - "auto": no name at all — matching is purely structural, first match wins.
type Discriminator struct {
	Mode string // "event", "path", or "auto"
	Path string // set only when Mode == "path"
}

// UnmarshalYAML accepts either a bare scalar ("event" | "auto") or a mapping
// ({path: <gjson path>}).
func (d *Discriminator) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var s string
		if err := node.Decode(&s); err != nil {
			return err
		}
		switch s {
		case "event", "auto":
			d.Mode = s
			return nil
		default:
			return fmt.Errorf(`invalid discriminator %q (must be "event", "auto", or {path: <gjson path>})`, s)
		}
	case yaml.MappingNode:
		var m struct {
			Path string `yaml:"path"`
		}
		if err := strictDecode(node, &m); err != nil {
			return fmt.Errorf("invalid discriminator: %w", err)
		}
		if m.Path == "" {
			return fmt.Errorf("discriminator {path: ...} requires a non-empty path")
		}
		d.Mode = "path"
		d.Path = m.Path
		return nil
	default:
		return fmt.Errorf("discriminator must be a string or a {path: ...} mapping")
	}
}

// Resolve computes the frame's name per this discriminator's mode.
// transportName is what the transport already knows (SSE event: line, etc.);
// data is the frame body used for "path" mode lookups.
func (d Discriminator) Resolve(transportName string, data []byte) string {
	switch d.Mode {
	case "path":
		return gjson.GetBytes(data, d.Path).String()
	case "auto":
		return ""
	default: // "event"
		return transportName
	}
}
