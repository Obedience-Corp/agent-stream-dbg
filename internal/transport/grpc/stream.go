package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	// jsonpb is marked deprecated in favor of google.golang.org/protobuf/
	// encoding/protojson — but dynamic.Message.MarshalJSONPB (the only
	// way to get canonical protobuf JSON out of a message grpcdynamic
	// produced) takes a *jsonpb.Marshaler, not protojson.MarshalOptions.
	// grpcdynamic.Stub itself is built on github.com/golang/protobuf's
	// old-API proto.Message (see its RecvMsg return type), so its
	// responses don't implement protoreflect.ProtoMessage and can't be
	// passed to protojson.Marshal directly. jsonpb.Marshaler internally
	// delegates to protojson under the hood (OrigName maps straight to
	// protojson's UseProtoNames), so this produces byte-identical output
	// to what protojson.Marshal would for an equivalent modern message —
	// the same canonical protobuf JSON shape the design's pipeline
	// describes, via the only entry point this library version exposes.
	"github.com/golang/protobuf/jsonpb"     //nolint:staticcheck // SA1019: see above
	"github.com/golang/protobuf/proto"      //nolint:staticcheck // SA1019: grpcdynamic.ServerStream/BidiStream.RecvMsg both return this old-API type; recvStream unifies them without wrapper types
	"github.com/jhump/protoreflect/desc"    //nolint:staticcheck // SA1019: same reasoning as reflect.go — grpcdynamic requires this type
	"github.com/jhump/protoreflect/dynamic" //nolint:staticcheck // SA1019: grpcdynamic's own message factory produces this concrete type; see MarshalJSONPB comment above
	"github.com/jhump/protoreflect/dynamic/grpcdynamic"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/lancekrogers/stream-debugger/internal/transport"
)

// recvStream is satisfied by both *grpcdynamic.ServerStream and
// *grpcdynamic.BidiStream — readLoop and emitStreamEnd operate on
// either, since receiving decoded messages and reading terminal trailers
// work identically regardless of whether the client side also sends.
type recvStream interface {
	RecvMsg() (proto.Message, error)
	Trailer() metadata.MD
}

// grpcStatusEventName is the fixed sentinel Frame.Name emitStreamEnd uses
// for the synthetic status/trailers frame — NOT affected by
// Config.Discriminator, which only resolves Frame.Name for a genuinely
// decoded response message. A dialect declares a rule against this exact
// name (match: {event: grpc_status}) to turn it into Kind=Error, same as
// any other named event; see emitStreamEnd's doc comment for why this is
// a fixed name rather than something Config.Discriminator computes.
const grpcStatusEventName = "grpc_status"

// jsonMarshaler renders protobuf-canonical JSON with original
// (snake_case) field names — matching every other dialect's field paths
// (agent_id, not agentId) so the same dialect YAML works over both SSE
// and gRPC unchanged. This is the one setting that, if wrong, would
// silently break every dialect rule for gRPC frames.
var jsonMarshaler = &jsonpb.Marshaler{OrigName: true}

// startStream discovers cfg.Method via reflection and opens either a
// server-stream (the common case, request built from cfg.Request) or a
// bidi stream (Send pushes further client messages after this returns),
// per what the method descriptor says — not a separate config flag
// duplicating information the descriptor already has. Either way it
// starts the read-loop goroutine that decodes responses into Frames().
// Called from Connect; conn must already be dialed.
func (t *Transport) startStream(ctx context.Context) error {
	md, err := Discover(ctx, t.conn, t.cfg.Method)
	if err != nil {
		return err
	}

	stub := grpcdynamic.NewStub(t.conn)

	if md.IsClientStreaming() && md.IsServerStreaming() {
		bidi, err := stub.InvokeRpcBidiStream(t.outgoingContext(ctx), md)
		if err != nil {
			return fmt.Errorf("grpc: start bidi stream for %s: %w", t.cfg.Method, err)
		}
		t.sendMethod = md
		t.sendStream = bidi
		t.wg.Add(1)
		go t.readLoop(md, bidi)
		return nil
	}

	if !md.IsServerStreaming() {
		return fmt.Errorf("grpc: method %q is neither server-streaming nor bidi-streaming (this transport only supports those two shapes)", t.cfg.Method)
	}

	req := dynamic.NewMessage(md.GetInputType())
	for name, val := range t.cfg.Request {
		if err := req.TrySetFieldByName(name, val); err != nil {
			return fmt.Errorf("grpc: set request field %q: %w", name, err)
		}
	}

	stream, err := stub.InvokeRpcServerStream(t.outgoingContext(ctx), md, req)
	if err != nil {
		return fmt.Errorf("grpc: start stream for %s: %w", t.cfg.Method, err)
	}

	t.wg.Add(1)
	go t.readLoop(md, stream)
	return nil
}

// Send pushes payload — protojson bytes, mirroring the decode direction
// in reverse — as a new client message on a bidi-streaming method's
// stream. For a server-streaming Config (the common case per
// architecture.md's "Transport.Send is a no-op for server-streaming
// dialects" framing), it always returns a clear, typed-enough-to-assert-
// on error, matching internal/transport/sse.Send's pattern for a
// transport whose request is fixed at Connect time — never a silent
// no-op. Which behavior applies is decided from t.sendStream, set only
// when startStream's descriptor check found a bidi method; there is no
// separate config flag duplicating that information.
func (t *Transport) Send(ctx context.Context, payload []byte) error {
	if t.sendStream == nil {
		return fmt.Errorf("grpc: send not supported — %s is not a bidi-streaming method", t.cfg.Method)
	}
	req := dynamic.NewMessage(t.sendMethod.GetInputType())
	if err := req.UnmarshalJSONPB(&jsonpb.Unmarshaler{}, payload); err != nil { //nolint:staticcheck // SA1019: see jsonMarshaler's doc comment — same jsonpb/dynamic pairing, reverse direction
		return fmt.Errorf("grpc: unmarshal send payload: %w", err)
	}
	if err := t.sendStream.SendMsg(req); err != nil {
		return fmt.Errorf("grpc: send: %w", err)
	}
	return nil
}

// readLoop drains stream, marshaling each response to protojson-shaped
// bytes and emitting one Frame per message, until the stream ends,
// errors, or Close signals shutdown. It is the sole writer to and closer
// of t.frames — see Close's doc comment for why that makes a
// send-after-close race impossible, and why every send here goes through
// emit's select rather than a bare channel send (a full, undrained
// buffer would otherwise deadlock Close forever waiting on this
// goroutine, since closing the conn only unblocks a goroutine blocked on
// network I/O, not one blocked on an unconsumed channel).
func (t *Transport) readLoop(md *desc.MethodDescriptor, stream recvStream) {
	defer t.wg.Done()
	defer close(t.frames)

	// Used only for error frames, where there's no successfully-decoded
	// message to resolve a discriminator-mode name from.
	fallbackName := md.GetOutputType().GetFullyQualifiedName()

	emit := func(f transport.Frame) bool {
		select {
		case t.frames <- f:
			return true
		case <-t.closed:
			return false
		}
	}

	for {
		msg, err := stream.RecvMsg()
		if err != nil {
			t.emitStreamEnd(stream, err, emit)
			return
		}

		dm, ok := msg.(*dynamic.Message)
		if !ok {
			if !emit(transport.Frame{Name: fallbackName, Timestamp: time.Now(), Err: fmt.Errorf("grpc: unexpected response message type %T", msg)}) {
				return
			}
			continue
		}

		name, toMarshal := t.resolveFrame(md, dm)

		data, err := toMarshal.MarshalJSONPB(jsonMarshaler)
		if err != nil {
			if !emit(transport.Frame{Name: name, Timestamp: time.Now(), Err: fmt.Errorf("grpc: marshal response to JSON: %w", err)}) {
				return
			}
			continue
		}

		if !emit(transport.Frame{
			Name:      name,
			Data:      data,
			Raw:       data,
			Timestamp: time.Now(),
		}) {
			return
		}
	}
}

// grpcStatusFrame is the JSON shape emitStreamEnd marshals into the
// synthetic grpc_status Frame's Data — a real, documented field a dialect
// rule can `match: {event: grpc_status}` against and pull fields out of
// (fields: {grpc_code: code, grpc_message: message, trailers: trailers}),
// exactly like any other named event.
type grpcStatusFrame struct {
	Type     string            `json:"type"`
	Code     string            `json:"code"`
	Message  string            `json:"message"`
	Trailers map[string]string `json:"trailers"`
}

// emitStreamEnd runs once RecvMsg has returned its terminal error — EOF
// for a clean end, otherwise the stream's actual failure — and decides
// whether to emit the synthetic grpc_status Frame: on a non-OK status, or
// on trailers sent even alongside a clean (OK/EOF) end. A genuinely clean
// end with no trailers emits nothing, same as before this existed.
//
// This is deliberately NOT Frame.Err: Err is reserved for transport-level
// malformation (unreadable bytes, an unexpected message shape — see the
// "unexpected response message type" and marshal-failure frames above)
// that no dialect rule could ever make sense of. A non-OK status is a
// well-formed thing the server intentionally sent; it belongs in the same
// Frame/Event pipeline as every other message, so a dialect — not this
// transport — decides what Kind it becomes. Frame.Name is always the
// fixed grpcStatusEventName sentinel here, never resolved via
// t.cfg.Discriminator: there is no decoded response message for oneof/
// field:type/message_type to operate on, only a status the RPC itself
// terminated with.
func (t *Transport) emitStreamEnd(stream recvStream, recvErr error, emit func(transport.Frame) bool) {
	st, ok := status.FromError(recvErr)
	if recvErr == io.EOF {
		st, ok = status.New(codes.OK, ""), true
	}
	trailers := stream.Trailer()
	if ok && st.Code() == codes.OK && len(trailers) == 0 {
		return
	}

	flatTrailers := make(map[string]string, len(trailers))
	for k, vals := range trailers {
		flatTrailers[k] = strings.Join(vals, ",")
	}

	data, err := json.Marshal(grpcStatusFrame{
		Type:     grpcStatusEventName,
		Code:     st.Code().String(),
		Message:  st.Message(),
		Trailers: flatTrailers,
	})
	if err != nil {
		emit(transport.Frame{Name: grpcStatusEventName, Timestamp: time.Now(), Err: fmt.Errorf("grpc: marshal status frame: %w", err)})
		return
	}
	emit(transport.Frame{Name: grpcStatusEventName, Data: data, Raw: data, Timestamp: time.Now()})
}

// resolveFrame computes Frame.Name and which message to marshal as
// Frame.Data, per t.cfg.Discriminator — gRPC's one genuine impedance
// mismatch with SSE's native event: name. The resolved name feeds the
// SAME unmodified mapping.Engine a discriminator: event dialect already
// expects from any transport; this function is the entire seam, and
// internal/mapping never learns gRPC exists.
//
// For "oneof" specifically, toMarshal is the unwrapped oneof member, not
// the outer wrapper message: a wrapper's protojson would nest every
// field one level down under the case name
// (e.g. {"agent_content":{"agent_id":"a", ...}}), which would silently
// break a dialect field path like `source: agent_id` — that path only
// works flat, exactly like SSE's data: line. Unwrapping is what makes
// "the same dialect YAML runs over SSE and gRPC unchanged" actually true
// for the oneof case, not just the happy path on paper. A scalar
// (non-message) oneof member has no fields to unwrap into, so it stays
// nested under its case name regardless — there's no flatter shape for a
// single scalar value to take.
//
// If the response message declares more than one top-level oneof (the
// auto-default only ever picks "oneof" mode when there's exactly one,
// but an explicit discriminator: oneof can still name a message with
// several), every oneof is checked in declaration order and the first
// one with a field actually populated wins — not just the first
// declared — since oneof groups are independent of each other and any
// of them could be the one a given message actually set.
func (t *Transport) resolveFrame(md *desc.MethodDescriptor, dm *dynamic.Message) (name string, toMarshal *dynamic.Message) {
	mode := t.cfg.Discriminator
	if mode == "" {
		// Auto: the design's stated happy path — a method whose response
		// has exactly one top-level oneof discriminates by its case name
		// by default; anything else falls back to message_type, the
		// safest generic default (never empty, never ambiguous).
		if len(md.GetOutputType().GetOneOfs()) == 1 {
			mode = "oneof"
		} else {
			mode = "message_type"
		}
	}

	switch mode {
	case "oneof":
		oneofs := md.GetOutputType().GetOneOfs()
		if len(oneofs) == 0 {
			return md.GetOutputType().GetFullyQualifiedName(), dm
		}
		var fd *desc.FieldDescriptor
		var val any
		for _, od := range oneofs {
			if candidateFd, candidateVal := dm.GetOneOfField(od); candidateFd != nil {
				fd, val = candidateFd, candidateVal
				break
			}
		}
		if fd == nil {
			return "", dm // every oneof declared but nothing populated — proto3 allows this
		}
		if inner, ok := val.(*dynamic.Message); ok {
			return fd.GetName(), inner
		}
		// A scalar (non-message) oneof case: nothing to unwrap.
		return fd.GetName(), dm
	case "field:type":
		v, err := dm.TryGetFieldByName(t.cfg.DiscriminatorField)
		if err != nil {
			return "", dm
		}
		s, _ := v.(string)
		return s, dm
	case "message_type":
		return md.GetOutputType().GetFullyQualifiedName(), dm
	default: // "none"
		return "", dm
	}
}
