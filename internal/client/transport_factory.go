package client

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/Obedience-Corp/stream-debugger/internal/bridge"
	"github.com/Obedience-Corp/stream-debugger/internal/config"
	"github.com/Obedience-Corp/stream-debugger/internal/mapping"
	"github.com/Obedience-Corp/stream-debugger/internal/transport"
	acptransport "github.com/Obedience-Corp/stream-debugger/internal/transport/acp"
	grpctransport "github.com/Obedience-Corp/stream-debugger/internal/transport/grpc"
	"github.com/Obedience-Corp/stream-debugger/internal/transport/replay"
	"github.com/Obedience-Corp/stream-debugger/internal/transport/sse"
)

// GRPCStreamingMethod is the UI-facing alias for a reflection-discovered
// streaming RPC. Keeping the alias here lets callers stay at the client
// boundary instead of depending on transport implementation details.
type GRPCStreamingMethod = grpctransport.StreamingMethod

// DiscoverGRPCMethods exposes reflection discovery to interactive callers
// without making them construct the transport package's lower-level Config.
// It dials only long enough to enumerate streaming RPCs and build starter
// request JSON; selecting a method still goes through NewTransport, so the
// configured stream uses the same path as every other client.
func DiscoverGRPCMethods(ctx context.Context, cfg *config.EnhancedConfig) ([]grpctransport.StreamingMethod, error) {
	if cfg == nil {
		return nil, fmt.Errorf("gRPC discovery requires a configuration")
	}
	metadataKey, metadataValue, _ := cfg.Transport.Auth.Metadata()
	return grpctransport.DiscoverStreamingMethods(ctx, grpctransport.Config{
		Target:        cfg.Transport.Target,
		Plaintext:     cfg.Transport.Plaintext,
		MetadataKey:   metadataKey,
		MetadataValue: metadataValue,
	})
}

func newTransport(cfg *config.EnhancedConfig, parser *bridge.Parser, vars mapping.InterpolationVars) (transport.Transport, error) {
	switch cfg.Transport.Type {
	case "", "sse":
		return newSSETransport(cfg, parser, vars)
	case "grpc":
		return newGRPCTransport(cfg, vars)
	case "replay":
		tr, err := replay.New(cfg.Transport.BaseURL, 0)
		if err != nil {
			return nil, err
		}
		return tr, nil
	case "acp":
		return newACPTransport(cfg, vars), nil
	default:
		return nil, fmt.Errorf("unsupported transport.type %q (want sse, grpc, replay, or acp)", cfg.Transport.Type)
	}
}

func newACPTransport(cfg *config.EnhancedConfig, vars mapping.InterpolationVars) transport.Transport {
	return acptransport.New(acptransport.Config{
		Command:                cfg.Transport.Command,
		Args:                   append([]string(nil), cfg.Transport.Args...),
		Cwd:                    cfg.Transport.Cwd,
		Env:                    append([]string(nil), cfg.Transport.Env...),
		Prompt:                 vars.Message,
		AutoApprovePermissions: cfg.Transport.AutoApprove,
		ClientName:             "stream-debugger",
	})
}

// NewTransport builds the configured raw transport for one rendered message.
// Callers that need frame-level access (rather than the parsed Events channel
// exposed by Client) use this shared factory so every product path executes
// the same dialect send template and transport selection.
func NewTransport(cfg *config.EnhancedConfig, parser *bridge.Parser, message string) (transport.Transport, error) {
	vars := config.InterpolationVarsFromConfig(cfg)
	vars.Message = message
	return newTransport(cfg, parser, vars)
}

func newSSETransport(cfg *config.EnhancedConfig, parser *bridge.Parser, vars mapping.InterpolationVars) (transport.Transport, error) {
	method, renderedURL, body, err := parser.RenderSend(vars)
	if err != nil {
		return nil, fmt.Errorf("failed to render send request: %w", err)
	}
	if cfg.Debug.Level != "" {
		renderedURL, err = withDebugQuery(renderedURL, cfg.Debug.Level)
		if err != nil {
			return nil, err
		}
	}
	headers := cfg.Transport.ResolvedHeaders()
	headers["Accept"] = "text/event-stream"
	if body != nil {
		headers["Content-Type"] = "application/json"
	}
	return sse.New(method, renderedURL, body, headers), nil
}

func withDebugQuery(rawURL, level string) (string, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid endpoint URL: %w", err)
	}
	q := u.Query()
	q.Set("debug", level)
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func newGRPCTransport(cfg *config.EnhancedConfig, vars mapping.InterpolationVars) (transport.Transport, error) {
	metadataKey, metadataValue, _ := cfg.Transport.Auth.Metadata()
	request, err := interpolateGRPCRequest(cfg.Transport.Request, vars)
	if err != nil {
		return nil, fmt.Errorf("failed to render gRPC request: %w", err)
	}
	return grpctransport.New(grpctransport.Config{
		Target:             cfg.Transport.Target,
		Plaintext:          cfg.Transport.Plaintext,
		MetadataKey:        metadataKey,
		MetadataValue:      metadataValue,
		Method:             cfg.Transport.GRPCMethod,
		Request:            request,
		Discriminator:      cfg.Transport.Discriminator,
		DiscriminatorField: cfg.Transport.DiscriminatorField,
		PreserveFieldNames: cfg.Transport.PreserveFieldNames,
		DescriptorSetPath:  cfg.Transport.DescriptorSet,
		ProtoFilePath:      cfg.Transport.ProtoFile,
		ProtoImportPaths:   cfg.Transport.ProtoImportPath,
	}), nil
}

// interpolateGRPCRequest renders placeholders in request fields at the same
// point a dialect's SSE send template is rendered. This keeps gRPC request
// configuration data-only while allowing common values such as {message},
// {session_id}, and declared vars to vary per turn.
func interpolateGRPCRequest(request map[string]any, vars mapping.InterpolationVars) (map[string]any, error) {
	if request == nil {
		return nil, nil
	}
	rendered, err := renderGRPCRequestValue(request, vars)
	if err != nil {
		return nil, err
	}
	out, ok := rendered.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("request must be an object")
	}
	return out, nil
}

func renderGRPCRequestValue(value any, vars mapping.InterpolationVars) (any, error) {
	switch v := value.(type) {
	case string:
		if !strings.Contains(v, "{") {
			return v, nil
		}
		if exact, ok := exactGRPCPlaceholderValue(v, vars); ok {
			return exact, nil
		}
		rendered, err := mapping.Interpolate(v, vars)
		if err != nil {
			return nil, err
		}
		// mapping.Interpolate escapes placeholders for JSON templates. Decode
		// that escaped content back into the string value a protobuf field
		// expects; request maps are already typed data, not JSON templates.
		var decoded string
		if err := json.Unmarshal([]byte(`"`+rendered+`"`), &decoded); err == nil {
			return decoded, nil
		}
		return rendered, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			rendered, err := renderGRPCRequestValue(child, vars)
			if err != nil {
				return nil, fmt.Errorf("request field %q: %w", key, err)
			}
			out[key] = rendered
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			rendered, err := renderGRPCRequestValue(child, vars)
			if err != nil {
				return nil, fmt.Errorf("request item %d: %w", i, err)
			}
			out[i] = rendered
		}
		return out, nil
	default:
		return value, nil
	}
}

func exactGRPCPlaceholderValue(value string, vars mapping.InterpolationVars) (any, bool) {
	switch value {
	case "{base_url}":
		return vars.BaseURL, true
	case "{session_id}":
		return vars.SessionID, true
	case "{message}":
		return vars.Message, true
	case "{agents}":
		return append([]string(nil), vars.Agents...), true
	}
	if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
		name := strings.TrimSuffix(strings.TrimPrefix(value, "{"), "}")
		if rendered, ok := vars.Vars[name]; ok {
			return rendered, true
		}
	}
	return nil, false
}
