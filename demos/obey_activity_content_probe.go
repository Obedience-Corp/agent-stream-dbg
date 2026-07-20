//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/client"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
)

func main() {
	cfg, err := config.LoadConfigFile("demos/configs/obey-activity-live.yaml")
	if err != nil {
		panic(err)
	}
	parser := bridge.NewParser(cfg.Dialect.File)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	tr, err := client.NewTransport(cfg, parser, "watch")
	if err != nil {
		panic(err)
	}
	if err := tr.Connect(ctx); err != nil {
		panic(err)
	}
	defer tr.Close()

	// After connect, fire SendMessage in background to generate deltas.
	go func() {
		time.Sleep(800 * time.Millisecond)
		sid := os.Getenv("OBEY_SESSION_ID")
		camp := "8a57dff6-753a-41c2-be15-47c3d8fc4ca8"
		cmd := exec.Command("grpcurl", "-plaintext", "-d", fmt.Sprintf(`{
			"session_id":%q,"campaign_id":%q,"message":"Reply with exactly: hello world","mode":"discussion","client_ref":"probe"
		}`, sid, camp), "unix:///tmp/obey.sock", "local.v1.LocalDaemonService/SendMessage")
		out, err := cmd.CombinedOutput()
		fmt.Fprintf(os.Stderr, "SendMessage err=%v out_len=%d\n", err, len(out))
	}()

	var n, content int
	var energy struct {
		// inline minimal tracking
		hits int
	}
	deadline := time.After(25 * time.Second)
loop:
	for {
		select {
		case <-deadline:
			break loop
		case frame, ok := <-tr.Frames():
			if !ok {
				break loop
			}
			n++
			evt, err := parser.Parse(frame.Name, frame.Data)
			if err != nil || evt == nil {
				continue
			}
			if evt.Kind == events.KindContent {
				content++
				energy.hits++
				src := evt.SourceID
				if src == "" {
					src = "?"
				}
				c := evt.Content
				if len(c) > 40 {
					c = c[:40]
				}
				fmt.Printf("CONTENT #%d agent=%s text=%q\n", content, src, c)
			} else if n <= 15 || evt.Kind == events.KindStreamStart || evt.Kind == events.KindStreamEnd {
				fmt.Printf("frame=%d kind=%v name=%q\n", n, evt.Kind, frame.Name)
			}
			if content >= 20 {
				break loop
			}
		}
	}
	fmt.Printf("SUMMARY frames=%d content=%d\n", n, content)
	if content == 0 {
		os.Exit(2)
	}
}
