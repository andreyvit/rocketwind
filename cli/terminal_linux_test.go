//go:build linux

package cli

import (
	"os"
	"testing"
)

func TestTerminalPTY(t *testing.T) {
	pty, err := os.OpenFile("/dev/ptmx", os.O_RDWR, 0)
	if err != nil {
		t.Skipf("PTY unavailable: %v", err)
	}
	defer pty.Close()
	if !terminalFile(pty) {
		t.Fatal("PTY not recognized as a terminal")
	}
}
