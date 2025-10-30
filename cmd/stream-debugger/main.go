package main

import (
	"context"
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
	// Parse command line arguments
	if len(os.Args) < 2 {
		return fmt.Errorf("usage: %s <command> [args]\n\nCommands:\n  stream <message>  - Start streaming debugger\n  replay <file>     - Replay session from log file", os.Args[0])
	}

	command := os.Args[1]

	switch command {
	case "stream":
		return runStream()
	case "replay":
		return runReplay()
	default:
		return fmt.Errorf("unknown command: %s", command)
	}
}

func runStream() error {
	// Get message from args
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: %s stream <message>", os.Args[0])
	}
	message := os.Args[2]

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	fmt.Printf("🔧 Configuration loaded\n")
	fmt.Printf("   Backend: %s\n", cfg.BackendURL)
	fmt.Printf("   Session: %s\n", cfg.SessionID)
	fmt.Printf("   Log Dir: %s\n\n", cfg.LogDir)

	// Create structured logger
	structuredLogger, err := logger.NewStructuredLogger(cfg)
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
	sseClient := client.NewSSEClient(cfg)

	fmt.Printf("🌐 Connecting to backend...\n")
	fmt.Printf("   Endpoint: %s\n", cfg.StreamEndpoint())
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
	model := visualizer.NewModel(cfg, sseClient, structuredLogger)

	// Start bubbletea program
	p := tea.NewProgram(model, tea.WithAltScreen())

	// Run the program
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}

	fmt.Printf("\n\n📊 Session Summary\n")
	fmt.Printf("   Duration: %v\n", "N/A") // TODO: Add duration tracking
	fmt.Printf("   Events: %v\n", "N/A")   // TODO: Add event counting
	fmt.Printf("   Logs saved to: %s\n", cfg.LogDir)

	return nil
}

func runReplay() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: %s replay <session-file>", os.Args[0])
	}

	sessionFile := os.Args[2]
	fmt.Printf("🔄 Replay functionality coming soon...\n")
	fmt.Printf("   File: %s\n", sessionFile)

	// TODO: Implement replay logic
	// 1. Read session log file
	// 2. Parse events
	// 3. Replay events through TUI at configurable speed

	return nil
}
