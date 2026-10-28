package file

import (
	"fmt"
	"os"
)

// OpenRegular rejects pipes/devices before a potentially blocking read. On Unix
// the open itself is nonblocking, so replacement with a FIFO cannot hang attach.
func OpenRegular(path string) (*os.File, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be a regular file", path)
	}
	f, err := os.OpenFile(path, os.O_RDONLY|regularOpenFlags, 0)
	if err != nil {
		return nil, err
	}
	actual, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !actual.Mode().IsRegular() {
		_ = f.Close()
		return nil, fmt.Errorf("%s must be a regular file", path)
	}
	return f, nil
}
