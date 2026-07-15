package grpc

import (
	"context"
	"fmt"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/reflect/protoreflect"

	// desc is deprecated in favor of protoreflect — but WrapMethod is the
	// documented bridge from protoreflect.MethodDescriptor to the legacy
	// *desc.MethodDescriptor grpcdynamic requires, the same currency
	// requirement reflect.go's Discover and descriptorset.go's
	// LoadDescriptorSet have. protocompile is itself the explicitly
	// mandated non-deprecated successor to protoreflect's own desc/
	// protoparse — used here directly, not the deprecated path.
	"github.com/jhump/protoreflect/desc" //nolint:staticcheck // SA1019: see above
)

// LoadProtoFile compiles protoPath at runtime via bufbuild/protocompile —
// the lowest-priority, "ship if cheap" fallback tier for servers where
// neither reflection nor a pre-compiled descriptor set is available.
// importPaths are searched for protoPath and its own imports, mirroring
// protoc's -I/--proto_path (protoPath itself is relative to one of
// them, or the working directory if importPaths is empty); well-known
// imports like google/protobuf/any.proto resolve automatically via
// protocompile.WithStandardImports, with no local copy needed.
//
// Produces the SAME *desc.MethodDescriptor type Discover and
// LoadDescriptorSet return: every step past acquisition takes that type
// and never learns which tier produced it.
func LoadProtoFile(ctx context.Context, protoPath string, importPaths []string, method string) (*desc.MethodDescriptor, error) {
	serviceName, methodName, err := splitMethod(method)
	if err != nil {
		return nil, err
	}

	compiler := protocompile.Compiler{
		Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
			ImportPaths: importPaths,
		}),
	}
	files, err := compiler.Compile(ctx, protoPath)
	if err != nil {
		return nil, fmt.Errorf("grpc: compile %q: %w", protoPath, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("grpc: compiling %q produced no file", protoPath)
	}

	d := files[0].FindDescriptorByName(protoreflect.FullName(serviceName))
	if d == nil {
		return nil, fmt.Errorf("grpc: service %q not found in %q", serviceName, protoPath)
	}
	sd, ok := d.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, fmt.Errorf("grpc: %q in %q is a %T, not a service", serviceName, protoPath, d)
	}
	md := sd.Methods().ByName(protoreflect.Name(methodName))
	if md == nil {
		return nil, fmt.Errorf("grpc: method %q not found on service %q in %q", methodName, serviceName, protoPath)
	}

	wrapped, err := desc.WrapMethod(md)
	if err != nil {
		return nil, fmt.Errorf("grpc: wrap method descriptor for %s: %w", method, err)
	}
	return wrapped, nil
}
