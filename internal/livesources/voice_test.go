package livesources

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDiscoveryVerifiesIdentityAndHidesStaleOrUnsafeEntries(t *testing.T) {
	v := Voice{Schema: "obey-voice.debug/v1", ID: "live", RuntimeID: "runtime", SessionID: "session", Token: strings.Repeat("t", 32), StartedAt: time.Now()}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" || r.Header.Get("Authorization") != "Bearer "+v.Token {
			http.Error(w, "unauthorized", 401)
			return
		}
		_ = json.NewEncoder(w).Encode(v)
	}))
	defer server.Close()
	v.Endpoint = server.URL
	root := t.TempDir()
	write := func(name string, entry Voice) {
		t.Helper()
		raw, _ := json.Marshal(entry)
		if err := os.WriteFile(filepath.Join(root, name+".json"), raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("live", v)
	stale := v
	stale.ID = "stale"
	write("stale", stale)
	unsafe := v
	unsafe.Endpoint = "https://example.invalid"
	write("unsafe", unsafe)
	os.WriteFile(filepath.Join(root, "partial.json"), []byte("{"), 0600)
	found, skipped, err := List(context.Background(), root)
	if err != nil || len(found) != 1 || found[0].ID != "live" || skipped != 3 {
		t.Fatal(found, skipped, err)
	}
	server.Close()
	found, skipped, err = List(context.Background(), root)
	if err != nil || len(found) != 0 || skipped != 4 {
		t.Fatal("dead session advertised", found, skipped, err)
	}
}

func TestLiveEndpointsRejectRedirectsAndNonLoopback(t *testing.T) {
	var calls int
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++ }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 302) }))
	defer redirect.Close()
	v := Voice{Schema: "obey-voice.debug/v1", ID: "live", RuntimeID: "runtime", Token: strings.Repeat("t", 32), Endpoint: redirect.URL}
	client := Client()
	defer client.CloseIdleConnections()
	if err := Check(context.Background(), client, v); err == nil || calls != 0 {
		t.Fatal("followed redirect", err, calls)
	}
	for _, endpoint := range []string{"http://localhost:80", "http://127.0.0.1:80/other", "http://127.0.0.1:80?token=x", "http://user@127.0.0.1:80", "http://192.168.1.1:80", "https://127.0.0.1:80"} {
		v.Endpoint = endpoint
		if err := v.Validate(); err == nil {
			t.Fatal("accepted", endpoint)
		}
	}
}
