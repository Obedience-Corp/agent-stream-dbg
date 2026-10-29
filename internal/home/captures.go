package home

import (
	"fmt"
	"time"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/captures"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type captureBrowser struct {
	root          string
	listing       captures.Listing
	cursor        int
	selected      string
	loading       bool
	generation    int
	err           string
	preferenceErr string
	folder        textinput.Model
}

type capturesLoaded struct {
	root       string
	generation int
	listing    captures.Listing
	err        error
}
type captureRefresh struct{ generation int }

func newCaptureBrowser(opts Options) captureBrowser {
	root, err := captures.LoadRoot(opts.UserConfigDir)
	b := captureBrowser{root: root, selected: opts.CapturePath, folder: textinput.New()}
	if err != nil {
		b.preferenceErr = err.Error()
		b.root, _ = captures.DefaultRoot()
	}
	b.folder.Prompt = ""
	b.folder.CharLimit = 4096
	b.folder.Width = 60
	return b
}

func (m model) startCaptures() (tea.Model, tea.Cmd) {
	m.screen = screenCaptures
	m.notice = ""
	m.status = ""
	m.captures.generation++
	m.captures.loading = false
	cmd := m.loadCaptures()
	return m, cmd
}

func (m *model) loadCaptures() tea.Cmd {
	if m.captures.loading {
		return nil
	}
	m.captures.loading = true
	return readCaptures(m.captures.root, m.captures.generation)
}

func readCaptures(root string, generation int) tea.Cmd {
	return func() tea.Msg {
		listing, err := captures.List(root)
		return capturesLoaded{root: root, generation: generation, listing: listing, err: err}
	}
}

func (m model) acceptCaptures(msg capturesLoaded) (tea.Model, tea.Cmd) {
	if m.screen != screenCaptures || msg.root != m.captures.root || msg.generation != m.captures.generation {
		return m, nil
	}
	b := &m.captures
	b.loading = false
	b.err = ""
	if msg.err != nil {
		b.err = msg.err.Error()
	}
	b.listing = msg.listing
	// Keep the selected identity when newer recordings arrive. If it disappears,
	// leave no selection until the user moves: Enter must not open another capture.
	if b.selected == "" && len(b.listing.Entries) > 0 {
		b.selected = b.listing.Entries[0].Path
	}
	b.cursor = -1
	for i, e := range b.listing.Entries {
		if e.Path == b.selected {
			b.cursor = i
			break
		}
	}
	generation := b.generation
	return m, tea.Tick(2*time.Second, func(time.Time) tea.Msg { return captureRefresh{generation} })
}

func (m model) updateCaptures(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	b := &m.captures
	switch msg.String() {
	case "ctrl+c":
		return m.exitQuit()
	case "q", "esc":
		m.screen = screenHome
		m.cursor = 0
		b.generation++
		b.loading = false
	case "up", "k":
		if len(b.listing.Entries) > 0 {
			b.cursor = max(0, b.cursor-1)
			b.selected = b.listing.Entries[b.cursor].Path
		}
	case "down", "j":
		if len(b.listing.Entries) > 0 {
			b.cursor = min(len(b.listing.Entries)-1, b.cursor+1)
			b.selected = b.listing.Entries[b.cursor].Path
		}
	case "r":
		// Invalidate the previous scheduled refresh before starting a new poll chain.
		if !b.loading {
			b.generation++
			cmd := m.loadCaptures()
			return m, cmd
		}
	case "c":
		m.screen = screenCaptureFolder
		b.generation++
		b.loading = false
		b.folder.SetValue(b.root)
		b.folder.Width = max(20, min(100, m.width-12))
		b.folder.Focus()
		m.status = ""
		return m, textinput.Blink
	case "enter":
		if b.cursor < 0 || b.cursor >= len(b.listing.Entries) {
			return m, nil
		}
		entry := b.listing.Entries[b.cursor]
		m.result = Result{Action: ActionObserve, Path: entry.Path, Follow: entry.Follow()}
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m model) updateCaptureFolder(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		return m.exitQuit()
	case "esc":
		return m.startCaptures()
	case "enter":
		root, err := captures.ResolveRoot(m.captures.folder.Value(), m.opts.Cwd)
		if err == nil {
			err = captures.SaveRoot(m.opts.UserConfigDir, root)
		}
		if err != nil {
			m.status = fmt.Sprintf("Could not use folder: %v", err)
			return m, nil
		}
		m.captures.root = root
		m.captures.selected = ""
		m.captures.listing = captures.Listing{}
		m.captures.preferenceErr = ""
		m.notice = ""
		return m.startCaptures()
	}
	var cmd tea.Cmd
	m.captures.folder, cmd = m.captures.folder.Update(msg)
	return m, cmd
}
