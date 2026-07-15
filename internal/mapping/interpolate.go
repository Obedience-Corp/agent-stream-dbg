package mapping

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// placeholderPattern matches {name} placeholders — an identifier
// immediately inside braces. JSON object syntax like {"key": ...} never
// matches, since the character after `{` there is `"`, not a letter or
// underscore; that's what lets templates mix JSON braces and placeholders
// unambiguously.
var placeholderPattern = regexp.MustCompile(`\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// InterpolationVars is the fixed set of run-config-derived values available
// to a dialect's setup:/send: templates. Dialects cannot reference env vars
// directly — only these run-config-sourced values are ever substituted, so
// a shared dialect file can never name or exfiltrate a credential.
type InterpolationVars struct {
	BaseURL   string
	SessionID string
	Message   string
	Agents    []string
	Vars      map[string]string // user-defined vars: from the run config
}

// knownNames returns every placeholder name available for substitution:
// the fixed set plus each declared run-config var.
func (v InterpolationVars) knownNames() map[string]bool {
	known := map[string]bool{
		"base_url":   true,
		"session_id": true,
		"message":    true,
		"agents":     true,
	}
	for name := range v.Vars {
		known[name] = true
	}
	return known
}

// ValidatePlaceholders checks that every {placeholder} referenced in
// template names a value that would actually be available at send time —
// the fixed set (base_url, session_id, message, agents) or a declared
// run-config var. It's meant to run at load time, before any request is
// sent, so a dialect naming an unknown or typo'd placeholder fails
// immediately instead of silently rendering it literally at send time.
func ValidatePlaceholders(template string, v InterpolationVars) error {
	known := v.knownNames()
	var unknown []string
	seen := map[string]bool{}
	for _, m := range placeholderPattern.FindAllStringSubmatch(template, -1) {
		name := m[1]
		if !known[name] && !seen[name] {
			unknown = append(unknown, name)
			seen[name] = true
		}
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown placeholder(s) %v (must be one of: base_url, session_id, message, agents, or a declared var)", unknown)
	}
	return nil
}

// Interpolate renders a setup:/send: template string, substituting
// {base_url}, {session_id}, {message}, {agents}, and any run-config var
// placeholder.
//
// {agents} is JSON-aware: it renders as a JSON array literal (e.g.
// ["a","b"]), for use unquoted in a template like `"agents": {agents}`.
// Every other placeholder renders as JSON-escaped string content, for use
// inside an existing quoted string like `"message": "{message}"` — this is
// what keeps arbitrary message content (quotes, backslashes, newlines, or
// deliberate injection attempts) from ever breaking out of its JSON string
// context.
func Interpolate(template string, v InterpolationVars) (string, error) {
	if err := ValidatePlaceholders(template, v); err != nil {
		return "", err
	}

	agentsJSON, err := json.Marshal(v.Agents)
	if err != nil {
		return "", fmt.Errorf("failed to render agents placeholder: %w", err)
	}

	values := map[string]string{
		"base_url":   jsonEscapeContent(v.BaseURL),
		"session_id": jsonEscapeContent(v.SessionID),
		"message":    jsonEscapeContent(v.Message),
		"agents":     string(agentsJSON),
	}
	for name, val := range v.Vars {
		values[name] = jsonEscapeContent(val)
	}

	return placeholderPattern.ReplaceAllStringFunc(template, func(match string) string {
		name := placeholderPattern.FindStringSubmatch(match)[1]
		return values[name]
	}), nil
}

// InterpolateJSON renders template like Interpolate, then re-validates the
// result is syntactically valid JSON — catching a malformed template or an
// unescaped edge case before it reaches the network.
func InterpolateJSON(template string, v InterpolationVars) (string, error) {
	rendered, err := Interpolate(template, v)
	if err != nil {
		return "", err
	}
	var probe any
	if err := json.Unmarshal([]byte(rendered), &probe); err != nil {
		return "", fmt.Errorf("interpolated body is not valid JSON: %w", err)
	}
	return rendered, nil
}

// jsonEscapeContent returns s escaped for safe embedding inside an existing
// JSON string's quotes (i.e. json.Marshal's output with the surrounding
// quotes stripped).
func jsonEscapeContent(s string) string {
	b, _ := json.Marshal(s) // string marshaling never fails
	return strings.TrimSuffix(strings.TrimPrefix(string(b), `"`), `"`)
}
