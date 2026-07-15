// Package grpc is the gRPC transport: reflection-first schema discovery,
// dynamicpb -> protojson decode into the same Frame contract every other
// transport uses, and the discriminator/error/bidi semantics gRPC needs
// that SSE has no equivalent of. This file is the dial layer only —
// establishing a connection, plaintext or TLS, with metadata auth
// attached to every RPC. Streaming, discovery, and decode land in later
// tasks of this sequence.
package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
)

// Config configures a gRPC dial. It deliberately doesn't import
// internal/config — like internal/transport/sse, this package stays
// decoupled from the config package; a caller resolves config.
// TransportConfig into this shape (target, plaintext, resolved metadata
// key/value) the same way internal/client/sse_client.go resolves
// config.TransportConfig into internal/transport/sse.New's plain args.
type Config struct {
	Target    string
	Plaintext bool

	// TLSRootCAs, if set, is the CA pool used to verify the server's
	// certificate. Nil uses the system pool. Ignored when Plaintext.
	TLSRootCAs *x509.CertPool

	// MetadataKey/MetadataValue, if MetadataKey is non-empty, are
	// attached to every outgoing RPC — the gRPC equivalent of an SSE
	// auth header.
	MetadataKey   string
	MetadataValue string
}

// Transport dials one gRPC target. Streaming, discovery, and Frame
// production are added by later tasks in this sequence; this type is
// the connection and per-RPC-metadata layer they build on.
type Transport struct {
	cfg  Config
	conn *grpc.ClientConn
}

// New returns a Transport for cfg. It does not dial — call Connect.
func New(cfg Config) *Transport {
	return &Transport{cfg: cfg}
}

// Name identifies this transport kind.
func (t *Transport) Name() string { return "grpc" }

// Connect dials cfg.Target and blocks until the connection is genuinely
// ready or ctx is done — unlike grpc.NewClient's default lazy/virtual
// connection (which would only surface a dial failure on the first RPC),
// a debugger needs "can't reach this target" to fail here, at Connect,
// with a wrapped and actionable error.
func (t *Transport) Connect(ctx context.Context) error {
	var creds credentials.TransportCredentials
	if t.cfg.Plaintext {
		creds = insecure.NewCredentials()
	} else {
		creds = credentials.NewTLS(&tls.Config{RootCAs: t.cfg.TLSRootCAs})
	}

	conn, err := grpc.NewClient(t.cfg.Target, grpc.WithTransportCredentials(creds))
	if err != nil {
		return fmt.Errorf("grpc: build client for %s: %w", t.cfg.Target, err)
	}

	conn.Connect()
	for {
		state := conn.GetState()
		if state == connectivity.Ready {
			break
		}
		if !conn.WaitForStateChange(ctx, state) {
			_ = conn.Close()
			if err := ctx.Err(); err != nil {
				return fmt.Errorf("grpc: connect to %s: %w", t.cfg.Target, err)
			}
			return fmt.Errorf("grpc: connect to %s: connection did not become ready (last state: %s)", t.cfg.Target, state)
		}
	}

	t.conn = conn
	return nil
}

// Conn returns the dialed connection, for use by the reflection client
// and streaming call in later tasks. Only valid after a successful
// Connect.
func (t *Transport) Conn() *grpc.ClientConn { return t.conn }

// outgoingContext attaches the configured metadata (if any) to ctx, for
// every RPC this transport makes.
func (t *Transport) outgoingContext(ctx context.Context) context.Context {
	if t.cfg.MetadataKey == "" {
		return ctx
	}
	return metadata.AppendToOutgoingContext(ctx, t.cfg.MetadataKey, t.cfg.MetadataValue)
}

// Close closes the underlying connection, if dialed.
func (t *Transport) Close() error {
	if t.conn == nil {
		return nil
	}
	return t.conn.Close()
}
