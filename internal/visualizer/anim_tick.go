package visualizer

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/lancekrogers/stream-debugger/internal/visualizer/anim"
)

// animTickMsg drives energy-strip / agent-pulse / flow-chase frames.
type animTickMsg struct{}

const animTickInterval = 80 * time.Millisecond

func animTickCmd() tea.Cmd {
	return tea.Tick(animTickInterval, func(time.Time) tea.Msg {
		return animTickMsg{}
	})
}

// streamTokensPerSec estimates live throughput for the current turn.
func (m InteractiveModel) streamTokensPerSec() float64 {
	if len(m.messages) == 0 {
		return 0
	}
	msg := m.messages[len(m.messages)-1]
	tokens := 0
	for _, r := range msg.AgentResponses {
		if r != nil {
			tokens += r.TokenCount
		}
	}
	if tokens == 0 {
		return 0
	}
	elapsed := time.Since(msg.Timestamp).Seconds()
	if elapsed < 0.05 {
		elapsed = 0.05
	}
	return float64(tokens) / elapsed
}

func (m InteractiveModel) animStyles() anim.Styles {
	return anim.DefaultStyles()
}

func (m InteractiveModel) hasHotViewportChrome() bool {
	if m.streaming {
		return true
	}
	if len(m.messages) == 0 {
		return false
	}
	msg := m.messages[len(m.messages)-1]
	for _, r := range msg.AgentResponses {
		if r != nil && !r.Completed && r.TokenCount > 0 {
			return true
		}
	}
	nodes := m.buildFlowNodes(msg.Events)
	for _, n := range nodes {
		if n != nil && n.InProgress {
			return true
		}
	}
	return false
}
