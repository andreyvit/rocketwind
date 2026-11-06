package rocketwind

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilesystemUnreadableDependencies(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"theme.css", "theme.json"} {
		path := filepath.Join(root, name)
		writeTestSource(t, path, "{}")
		if err := os.Chmod(path, 0); err != nil {
			t.Fatal(err)
		}
	}
	fs := Filesystem{}
	if _, err := fs.LoadStylesheet("./theme.css", root); !os.IsPermission(err) {
		t.Fatalf("stylesheet: %v", err)
	}
	if _, err := fs.LoadModule("./theme.json", root, "config"); !os.IsPermission(err) {
		t.Fatalf("configuration: %v", err)
	}
}

func TestFilesystemUnavailableWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	writeTestSource(t, filepath.Join(root, "theme.css"), ".a{color:red}")
	cwd := filepath.Join(root, "removed")
	if err := os.Mkdir(cwd, 0755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(cwd)
	if err := os.Chmod(cwd, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(cwd, 0755)
	if _, err := (Filesystem{}).LoadStylesheet("./theme.css", ""); !os.IsPermission(err) {
		t.Fatalf("relative load: %v", err)
	}
	if _, err := (Filesystem{Root: "."}).LoadStylesheet("./theme.css", root); !os.IsPermission(err) {
		t.Fatalf("relative asset root: %v", err)
	}
}
