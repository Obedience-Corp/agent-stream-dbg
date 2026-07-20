package main

import (
	"fmt"
	neturl "net/url"
	"os"

	"github.com/Obedience-Corp/stream-debugger/internal/help"
	"github.com/Obedience-Corp/stream-debugger/internal/mapping"
	"github.com/urfave/cli/v2"
)

func main() {
	cli.HelpPrinter = help.CustomHelpPrinter

	app := &cli.App{
		Name:  "stream-debugger",
		Usage: "Debug SSE streaming responses from multi-agent systems",
		Description: `A configuration-driven CLI tool for visualizing and debugging Server-Sent Events (SSE) streaming.

   Supports two primary modes:
   1. Interactive Mode: Multi-turn chat with real-time visualization (default)
   2. Stream Mode: Single message for CI/CD and scripting

   Running without --config starts first-run setup in the TUI and creates a
   starter config when no existing config can be found.

   Press Ctrl+T in interactive mode to toggle between RAW (SSE) and PARSED (agent-organized) views.`,
		Version: "1.0.0",
		Flags: []cli.Flag{
			&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Usage: "Path to YAML configuration file (optional for interactive mode)"},
			&cli.StringFlag{Name: "dialect", Usage: "Embedded dialect name or path to a dialect YAML file (overrides config)"},
		},
		Commands: []*cli.Command{
			{
				Name: "stream", Usage: "Send single message and exit (CI/CD mode)", ArgsUsage: "<message>",
				Description: `Sends a single message to the configured backend and displays the streaming response.
   Useful for CI/CD pipelines, scripting, and quick one-off tests.

   ⚠️  IMPORTANT: Flags must come BEFORE the message argument!

   Examples:
     $ stream-debugger stream --config config.yaml "Hello"
     $ stream-debugger stream -c my-api.yaml "What is consciousness?"`,
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Usage: "Path to YAML configuration file", Required: true},
					&cli.StringFlag{Name: "dialect", Usage: "Embedded dialect name or path to a dialect YAML file (overrides config)"},
				}, Action: streamAction,
			},
			{
				Name: "replay", Usage: "Replay session from log file", ArgsUsage: "<session-file>",
				Description: `Replays a recorded session from log files.

   Example:
     $ stream-debugger replay logs/by-session/session_*.jsonl`,
				Flags: []cli.Flag{&cli.StringFlag{Name: "dialect", Usage: "Embedded dialect name or path to a dialect YAML file"}}, Action: replayAction,
			},
			{
				Name: "timeline", Usage: "Visualize agent execution timeline", ArgsUsage: "<session-file>",
				Description: `Generates a visual timeline showing which agents ran in parallel,
   execution durations, and performance metrics.

   Example:
     $ stream-debugger timeline logs/by-session/session_*.jsonl`,
				Flags: []cli.Flag{&cli.StringFlag{Name: "dialect", Usage: "Embedded dialect name or path to a dialect YAML file"}}, Action: timelineAction,
			},
			{
				Name: "init", Usage: "Infer a commented draft dialect from a live stream or recording",
				Description: `Observes frames — live over SSE or from a recorded JSONL fixture — and
   writes a commented, editable draft dialect YAML to stdout. A draft is a
   starting point, not a finished dialect: review every UNCLASSIFIED rule
   and guessed field before using it.

   Examples:
     $ stream-debugger init --url http://localhost:8080/stream --send "hi" > my.yaml
     $ stream-debugger init --from logs/by-session/session.jsonl > my.yaml`,
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
     $ stream-debugger explain --dialect dialects/reference.yaml --from testdata/fixtures/session.jsonl`,
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
	configPath, created, err := resolveInteractiveConfig(c.String("config"))
	if err != nil {
		return err
	}
	if created {
		fmt.Printf("No config found. Created a starter config at: %s\n", configPath)
		fmt.Println("Opening the setup panel. Fill in your backend details, then press Ctrl+S to save.")
	}
	return runInteractiveWithOptions(configPath, c.String("dialect"), created)
}

func streamAction(c *cli.Context) error {
	if c.NArg() < 1 {
		return fmt.Errorf("message argument is required")
	}
	return runStream(c.String("config"), c.Args().Get(0), c.String("dialect"))
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
