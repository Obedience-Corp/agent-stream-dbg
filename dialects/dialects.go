// Package dialects embeds the dialect YAML documents shipped with the
// agent-stream-dbg binary.
package dialects

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// DefaultName is the dialect used when a caller does not provide a source.
const DefaultName = "brainyard"

// FS contains every shipped dialect document. Keeping the embed point in the
// package that owns the YAML files makes the binary independent of its build
// machine's source tree.
//
//go:embed *.yaml
var FS embed.FS

// Available returns the names of all embedded dialects, without the .yaml
// suffix, in stable order.
func Available() []string {
	paths, err := fs.Glob(FS, "*.yaml")
	if err != nil {
		return nil
	}
	names := make([]string, 0, len(paths))
	for _, name := range paths {
		names = append(names, strings.TrimSuffix(path.Base(name), ".yaml"))
	}
	sort.Strings(names)
	return names
}

// Load reads an embedded dialect by bare name. A .yaml suffix and path
// components are accepted for convenience; explicit filesystem paths are
// resolved by the bridge loader before this function is called.
func Load(name string) ([]byte, error) {
	requested := strings.TrimSpace(name)
	if requested == "" {
		requested = DefaultName
	}
	base := path.Base(strings.ReplaceAll(requested, "\\", "/"))
	base = strings.TrimSuffix(base, ".yaml")
	data, err := FS.ReadFile(base + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("embedded dialect %q not found; available embedded dialects: %s", name, strings.Join(Available(), ", "))
	}
	return data, nil
}
