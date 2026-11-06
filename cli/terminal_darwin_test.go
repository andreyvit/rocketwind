package cli

import (
	"bytes"
	"os"
	"syscall"
	"testing"
	"unsafe"
)

func TestTerminalPTY(t *testing.T) {
	master, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	for _, request := range []uintptr{syscall.TIOCPTYGRANT, syscall.TIOCPTYUNLK} {
		if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), request, 0); err != 0 {
			t.Fatal(err)
		}
	}
	var name [128]byte
	if _, _, err := syscall.Syscall(syscall.SYS_IOCTL, master.Fd(), syscall.TIOCPTYGNAME, uintptr(unsafe.Pointer(&name[0]))); err != 0 {
		t.Fatal(err)
	}
	slave, err := os.OpenFile(string(name[:bytes.IndexByte(name[:], 0)]), os.O_RDWR|syscall.O_NOCTTY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer slave.Close()
	if !terminalFile(slave) {
		t.Fatal("PTY not recognized as a terminal")
	}
}
