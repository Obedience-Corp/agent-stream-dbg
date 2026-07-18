package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/jhump/protoreflect/desc" //nolint:staticcheck // grpcreflect and grpcdynamic use desc descriptors as their shared API
	"github.com/jhump/protoreflect/grpcreflect"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/descriptorpb"
)

// StreamingMethod is the schema information the config UI needs to turn a
// reflected RPC into a usable starter configuration. It intentionally
// contains values rather than descriptors so callers do not need to know
// about the reflection implementation or keep a connection alive.
type StreamingMethod struct {
	Service         string
	Name            string
	Path            string
	Shape           string
	InputType       string
	OutputType      string
	RequestJSON     string
	Discriminator   string
	ClientStreaming bool
	ServerStreaming bool
	Supported       bool
}

// DiscoverStreamingMethods lists streaming RPCs exposed by server
// reflection. It is deliberately reflection-only: descriptor-set and proto
// file fallbacks resolve a configured method, but cannot safely enumerate a
// user's intended RPC without an explicit service/method choice.
func DiscoverStreamingMethods(ctx context.Context, cfg Config) ([]StreamingMethod, error) {
	conn, err := dial(ctx, cfg)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if cfg.MetadataKey != "" {
		ctx = metadata.AppendToOutgoingContext(ctx, cfg.MetadataKey, cfg.MetadataValue)
	}
	reflectClient := grpcreflect.NewClientAuto(ctx, conn)
	defer reflectClient.Reset()

	serviceNames, err := reflectClient.ListServices()
	if err != nil {
		return nil, wrapReflectionListError(conn, err)
	}

	methods := make([]StreamingMethod, 0)
	for _, serviceName := range serviceNames {
		// Reflection registers its own service in the service list. It is an
		// implementation detail, not an agent stream a user can configure.
		if serviceName == "grpc.reflection.v1.ServerReflection" || serviceName == "grpc.reflection.v1alpha.ServerReflection" {
			continue
		}
		svc, err := reflectClient.ResolveService(serviceName)
		if err != nil {
			return nil, fmt.Errorf("grpc: resolve service %q via reflection: %w", serviceName, err)
		}
		for _, method := range svc.GetMethods() {
			if !method.IsClientStreaming() && !method.IsServerStreaming() {
				continue
			}
			requestJSON, err := requestExample(method.GetInputType())
			if err != nil {
				return nil, fmt.Errorf("grpc: build request example for %s/%s: %w", serviceName, method.GetName(), err)
			}
			methods = append(methods, StreamingMethod{
				Service:         string(serviceName),
				Name:            method.GetName(),
				Path:            "/" + serviceName + "/" + method.GetName(),
				Shape:           streamingShape(method.IsClientStreaming(), method.IsServerStreaming()),
				InputType:       method.GetInputType().GetFullyQualifiedName(),
				OutputType:      method.GetOutputType().GetFullyQualifiedName(),
				RequestJSON:     requestJSON,
				Discriminator:   responseDiscriminator(method.GetOutputType()),
				ClientStreaming: method.IsClientStreaming(),
				ServerStreaming: method.IsServerStreaming(),
				Supported:       method.IsServerStreaming(),
			})
		}
	}

	sort.Slice(methods, func(i, j int) bool { return methods[i].Path < methods[j].Path })
	return methods, nil
}

func wrapReflectionListError(conn *grpc.ClientConn, err error) error {
	switch classifyReflectionError(err) {
	case reflectionNoService:
		return fmt.Errorf("%w (target has no reflection.Register(s) — choose a method manually, or use --descriptor-set/--proto-file): %w", ErrReflectionUnavailable, err)
	case reflectionTransient:
		return fmt.Errorf("%w (server unreachable at target %q while contacting reflection): %w", ErrReflectionTransient, conn.Target(), err)
	default:
		return fmt.Errorf("grpc: list services via reflection: %w", err)
	}
}

func streamingShape(clientStreaming, serverStreaming bool) string {
	switch {
	case clientStreaming && serverStreaming:
		return "bidi-streaming"
	case serverStreaming:
		return "server-streaming"
	case clientStreaming:
		return "client-streaming"
	default:
		return "unary"
	}
}

func responseDiscriminator(message *desc.MessageDescriptor) string {
	oneofs := 0
	for _, oneof := range message.GetOneOfs() {
		if !oneof.IsSynthetic() {
			oneofs++
		}
	}
	if oneofs == 1 {
		return "oneof"
	}
	return "message_type"
}

// requestExample produces valid ProtoJSON-shaped starter data. Values are
// intentionally neutral: users still need to replace identifiers and
// required business fields, but they no longer need to know every field name
// before they can connect.
func requestExample(message *desc.MessageDescriptor) (string, error) {
	value := requestValue(message, map[string]bool{}, 0)
	data, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func requestValue(message *desc.MessageDescriptor, stack map[string]bool, depth int) map[string]any {
	result := make(map[string]any)
	if message == nil || depth > 4 {
		return result
	}
	name := message.GetFullyQualifiedName()
	if stack[name] {
		return result
	}
	stack[name] = true
	defer delete(stack, name)

	for _, field := range message.GetFields() {
		// Oneof request fields need a user-selected case. Including every case
		// would create invalid ProtoJSON, so leave the choice visible in the
		// schema picker rather than pretending a zero value is meaningful.
		if field.GetOneOf() != nil && !field.GetOneOf().IsSynthetic() {
			continue
		}
		if field.IsMap() {
			result[field.GetName()] = map[string]any{}
			continue
		}
		value := fieldExample(field, stack, depth)
		if field.IsRepeated() {
			result[field.GetName()] = []any{}
		} else {
			result[field.GetName()] = value
		}
	}
	return result
}

func fieldExample(field *desc.FieldDescriptor, stack map[string]bool, depth int) any {
	if field.GetMessageType() != nil {
		return requestValue(field.GetMessageType(), stack, depth+1)
	}
	if field.GetEnumType() != nil {
		values := field.GetEnumType().GetValues()
		if len(values) > 0 {
			return values[0].GetName()
		}
		return ""
	}
	switch field.GetType() {
	case descriptorpb.FieldDescriptorProto_TYPE_BOOL:
		return false
	case descriptorpb.FieldDescriptorProto_TYPE_BYTES,
		descriptorpb.FieldDescriptorProto_TYPE_STRING:
		return ""
	case descriptorpb.FieldDescriptorProto_TYPE_DOUBLE,
		descriptorpb.FieldDescriptorProto_TYPE_FLOAT:
		return 0.0
	case descriptorpb.FieldDescriptorProto_TYPE_UINT32,
		descriptorpb.FieldDescriptorProto_TYPE_UINT64,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED32,
		descriptorpb.FieldDescriptorProto_TYPE_FIXED64:
		return uint64(0)
	default:
		return int64(0)
	}
}
