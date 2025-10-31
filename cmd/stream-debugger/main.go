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
	flag.Parse()

	args := flag.Args()

	// If no command and no config, show usage
	if len(args) == 0 && *configPath == "" {
		return fmt.Errorf("usage: %s [--config FILE] [COMMAND]\n\nCommands:\n  replay <file>     - Replay session from log file\n  timeline <file>   - Show timeline visualization\n\nIf no command is provided, starts interactive TUI mode (requires --config)", os.Args[0])
	}

	// If config provided but no command, run interactive mode
	if *configPath != "" && len(args) == 0 {
		return runInteractive(*configPath)
	}

	// Handle legacy commands
	if len(args) > 0 {
		command := args[0]
		switch command {
		case "stream":
			// Legacy: stream "message" --config file.yaml
			if len(args) < 2 {
				return fmt.Errorf("usage: %s stream <message> --config <file>", os.Args[0])
			}
			message := args[1]
			if *configPath == "" {
				return fmt.Errorf("--config flag is required")
			}
			return runLegacyStream(*configPath, message)
		case "replay":
			if len(args) < 2 {
				return fmt.Errorf("usage: %s replay <session-file>", os.Args[0])
			}
			return runReplay(args[1])
		case "timeline":
			if len(args) < 2 {
				return fmt.Errorf("usage: %s timeline <session-file>", os.Args[0])
			}
			return runTimeline(args[1])
		default:
			return fmt.Errorf("unknown command: %s", command)
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
