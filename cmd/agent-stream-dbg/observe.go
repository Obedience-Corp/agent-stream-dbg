package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	filetransport "github.com/Obedience-Corp/agent-stream-dbg/internal/transport/file"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/visualizer"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/urfave/cli/v2"
)

func observeAction(c *cli.Context) error {
	if c.NArg() != 1 {
		return fmt.Errorf("observe requires one explicit capture file path")
	}
	path, err := filepath.Abs(c.Args().First())
	if err != nil {
		return err
	}
	parser, err := bridge.NewParserFor(c.String("dialect"))
	if err != nil {
		return err
	}
	manifestID := ""
	manifest := filepath.Join(filepath.Dir(path), "manifest.json")
	f, err := os.Open(manifest)
	if err == nil {
		data, readErr := io.ReadAll(io.LimitReader(f, (64<<10)+1))
		closeErr := f.Close()
		if readErr != nil {
			return readErr
		}
		if closeErr != nil {
			return closeErr
		}
		manifestID, err = visualizer.ManifestCaptureID(data)
		if err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	source := filetransport.New(path, c.Bool("follow"))
	model := visualizer.NewObserverModel(ctx, source, parser, path, c.Bool("follow"), manifestID)
	defer model.Close()
	_, err = tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx)).Run()
	return err
}
