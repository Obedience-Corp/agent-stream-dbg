package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// StarterConfig is the minimal, backend-neutral configuration used to guide a
// first interactive launch into the TUI configuration panel.
const StarterConfig = `# Stream Debugger starter configuration.
# Fill in the connection details in the TUI, then press Ctrl+S to save.
# If the backend requires bearer auth, set API_KEY in .env; the TUI can save
# that environment-variable reference without storing the secret.
transport:
  type: sse
  base_url: ""
  stream_endpoint:
    url: ""
    method: GET
    auth:
      type: none

dialect:
  file: ""

vars: {}

session:
  auto_setup: true

logging:
  dir: ./logs
`

// WriteStarterConfig creates a new starter config without overwriting an
// existing file. The file is private because it may later contain local
// configuration details or references to credentials.
func WriteStarterConfig(path string) error {
	if path == "" {
		return fmt.Errorf("starter config path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create starter config: %w", err)
	}
	if _, err := file.WriteString(StarterConfig); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("write starter config: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close starter config: %w", err)
	}
	return nil
}
