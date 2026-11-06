package rocketwind

import (
	"net"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
)

func TestScannerDisappearingEntries(t *testing.T) {
	root := scannerTestRoot(t)
	for _, name := range []string{"a.html", "b.html"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("flex"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	// ReadDir has already read b.html by the time a.html is visited.
	s := NewScanner([]Source{{Base: root, Pattern: "**/*"}})
	var visited []string
	s.walk(func(name, canonical string, info os.FileInfo) {
		if info.IsDir() {
			return
		}
		visited = append(visited, filepath.Base(name))
		if err := os.Remove(filepath.Join(root, "b.html")); err != nil {
			t.Fatal(err)
		}
	})
	if !reflect.DeepEqual(visited, []string{"a.html"}) {
		t.Fatal(visited)
	}
}

func TestScannerBrokenLinksAndSpecialFiles(t *testing.T) {
	root := scannerTestRoot(t)
	t.Chdir(root)
	if err := os.Symlink(filepath.Join(root, "absent.html"), filepath.Join(root, "broken.html")); err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("unix", "socket.html")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	s := NewScanner([]Source{{Base: root, Pattern: "**/*"}})
	if got := s.Files(); len(got) != 0 {
		t.Fatalf("invalid inputs: %v", got)
	}
}

func TestScannerPathRemovedAfterStat(t *testing.T) {
	for _, link := range []bool{false, true} {
		t.Run(map[bool]string{false: "root", true: "symlink"}[link], func(t *testing.T) {
			root := scannerTestRoot(t)
			target := filepath.Join(root, "target.html")
			if err := os.WriteFile(target, []byte("flex"), 0600); err != nil {
				t.Fatal(err)
			}
			source := Source{Base: root, Pattern: "target.html"}
			remove := root
			if link {
				remove = filepath.Join(root, "link.html")
				if err := os.Symlink(target, remove); err != nil {
					t.Fatal(err)
				}
				source.Pattern = "link.html"
			}
			s := NewScanner([]Source{source})
			var files []string
			s.walkWithStat(func(name, canonical string, info os.FileInfo) {
				if !info.IsDir() {
					files = append(files, name)
				}
			}, func(name string) (os.FileInfo, error) {
				info, err := os.Stat(name)
				if err == nil && name == remove {
					if err := os.RemoveAll(remove); err != nil {
						t.Fatal(err)
					}
				}
				return info, err
			})
			if len(files) != 0 {
				t.Fatalf("visited vanished input: %v", files)
			}
		})
	}
}

func TestScannerInvalidBraceRange(t *testing.T) {
	root := scannerTestRoot(t)
	name := "{1..3..0}"
	if err := os.WriteFile(filepath.Join(root, name), []byte("flex"), 0600); err != nil {
		t.Fatal(err)
	}
	s := NewScanner([]Source{{Base: root, Pattern: name}})
	if got := s.Scan(); !slices.Contains(got, "flex") {
		t.Fatalf("literal brace pattern did not match: %v", got)
	}
}

func TestWatchSymlinkRoot(t *testing.T) {
	root := scannerTestRoot(t)
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	s := NewScanner([]Source{{Base: root, Pattern: "**/*"}})
	s.Scan()
	if got := s.watchDirectory(link); !reflect.DeepEqual(got, []Glob{{link, "*"}}) {
		t.Fatal(got)
	}
}

func TestWatchStaticInputs(t *testing.T) {
	root := scannerTestRoot(t)
	file := filepath.Join(root, "app.html")
	if err := os.WriteFile(file, nil, 0600); err != nil {
		t.Fatal(err)
	}
	if got := optimizeScanGlobs([]Glob{{root, ""}}); !reflect.DeepEqual(got, []Glob{{root, "**/*"}}) {
		t.Fatal(got)
	}
	// Oxide exposes both the parent watcher and the original static file glob.
	if got := optimizeScanGlobs([]Glob{{root, "app.html"}}); !reflect.DeepEqual(got, []Glob{{root, "app.html"}, {file, ""}}) {
		t.Fatal(got)
	}
	got := optimizeScanGlobsWithStat([]Glob{{root, "*"}}, func(name string) (os.FileInfo, error) {
		if name != root {
			t.Fatal(name)
		}
		return nil, &os.PathError{Op: "stat", Path: name, Err: os.ErrNotExist}
	})
	if len(got) != 0 {
		t.Fatalf("watched vanished root: %v", got)
	}
}

func scannerTestRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}
