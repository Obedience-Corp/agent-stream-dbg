package captures

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Preferences are separate from stream YAML configs so discovery never lists them.
type Preferences struct {
	Root string `json:"obey_voice_captures"`
}

func LoadRoot(configDir string) (string, error) {
	raw, err := readBounded(filepath.Join(configDir, "capture-browser.json"))
	if os.IsNotExist(err) {
		return DefaultRoot()
	}
	if err != nil {
		return "", fmt.Errorf("load capture folder: %w", err)
	}
	var pref Preferences
	if err := json.Unmarshal(raw, &pref); err != nil {
		return "", fmt.Errorf("load capture folder: %w", err)
	}
	if pref.Root == "" {
		return DefaultRoot()
	}
	return pref.Root, nil
}

// ResolveRoot accepts a pasted captures-folder path, including ~ and spaces.
func ResolveRoot(input, cwd string) (string, error) {
	path := strings.TrimSpace(input)
	if len(path) >= 2 && ((path[0] == '"' && path[len(path)-1] == '"') || (path[0] == '\'' && path[len(path)-1] == '\'')) {
		path = path[1 : len(path)-1]
	}
	if path == "" {
		return "", fmt.Errorf("choose a captures folder")
	}
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		if path == "~" {
			path = home
		} else {
			path = filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(cwd, path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", fmt.Errorf("open captures folder: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("choose the captures folder, not a file")
	}
	return path, nil
}

func SaveRoot(configDir, root string) error {
	raw, err := json.MarshalIndent(Preferences{Root: root}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(configDir, ".capture-browser-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(append(raw, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), filepath.Join(configDir, "capture-browser.json")); err != nil {
		return fmt.Errorf("remember captures folder: %w", err)
	}
	return nil
}
