package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/client"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/logger"
	"github.com/lancekrogers/stream-debugger/internal/visualizer"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	// Define flags
	configPath := flag.String("config", "", "Path to configuration file (YAML)")
	helpFlag := flag.Bool("help", false, "Show help message")
	flag.BoolVar(helpFlag, "h", false, "Show help message (shorthand)")

	flag.Parse()

	args := flag.Args()

	// Handle help flag
	if *helpFlag {
		printHelp()
		return nil
	}

	// Handle help command
	if len(args) > 0 && args[0] == "help" {
		if len(args) > 1 {
			printCommandHelp(args[1])
		} else {
			printHelp()
		}
		return nil
	}

	// If no command and no config, show usage
	if len(args) == 0 && *configPath == "" {
		printHelp()
		return fmt.Errorf("\nError: No command or config provided")
	}

	// If config provided but no command, run interactive mode
	if *configPath != "" && len(args) == 0 {
		return runInteractive(*configPath)
	}

	// Handle commands
	if len(args) > 0 {
		command := args[0]
		switch command {
		case "stream":
			// stream "message" --config file.yaml
			if len(args) < 2 {
				printCommandHelp("stream")
				return fmt.Errorf("\nError: Missing message argument")
			}
			message := args[1]
			if *configPath == "" {
				printCommandHelp("stream")
				return fmt.Errorf("\nError: --config flag is required")
			}
			return runLegacyStream(*configPath, message)
		case "replay":
			if len(args) < 2 {
				printCommandHelp("replay")
				return fmt.Errorf("\nError: Missing session file argument")
			}
			return runReplay(args[1])
		case "timeline":
			if len(args) < 2 {
				printCommandHelp("timeline")
				return fmt.Errorf("\nError: Missing session file argument")
			}
			return runTimeline(args[1])
		default:
			printHelp()
			return fmt.Errorf("\nError: Unknown command: %s", command)
		}
	}

	return nil
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
	defer structuredLogger.Close()

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
	model := visualizer.NewModel(oldCfg, sseClient, structuredLogger)

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

func printHelp() {
	fmt.Printf(`stream-debugger - Debug SSE streaming responses from multi-agent systems

USAGE:
  stream-debugger [OPTIONS] [COMMAND]

MODES:
  Interactive (default):
    stream-debugger --config FILE
      Multi-turn chat interface with real-time visualization
      Press Ctrl+T to toggle views, Ctrl+C to quit

  stream <message> --config FILE
      Send single message and exit (CI/CD mode)
      ⚠️  Message MUST come before --config flag

COMMANDS:
  stream <message>   Send single message (CI/CD, scripting)
  replay <file>      Replay session from log file (coming soon)
  timeline <file>    Visualize agent execution timeline (coming soon)
  help [command]     Show help for command

OPTIONS:
  --config FILE      Path to YAML configuration file
  -h, --help         Show this help message

EXAMPLES:
  # Interactive mode (recommended for development)
  $ stream-debugger --config config.yaml

  # Stream mode - note message comes BEFORE --config!
  $ stream-debugger stream "What is consciousness?" --config config.yaml

  # Timeline analysis
  $ stream-debugger timeline logs/by-session/session_*.jsonl

  # Get help for a specific command
  $ stream-debugger help stream

KEYBOARD CONTROLS (Interactive Mode):
  Type & Enter       Send message
  Ctrl+T             Toggle RAW (SSE) ↔ PARSED (agent responses)
  ↑ ↓ PgUp PgDn      Scroll through content
  Home / End         Jump to top/bottom
  Ctrl+C             Quit

MORE INFO:
  Documentation: USAGE.md, QUICK_REFERENCE.md
  GitHub: https://github.com/lancekrogers/stream-debugger
`)
}

func printCommandHelp(command string) {
	switch command {
	case "stream":
		fmt.Printf(`stream-debugger stream - Send single message (CI/CD mode)

USAGE:
  stream-debugger stream <message> --config FILE

DESCRIPTION:
  Sends a single message to the configured backend and displays the
  streaming response. Useful for CI/CD pipelines, scripting, and
  quick one-off tests. Exits automatically when response completes.

  ⚠️  IMPORTANT: The message must come BEFORE the --config flag!

EXAMPLES:
  # Correct syntax
  $ stream-debugger stream "Hello" --config config.yaml
  $ stream-debugger stream "What is consciousness?" --config my-api.yaml

  # ❌ WRONG - will fail
  $ stream-debugger stream --config config.yaml "Hello"

OPTIONS:
  --config FILE      Path to YAML configuration file (required)

USE CASES:
  - CI/CD testing: Test streaming responses in automated pipelines
  - Scripting: Batch process multiple messages
  - Quick tests: One-off message without interactive mode

SEE ALSO:
  Interactive mode for multi-turn conversations:
    $ stream-debugger --config config.yaml
`)

	case "replay":
		fmt.Printf(`stream-debugger replay - Replay session from logs

USAGE:
  stream-debugger replay <session-file>

DESCRIPTION:
  Replays a recorded session from log files. (Coming soon)

EXAMPLE:
  $ stream-debugger replay logs/by-session/session_*.jsonl
`)

	case "timeline":
		fmt.Printf(`stream-debugger timeline - Visualize execution timeline

USAGE:
  stream-debugger timeline <session-file>

DESCRIPTION:
  Generates a visual timeline showing which agents ran in parallel,
  execution durations, and performance metrics. (Coming soon)

EXAMPLE:
  $ stream-debugger timeline logs/by-session/session_*.jsonl
`)

	default:
		fmt.Printf("Unknown command: %s\n\n", command)
		printHelp()
	}
}

func runReplay(sessionFile string) error {
	fmt.Printf("🔄 Replay functionality coming soon...\n")
	fmt.Printf("   File: %s\n", sessionFile)

	// TODO: Implement replay logic
	return nil
}

func runTimeline(sessionFile string) error {
	fmt.Printf("📊 Timeline visualization coming soon...\n")
	fmt.Printf("   File: %s\n", sessionFile)

	// TODO: Implement timeline logic
	return nil
}
