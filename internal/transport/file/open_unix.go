//go:build unix

package file

import "syscall"

const regularOpenFlags = syscall.O_NONBLOCK
