package visualizer

import (
	"fmt"
	"os"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/lancekrogers/stream-debugger/internal/events"
)

// handleKeyMsg handles tea.KeyMsg within Update, extracted verbatim (pure
// reorganization, no behavior change) so Update itself fits this project's
// file-size standard. The bool return is "handled": true means Update should
// return (model, cmd) immediately; false means no binding matched and Update
// should fall through to its own shared tail logic (e.g. forwarding
// unhandled keystrokes to the textarea while in insert mode).
func (m InteractiveModel) handleKeyMsg(msg tea.KeyMsg) (InteractiveModel, tea.Cmd, bool) {
	// Global ctrl bindings
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit, true
	}
	if msg.Type == tea.KeyCtrlT {
		// Toggle view mode (used by Events and Messages panes to switch RAW↔PARSED views)
		if m.viewMode == ViewModeRaw {
			m.viewMode = ViewModeParsed
		} else {
			m.viewMode = ViewModeRaw
		}
		m.contentDirty = true
		m.refreshViewportContent()
		return m, nil, true
	}
	if msg.Type == tea.KeyCtrlN {
		// Start a new session
		return m, m.newSessionCmd(), true
	}
	if msg.Type == tea.KeyCtrlF {
		m.follow = !m.follow
		if m.follow {
			m.viewport.GotoBottom()
		}
		return m, nil, true
	}
	if msg.Type == tea.KeyCtrlS {
		// Save events to file with full expanded content (human-readable)
		if len(m.messages) > 0 {
			lastMsg := m.messages[len(m.messages)-1]
			filename := fmt.Sprintf("events_%s.txt", time.Now().Format("20060102_150405"))

			var output strings.Builder
			output.WriteString("# Stream Debugger Event Export\n")
			output.WriteString(fmt.Sprintf("# Timestamp: %s\n", time.Now().Format(time.RFC3339)))
			output.WriteString(fmt.Sprintf("# Session: %s\n", m.cfg.Session.ID))
			output.WriteString(fmt.Sprintf("# Event Count: %d\n\n", len(lastMsg.Events)))
			output.WriteString(strings.Repeat("=", 80) + "\n\n")

			for i, evt := range lastMsg.Events {
				// Event header
				output.WriteString(fmt.Sprintf("## Event %d: %s\n", i+1, evt.Name))
				output.WriteString(strings.Repeat("-", 40) + "\n")

				// Expanded content (same as TUI shows when expanded)
				expanded := m.renderEventExpandedPlainText(evt, &lastMsg)
				output.WriteString(expanded)
				output.WriteString("\n\n")
			}

			err := os.WriteFile(filename, []byte(output.String()), 0644)
			if err != nil {
				m.saveStatus = fmt.Sprintf("Save failed: %v", err)
			} else {
				m.saveStatus = fmt.Sprintf("Saved to %s", filename)
			}
			m.contentDirty = true
			m.refreshViewportContent()
		}
		return m, nil, true
	}
	if msg.Type == tea.KeyEsc {
		if m.insertMode {
			m.insertMode = false
			m.textarea.Blur()
		}
		return m, nil, true
	}
	if msg.Type == tea.KeyEnter {
		// In insert mode, Enter sends the message. In normal mode, let the
		// key fall through to pane-specific handling (e.g., Flow expand).
		if m.insertMode && !m.streaming {
			message := m.textarea.Value()
			if strings.TrimSpace(message) != "" {
				m.messages = append(m.messages, Message{Text: message, Timestamp: time.Now(), Streaming: true})
				m.flowTurnIndex = len(m.messages) - 1
				m.textarea.Reset()
				m.streaming = true
				m.contentDirty = true
				return m, m.startStreamingCmd(message, len(m.messages)-1), true
			}
			return m, nil, true
		}
		// Not in insert mode: do not return here; handle below.
	}

	// Insert toggle
	if msg.String() == "i" && !m.insertMode {
		m.insertMode = true
		m.textarea.Focus()
		return m, nil, true
	}

	// Pane switching and debug toggles (works in normal mode only)
	if !m.insertMode {
		switch msg.String() {
		case "1", "f1":
			m.activePane = PaneFlow
			m.contentDirty = true
			m.refreshViewportContent()
			return m, nil, true
		case "2", "f2":
			m.activePane = PaneApp
			m.contentDirty = true
			m.refreshViewportContent()
			return m, nil, true
		case "3", "f3":
			m.activePane = PaneTimeline
			m.contentDirty = true
			m.refreshViewportContent()
			return m, nil, true
		case "4", "f4":
			m.activePane = PaneEvents
			m.contentDirty = true
			m.refreshViewportContent()
			return m, nil, true
		case "5", "f5":
			m.activePane = PaneMessages
			m.contentDirty = true
			m.refreshViewportContent()
			return m, nil, true
		// Debug level toggles (F7=verbose, F8=full, F9=off)
		case "f7":
			m.cfg.Debug.Level = "verbose"
			m.contentDirty = true
			m.refreshViewportContent()
			return m, nil, true
		case "f8":
			m.cfg.Debug.Level = "full"
			m.contentDirty = true
			m.refreshViewportContent()
			return m, nil, true
		case "f9":
			m.cfg.Debug.Level = ""
			m.contentDirty = true
			m.refreshViewportContent()
			return m, nil, true
		}
	}

	// Normal-mode navigation only when not inserting
	if !m.insertMode {
		switch msg.Type {
		case tea.KeyUp:
			m.viewport.ScrollUp(1)
			m.follow = false
			return m, nil, true
		case tea.KeyDown:
			m.viewport.ScrollDown(1)
			m.follow = false
			return m, nil, true
		case tea.KeyPgUp:
			m.viewport.HalfPageUp()
			m.follow = false
			return m, nil, true
		case tea.KeyPgDown:
			m.viewport.HalfPageDown()
			m.follow = false
			return m, nil, true
		case tea.KeyHome:
			m.viewport.GotoTop()
			m.follow = false
			return m, nil, true
		case tea.KeyEnd:
			m.viewport.GotoBottom()
			m.follow = true
			return m, nil, true
		case tea.KeyLeft:
			if m.xOffset > 0 {
				m.xOffset--
				m.contentDirty = true
			}
			return m, nil, true
		case tea.KeyRight:
			m.xOffset++
			m.contentDirty = true
			return m, nil, true
		}
		switch msg.String() {
		case "j":
			// Flow pane: move selection down; Events pane: navigate events; others: scroll viewport
			if m.activePane == PaneFlow {
				if m.selectedStepIndex < 5 { // 6 steps: 0-5
					m.selectedStepIndex++
					m.contentDirty = true
					m.refreshViewportContent()
				}
			} else if m.activePane == PaneEvents {
				// Navigate down in visible events list
				visibleCount := m.countVisibleEvents()
				if m.selectedEventIdx < visibleCount-1 {
					m.selectedEventIdx++
					m.contentDirty = true
					m.refreshViewportContent()
					// Scroll viewport down to keep selection visible
					m.viewport.ScrollDown(3)
				}
			} else {
				m.viewport.ScrollDown(1)
				m.follow = false
			}
			return m, nil, true
		case "k":
			// Flow pane: move selection up; Events pane: navigate events; others: scroll viewport
			if m.activePane == PaneFlow {
				if m.selectedStepIndex > 0 {
					m.selectedStepIndex--
					m.contentDirty = true
					m.refreshViewportContent()
				}
			} else if m.activePane == PaneEvents {
				// Navigate up in visible events list
				if m.selectedEventIdx > 0 {
					m.selectedEventIdx--
					m.contentDirty = true
					m.refreshViewportContent()
					// Scroll viewport up to keep selection visible
					m.viewport.ScrollUp(3)
				}
			} else {
				m.viewport.ScrollUp(1)
				m.follow = false
			}
			return m, nil, true
		case "[":
			// Flow pane: previous turn
			if m.activePane == PaneFlow {
				if m.flowTurnIndex > 0 {
					m.flowTurnIndex--
					m.contentDirty = true
					m.refreshViewportContent()
				}
			}
			return m, nil, true
		case "]":
			// Flow pane: next turn
			if m.activePane == PaneFlow {
				if m.flowTurnIndex < len(m.messages)-1 {
					m.flowTurnIndex++
					m.contentDirty = true
					m.refreshViewportContent()
				}
			}
			return m, nil, true
		case "enter":
			// Flow pane: toggle expand/collapse
			if m.activePane == PaneFlow {
				steps := m.dialectFlow.stages()
				if m.selectedStepIndex < len(steps) {
					step := steps[m.selectedStepIndex]
					m.flowExpanded[step] = !m.flowExpanded[step]
					m.contentDirty = true
					m.refreshViewportContent()
				}
			} else if m.activePane == PaneEvents {
				// Events pane: toggle expand/collapse for selected event
				m.eventExpanded[m.selectedEventIdx] = !m.eventExpanded[m.selectedEventIdx]
				m.contentDirty = true
				m.refreshViewportContent()
			}
			return m, nil, true
		case "o":
			// Flow pane: open in App pane with focus on selected step's content
			if m.activePane == PaneFlow {
				steps := m.dialectFlow.stages()
				if m.selectedStepIndex < len(steps) {
					step := steps[m.selectedStepIndex]
					switch {
					case m.dialectFlow.stageRole(step) == events.RoleAggregator:
						m.appFocus = AppFocusAggregator
					default:
						m.appFocus = AppFocusAgents
					}
					m.activePane = PaneApp
					m.contentDirty = true
					m.refreshViewportContent()
				}
			}
			return m, nil, true
		case "f":
			// App pane: toggle focus between Agents and Aggregator
			if m.activePane == PaneApp {
				if m.appFocus == AppFocusAgents {
					m.appFocus = AppFocusAggregator
				} else {
					m.appFocus = AppFocusAgents
				}
				m.contentDirty = true
				m.refreshViewportContent()
			}
			return m, nil, true
		case "p":
			// Flow pane: toggle prompt ref display
			if m.activePane == PaneFlow {
				m.showPromptRef = !m.showPromptRef
				m.contentDirty = true
				m.refreshViewportContent()
			}
			return m, nil, true
		case "a":
			// Flow pane: toggle continuous view (all turns)
			if m.activePane == PaneFlow {
				m.flowContinuous = !m.flowContinuous
				m.contentDirty = true
				m.refreshViewportContent()
			}
			return m, nil, true
		case "t":
			// Events pane: toggle token visibility
			if m.activePane == PaneEvents {
				m.showTokens = !m.showTokens
				m.contentDirty = true
				m.refreshViewportContent()
			}
			return m, nil, true
		case "W":
			// Events pane: toggle aggregator-only filter
			if m.activePane == PaneEvents {
				m.eventsAggregatorOnly = !m.eventsAggregatorOnly
				m.contentDirty = true
				m.refreshViewportContent()
			}
			return m, nil, true
		case "g":
			m.viewport.GotoTop()
			m.follow = false
			return m, nil, true
		case "G":
			m.viewport.GotoBottom()
			m.follow = true
			return m, nil, true
		case " ":
			m.viewport.HalfPageDown()
			m.follow = false
			return m, nil, true
		case "b":
			m.viewport.HalfPageUp()
			m.follow = false
			return m, nil, true
		case "H":
			m.showHistory = !m.showHistory
			m.contentDirty = true
			if !m.showHistory {
				m.follow = true
				m.viewport.GotoBottom()
			}
			return m, nil, true
		case "w":
			m.wrap = !m.wrap
			m.contentDirty = true
			if m.wrap {
				m.xOffset = 0
			}
			return m, nil, true
		case "h":
			if m.xOffset > 0 {
				m.xOffset--
				m.contentDirty = true
			}
			return m, nil, true
		case "l":
			m.xOffset++
			m.contentDirty = true
			return m, nil, true
		}
	}

	return m, nil, false
}
