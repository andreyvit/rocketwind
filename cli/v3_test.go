package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/andreyvit/rocketwind"
)

func TestCompatibilityFlag(t *testing.T) {
	for _, version := range []string{"3", "3.4.17", "4", "4.3.4"} {
		f, err := parseFlags([]string{"--compat=" + version})
		if err != nil || (f.compatibility == rocketwind.Tailwind3) != strings.HasPrefix(version, "3") {
			t.Fatal(version, f, err)
		}
	}
	if _, err := parseFlags([]string{"--compat=2"}); err == nil {
		t.Fatal("unsupported version accepted")
	}
}

func TestTailwind3CLI(t *testing.T) {
	root := t.TempDir()
	write(t, root, "theme.json", `{"prefix":"tw-","theme":{"extend":{"colors":{"brand":"#123456"}}},"safelist":["tw-font-bold"],"content":["*.html"]}`)
	write(t, root, "index.html", `<div class="tw-flex tw-text-brand hover:tw-bg-black"></div>`)
	write(t, root, "admin.css", `.tw-flex { display: flex; }`)
	var out, stderr bytes.Buffer
	code := Run(context.Background(), []string{"--compat=3", "--cwd", root, "--config", "theme.json", "--exclude", "admin.css"}, Options{Stdout: &out, Stderr: &stderr})
	if code != 0 {
		t.Fatal(code, stderr.String())
	}
	css := out.String()
	for _, want := range []string{"tailwindcss v3.4.17", ".tw-font-bold", "rgb(18 52 86", ".hover\\:tw-bg-black:hover"} {
		if !strings.Contains(css, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(css, ".tw-flex {") {
		t.Fatal("duplicated excluded helper")
	}
}
