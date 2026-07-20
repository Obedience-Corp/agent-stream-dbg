//go:build ignore

// Offline-friendly probe against a live Obey daemon.
//
//	go run demos/obey_live_probe.go demos/configs/obey-live-demo.yaml
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/client"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/events"
)

func main() {
	path := "demos/configs/obey-live-demo.yaml"
	if len(os.Args) > 1 {
		path = os.Args[1]
	}
	cfg, err := config.LoadConfigFile(path)
	if err != nil {
		panic(err)
	}
	parser := bridge.NewParser(cfg.Dialect.File)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tr, err := client.NewTransport(cfg, parser, "watch")
	if err != nil {
		panic(err)
	}
	if err := tr.Connect(ctx); err != nil {
		panic(err)
	}
	defer tr.Close()

	n, content, detail, session, other := 0, 0, 0, 0, 0
	deadline := time.After(6 * time.Second)
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
				fmt.Printf("frame=%d name=%q parse_err=%v\n", n, frame.Name, err)
				continue
			}
			switch evt.Kind {
			case events.KindContent:
				content++
			case events.KindDetail:
				detail++
			case events.KindSessionStart, events.KindSessionEnd:
				session++
			default:
				other++
			}
			fmt.Printf("frame=%d name=%q kind=%v\n", n, frame.Name, evt.Kind)
			if n >= 10 {
				break loop
			}
		}
	}
	fmt.Printf("SUMMARY frames=%d session=%d content=%d detail=%d other=%d\n", n, session, content, detail, other)
	if n == 0 {
		os.Exit(1)
	}
}
