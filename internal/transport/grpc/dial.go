// Package grpc is the gRPC transport: reflection-first schema discovery,
// dynamic decode into the same Frame contract every other transport
// uses, and the discriminator/error/bidi semantics gRPC needs that SSE
// has no equivalent of. This file is the dial layer — establishing a
// connection, plaintext or TLS, with metadata auth attached to every
// RPC — plus Connect's orchestration of discovery and streaming (stream.go).
package grpc

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/connectivity"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/jhump/protoreflect/desc" //nolint:staticcheck // SA1019: same reasoning as stream.go — grpcdynamic requires this type
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"

	"github.com/lancekrogers/stream-debugger/internal/transport"
)

// Config configures a gRPC dial and, if Method is set, the single
// streaming call Connect starts. It deliberately doesn't import
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

	// Method is the fully-qualified RPC path, e.g.
	// "/agent.v1.AgentService/StreamSession". If empty, Connect only
	// dials — Frames() will never yield anything. If set, Connect also
	// discovers the method via reflection and starts the server-stream.
	Method string

	// Request holds field name -> value pairs set on the request message
	// via TrySetFieldByName before the call opens. Deliberately minimal —
	// richer request templating (mirroring a dialect's send:) is out of
	// scope until it's actually needed.
	Request map[string]any

	// Discriminator resolves Frame.Name from a decoded message — gRPC's
	// one genuine impedance mismatch with SSE's native event: name.
	// One of "oneof" (the populated oneof case's field name — the happy
	// path; if the response declares more than one top-level oneof, the
	// first one with a field actually populated wins, checked in
	// declaration order), "field:type" (DiscriminatorField's string
	// value), "message_type" (the response type's full proto name), or
	// "none"/"" (Frame.Name stays empty; the dialect uses discriminator:
	// auto, same as SSE's openai.yaml). This is purely a transport-layer
	// concern: the resolved name feeds the SAME unmodified mapping.Engine
	// a discriminator: event dialect already expects from any transport.
	Discriminator string

	// DiscriminatorField is the field name Discriminator "field:type"
	// reads as Frame.Name. Ignored for every other mode.
	DiscriminatorField string

	// DescriptorSetPath, if set, resolves Method against a compiled
	// FileDescriptorSet on disk instead of via server reflection — for
	// targets with reflection disabled (common in production). See
	// LoadDescriptorSet. Checked before Connect ever dials the network:
	// a bad path fails fast and locally, the same way a malformed dialect
	// file fails before any connection is attempted. Takes priority over
	// ProtoFilePath if both are set (cheaper: no compilation).
	DescriptorSetPath string

	// ProtoFilePath, if set (and DescriptorSetPath is not), resolves
	// Method by compiling this .proto file at runtime via
	// bufbuild/protocompile — the lowest-priority fallback tier, for
	// targets where neither reflection nor a descriptor set is
	// available. ProtoImportPaths are searched for the file and its own
	// imports, mirroring protoc's -I/--proto_path. See LoadProtoFile.
	// Checked before Connect ever dials, same as DescriptorSetPath.
	ProtoFilePath    string
	ProtoImportPaths []string
}

// Transport dials one gRPC target and, when Config.Method is set,
// streams it into Frames(). Discovery and decode are added by this task;
// error/trailer semantics and Send land in the next sequence.
type Transport struct {
	cfg  Config
	conn *grpc.ClientConn

	frames chan transport.Frame
	// closed is closed by Close before anything else, so a read-loop
	// goroutine blocked sending to a full, undrained frames channel
	// unblocks immediately — closing the conn alone only unblocks a
	// goroutine blocked on network I/O, not one blocked on a channel
	// send with nobody consuming Frames().
	closed    chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once

	// sendMethod/sendStream are set (non-nil) only when Connect discovers
	// cfg.Method is bidi-streaming — the method descriptor already knows
	// this, so Send's behavior is decided from these, not a separate
	// config flag duplicating information the descriptor already has. Both
	// stay nil for a server-streaming Config, the common case, and Send
	// always errors in that case.
	sendMethod *desc.MethodDescriptor
	sendStream *grpcdynamic.BidiStream
	// sendMu serializes Send calls: grpc.ClientStream.SendMsg is
	// documented as unsafe to call on the same stream from different
	// goroutines (concurrent Send + Recv from two goroutines is fine;
	// concurrent Send + Send is not). Send is a public method with no
	// visible warning otherwise, so this makes "call Send from wherever"
	// safe by construction rather than relying on every caller to
	// serialize its own calls.
	sendMu sync.Mutex
}

// New returns a Transport for cfg. It does not dial — call Connect.
func New(cfg Config) *Transport {
	return &Transport{cfg: cfg, frames: make(chan transport.Frame, 32), closed: make(chan struct{})}
}

// Name identifies this transport kind.
func (t *Transport) Name() string { return "grpc" }

// Connect dials cfg.Target and blocks until the connection is genuinely
// ready or ctx is done — unlike grpc.NewClient's default lazy/virtual
// connection (which would only surface a dial failure on the first RPC),
// a debugger needs "can't reach this target" to fail here, at Connect,
// with a wrapped and actionable error. If cfg.Method is set, Connect
// also resolves that method — via a descriptor set if
// cfg.DescriptorSetPath is set, via runtime .proto compilation if
// cfg.ProtoFilePath is set, reflection otherwise — and starts streaming
// it into Frames(); see stream.go.
//
// A configured DescriptorSetPath or ProtoFilePath is resolved FIRST,
// before any dial: both are pure local operations with no network
// dependency, so a missing or malformed file fails fast and locally,
// exactly like a malformed dialect file would, rather than only
// surfacing after a (wasted) successful dial. Setting both is rejected
// outright rather than silently preferring one: there's no fallback-on-
// failure logic here (only ever one attempt), so a Config with both set
// is always either a mistake or stale leftover config, never a
// meaningful combination — silently picking one would let a stale
// descriptor set shadow every edit to a proto file the caller thinks is
// the one in effect.
func (t *Transport) Connect(ctx context.Context) error {
	if t.cfg.DescriptorSetPath != "" && t.cfg.ProtoFilePath != "" {
		return fmt.Errorf("grpc: DescriptorSetPath and ProtoFilePath are both set — pick one schema-acquisition tier, not both")
	}

	var md *desc.MethodDescriptor
	switch {
	case t.cfg.Method == "":
		// Nothing to resolve.
	case t.cfg.DescriptorSetPath != "":
		resolved, err := LoadDescriptorSet(t.cfg.DescriptorSetPath, t.cfg.Method)
		if err != nil {
			return err
		}
		md = resolved
	case t.cfg.ProtoFilePath != "":
		resolved, err := LoadProtoFile(ctx, t.cfg.ProtoFilePath, t.cfg.ProtoImportPaths, t.cfg.Method)
		if err != nil {
			return err
		}
		md = resolved
	}

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

	if t.cfg.Method == "" {
		return nil
	}
	if err := t.startStream(ctx, md); err != nil {
		_ = conn.Close()
		t.conn = nil
		return err
	}
	return nil
}

// Frames returns the channel of decoded frames. Closed once the stream
// ends, errors, or ctx is canceled. Empty (never yields) if cfg.Method
// was unset.
func (t *Transport) Frames() <-chan transport.Frame {
	return t.frames
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

// Close signals the read-loop goroutine to stop (unblocking it whether
// it's waiting on network I/O or blocked sending to a full, undrained
// Frames() channel), closes the underlying connection, then waits for
// that goroutine to fully exit before returning — so, exactly like
// internal/transport/sse, a send on Frames() can never race a close by
// construction: only that goroutine ever closes the channel, and Close
// never returns before it has.
func (t *Transport) Close() error {
	var closeErr error
	t.closeOnce.Do(func() {
		close(t.closed)
		if t.conn != nil {
			closeErr = t.conn.Close()
		}
		t.wg.Wait()
	})
	return closeErr
}
