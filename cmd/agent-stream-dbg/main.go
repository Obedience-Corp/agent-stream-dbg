package main

import (
	"fmt"
	neturl "net/url"
	"os"
	"strings"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/help"
	"github.com/Obedience-Corp/agent-stream-dbg/internal/mapping"
	"github.com/urfave/cli/v2"
)

func main() {
	cli.HelpPrinter = help.CustomHelpPrinter

	app := &cli.App{
		Name:  "agent-stream-dbg",
		Usage: "TUI debugger for multi-agent event streams (SSE, gRPC, ACP, replay)",
		Description: `Terminal debugger for multi-agent backends you're building.

   Point it at a live endpoint or a recorded fixture, watch agents in a multi-pane
   TUI, and decode frames with YAML dialects (what the bytes mean).

   Transports: SSE, gRPC, ACP (stdio), and replay (JSONL fixtures).

   Running without --config opens the home hub: select, create, or edit run
   configs, try offline demos, then open an interactive session.

   Pass --config / -c to jump straight into interactive mode.

   Press Ctrl+T in interactive mode to toggle between RAW (wire) and PARSED
   (agent-organized) views.`,
		Version: "1.0.0",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Usage: "Path to YAML configuration file (optional for interactive mode)"},
			&cli.StringFlag{Name: "dialect", Usage: "Embedded dialect name or path to a dialect YAML file (overrides config)"},
			&cli.BoolFlag{Name: "otel-propagate", Usage: "Inject W3C traceparent on outbound connect/send (generates a root if none inbound)"},
			&cli.StringFlag{Name: "otel-traceparent", Usage: "Force session W3C traceparent (00-traceid-spanid-flags)"},
		},
		Commands: []*cli.Command{
			{
				Name: "stream", Usage: "Send single message and exit (CI/CD mode)", ArgsUsage: "<message>",
				Description: `Sends a single message to the configured backend and displays the streaming response.
   Useful for CI/CD pipelines, scripting, and quick one-off tests.

   ⚠️  IMPORTANT: Flags must come BEFORE the message argument!

   Examples:
     $ agent-stream-dbg stream --config config.yaml "Hello"
     $ agent-stream-dbg stream -c my-api.yaml "What is consciousness?"`,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Usage: "Path to YAML configuration file", Required: true},
					&cli.StringFlag{Name: "dialect", Usage: "Embedded dialect name or path to a dialect YAML file (overrides config)"},
					&cli.BoolFlag{Name: "otel-propagate", Usage: "Inject W3C traceparent on outbound connect/send"},
					&cli.StringFlag{Name: "otel-traceparent", Usage: "Force session W3C traceparent"},
				}, Action: streamAction,
			},
			{
				Name: "replay", Usage: "Replay session from log file", ArgsUsage: "<session-file>",
				Description: `Replays a recorded session from log files.

   Example:
     $ agent-stream-dbg replay logs/by-session/session_*.jsonl`,
				Flags: []cli.Flag{&cli.StringFlag{Name: "dialect", Usage: "Embedded dialect name or path to a dialect YAML file"}}, Action: replayAction,
			},
			{
				Name: "timeline", Usage: "Visualize agent execution timeline", ArgsUsage: "<session-file>",
				Description: `Generates a visual timeline showing which agents ran in parallel,
   execution durations, and performance metrics.

   Example:
     $ agent-stream-dbg timeline logs/by-session/session_*.jsonl`,
				Flags: []cli.Flag{&cli.StringFlag{Name: "dialect", Usage: "Embedded dialect name or path to a dialect YAML file"}}, Action: timelineAction,
			},
			{
				Name: "init", Usage: "Infer a commented draft dialect from a live stream or recording",
				Description: `Observes frames — live over SSE or from a recorded JSONL fixture — and
   writes a commented, editable draft dialect YAML to stdout. A draft is a
   starting point, not a finished dialect: review every UNCLASSIFIED rule
   and guessed field before using it.

   Examples:
     $ agent-stream-dbg init --url http://localhost:8080/stream --send "hi" > my.yaml
     $ agent-stream-dbg init --from logs/by-session/session.jsonl > my.yaml`,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "url", Usage: "SSE endpoint to sample live (mutually exclusive with --from)"},
					&cli.StringFlag{Name: "send", Usage: "Message to send when sampling --url (sent as a ?message= query param)"},
					&cli.StringFlag{Name: "from", Usage: "JSONL fixture to sample from (mutually exclusive with --url)"},
				}, Action: initAction,
			},
			{
				Name: "explain", Usage: "Trace every frame through a dialect: which rule matched, what was extracted, why not",
				Description: `Debug your own config: for every frame, shows which rule matched (or a
   hint for why none did) and, per matched rule, each extracted field's
   declared path and resolved value — flagging empty extractions, which
   usually mean the path is wrong.

   Exit code reflects health: 0 if every frame matched with no empty
   extractions, 1 otherwise — usable as a CI check for a shipped dialect.

   Example:
     $ agent-stream-dbg explain --dialect dialects/reference.yaml --from testdata/fixtures/session.jsonl`,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "dialect", Required: true, Usage: "Path to the dialect YAML file"},
					&cli.StringFlag{Name: "from", Usage: "JSONL fixture to trace (mutually exclusive with --url)"},
					&cli.StringFlag{Name: "url", Usage: "SSE endpoint to trace live (mutually exclusive with --from)"},
					&cli.StringFlag{Name: "send", Usage: "Message to send when tracing --url"},
				}, Action: explainAction,
			},
		},
		Action: defaultAction,
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func defaultAction(c *cli.Context) error {
	// Explicit -c: power-user / demo / CI path straight into interactive.
	if strings.TrimSpace(c.String("config")) != "" {
		configPath, err := resolveExplicitConfig(c.String("config"))
		if err != nil {
			return err
		}
		return runInteractiveWithOptions(configPath, c.String("dialect"), false, correlationFromCLI(c))
	}
	// Bare launch: home hub (list / create / offline demos / open session).
	return runHome(c.String("dialect"))
}

func streamAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("message argument is required")
	}
	return runStream(c.String("config"), c.Args().Get(0), c.String("dialect"), correlationFromCLI(c))
}

// correlationFlags carries global OTel correlation CLI/env options applied
// after YAML load (not stored in run configs for v1).
type correlationFlags struct {
	propagate   bool
	traceparent string
}

func correlationFromCLI(c *cli.Context) correlationFlags {
	f := correlationFlags{
		propagate:   c.Bool("otel-propagate"),
		traceparent: strings.TrimSpace(c.String("otel-traceparent")),
	}
	// Env fallbacks when flags unset (standard-ish for local tooling).
	if !f.propagate {
		if v := strings.TrimSpace(os.Getenv("OTEL_PROPAGATE")); v == "1" || strings.EqualFold(v, "true") {
			f.propagate = true
		}
	}
	if f.traceparent == "" {
		f.traceparent = strings.TrimSpace(os.Getenv("TRACEPARENT"))
	}
	return f
}

func replayAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("session file argument is required")
	}
	return runReplay(c.Args().Get(0), c.String("dialect"))
}

func timelineAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("session file argument is required")
	}
	return runTimeline(c.Args().Get(0), c.String("dialect"))
}

func initAction(c *cli.Context) error {
	url := c.String("url")
	from := c.String("from")
	send := c.String("send")
	if url == "" && from == "" {
		return fmt.Errorf("one of --url or --from is required")
	}
	if url != "" && from != "" {
		return fmt.Errorf("--url and --from are mutually exclusive")
	}
	if from != "" {
		return runInit(from, from)
	}
	target := url
	if send != "" {
		target = url + "?message=" + neturl.QueryEscape(send)
	}
	return runInitLive(target, url)
}

func explainAction(c *cli.Context) error {
	dialectPath := c.String("dialect")
	url := c.String("url")
	from := c.String("from")
	send := c.String("send")
	if url == "" && from == "" {
		return fmt.Errorf("one of --url or --from is required")
	}
	if url != "" && from != "" {
		return fmt.Errorf("--url and --from are mutually exclusive")
	}
	engine, err := mapping.LoadFile(dialectPath)
	if err != nil {
		return fmt.Errorf("failed to load dialect: %w", err)
	}
	if from != "" {
		return runExplain(engine, from)
	}
	target := url
	if send != "" {
		target = url + "?message=" + neturl.QueryEscape(send)
	}
	return runExplainLive(engine, target)
}
