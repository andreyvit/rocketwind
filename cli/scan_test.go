package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestScanManifest(t *testing.T) {
	for _, compat := range []string{"3", "4"} {
		t.Run(compat, func(t *testing.T) {
			root := t.TempDir()
			write(t, root, "page.gox", `<div class="flex grid flex application-card"></div>`)
			write(t, root, "input.css", `@tailwind utilities;`)
			config := `{"content":["./*.gox"],"safelist":["underline"],"blocklist":["hidden"]}`
			if compat == "4" {
				write(t, root, "input.css", `@import "tailwindcss" source(none); @source "./*.gox"; @source inline("underline"); @source not inline("hidden");`)
				config = `{}`
			}
			write(t, root, "theme.json", config)
			write(t, root, "existing.css", `.grid {display:grid}`)
			var out, stderr bytes.Buffer
			args := []string{"scan", "--cwd", root, "--compat", compat, "--config", "theme.json", "-i", "input.css", "-o", "classes.txt", "--exclude", "existing.css", "--classes", "hidden flex italic"}
			if code := Run(context.Background(), args, Options{Stdout: &out, Stderr: &stderr}); code != 0 {
				t.Fatal(code, stderr.String())
			}
			data, err := os.ReadFile(filepath.Join(root, "classes.txt"))
			if err != nil {
				t.Fatal(err)
			}
			names := strings.Fields(string(data))
			if !slices.IsSorted(names) || !slices.Equal(names, slices.Compact(slices.Clone(names))) || !strings.HasSuffix(string(data), "\n") {
				t.Fatal(string(data))
			}
			for _, name := range []string{"flex", "underline", "italic", "application-card"} {
				if !slices.Contains(names, name) {
					t.Fatal(name, names)
				}
			}
			for _, name := range []string{"hidden", "grid"} {
				if slices.Contains(names, name) {
					t.Fatal(name, names)
				}
			}
			out.Reset()
			stderr.Reset()
			knownArgs := append(slices.Clone(args), "--known", "-o", "-")
			if code := Run(context.Background(), knownArgs, Options{Stdout: &out, Stderr: &stderr}); code != 0 {
				t.Fatal(code, stderr.String())
			}
			if got := strings.Fields(out.String()); !slices.Equal(got, []string{"flex", "italic", "underline"}) {
				t.Fatal(got)
			}
			// The manifest remains useful to an application that compiles CSS at startup.
			args = []string{"--cwd", root, "--compat", compat, "--config", "theme.json", "-i", "input.css", "--classes", string(data)}
			out.Reset()
			stderr.Reset()
			if code := Run(context.Background(), args, Options{Stdout: &out, Stderr: &stderr}); code != 0 {
				t.Fatal(code, stderr.String())
			}
			if !strings.Contains(out.String(), ".italic") || !strings.Contains(out.String(), ".underline") {
				t.Fatal(out.String())
			}
		})
	}
	root := t.TempDir()
	write(t, root, "input.css", `@import "tailwindcss" source(none);`)
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"scan", "--cwd", root, "-i", "input.css", "--silent"}, Options{Stdout: &out, Stderr: &stderr}); code != 0 || out.Len() != 0 {
		t.Fatal(code, out.String(), stderr.String())
	}
	if code := Run(context.Background(), []string{"scan", "--map"}, Options{Stdout: &out, Stderr: &stderr}); code == 0 {
		t.Fatal("scan accepted a source map")
	}
}

func TestScanWatch(t *testing.T) {
	root := t.TempDir()
	write(t, root, "input.css", `@import "tailwindcss" source(none); @source "*.gox"; @source inline("underline");`)
	write(t, root, "page.gox", `<div class="flex" />`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var out, stderr lockedBuffer
	done := make(chan int, 1)
	go func() {
		done <- Run(ctx, []string{"scan", "--cwd", root, "-i", "input.css", "-o", "classes.txt", "--watch=always", "--poll=10"}, Options{Stdout: &out, Stderr: &stderr})
	}()
	defer func() {
		cancel()
		if code := <-done; code != 0 {
			t.Error(code, stderr.String())
		}
	}()
	read := func() []string {
		data, _ := os.ReadFile(filepath.Join(root, "classes.txt"))
		return strings.Fields(string(data))
	}
	await(t, func() bool { return slices.Contains(read(), "flex") })
	write(t, root, "next.gox", `<div class="grid" />`)
	await(t, func() bool { return slices.Contains(read(), "grid") })
	write(t, root, "input.css", `@import "tailwindcss" source(none); @source "*.gox"; @source inline("overline");`)
	await(t, func() bool {
		names := read()
		return slices.Contains(names, "overline") && !slices.Contains(names, "underline")
	})
}

func TestLegacyContentNegativeGlobs(t *testing.T) {
	for _, compat := range []string{"3", "4"} {
		root := t.TempDir()
		write(t, root, "views/page.html", `<div class="flex"></div>`)
		write(t, root, "assets/unused.css", `.w-7 {width:1.75rem}`)
		write(t, root, "views/skip.html", `<div class="z-50"></div>`)
		write(t, root, "theme.json", `{"content":["./views/**/*","./assets/**/*","!**/*.css",{"base":"`+filepath.ToSlash(root)+`","pattern":"!views/skip.html"}]}`)
		write(t, root, "input.css", `@tailwind utilities;`)
		var out, stderr bytes.Buffer
		args := []string{"scan", "--known", "--cwd", root, "--compat", compat, "--config", "theme.json", "-i", "input.css"}
		if code := Run(context.Background(), args, Options{Stdout: &out, Stderr: &stderr}); code != 0 || out.String() != "flex\n" {
			t.Fatal(compat, code, out.String(), stderr.String())
		}
	}
}
