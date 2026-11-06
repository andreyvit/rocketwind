//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package cli

import (
	"syscall"
	"unsafe"
)

func terminalDescriptor(fd uintptr) bool {
	var size [4]uint16
	_, _, err := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size)))
	return err == 0
}
