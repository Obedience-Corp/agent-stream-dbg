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

func newSSETransport(cfg *config.EnhancedConfig, parser *bridge.Parser, vars mapping.InterpolationVars) (transport.Transport, error) {
	method, renderedURL, body, err := parser.RenderSend(vars)
	if err != nil {
		return nil, fmt.Errorf("failed to render send request: %w", err)
	}
	if method != "GET" || body != nil {
		return nil, fmt.Errorf("stream mode can only execute a GET-style send (no body) today; dialect declared %s with a body — needs a POST-capable transport", method)
	}
	if cfg.Debug.Level != "" {
		u, err := url.Parse(renderedURL)
		if err != nil {
			return nil, fmt.Errorf("invalid endpoint URL: %w", err)
		}
		q := u.Query()
		q.Set("debug", cfg.Debug.Level)
		u.RawQuery = q.Encode()
		renderedURL = u.String()
	}
	headers := cfg.Transport.ResolvedHeaders()
	headers["Accept"] = "text/event-stream"
	return sse.New(method, renderedURL, nil, headers), nil
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
