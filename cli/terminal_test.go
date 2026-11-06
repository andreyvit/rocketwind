package cli

import (
	"os"
	"strings"
	"testing"
)

func TestTerminalFiles(t *testing.T) {
	if terminalFile(strings.NewReader("")) {
		t.Fatal("reader is a terminal")
	}
	null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if terminalFile(null) {
		t.Fatal("null device is a terminal")
	}
	file, err := os.CreateTemp(t.TempDir(), "output")
	if err != nil {
		t.Fatal(err)
	}
	if terminalFile(file) {
		t.Fatal("ordinary file is a terminal")
	}
	file.Close()
	if terminalFile(file) {
		t.Fatal("closed file is a terminal")
	}
}
