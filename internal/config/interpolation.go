package config

import (
	"maps"

	"github.com/lancekrogers/stream-debugger/internal/mapping"
)

// InterpolationVarsFromConfig builds the fixed interpolation context shared by
// setup and send sites. User vars are copied so a caller cannot mutate the run
// configuration while rendering a request; mapping protects the built-in
// names from being shadowed by those user vars.
func InterpolationVarsFromConfig(cfg *EnhancedConfig) mapping.InterpolationVars {
	if cfg == nil {
		return mapping.InterpolationVars{}
	}
	return mapping.InterpolationVars{
		BaseURL:   cfg.Transport.BaseURL,
		SessionID: cfg.Session.ID,
		Agents:    append([]string(nil), cfg.Session.DefaultAgents...),
		Vars:      maps.Clone(cfg.Vars),
	}
}
