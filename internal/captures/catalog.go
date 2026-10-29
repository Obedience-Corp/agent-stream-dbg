// Package captures discovers Obey Voice bundles without reading conversation content.
package captures

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
)

const manifestLimit = 64 << 10

// Entry uses the actual on-disk location, never the manifest's original directory.
type Entry struct {
	Path      string
	CaptureID string    `json:"capture_id"`
	SessionID string    `json:"session_id"`
	StartedAt time.Time `json:"started_at"`
	Status    string    `json:"status"`
	Mode      string    `json:"mode"`
	Schema    string    `json:"schema"`
}

// Follow reflects an unsealed manifest, not a claim that its producer is alive.
func (e Entry) Follow() bool { return e.Status == "open" || e.Status == "sealing" }

type Listing struct {
	Entries []Entry
	Skipped int
	Missing bool
}

func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find Obey Voice recordings: %w", err)
	}
	return filepath.Join(home, ".obey", "agents", "voice", "obey-voice", "captures"), nil
}

// List scans one level of bundle directories. Only bounded metadata is read.
func List(root string) (Listing, error) {
	var result Listing
	dirs, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		result.Missing = true
		return result, nil
	}
	if err != nil {
		return result, fmt.Errorf("read captures folder: %w", err)
	}
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}
		path := filepath.Join(root, dir.Name())
		entry, err := readEntry(path)
		if err != nil {
			result.Skipped++
			continue
		}
		result.Entries = append(result.Entries, entry)
	}
	sort.Slice(result.Entries, func(i, j int) bool {
		a, b := result.Entries[i], result.Entries[j]
		if a.StartedAt.Equal(b.StartedAt) {
			return a.Path < b.Path
		}
		return a.StartedAt.After(b.StartedAt)
	})
	return result, nil
}

func readEntry(dir string) (Entry, error) {
	var entry Entry
	raw, err := readBounded(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return entry, err
	}
	if err := json.Unmarshal(raw, &entry); err != nil {
		return entry, err
	}
	if entry.Schema != "obey-voice.capture/v1" || entry.CaptureID == "" {
		return entry, fmt.Errorf("not an Obey Voice capture manifest")
	}
	entry.Path = filepath.Join(dir, "events.jsonl")
	info, err := os.Stat(entry.Path)
	if err != nil {
		return entry, err
	}
	if !info.Mode().IsRegular() {
		return entry, fmt.Errorf("events must be a regular file")
	}
	return entry, nil
}

func readBounded(path string) ([]byte, error) {
	f, err := filetransport.OpenRegular(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, manifestLimit+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > manifestLimit {
		return nil, fmt.Errorf("metadata exceeds %d bytes", manifestLimit)
	}
	return raw, nil
}
