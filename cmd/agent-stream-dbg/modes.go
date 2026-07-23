package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/bridge"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/client"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/config"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/logger"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/tracectx"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/visualizer"
)

func runInteractiveWithOptions(configPath, dialectOverride string, openConfigPanel bool, corrFlags ...correlationFlags) error {
	fmt.Printf("🚀 agent-stream-dbg - Interactive Mode\n\n")
	cfg, err := config.LoadConfigFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	applyDialectOverride(cfg, dialectOverride)
	if len(corrFlags) > 0 {
		applyCorrelationFlags(cfg, corrFlags[0])
	}
	// Fail fast on invalid forced traceparent before any network I/O.
	corr, err := client.NewCorrelatorFromConfig(cfg)
	if err != nil {
		return err
	}
	parser, err := bridge.NewParserFor(cfg.Dialect.File)
	if err != nil {
		return fmt.Errorf("failed to load dialect: %w", err)
	}
	programCtx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	fmt.Printf("🔧 Configuration loaded from: %s\n", configPath)
	fmt.Printf("   Backend: %s\n", streamTarget(cfg))
	fmt.Printf("   Session: %s\n", cfg.Session.ID)
	fmt.Printf("   Log Dir: %s\n\n", cfg.LogDir)

	// Missing auth env (e.g. API_KEY): fail with .env setup instructions unless
	// we are opening the config panel explicitly (e.g. home hub "e" to edit).
	// Secrets live in the environment, not the YAML panel.
	if !openConfigPanel {
		if err := cfg.ValidateAuth(); err != nil {
			return err
		}
	}

	// A newly-created starter config has no endpoint yet. Defer its setup
	// handshake until the user saves the first-run panel and sends a message.
	// Existing configs keep the historical eager setup behavior.
	if cfg.Session.AutoSetup && !openConfigPanel {
		fmt.Printf("🔄 Auto-setting up session...\n")
		vars := config.InterpolationVarsFromConfig(cfg)
		headers := cfg.Transport.ResolvedHeaders()
		if corr != nil {
			corr.InjectHTTPHeaders(headers)
		}
		sessionID, err := parser.RunSetup(programCtx, vars, headers, nil)
		if err != nil {
			return fmt.Errorf("failed to setup session: %w", err)
		}
		cfg.Session.ID = sessionID
		fmt.Printf("✅ Session ready: %s\n\n", sessionID)
	}
	// Share TraceID with the interactive model correlator (new instance).
	seedCorrelationFromCorrelator(cfg, corr)

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

func runStream(configPath string, message string, dialectOverride string, corrFlags ...correlationFlags) error {
	cfg, err := config.LoadConfigFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	applyDialectOverride(cfg, dialectOverride)
	if len(corrFlags) > 0 {
		applyCorrelationFlags(cfg, corrFlags[0])
	}
	corr, err := client.NewCorrelatorFromConfig(cfg)
	if err != nil {
		return err
	}
	parser, err := bridge.NewParserFor(cfg.Dialect.File)
	if err != nil {
		return fmt.Errorf("failed to load dialect: %w", err)
	}
	// Stream mode is non-interactive: fail fast if required secrets are missing.
	if err := cfg.ValidateAuth(); err != nil {
		return err
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
		headers := cfg.Transport.ResolvedHeaders()
		if corr != nil {
			corr.InjectHTTPHeaders(headers)
		}
		sessionID, err := parser.RunSetup(ctx, vars, headers, nil)
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
	if corr != nil {
		structuredLogger.SetSessionTrace(corr.Current())
	}
	fmt.Printf("📝 Structured logging initialized\n")
	fmt.Printf("   Event types: %s/by-event-type/\n", cfg.LogDir)
	fmt.Printf("   Agents:      %s/by-agent/\n", cfg.LogDir)
	fmt.Printf("   Session:     %s/by-session/\n", cfg.LogDir)
	fmt.Printf("   API calls:   %s/api-calls/\n\n", cfg.LogDir)

	// Share the same correlator instance used for setup inject.
	streamClient := client.NewClientWithCorrelator(cfg, corr)
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

func applyCorrelationFlags(cfg *config.EnhancedConfig, f correlationFlags) {
	if cfg == nil {
		return
	}
	cfg.Correlation.Observe = true
	if f.propagate {
		cfg.Correlation.Propagate = true
		cfg.Correlation.Generate = true
	}
	if f.traceparent != "" {
		cfg.Correlation.Traceparent = f.traceparent
	}
}

// seedCorrelationFromCorrelator pins cfg.Correlation.Traceparent to the
// session TraceID already chosen by corr (force or generate) so a second
// correlator built from cfg joins the same trace.
func seedCorrelationFromCorrelator(cfg *config.EnhancedConfig, corr *tracectx.Correlator) {
	if cfg == nil || corr == nil {
		return
	}
	cur := corr.Current()
	if !cur.Valid() {
		return
	}
	if strings.TrimSpace(cfg.Correlation.Traceparent) != "" {
		return
	}
	span := cur.RemoteSpan
	if span == "" {
		span = cur.SpanID
	}
	if span == "" {
		return
	}
	flags := cur.Flags
	if flags == "" {
		flags = "01"
	}
	cfg.Correlation.Traceparent = "00-" + cur.TraceID + "-" + span + "-" + flags
}

// streamTarget returns a human-readable connection target for logging.
func streamTarget(cfg *config.EnhancedConfig) string {
	switch cfg.Transport.Type {
	case "grpc":
		return cfg.Transport.Target
	case "replay":
		return cfg.Transport.BaseURL
	case "acp":
		if len(cfg.Transport.Args) == 0 {
			return cfg.Transport.Command
		}
		return cfg.Transport.Command + " " + strings.Join(cfg.Transport.Args, " ")
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
