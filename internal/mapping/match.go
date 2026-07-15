package mapping

import (
	"fmt"
	"path"
	"strings"

	"github.com/tidwall/gjson"
	"gopkg.in/yaml.v3"
)

// MatchSpec is one rule's match criteria, parsed directly from YAML.
// Multiple set fields are ANDed; the engine evaluates rules in document
// order and the first whole match wins.
type MatchSpec struct {
	Event  stringOrList `yaml:"event,omitempty"`
	Path   string       `yaml:"path,omitempty"`
	Equals any          `yaml:"equals,omitempty"`
	Exists string       `yaml:"exists,omitempty"`
	Raw    string       `yaml:"raw,omitempty"`
}

// stringOrList accepts either a bare scalar or a YAML sequence, e.g.
// {event: x} or {event: [a, b]}.
type stringOrList []string

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *stringOrList) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var str string
		if err := node.Decode(&str); err != nil {
			return err
		}
		*s = []string{str}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := node.Decode(&list); err != nil {
			return err
		}
		*s = list
		return nil
	default:
		return fmt.Errorf("expected a string or a list of strings")
	}
}

// validate checks the match spec is internally consistent and its glob
// patterns compile, without needing real frame data.
func (m MatchSpec) validate() error {
	if len(m.Event) == 0 && m.Path == "" && m.Exists == "" && m.Raw == "" {
		return fmt.Errorf("match must set at least one of event, path, exists, or raw")
	}
	if m.Path != "" && m.Equals == nil {
		return fmt.Errorf("match.path requires match.equals")
	}
	for _, pattern := range m.Event {
		if _, err := path.Match(pattern, ""); err != nil {
			return fmt.Errorf("invalid event glob %q: %w", pattern, err)
		}
	}
	return nil
}

// Matches reports whether this rule's match criteria are satisfied by the
// given frame name and data. All set conditions are ANDed.
func (m MatchSpec) Matches(name string, data []byte) bool {
	if len(m.Event) > 0 && !matchesAnyGlob(m.Event, name) {
		return false
	}
	if m.Path != "" {
		result := gjson.GetBytes(data, m.Path)
		if !result.Exists() || result.String() != fmt.Sprintf("%v", m.Equals) {
			return false
		}
	}
	if m.Exists != "" && !gjson.GetBytes(data, m.Exists).Exists() {
		return false
	}
	if m.Raw != "" && strings.TrimSpace(string(data)) != m.Raw {
		return false
	}
	return true
}

func matchesAnyGlob(patterns []string, name string) bool {
	for _, p := range patterns {
		if ok, _ := path.Match(p, name); ok {
			return true
		}
	}
	return false
}
