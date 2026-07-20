package visualizer

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/Obedience-Corp/agent-stream-dbg/internal/visualizer/anim"
)

// animTickMsg drives energy decay sampling + flow-chase glyph frames.
type animTickMsg struct{}

const animTickInterval = 80 * time.Millisecond

func animTickCmd() tea.Cmd {
	return tea.Tick(animTickInterval, func(time.Time) tea.Msg {
		return animTickMsg{}
	})
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

// agentEnergyLevel estimates 0–1 activity for one agent from its token rate
// since stream start (not a free-running animation).
func agentEnergyLevel(ar *AgentResponse, streaming bool) float64 {
	if ar == nil || ar.TokenCount == 0 {
		if streaming {
			return 0.1
		}
		return 0
	}
	elapsed := time.Since(ar.StartTime).Seconds()
	if ar.StartTime.IsZero() {
		elapsed = 1
	}
	if elapsed < 0.05 {
		elapsed = 0.05
	}
	if ar.Completed && ar.DurationMs > 0 {
		elapsed = float64(ar.DurationMs) / 1000.0
	}
	rate := float64(ar.TokenCount) / elapsed
	return anim.NormalizeRate(rate)
}
