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
	"github.com/lancekrogers/stream-debugger/internal/logger"
	"github.com/lancekrogers/stream-debugger/internal/visualizer"
)

func runInteractiveWithOptions(configPath, dialectOverride string, openConfigPanel bool) error {
	fmt.Printf("🚀 Stream Debugger - Interactive Mode\n\n")
	cfg, err := config.LoadConfigFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	applyDialectOverride(cfg, dialectOverride)
	parser, err := bridge.NewParserFor(cfg.Dialect.File)
	if err != nil {
		return fmt.Errorf("failed to load dialect: %w", err)
	}
	programCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("🔧 Configuration loaded from: %s\n", configPath)
	fmt.Printf("   Backend: %s\n", cfg.Transport.BaseURL)
	fmt.Printf("   Session: %s\n", cfg.Session.ID)
	fmt.Printf("   Log Dir: %s\n\n", cfg.LogDir)
	// A newly-created starter config has no endpoint yet. Defer its setup
	// handshake until the user saves the first-run panel and sends a message.
	// Existing configs keep the historical eager setup behavior.
	if cfg.Session.AutoSetup && !openConfigPanel {
		fmt.Printf("🔄 Auto-setting up session...\n")
		vars := config.InterpolationVarsFromConfig(cfg)
		sessionID, err := parser.RunSetup(programCtx, vars, cfg.Transport.ResolvedHeaders(), nil)
		if err != nil {
			return fmt.Errorf("failed to setup session: %w", err)
		}
		cfg.Session.ID = sessionID
		fmt.Printf("✅ Session ready: %s\n\n", sessionID)
	}

	fmt.Printf("✅ Starting interactive TUI...\n\n")
	model := visualizer.NewInteractiveModelWithContextAndConfigPathAndOpenConfig(cfg, programCtx, configPath, openConfigPanel)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(programCtx))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	fmt.Printf("\n\n📊 Session Summary\n")
	fmt.Printf("   Logs saved to: %s\n", cfg.LogDir)
	return nil
}

func runStream(configPath string, message string, dialectOverride string) error {
	cfg, err := config.LoadConfigFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	applyDialectOverride(cfg, dialectOverride)
	parser, err := bridge.NewParserFor(cfg.Dialect.File)
	if err != nil {
		return fmt.Errorf("failed to load dialect: %w", err)
	}
	transportName := cfg.Transport.Type
	if transportName == "" {
		transportName = "sse"
	}
	fmt.Printf("🔧 Configuration loaded\n")
	fmt.Printf("   Transport: %s\n", transportName)
	fmt.Printf("   Backend: %s\n", streamTarget(cfg))
	fmt.Printf("   Session: %s\n", cfg.Session.ID)
	fmt.Printf("   Log Dir: %s\n\n", cfg.LogDir)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if cfg.Session.AutoSetup {
		fmt.Printf("🔄 Auto-setting up session...\n")
		vars := config.InterpolationVarsFromConfig(cfg)
		sessionID, err := parser.RunSetup(ctx, vars, cfg.Transport.ResolvedHeaders(), nil)
		if err != nil {
			return fmt.Errorf("failed to setup session: %w", err)
		}
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

	streamClient := client.NewClient(cfg)
	defer streamClient.Close()
	fmt.Printf("🌐 Connecting to backend...\n")
	fmt.Printf("   Target: %s\n", streamTarget(cfg))
	fmt.Printf("   Message: \"%s\"\n\n", message)
	if err := streamClient.Connect(ctx, message); err != nil {
		return fmt.Errorf("failed to connect %s transport: %w", transportName, err)
	}
	fmt.Printf("✅ Connected! Starting TUI...\n\n")
	model := visualizer.NewModelWithContext(cfg, streamClient, structuredLogger, message, ctx)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithContext(ctx))
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	fmt.Printf("\n\n📊 Session Summary\n")
	fmt.Printf("   Logs saved to: %s\n", cfg.LogDir)
	return nil
}

// streamTarget returns a human-readable connection target for logging.
func streamTarget(cfg *config.EnhancedConfig) string {
	switch cfg.Transport.Type {
	case "grpc":
		return cfg.Transport.Target
	case "replay":
		return cfg.Transport.BaseURL
	default:
		return cfg.StreamEndpointURL()
	}
}

func runReplay(sessionFile, dialectSource string) error {
	fmt.Printf("🔄 Replaying session from: %s\n\n", sessionFile)
	evts, err := loadEventsFromFile(sessionFile, dialectSource)
	if err != nil {
		return fmt.Errorf("failed to load events: %w", err)
	}
	if len(evts) == 0 {
		return fmt.Errorf("no events found in file")
	}
	fmt.Printf("📝 Loaded %d events\n\n", len(evts))
	tv := visualizer.NewTimelineVisualizer()
	for _, evt := range evts {
		tv.AddEvent(evt)
	}
	fmt.Println(tv.RenderDetailedLog())
	fmt.Println()
	fmt.Println(tv.RenderParallelSummary())
	return nil
}

func runTimeline(sessionFile, dialectSource string) error {
	fmt.Printf("📊 Generating timeline from: %s\n\n", sessionFile)
	evts, err := loadEventsFromFile(sessionFile, dialectSource)
	if err != nil {
		return fmt.Errorf("failed to load events: %w", err)
	}
	if len(evts) == 0 {
		return fmt.Errorf("no events found in file")
	}
	fmt.Printf("📝 Loaded %d events\n\n", len(evts))
	tv := visualizer.NewTimelineVisualizer()
	for _, evt := range evts {
		tv.AddEvent(evt)
	}
	fmt.Println(tv.RenderTimeline(120))
	fmt.Println()
	fmt.Println(tv.RenderParallelSummary())
	return nil
}
