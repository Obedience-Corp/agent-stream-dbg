package grpc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	// desc is marked deprecated in favor of google.golang.org/protobuf's
	// upstream protoreflect API — but grpcreflect.Client.ResolveService
	// and grpcdynamic.Stub's InvokeRpc*Stream methods (both explicitly
	// mandated by this phase's design, see architecture.md's gRPC
	// section) still take *desc.ServiceDescriptor/*desc.MethodDescriptor
	// as their public API in the current stable release (v1.18.0, Jan
	// 2026); neither has been ported to the upstream protoreflect types.
	// There is no version of this exact combination — reflection
	// discovery feeding a dynamic invocation — that avoids desc. Using it
	// here is the necessary currency between those two packages, not an
	// oversight.
	"github.com/jhump/protoreflect/desc" //nolint:staticcheck // SA1019: see above; grpcdynamic requires this type
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrReflectionUnavailable identifies "the server doesn't support
// reflection" distinctly from "reflection works but that service/method
// doesn't exist" — the exact condition the descriptor-set fallback
// (sequence 05) needs to detect and react to, not just a generic
// wrapped error string.
var ErrReflectionUnavailable = errors.New("grpc: server reflection unavailable")

// ErrReflectionTransient identifies a temporary connectivity failure while
// contacting reflection, distinct from a server that deliberately does not
// register the reflection service.
var ErrReflectionTransient = errors.New("grpc: reflection connection transient")

// Discover resolves method — a fully-qualified RPC path shaped like
// "/agent.v1.AgentService/StreamSession" — to its *desc.MethodDescriptor
// via server reflection. Zero generated code, zero SDK: this is what
// makes "point it at your endpoint and look" true for gRPC.
//
// The returned descriptor is exactly what grpcdynamic.Stub's
// InvokeRpc*Stream methods expect (sequence 03) — protoreflect's own
// desc/protoparse text-parsing path is deprecated in favor of
// bufbuild/protocompile, but grpcreflect and grpcdynamic still compose
// through desc.MethodDescriptor as their shared currency; that's what
// this returns rather than a bare protoreflect.MethodDescriptor.
func Discover(ctx context.Context, conn *grpc.ClientConn, method string) (*desc.MethodDescriptor, error) {
	serviceName, methodName, err := splitMethod(method)
	if err != nil {
		return nil, err
	}

	client := grpcreflect.NewClientAuto(ctx, conn)
	defer client.Reset()

	svc, err := client.ResolveService(serviceName)
	if err != nil {
		switch classifyReflectionError(err) {
		case reflectionNoService:
			return nil, fmt.Errorf("%w (target has no reflection.Register(s) — use --descriptor-set or a .proto instead): %w", ErrReflectionUnavailable, err)
		case reflectionTransient:
			return nil, fmt.Errorf("%w (server unreachable at target %q while contacting reflection): %w", ErrReflectionTransient, conn.Target(), err)
		}
		return nil, fmt.Errorf("grpc: resolve service %q via reflection: %w", serviceName, err)
	}

	md := svc.FindMethodByName(methodName)
	if md == nil {
		return nil, fmt.Errorf("grpc: method %q not found on service %q (reflection worked; check the configured method path)", methodName, serviceName)
	}
	return md, nil
}

type reflectionErrorClass string

const (
	reflectionOther     reflectionErrorClass = "other"
	reflectionNoService reflectionErrorClass = "no_service"
	reflectionTransient reflectionErrorClass = "transient"
)

func classifyReflectionError(err error) reflectionErrorClass {
	switch status.Code(err) {
	case codes.Unimplemented:
		return reflectionNoService
	case codes.Unavailable, codes.DeadlineExceeded:
		return reflectionTransient
	default:
		return reflectionOther
	}
}

// splitMethod parses "/pkg.Service/Method" into ("pkg.Service", "Method").
func splitMethod(method string) (serviceName, methodName string, err error) {
	trimmed := strings.TrimPrefix(method, "/")
	idx := strings.LastIndex(trimmed, "/")
	if idx < 0 || idx == len(trimmed)-1 {
		return "", "", fmt.Errorf("grpc: invalid method %q (want \"/package.Service/Method\")", method)
	}
	return trimmed[:idx], trimmed[idx+1:], nil
}
