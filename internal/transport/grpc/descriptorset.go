package grpc

import (
	"fmt"
	"os"

	// desc is deprecated in favor of protoreflect — but WrapMethod is the
	// documented bridge from protodesc's protoreflect.MethodDescriptor to
	// the legacy *desc.MethodDescriptor grpcdynamic requires, the same
	// currency requirement reflect.go's Discover has. This tier starts on
	// the modern, non-deprecated protodesc/protoregistry APIs (parsing the
	// descriptor set) and only crosses into desc at the very last step, to
	// produce the exact same return type Discover's reflection path does.
	"github.com/jhump/protoreflect/desc" //nolint:staticcheck // SA1019: see above
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/descriptorpb"
)

// LoadDescriptorSet resolves method against a compiled FileDescriptorSet
// read from path — the fallback tier for servers with reflection
// disabled ("exactly where the hard bugs live", per architecture.md).
// Produces the SAME *desc.MethodDescriptor type Discover's reflection
// path returns: every step past acquisition (streaming, decode,
// discriminator, errors, send) takes that type and never learns which
// tier produced it.
//
// path is generated with `protoc --descriptor_set_out=... --include_imports`
// (or `buf build -o ...`) — see `just testdata-descriptorset` for the
// exact command this package's own tests use. --include_imports matters:
// without it, a .proto that imports another (like agentstream.proto's
// google/protobuf/any.proto) produces a set protodesc.NewFiles can't
// fully resolve.
func LoadDescriptorSet(path, method string) (*desc.MethodDescriptor, error) {
	serviceName, methodName, err := splitMethod(method)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("grpc: read descriptor set %q: %w", path, err)
	}

	var fdSet descriptorpb.FileDescriptorSet
	if err := proto.Unmarshal(data, &fdSet); err != nil {
		return nil, fmt.Errorf("grpc: parse descriptor set %q (want a compiled FileDescriptorSet, e.g. from `protoc --descriptor_set_out`): %w", path, err)
	}

	files, err := protodesc.NewFiles(&fdSet)
	if err != nil {
		return nil, fmt.Errorf("grpc: build file descriptors from %q: %w", path, err)
	}

	d, err := files.FindDescriptorByName(protoreflect.FullName(serviceName))
	if err != nil {
		return nil, fmt.Errorf("grpc: service %q not found in descriptor set %q: %w", serviceName, path, err)
	}
	sd, ok := d.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("grpc: %q in descriptor set %q is a %T, not a service", serviceName, path, d)
	}
	md := sd.Methods().ByName(protoreflect.Name(methodName))
	if md == nil {
		return nil, fmt.Errorf("grpc: method %q not found on service %q in descriptor set %q", methodName, serviceName, path)
	}

	wrapped, err := desc.WrapMethod(md)
	if err != nil {
		return nil, fmt.Errorf("grpc: wrap method descriptor for %s: %w", method, err)
	}
	return wrapped, nil
}
