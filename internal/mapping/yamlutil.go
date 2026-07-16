package mapping

import (
	"bytes"

	"gopkg.in/yaml.v3"
)

// strictDecode decodes a YAML node into out with KnownFields enforcement,
// so a typo'd key inside a union-typed field (discriminator, match, value
// forms) errors at load instead of being silently dropped.
func strictDecode(node *yaml.Node, out any) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err := enc.Encode(node); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	dec := yaml.NewDecoder(&buf)
	dec.KnownFields(true)
	return dec.Decode(out)
}
