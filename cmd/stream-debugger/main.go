package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/client"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/events"
	"github.com/lancekrogers/stream-debugger/internal/help"
	"github.com/lancekrogers/stream-debugger/internal/logger"
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
	return runLegacyStream(configPath, message)
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
	fmt.Printf("   Backend: %s\n", cfg.Backend.BaseURL)
	fmt.Printf("   Session: %s\n", cfg.Session.ID)
	fmt.Printf("   Log Dir: %s\n\n", cfg.LogDir)

	// Auto-setup session if configured
	if cfg.Session.AutoSetup {
		fmt.Printf("🔄 Auto-setting up session...\n")

		setupClient := client.NewSessionSetupClient(cfg, cfg.APIKey)
		sessionResp, err := setupClient.CreateOrGetSession()
		if err != nil {
			return fmt.Errorf("failed to setup session: %w", err)
		}

		// Update config with actual session ID
		cfg.Session.ID = sessionResp.SessionID

		if sessionResp.Created {
			fmt.Printf("✅ Session created: %s\n", sessionResp.SessionID)
		} else {
			fmt.Printf("✅ Using existing session: %s\n", sessionResp.SessionID)
		}
		fmt.Printf("   Active agents: %v\n\n", sessionResp.ActiveAgents)
	}

	fmt.Printf("✅ Starting interactive TUI...\n\n")

	// Create interactive TUI model
	model := visualizer.NewInteractiveModel(cfg, cfg.APIKey)

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

func runLegacyStream(configPath string, message string) error {
	// Load YAML configuration
	cfg, err := config.LoadConfigFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	fmt.Printf("🔧 Configuration loaded\n")
	fmt.Printf("   Backend: %s\n", cfg.Backend.BaseURL)
	fmt.Printf("   Session: %s\n", cfg.Session.ID)
	fmt.Printf("   Log Dir: %s\n\n", cfg.LogDir)

	// Auto-setup session if configured
	if cfg.Session.AutoSetup {
		fmt.Printf("🔄 Auto-setting up session...\n")

		setupClient := client.NewSessionSetupClient(cfg, cfg.APIKey)
		sessionResp, err := setupClient.CreateOrGetSession()
		if err != nil {
			return fmt.Errorf("failed to setup session: %w", err)
		}

		// Update config with actual session ID
		cfg.Session.ID = sessionResp.SessionID

		if sessionResp.Created {
			fmt.Printf("✅ Session created: %s\n", sessionResp.SessionID)
		} else {
			fmt.Printf("✅ Using existing session: %s\n", sessionResp.SessionID)
		}
		fmt.Printf("   Active agents: %v\n\n", sessionResp.ActiveAgents)
	}

	// Create structured logger (using old config format for compatibility)
	oldCfg := &config.Config{
		BackendURL:       cfg.Backend.BaseURL,
		APIKey:           cfg.APIKey,
		SessionID:        cfg.Session.ID,
		LogDir:           cfg.LogDir,
		EnableColors:     cfg.EnableColors,
		MaxAgentsVisible: cfg.MaxAgentsVisible,
	}

	structuredLogger, err := logger.NewStructuredLogger(oldCfg)
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
	sseClient := client.NewSSEClient(oldCfg)

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
    model := visualizer.NewModel(oldCfg, sseClient, structuredLogger, message)

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

// loadEventsFromFile reads a JSONL session log file and parses events
func loadEventsFromFile(filePath string) ([]*events.Event, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	parser := events.NewParser()
	var result []*events.Event

	// Read file line by line (JSONL format)
	scanner := bufio.NewScanner(file)
	// Increase buffer size for long lines
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Parse the event using ParseRaw (extracts type from JSON)
		evt, err := parser.ParseRaw(line)
		if err != nil {
			// Log warning but continue - some lines might be metadata
			fmt.Fprintf(os.Stderr, "Warning: skipping line %d: %v\n", lineNum, err)
			continue
		}

		result = append(result, evt)
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return result, nil
}
