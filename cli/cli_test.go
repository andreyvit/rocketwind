package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/andreyvit/rocketwind"
)

func TestCompileFilesAndExclusions(t *testing.T) {
	root := t.TempDir()
	write(t, root, "src/input.css", `@import "tailwindcss"; @import "./card.css";`)
	write(t, root, "src/card.css", `.card {background:url(./card.png)}`)
	write(t, root, "views/template.gox", `<div class="flex p-big hover:text-brand"></div>`)
	write(t, root, "admin.css", `.flex {display:flex}`)
	write(t, root, "theme.json", `{"theme":{"extend":{"spacing":{"big":"27rem"},"colors":{"brand":"purple"}}}}`)
	var out, stderr bytes.Buffer
	args := []string{"build", "--cwd", root, "-i", "src/input.css", "-o", "dist/result.css", "--config", "theme.json", "--exclude", "admin.css", "--content", "views/**/*", "--classes", "grid"}
	if code := Run(context.Background(), args, Options{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &stderr}); code != 0 {
		t.Fatal(code, stderr.String())
	}
	data, err := os.ReadFile(filepath.Join(root, "dist/result.css"))
	if err != nil {
		t.Fatal(err)
	}
	css := string(data)
	for _, want := range []string{".grid {", ".p-big {", "27rem", "purple", ".hover\\:text-brand", "url(./card.png)"} {
		if !strings.Contains(css, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(css, ".flex {") || out.Len() != 0 || !strings.Contains(stderr.String(), "Done in") {
		t.Fatal(css, out.String(), stderr.String())
	}
}

func TestStdinStdoutAndProgrammaticPlugins(t *testing.T) {
	root := t.TempDir()
	write(t, root, "index.html", `<div class="flex brand"></div>`)
	var out, stderr bytes.Buffer
	modules := map[string]rocketwind.Module{"custom": {Plugin: func(api *rocketwind.PluginAPI) error {
		return api.AddUtilities([]*rocketwind.Node{{Kind: "rule", Name: ".brand", Nodes: []*rocketwind.Node{{Kind: "declaration", Name: "color", Value: "red"}}}})
	}}}
	if code := Run(context.Background(), []string{"--cwd", root, "--input=-", "--output=-", "--silent"}, Options{Stdin: strings.NewReader(`@plugin "custom"; @tailwind utilities;`), Stdout: &out, Stderr: &stderr, Modules: modules}); code != 0 {
		t.Fatal(code, stderr.String())
	}
	if !strings.Contains(out.String(), ".flex {") || !strings.Contains(out.String(), "color: red") || stderr.Len() != 0 {
		t.Fatal(out.String(), stderr.String())
	}
	for _, args := range [][]string{{"--help"}, {"build", "-h"}, {"--version"}} {
		out.Reset()
		stderr.Reset()
		if code := Run(context.Background(), args, Options{Stdout: &out, Stderr: &stderr}); code != 0 || !strings.Contains(out.String(), "Rocketwind") {
			t.Fatal(code, out.String(), stderr.String())
		}
	}
}

func TestCommandErrors(t *testing.T) {
	root := t.TempDir()
	write(t, root, "input.css", `@tailwind utilities;`)
	write(t, root, "bad.css", `@theme {color:red}`)
	write(t, root, "bad-existing.css", `.invalid {`)
	for _, args := range [][]string{
		{"-i"}, {"--poll=0"}, {"--poll=foo"},
		{"--input", "missing.css"}, {"--input", "input.css", "--output", "./input.css"},
		{"--input", "bad.css"}, {"--exclude", "missing.css"}, {"--cwd", filepath.Join(root, "missing")},
		{"--exclude", "bad-existing.css"},
		{"--input", "input.css", "--output", "input.css/child"},
	} {
		var out, stderr bytes.Buffer
		if code := Run(context.Background(), append([]string{"--cwd", root}, args...), Options{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &stderr}); code == 0 || stderr.Len() == 0 {
			t.Fatal(args, code, stderr.String())
		}
	}
	write(t, root, "root.css", `@tailwind utilities source("./absent");`)
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"--cwd", root, "-i", "root.css"}, Options{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &stderr}); code == 0 || !strings.Contains(stderr.String(), "source path") {
		t.Fatal(code, stderr.String())
	}
	stderr.Reset()
	if code := Run(context.Background(), []string{"--cwd", root, "-i", "-"}, Options{Stdin: failedIO{}, Stdout: &out, Stderr: &stderr}); code == 0 {
		t.Fatal("stdin error lost")
	}
	stderr.Reset()
	if code := Run(context.Background(), []string{"--cwd", root, "--classes", "flex"}, Options{Stdin: strings.NewReader(""), Stdout: failedIO{}, Stderr: &stderr}); code == 0 {
		t.Fatal("stdout error lost")
	}
}

func TestWatchRebuildsContentAndDependencies(t *testing.T) {
	root := t.TempDir()
	write(t, root, "input.css", `@import "./theme.css"; @tailwind utilities;`)
	write(t, root, "theme.css", `@theme {--color-brand: red}`)
	write(t, root, "index.gox", `<div class="flex text-brand"></div>`)
	write(t, root, "admin.css", `.grid {display:grid}`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var stderr lockedBuffer
	done := make(chan int, 1)
	go func() {
		done <- Run(ctx, []string{"--cwd", root, "-i", "input.css", "-o", "out.css", "--exclude", "admin.css", "--watch=always", "--poll=10"}, Options{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: &stderr})
	}()
	defer func() {
		cancel()
		if code := <-done; code != 0 {
			t.Error(code, stderr.String())
		}
	}()
	read := func() string { data, _ := os.ReadFile(filepath.Join(root, "out.css")); return string(data) }
	await(t, func() bool { return strings.Contains(read(), ".flex {") })
	write(t, root, "new.gox", `<div class="block grid"></div>`)
	await(t, func() bool { return strings.Contains(read(), ".block {") })
	if strings.Contains(read(), ".grid {") {
		t.Fatal("existing grid emitted")
	}
	before := read()
	write(t, root, "theme.css", `@theme {color:red}`)
	await(t, func() bool { return strings.Contains(stderr.String(), "@theme") })
	if read() != before {
		t.Fatal("failed rebuild replaced prior CSS")
	}
	write(t, root, "theme.css", `@theme {--color-brand: blue}`)
	await(t, func() bool { return strings.Contains(read(), "--color-brand: blue") })
	write(t, root, "admin.css", ``)
	await(t, func() bool { return strings.Contains(read(), ".grid {") })
	first, _ := os.Stat(filepath.Join(root, "out.css"))
	write(t, root, "index.gox", `<div class="flex text-brand"><!-- unrelated edit --></div>`)
	// Wait for a later scan without requiring an output write.
	await(t, func() bool { return time.Since(first.ModTime()) > 50*time.Millisecond })
	last, _ := os.Stat(filepath.Join(root, "out.css"))
	if !first.ModTime().Equal(last.ModTime()) {
		t.Fatal("unchanged CSS was rewritten")
	}
	before = read()
	if err := os.Remove(filepath.Join(root, "input.css")); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return strings.Contains(stderr.String(), "no such file") })
	if read() != before {
		t.Fatal("removed input replaced the last successful output")
	}
	write(t, root, "input.css", `@tailwind utilities; @source inline("justify-center");`)
	await(t, func() bool { return strings.Contains(read(), ".justify-center") })
}

func TestWatchStopsWhenStdinCloses(t *testing.T) {
	root := t.TempDir()
	write(t, root, "index.html", `<div class="flex"></div>`)
	for _, args := range [][]string{{"--watch"}, {"--watch", "true"}} {
		if len(args) > 1 {
			args = []string{"--watch=true"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		var out, stderr bytes.Buffer
		code := Run(ctx, append([]string{"--cwd", root, "--poll", "20"}, args...), Options{Stdin: strings.NewReader(""), Stdout: &out, Stderr: &stderr})
		if ctx.Err() != nil || code != 0 {
			t.Error(code, stderr.String(), ctx.Err())
		}
		cancel()
	}
}

func TestOutputPreservesSymlinksAndPermissions(t *testing.T) {
	root := t.TempDir()
	write(t, root, "actual.css", "old")
	actual, link := filepath.Join(root, "actual.css"), filepath.Join(root, "linked.css")
	if err := os.Chmod(actual, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(actual, link); err != nil {
		t.Fatal(err)
	}
	if err := outputFile(link, "new"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(actual)
	if err != nil || string(data) != "new" {
		t.Fatal(string(data), err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatal("output symlink replaced", err)
	}
	info, err = os.Stat(actual)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("permissions changed", err)
	}
	if err := os.Remove(actual); err != nil {
		t.Fatal(err)
	}
	if err := outputFile(link, "restored"); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(actual)
	if err != nil || string(data) != "restored" {
		t.Fatal(string(data), err)
	}
}

type failedIO struct{}

func (failedIO) Read([]byte) (int, error)  { return 0, errors.New("read failure") }
func (failedIO) Write([]byte) (int, error) { return 0, errors.New("write failure") }

type lockedBuffer struct {
	sync.Mutex
	bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.Write(p)
}
func (b *lockedBuffer) String() string { b.Lock(); defer b.Unlock(); return b.Buffer.String() }
func (b *lockedBuffer) WriteString(s string) (int, error) {
	b.Lock()
	defer b.Unlock()
	return b.Buffer.WriteString(s)
}
func write(t *testing.T, root, name, text string) {
	t.Helper()
	name = filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}
func await(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for rebuild")
		}
		time.Sleep(5 * time.Millisecond)
	}
}
