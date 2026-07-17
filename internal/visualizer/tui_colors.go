package visualizer

import "hash/fnv"

var agentColorPalette = []string{"13", "9", "10", "12", "14", "5", "3", "6", "11", "1", "2", "4"}

// getAgentColor maps an agent ID to a configured or deterministic palette
// color, with plain gray as the fallback for an empty unconfigured ID.
func getAgentColor(agentID string, agentColors map[string]string) string {
	if color, ok := agentColors[agentID]; ok {
		return color
	}
	if agentID == "" {
		return "7"
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(agentID))
	return agentColorPalette[h.Sum32()%uint32(len(agentColorPalette))]
}
