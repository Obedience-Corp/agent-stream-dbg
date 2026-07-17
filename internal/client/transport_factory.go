package client

import (
	"fmt"
	"net/url"

	"github.com/lancekrogers/stream-debugger/internal/bridge"
	"github.com/lancekrogers/stream-debugger/internal/config"
	"github.com/lancekrogers/stream-debugger/internal/mapping"
	"github.com/lancekrogers/stream-debugger/internal/transport"
	grpctransport "github.com/lancekrogers/stream-debugger/internal/transport/grpc"
	"github.com/lancekrogers/stream-debugger/internal/transport/replay"
	"github.com/lancekrogers/stream-debugger/internal/transport/sse"
)

func newTransport(cfg *config.EnhancedConfig, parser *bridge.Parser, vars mapping.InterpolationVars) (transport.Transport, error) {
	switch cfg.Transport.Type {
	case "", "sse":
		return newSSETransport(cfg, parser, vars)
	case "grpc":
		return newGRPCTransport(cfg), nil
	case "replay":
		tr, err := replay.New(cfg.Transport.BaseURL, 0)
		if err != nil {
			return nil, err
		}
		return tr, nil
	default:
		return nil, fmt.Errorf("unsupported transport.type %q (want sse, grpc, or replay)", cfg.Transport.Type)
	}
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

func newGRPCTransport(cfg *config.EnhancedConfig) transport.Transport {
	metadataKey, metadataValue, _ := cfg.Transport.Auth.Metadata()
	return grpctransport.New(grpctransport.Config{
		Target:             cfg.Transport.Target,
		Plaintext:          cfg.Transport.Plaintext,
		MetadataKey:        metadataKey,
		MetadataValue:      metadataValue,
		Method:             cfg.Transport.GRPCMethod,
		Discriminator:      cfg.Transport.Discriminator,
		PreserveFieldNames: cfg.Transport.PreserveFieldNames,
		DescriptorSetPath:  cfg.Transport.DescriptorSet,
		ProtoFilePath:      cfg.Transport.ProtoFile,
		ProtoImportPaths:   cfg.Transport.ProtoImportPath,
	})
}
