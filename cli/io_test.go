package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingOutput struct {
	file   *os.File
	stage  string
	err    error
	closed bool
}

func (f *failingOutput) Name() string { return f.file.Name() }
func (f *failingOutput) Chmod(mode os.FileMode) error {
	if f.stage == "chmod" {
		return f.err
	}
	return f.file.Chmod(mode)
}
func (f *failingOutput) Write(data []byte) (int, error) {
	if f.stage == "write" {
		_, _ = f.file.Write(data[:1])
		return 1, f.err
	}
	return f.file.Write(data)
}
func (f *failingOutput) Close() error {
	f.closed = true
	err := f.file.Close()
	if f.stage == "close" {
		return f.err
	}
	return err
}

func TestOutputFailuresPreservePriorCSS(t *testing.T) {
	for _, stage := range []string{"create", "chmod", "write", "close"} {
		t.Run(stage, func(t *testing.T) {
			root := t.TempDir()
			name := filepath.Join(root, "app.css")
			if err := os.WriteFile(name, []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			boom := errors.New(stage + " failed")
			var file *failingOutput
			err := outputFileUsing(name, "new CSS", func(dir, pattern string) (outputTemp, error) {
				if stage == "create" {
					return nil, boom
				}
				f, err := os.CreateTemp(dir, pattern)
				if err != nil {
					return nil, err
				}
				file = &failingOutput{file: f, stage: stage, err: boom}
				return file, nil
			})
			if !errors.Is(err, boom) {
				t.Fatalf("lost failure: %v", err)
			}
			if file != nil && !file.closed {
				t.Fatal("temporary file left open")
			}
			data, err := os.ReadFile(name)
			if err != nil || string(data) != "old" {
				t.Fatalf("last successful output changed: %s %v", data, err)
			}
			entries, err := os.ReadDir(root)
			if err != nil || len(entries) != 1 || entries[0].Name() != "app.css" {
				t.Fatalf("temporary output left behind: %v %v", entries, err)
			}
		})
	}
}

func TestCompilerUnavailableWorkingDirectory(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	if err := os.Chmod(root, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0700)
	var stderr bytes.Buffer
	if code := Run(context.Background(), []string{"--cwd", "."}, Options{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: &stderr}); code == 0 || !strings.Contains(stderr.String(), "permission denied") {
		t.Fatalf("%d %s", code, stderr.String())
	}
}

func TestDefaultStderr(t *testing.T) {
	var out bytes.Buffer
	if code := Run(context.Background(), []string{"--help"}, Options{Stdout: &out}); code != 0 || !strings.Contains(out.String(), "Rocketwind") {
		t.Fatalf("%d %s", code, out.String())
	}
}

func TestCompilerSourceRootGlob(t *testing.T) {
	root := t.TempDir()
	write(t, root, "views/index.html", `<div class="flex"></div>`)
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"--cwd", root, "-i", "-"}, Options{Stdin: strings.NewReader(`@tailwind utilities source("./views/*.html");`), Stdout: &out, Stderr: &stderr}); code != 0 || !strings.Contains(out.String(), ".flex {") {
		t.Fatalf("%d %s\n%s", code, stderr.String(), out.String())
	}
}

func TestDiagnosticsAcrossVolumes(t *testing.T) {
	path := `D:\styles\app.css`
	if got := displayRelativePath(path, "", errors.New("different volumes"), false); got != path {
		t.Fatalf("lost original path: %q", got)
	}
}
