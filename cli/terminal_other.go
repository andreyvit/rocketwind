//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !windows

package cli

func terminalDescriptor(fd uintptr) bool { return false }
