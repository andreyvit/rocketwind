package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andreyvit/rocketwind"
)

func TestSourceMaps(t *testing.T) {
	root := t.TempDir()
	write(t, root, "src/input.css", `@import "./theme.css"; @tailwind utilities;`)
	write(t, root, "src/theme.css", `@theme {--color-brand: red}`)
	write(t, root, "index.gox", `<div class="flex text-brand"></div>`)
	for _, mode := range []string{"inline", "file", "stdout-file", "stdin", "default", "disabled"} {
		t.Run(mode, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			args := []string{"--cwd", root, "-i", "src/input.css", "--silent"}
			switch mode {
			case "inline":
				args = append(args, "--map", "-o", "out.css")
			case "file":
				args = append(args, "--map", "maps/out.css.map", "-o", "dist/out.css")
			case "stdout-file":
				args = append(args, "--map=maps/stdout.css.map")
			case "stdin":
				args = []string{"--cwd", root, "-i", "-", "--map=true", "--silent"}
			case "default":
				args = []string{"--cwd", root, "--map", "--silent"}
			case "disabled":
				args = append(args, "--map=false")
			}
			if code := Run(context.Background(), args, Options{Stdin: strings.NewReader(`@tailwind utilities;`), Stdout: &stdout, Stderr: &stderr}); code != 0 {
				t.Fatal(code, stderr.String())
			}
			css := stdout.String()
			if mode == "inline" || mode == "file" {
				name := "out.css"
				if mode == "file" {
					name = "dist/out.css"
				}
				data, err := os.ReadFile(filepath.Join(root, name))
				if err != nil {
					t.Fatal(err)
				}
				css = string(data)
			}
			_, mapURL, found := strings.Cut(css, "/*# sourceMappingURL=")
			if mode == "disabled" {
				if found {
					t.Fatal("disabled map emitted")
				}
				return
			}
			if !found {
				t.Fatal(css)
			}
			mapURL = strings.TrimSuffix(mapURL, " */\n")
			var data []byte
			var err error
			if encoded, ok := strings.CutPrefix(mapURL, "data:application/json;base64,"); ok {
				data, err = base64.StdEncoding.DecodeString(encoded)
			} else {
				base := root
				if mode == "file" {
					base = filepath.Join(root, "dist")
				}
				data, err = os.ReadFile(filepath.Join(base, mapURL))
			}
			if err != nil {
				t.Fatal(err)
			}
			var sourceMap rocketwind.SourceMap
			if err := json.Unmarshal(data, &sourceMap); err != nil {
				t.Fatal(err)
			}
			if sourceMap.Version != 3 || sourceMap.Mappings == "" {
				t.Fatal(string(data))
			}
			if _, err := sourceMap.Visualize(css); err != nil {
				t.Fatal(err)
			}
			if mode == "stdin" && sourceMap.Sources[0] != "<stdin>" {
				t.Fatal(sourceMap.Sources)
			}
			if mode == "file" {
				if mapURL != "../maps/out.css.map" {
					t.Fatal(mapURL)
				}
				found := false
				for _, name := range sourceMap.Sources {
					if filepath.Base(name) == "theme.css" {
						found = true
					}
				}
				if !found {
					t.Fatal("missing imported source", sourceMap.Sources)
				}
			}
		})
	}
}

func TestSourceMapErrorsAndFileProtection(t *testing.T) {
	root := t.TempDir()
	write(t, root, "input.css", `@tailwind utilities;`)
	write(t, root, "base.css", `.grid{display:grid}`)
	write(t, root, "config.json", `{}`)
	write(t, root, "import.css", `@import "./input.css";`)
	if err := os.Link(filepath.Join(root, "input.css"), filepath.Join(root, "alias.css")); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "directory"), 0755); err != nil {
		t.Fatal(err)
	}
	for _, extra := range [][]string{
		{"--map", "-"}, {"--map=input.css"}, {"--map=alias.css"},
		{"--map=base.css", "--exclude=base.css"}, {"--map=config.json", "--config=config.json"},
		{"--map=out.css", "-o", "out.css"}, {"--map=directory"},
		{"--map=input.css/map"},
		{"-i", "import.css", "--map=input.css"},
	} {
		var out bytes.Buffer
		if code := Run(context.Background(), append([]string{"--cwd", root, "-i", "input.css"}, extra...), Options{Stdout: io.Discard, Stderr: &out}); code == 0 {
			t.Fatal("accepted", extra, out.String())
		}
	}
	data, _ := os.ReadFile(filepath.Join(root, "input.css"))
	if string(data) != `@tailwind utilities;` {
		t.Fatal("overwrote input")
	}
}

func TestWatchUpdatesMapWithoutRewritingCSS(t *testing.T) {
	root := t.TempDir()
	write(t, root, "input.css", `@tailwind utilities;`)
	write(t, root, "index.gox", `<div class="flex"></div>`)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var stderr lockedBuffer
	done := make(chan int, 1)
	go func() {
		done <- Run(ctx, []string{"--cwd", root, "-i", "input.css", "-o", "out.css", "--map=out.css.map", "--watch=always", "--poll=10"}, Options{Stdin: strings.NewReader(""), Stdout: io.Discard, Stderr: &stderr})
	}()
	defer func() {
		cancel()
		if code := <-done; code != 0 {
			t.Error(code, stderr.String())
		}
	}()
	read := func(name string) string { data, _ := os.ReadFile(filepath.Join(root, name)); return string(data) }
	await(t, func() bool { return strings.Contains(read("out.css"), ".flex {") })
	initial := read("out.css.map")
	stat, _ := os.Stat(filepath.Join(root, "out.css"))
	write(t, root, "input.css", "\n\n@tailwind utilities;")
	await(t, func() bool { return read("out.css.map") != initial })
	after, _ := os.Stat(filepath.Join(root, "out.css"))
	if !after.ModTime().Equal(stat.ModTime()) {
		t.Fatal("rewrote identical CSS")
	}
	write(t, root, "index.gox", `<div class="flex grid"></div>`)
	await(t, func() bool { return strings.Contains(read("out.css"), ".grid {") })
}
