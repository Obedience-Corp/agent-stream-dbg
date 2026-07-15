package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/bridge"
	"github.com/lancekrogers/stream-debugger/internal/client"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/help"
	"github.com/lancekrogers/stream-debugger/internal/logger"
	"github.com/lancekrogers/stream-debugger/internal/mapping"
	"github.com/lancekrogers/stream-debugger/internal/transport/replay"
	"github.com/lancekrogers/stream-debugger/internal/visualizer"
	"github.com/urfave/cli/v2"
)

func main() {
	// Set custom help printer with Lipgloss styling
	cli.HelpPrinter = help.CustomHelpPrinter

	app := &cli.App{
		Name:  "stream-debugger",
		Usage: "Debug SSE streaming responses from multi-agent systems",
		Description: `A configuration-driven CLI tool for visualizing and debugging Server-Sent Events (SSE) streaming.

   Supports two primary modes:
   1. Interactive Mode: Multi-turn chat with real-time visualization (default)
   2. Stream Mode: Single message for CI/CD and scripting

   Press Ctrl+T in interactive mode to toggle between RAW (SSE) and PARSED (agent-organized) views.`,
		Version: "1.0.0",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:    "config",
				Aliases: []string{"c"},
				Usage:   "Path to YAML configuration file",
			},
		},
		Commands: []*cli.Command{
			{
				Name:      "stream",
				Usage:     "Send single message and exit (CI/CD mode)",
				ArgsUsage: "<message>",
				Description: `Sends a single message to the configured backend and displays the streaming response.
   Useful for CI/CD pipelines, scripting, and quick one-off tests.

   ⚠️  IMPORTANT: Flags must come BEFORE the message argument!

   Examples:
     $ stream-debugger stream --config config.yaml "Hello"
     $ stream-debugger stream -c my-api.yaml "What is consciousness?"`,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:     "config",
						Aliases:  []string{"c"},
						Usage:    "Path to YAML configuration file",
						Required: true,
					},
				},
				Action: streamAction,
			},
			{
				Name:      "replay",
				Usage:     "Replay session from log file",
				ArgsUsage: "<session-file>",
				Description: `Replays a recorded session from log files.

   Example:
     $ stream-debugger replay logs/by-session/session_*.jsonl`,
				Action: replayAction,
			},
			{
				Name:      "timeline",
				Usage:     "Visualize agent execution timeline",
				ArgsUsage: "<session-file>",
				Description: `Generates a visual timeline showing which agents ran in parallel,
   execution durations, and performance metrics.

   Example:
     $ stream-debugger timeline logs/by-session/session_*.jsonl`,
				Action: timelineAction,
			},
		},
		Action: defaultAction, // Interactive mode when no command
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// defaultAction runs when no command is provided (interactive mode)
func defaultAction(c *cli.Context) error {
	configPath := c.String("config")
	if configPath == "" {
		return fmt.Errorf("--config flag is required for interactive mode")
	}
	return runInteractive(configPath)
}

// streamAction handles the stream command
func streamAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("message argument is required")
	}
	message := c.Args().Get(0)
	configPath := c.String("config")
	return runStream(configPath, message)
}

// replayAction handles the replay command
func replayAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("session file argument is required")
	}
	sessionFile := c.Args().Get(0)
	return runReplay(sessionFile)
}

// timelineAction handles the timeline command
func timelineAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("session file argument is required")
	}
	sessionFile := c.Args().Get(0)
	return runTimeline(sessionFile)
}

func runInteractive(configPath string) error {
	fmt.Printf("🚀 Stream Debugger - Interactive Mode\n\n")

	// Load YAML configuration
	cfg, err := config.LoadConfigFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	fmt.Printf("🔧 Configuration loaded from: %s\n", configPath)
	fmt.Printf("   Backend: %s\n", cfg.Transport.BaseURL)
	fmt.Printf("   Session: %s\n", cfg.Session.ID)
	fmt.Printf("   Log Dir: %s\n\n", cfg.LogDir)

	// Auto-setup session if configured
	if cfg.Session.AutoSetup {
		fmt.Printf("🔄 Auto-setting up session...\n")

		vars := mapping.InterpolationVars{
			BaseURL:   cfg.Transport.BaseURL,
			SessionID: cfg.Session.ID,
			Agents:    cfg.Session.DefaultAgents,
		}
		sessionID, err := bridge.RunSetup(context.Background(), vars, cfg.Transport.ResolvedHeaders(), nil)
		if err != nil {
			return fmt.Errorf("failed to setup session: %w", err)
		}

		// Update config with actual session ID
		cfg.Session.ID = sessionID

		fmt.Printf("✅ Session ready: %s\n\n", sessionID)
	}

	fmt.Printf("✅ Starting interactive TUI...\n\n")

	// Create interactive TUI model
	model := visualizer.NewInteractiveModel(cfg)

	// Start bubbletea program
	p := tea.NewProgram(model, tea.WithAltScreen())

	// Run the program
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	fmt.Printf("\n\n📊 Session Summary\n")
	fmt.Printf("   Logs saved to: %s\n", cfg.LogDir)

	return nil
}

func runStream(configPath string, message string) error {
	// Load YAML configuration
	cfg, err := config.LoadConfigFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	fmt.Printf("🔧 Configuration loaded\n")
	fmt.Printf("   Backend: %s\n", cfg.Transport.BaseURL)
	fmt.Printf("   Session: %s\n", cfg.Session.ID)
	fmt.Printf("   Log Dir: %s\n\n", cfg.LogDir)

	// Auto-setup session if configured
	if cfg.Session.AutoSetup {
		fmt.Printf("🔄 Auto-setting up session...\n")

		vars := mapping.InterpolationVars{
			BaseURL:   cfg.Transport.BaseURL,
			SessionID: cfg.Session.ID,
			Agents:    cfg.Session.DefaultAgents,
		}
		sessionID, err := bridge.RunSetup(context.Background(), vars, cfg.Transport.ResolvedHeaders(), nil)
		if err != nil {
			return fmt.Errorf("failed to setup session: %w", err)
		}

		// Update config with actual session ID
		cfg.Session.ID = sessionID

		fmt.Printf("✅ Session ready: %s\n\n", sessionID)
	}

	structuredLogger, err := logger.NewStructuredLogger(cfg)
	if err != nil {
		return fmt.Errorf("failed to create logger: %w", err)
	}
	defer func() { _ = structuredLogger.Close() }()

	fmt.Printf("📝 Structured logging initialized\n")
	fmt.Printf("   Event types: %s/by-event-type/\n", cfg.LogDir)
	fmt.Printf("   Agents:      %s/by-agent/\n", cfg.LogDir)
	fmt.Printf("   Session:     %s/by-session/\n", cfg.LogDir)
	fmt.Printf("   API calls:   %s/api-calls/\n\n", cfg.LogDir)

	// Create SSE client
	sseClient := client.NewSSEClient(cfg)

	fmt.Printf("🌐 Connecting to backend...\n")
	fmt.Printf("   Endpoint: %s\n", cfg.StreamEndpointURL())
	fmt.Printf("   Message: \"%s\"\n\n", message)

	// Connect to SSE endpoint
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		fmt.Println("\n\n🛑 Interrupt received, shutting down...")
		cancel()
	}()

	if err := sseClient.Connect(ctx, message); err != nil {
		return fmt.Errorf("failed to connect to SSE endpoint: %w", err)
	}

	fmt.Printf("✅ Connected! Starting TUI...\n\n")

	// Create TUI model
	model := visualizer.NewModel(cfg, sseClient, structuredLogger, message)

	// Start bubbletea program
	p := tea.NewProgram(model, tea.WithAltScreen())

	// Run the program
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	fmt.Printf("\n\n📊 Session Summary\n")
	fmt.Printf("   Logs saved to: %s\n", cfg.LogDir)

	return nil
}

func runReplay(sessionFile string) error {
	fmt.Printf("🔄 Replaying session from: %s\n\n", sessionFile)

	// Load events from log file
	evts, err := loadEventsFromFile(sessionFile)
	if err != nil {
		return fmt.Errorf("failed to load events: %w", err)
	}

	if len(evts) == 0 {
		return fmt.Errorf("no events found in file")
	}

	fmt.Printf("📝 Loaded %d events\n\n", len(evts))

	// Create timeline visualizer and add events
	tv := visualizer.NewTimelineVisualizer()
	for _, evt := range evts {
		tv.AddEvent(evt)
	}

	// Show detailed log for replay
	fmt.Println(tv.RenderDetailedLog())
	fmt.Println()
	fmt.Println(tv.RenderParallelSummary())

	return nil
}

func runTimeline(sessionFile string) error {
	fmt.Printf("📊 Generating timeline from: %s\n\n", sessionFile)

	// Load events from log file
	evts, err := loadEventsFromFile(sessionFile)
	if err != nil {
		return fmt.Errorf("failed to load events: %w", err)
	}

	if len(evts) == 0 {
		return fmt.Errorf("no events found in file")
	}

	fmt.Printf("📝 Loaded %d events\n\n", len(evts))

	// Create timeline visualizer and add events
	tv := visualizer.NewTimelineVisualizer()
	for _, evt := range evts {
		tv.AddEvent(evt)
	}

	// Render timeline (default width 120)
	fmt.Println(tv.RenderTimeline(120))
	fmt.Println()
	fmt.Println(tv.RenderParallelSummary())

	return nil
}

// loadEventsFromFile reads a JSONL session log file through the replay
// transport — the same Transport → Frame → dialect-engine path a live SSE
// stream goes through, so timeline/replay decode events identically to a
// real session rather than through a special-cased file reader.
func loadEventsFromFile(filePath string) ([]*events.Event, error) {
	tr, err := replay.New(filePath, 0)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := tr.Connect(ctx); err != nil {
		return nil, fmt.Errorf("failed to start replay: %w", err)
	}
	defer func() { _ = tr.Close() }()

	parser := bridge.NewParser()
	var result []*events.Event
	for frame := range tr.Frames() {
		if frame.Err != nil {
			fmt.Fprintf(os.Stderr, "Warning: malformed frame: %v\n", frame.Err)
			continue
		}
		evt, _ := parser.Parse(frame.Name, frame.Data)
		result = append(result, evt)
	}

	return result, nil
}
