package config

import (
	"fmt"
	"strings"
)

// TemplateKind selects a starting transport shape for a new run config.
type TemplateKind string

const (
	TemplateSSE    TemplateKind = "sse"
	TemplateGRPC   TemplateKind = "grpc"
	TemplateACP    TemplateKind = "acp"
	TemplateReplay TemplateKind = "replay"
)

// NewConfigYAML builds a backend-neutral YAML document for a new run config.
func NewConfigYAML(kind TemplateKind, dialect, baseURL, endpoint, authEnv, grpcTarget, grpcMethod, acpCommand, replayPath string) string {
	dialect = strings.TrimSpace(dialect)
	if dialect == "" {
		dialect = "brainyard"
	}

	var body strings.Builder
	body.WriteString("# Stream Debugger run configuration\n")
	body.WriteString("# Edit in the TUI home hub or with your editor. Secrets stay in env vars.\n")
	body.WriteString("transport:\n")

	switch kind {
	case TemplateGRPC:
		target := strings.TrimSpace(grpcTarget)
		if target == "" {
			target = "localhost:50051"
		}
		method := strings.TrimSpace(grpcMethod)
		body.WriteString("  type: grpc\n")
		body.WriteString(fmt.Sprintf("  target: %q\n", target))
		if method != "" {
			body.WriteString(fmt.Sprintf("  method: %q\n", method))
		} else {
			body.WriteString("  method: \"\"\n")
		}
		body.WriteString("  plaintext: true\n")
		body.WriteString("  discriminator: oneof\n")
		if strings.TrimSpace(authEnv) != "" {
			body.WriteString("  auth:\n")
			body.WriteString("    type: metadata\n")
			body.WriteString("    header_name: authorization\n")
			body.WriteString(fmt.Sprintf("    token_env: %q\n", authEnv))
		} else {
			body.WriteString("  auth:\n")
			body.WriteString("    type: none\n")
		}
	case TemplateACP:
		cmd := strings.TrimSpace(acpCommand)
		if cmd == "" {
			cmd = "grok"
		}
		body.WriteString("  type: acp\n")
		body.WriteString(fmt.Sprintf("  command: %q\n", cmd))
		body.WriteString("  args: []\n")
		body.WriteString("  auto_approve: true\n")
	case TemplateReplay:
		path := strings.TrimSpace(replayPath)
		body.WriteString("  type: replay\n")
		body.WriteString(fmt.Sprintf("  base_url: %q\n", path))
	default: // SSE
		body.WriteString("  type: sse\n")
		body.WriteString(fmt.Sprintf("  base_url: %q\n", strings.TrimSpace(baseURL)))
		ep := strings.TrimSpace(endpoint)
		if ep == "" {
			ep = "/v1/stream"
		}
		body.WriteString("  stream_endpoint:\n")
		body.WriteString(fmt.Sprintf("    url: %q\n", ep))
		body.WriteString("    method: GET\n")
		body.WriteString("    headers:\n")
		body.WriteString("      Accept: text/event-stream\n")
		body.WriteString("    auth:\n")
		if strings.TrimSpace(authEnv) != "" {
			body.WriteString("      type: bearer\n")
			body.WriteString(fmt.Sprintf("      token_env: %q\n", authEnv))
		} else {
			body.WriteString("      type: none\n")
		}
	}

	body.WriteString("\ndialect:\n")
	body.WriteString(fmt.Sprintf("  file: %q\n", dialect))
	body.WriteString("\nvars: {}\n")
	body.WriteString("\nsession:\n")
	body.WriteString("  auto_setup: true\n")
	body.WriteString("\nlogging:\n")
	body.WriteString("  dir: ./logs\n")
	return body.String()
}
