package testutil

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

// ReferenceSessionFixturePath finds the shipped multi-agent session fixture
// without coupling tests to a dialect or participant identity.
func ReferenceSessionFixturePath(dir string) (string, error) {
	paths, err := filepath.Glob(filepath.Join(dir, "*-session.jsonl"))
	if err != nil {
		return "", err
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		if bytes.Contains(data, []byte(`"flow_step_start"`)) &&
			bytes.Contains(data, []byte(`"session_complete"`)) {
			return path, nil
		}
	}
	return "", fmt.Errorf("no reference session fixture in %s", dir)
}
