package help

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/urfave/cli/v2"
)

// Style definitions matching the TUI palette
var (
	// Header styles
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("6")). // Cyan - matches TUI headers
			MarginTop(1).
			MarginBottom(1)

	sectionStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("6")) // Cyan

	// Content styles
	commandStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("10")). // Green - active/interactive
			Bold(true)

	descriptionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("7")) // Light gray

	flagStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("14")) // Cyan - matches event types

	usageStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("8")) // Gray - secondary text

	warningStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("11")). // Yellow - important
			Bold(true)
)

// CustomHelpPrinter is the styled help printer for the CLI
func CustomHelpPrinter(w io.Writer, templ string, data interface{}) {
	var styled string

	// Apply Lipgloss styling based on data type
	switch v := data.(type) {
	case *cli.App:
		styled = styleAppHelp(v)
	case cli.Command:
		styled = styleCommandHelp(&v)
	case *cli.Command:
		styled = styleCommandHelp(v)
	default:
		// Fallback to default rendering
		cli.HelpPrinterCustom(w, templ, data, nil)
		return
	}

	_, _ = fmt.Fprint(w, styled)
}

// styleAppHelp styles the main app help
func styleAppHelp(app *cli.App) string {
	var b strings.Builder

	// Top banner with name and version
	banner := fmt.Sprintf("%s v%s", strings.ToUpper(app.Name), app.Version)
	b.WriteString(titleStyle.Render(banner))
	b.WriteString("\n\n")

	// Usage
	b.WriteString(sectionStyle.Render("USAGE"))
	b.WriteString("\n  ")
	usageText := fmt.Sprintf("%s [global options] command [command options]", app.Name)
	b.WriteString(usageStyle.Render(usageText))
	b.WriteString("\n\n")

	// Description
	if app.Description != "" {
		b.WriteString(sectionStyle.Render("DESCRIPTION"))
		b.WriteString("\n")
		descLines := strings.Split(strings.TrimSpace(app.Description), "\n")
		for _, line := range descLines {
			b.WriteString("  ")
			b.WriteString(descriptionStyle.Render(strings.TrimSpace(line)))
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}

	// Commands section
	if len(app.VisibleCommands()) > 0 {
		b.WriteString(sectionStyle.Render("COMMANDS"))
		b.WriteString("\n")

		for _, cmd := range app.VisibleCommands() {
			cmdName := commandStyle.Render(fmt.Sprintf("  %-12s", cmd.Name))
			cmdDesc := descriptionStyle.Render(cmd.Usage)
			_, _ = fmt.Fprintf(&b, "%s  %s\n", cmdName, cmdDesc)
		}
		b.WriteString("\n")
	}

	// Global flags
	if len(app.VisibleFlags()) > 0 {
		b.WriteString(sectionStyle.Render("GLOBAL OPTIONS"))
		b.WriteString("\n")

		for _, flag := range app.VisibleFlags() {
			b.WriteString(formatFlag(flag))
		}
		b.WriteString("\n")
	}

	// Footer
	footer := usageStyle.Render(fmt.Sprintf(
		"Run '%s [command] --help' for more information on a command.",
		app.Name,
	))
	b.WriteString(footer)
	b.WriteString("\n")

	return b.String()
}

// styleCommandHelp styles individual command help
func styleCommandHelp(cmd *cli.Command) string {
	var b strings.Builder

	// Command title
	cmdTitle := fmt.Sprintf("%s - %s", strings.ToUpper(cmd.Name), cmd.Usage)
	b.WriteString(titleStyle.Render(cmdTitle))
	b.WriteString("\n\n")

	// Usage pattern
	b.WriteString(sectionStyle.Render("USAGE"))
	b.WriteString("\n  ")

	usagePattern := fmt.Sprintf("agent-stream-dbg %s", cmd.Name)
	if len(cmd.VisibleFlags()) > 0 {
		usagePattern += " [options]"
	}
	if cmd.ArgsUsage != "" {
		usagePattern += " " + cmd.ArgsUsage
	}
	b.WriteString(usageStyle.Render(usagePattern))
	b.WriteString("\n\n")

	// Description
	if cmd.Description != "" {
		b.WriteString(sectionStyle.Render("DESCRIPTION"))
		b.WriteString("\n")

		// Parse description for warnings and examples
		descLines := strings.Split(strings.TrimSpace(cmd.Description), "\n")
		for _, line := range descLines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				b.WriteString("\n")
				continue
			}

			// Highlight warnings
			if strings.Contains(trimmed, "⚠️") || strings.Contains(trimmed, "IMPORTANT") {
				b.WriteString("  ")
				b.WriteString(warningStyle.Render(trimmed))
				b.WriteString("\n")
			} else if strings.HasPrefix(trimmed, "$") {
				// Style example commands
				b.WriteString("    ")
				b.WriteString(flagStyle.Render(trimmed))
				b.WriteString("\n")
			} else if strings.HasPrefix(trimmed, "Examples:") {
				b.WriteString("\n  ")
				b.WriteString(sectionStyle.Render(trimmed))
				b.WriteString("\n")
			} else {
				b.WriteString("  ")
				b.WriteString(descriptionStyle.Render(trimmed))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	// Flags/Options
	if len(cmd.VisibleFlags()) > 0 {
		b.WriteString(sectionStyle.Render("OPTIONS"))
		b.WriteString("\n")

		for _, flag := range cmd.VisibleFlags() {
			b.WriteString(formatFlag(flag))
		}
	}

	return b.String()
}

// formatFlag formats a single flag with Lipgloss styling
func formatFlag(flag cli.Flag) string {
	names := flag.Names()
	if len(names) == 0 {
		return ""
	}

	var nameStr string
	if len(names) == 1 {
		if len(names[0]) == 1 {
			nameStr = fmt.Sprintf("-%s", names[0])
		} else {
			nameStr = fmt.Sprintf("--%s", names[0])
		}
	} else {
		// Show short and long form
		short := ""
		long := ""
		for _, name := range names {
			if len(name) == 1 {
				short = name
			} else {
				long = name
			}
		}
		if short != "" && long != "" {
			nameStr = fmt.Sprintf("-%s, --%s", short, long)
		} else if long != "" {
			nameStr = fmt.Sprintf("--%s", long)
		} else {
			nameStr = fmt.Sprintf("-%s", short)
		}
	}

	// Get flag usage/description
	var usage string
	switch f := flag.(type) {
	case *cli.StringFlag:
		usage = f.Usage
	case *cli.BoolFlag:
		usage = f.Usage
	case *cli.IntFlag:
		usage = f.Usage
	default:
		// Generic fallback
		if docFlag, ok := flag.(interface{ GetUsage() string }); ok {
			usage = docFlag.GetUsage()
		}
	}

	styledName := flagStyle.Render(fmt.Sprintf("  %-24s", nameStr))
	styledUsage := descriptionStyle.Render(usage)

	return fmt.Sprintf("%s  %s\n", styledName, styledUsage)
}
