package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/livesources"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/transport/sse"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/visualizer"
	tea "github.com/charmbracelet/bubbletea"
)

func runLiveVoice(path string) error {
	v, err := livesources.Read(path)
	if err != nil {
		return err
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	client := livesources.Client()
	defer client.CloseIdleConnections()
	if err := livesources.Check(ctx, client, v); err != nil {
		return err
	}
	parser, err := bridge.NewParserFor("obey-voice")
	if err != nil {
		return err
	}
	source := sse.New("GET", v.Endpoint+"/events", nil, map[string]string{"Authorization": "Bearer " + v.Token}).WithClient(client)
	model := visualizer.NewObserverModel(ctx, source, parser, v.AgentName+" · "+v.SessionID, true, v.ID)
	defer model.Close()
	_, err = tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	return err
}
