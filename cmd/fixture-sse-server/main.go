// Command fixture-sse-server replays a JSONL fixture as SSE for offline demos
// and VHS recordings. Not a production server.
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/testutil"
)

func main() {
	fixture := flag.String("fixture", "testdata/fixtures/brainyard-session.jsonl", "JSONL fixture path")
	delay := flag.Duration("delay", 90*time.Millisecond, "delay between SSE frames (shows motion)")
	addr := flag.String("addr", "127.0.0.1:18765", "listen address")
	flag.Parse()

	// NewMockSSEServer owns the handler we want; bind it to a fixed port via
	// a one-hop reverse proxy so demo configs / VHS tapes use a stable URL.
	mock, err := testutil.NewMockSSEServer(*fixture, *delay)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fixture-sse-server: %v\n", err)
		os.Exit(1)
	}
	defer mock.Close()

	target := mock.URL()
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "listen %s: %v\n", *addr, err)
		os.Exit(1)
	}

	srv := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			req, err := http.NewRequestWithContext(r.Context(), r.Method, target+r.URL.RequestURI(), r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			req.Header = r.Header.Clone()
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadGateway)
				return
			}
			defer resp.Body.Close()
			for k, vv := range resp.Header {
				for _, v := range vv {
					w.Header().Add(k, v)
				}
			}
			w.WriteHeader(resp.StatusCode)
			flusher, _ := w.(http.Flusher)
			buf := make([]byte, 4096)
			for {
				n, readErr := resp.Body.Read(buf)
				if n > 0 {
					_, _ = w.Write(buf[:n])
					if flusher != nil {
						flusher.Flush()
					}
				}
				if readErr != nil {
					return
				}
			}
		}),
	}

	fmt.Fprintf(os.Stderr, "fixture-sse-server: replaying %s on http://%s (delay %s)\n", *fixture, *addr, *delay)
	if err := srv.Serve(ln); err != nil {
		fmt.Fprintf(os.Stderr, "serve: %v\n", err)
		os.Exit(1)
	}
}
