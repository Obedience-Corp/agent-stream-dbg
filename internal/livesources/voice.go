// Package livesources discovers local read-only diagnostic endpoints.
package livesources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
)

type Voice struct {
	Schema    string    `json:"schema"`
	ID        string    `json:"id"`
	RuntimeID string    `json:"runtime_id"`
	SessionID string    `json:"session_id"`
	AgentName string    `json:"agent_name"`
	Endpoint  string    `json:"endpoint"`
	Token     string    `json:"token"`
	StartedAt time.Time `json:"started_at"`
	Path      string    `json:"-"`
}

func (v Voice) Validate() error {
	u, err := url.Parse(v.Endpoint)
	if err != nil || u.Scheme != "http" || u.Hostname() != "127.0.0.1" || u.Port() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return fmt.Errorf("live diagnostics must use a local loopback endpoint")
	}
	if v.Schema != "obey-voice.debug/v1" || v.ID == "" || v.RuntimeID == "" || len(v.Token) < 16 || len(v.Token) > 256 {
		return fmt.Errorf("unsupported or incomplete live descriptor")
	}
	return nil
}
func Client() *http.Client {
	return &http.Client{Transport: &http.Transport{Proxy: nil, ResponseHeaderTimeout: 2 * time.Second}, CheckRedirect: func(*http.Request, []*http.Request) error { return fmt.Errorf("live endpoint redirected") }}
}
func Read(path string) (Voice, error) {
	var v Voice
	f, err := filetransport.OpenRegular(path)
	if err != nil {
		return v, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, 16385))
	if err != nil {
		return v, err
	}
	if len(raw) > 16384 {
		return v, fmt.Errorf("live descriptor too large")
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, err
	}
	v.Endpoint = strings.TrimSuffix(v.Endpoint, "/")
	v.Path = path
	return v, v.Validate()
}
func Check(ctx context.Context, client *http.Client, v Voice) error {
	if err := v.Validate(); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.Endpoint+"/health", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+v.Token)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("conversation is no longer available")
	}
	var current Voice
	err = json.NewDecoder(io.LimitReader(resp.Body, 16384)).Decode(&current)
	if err != nil || current.Schema != v.Schema || current.ID != v.ID || current.RuntimeID != v.RuntimeID {
		return fmt.Errorf("conversation endpoint identity changed")
	}
	return nil
}
func List(ctx context.Context, root string) ([]Voice, int, error) {
	entries, err := os.ReadDir(root)
	if os.IsNotExist(err) {
		return nil, 0, nil
	}
	if err != nil {
		return nil, 0, err
	}
	// File names are untrusted and no endpoint outside loopback is contacted.
	candidates := []Voice{}
	skipped := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		v, err := Read(filepath.Join(root, e.Name()))
		if err != nil {
			skipped++
			continue
		}
		candidates = append(candidates, v)
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].StartedAt.After(candidates[j].StartedAt) })
	if len(candidates) > 64 {
		skipped += len(candidates) - 64
		candidates = candidates[:64]
	}
	client := Client()
	defer client.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	type result struct {
		v   Voice
		err error
	}
	jobs := make(chan Voice)
	results := make(chan result, len(candidates))
	for range 4 {
		go func() {
			for v := range jobs {
				results <- result{v, Check(ctx, client, v)}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, v := range candidates {
			jobs <- v
		}
	}()
	out := []Voice{}
	for range candidates {
		r := <-results
		if r.err != nil {
			skipped++
		} else {
			out = append(out, r.v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].StartedAt.After(out[j].StartedAt) })
	return out, skipped, nil
}
