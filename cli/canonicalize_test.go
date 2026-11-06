package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andreyvit/rocketwind"
)

func TestUpstreamCanonicalizeCommand(t *testing.T) {
	data, err := os.ReadFile("../testdata/upstream/cli-canonicalize.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Test, Kind string
		Options    struct {
			Argv                    []string
			Cwd, CSS, Format, Input string
			Stdin                   *string
			StdinIsTTY, StdoutIsTTY bool
		}
		Result struct {
			ExitCode       int
			Stdout, Stderr string
		}
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 12 {
		t.Fatal(len(cases))
	}
	for _, tt := range cases {
		t.Run(tt.Test, func(t *testing.T) {
			root := t.TempDir()
			entries, err := os.ReadDir("../testdata/upstream/cli-canonicalize")
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				data, err := os.ReadFile(filepath.Join("../testdata/upstream/cli-canonicalize", entry.Name()))
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(root, entry.Name()), data, 0600); err != nil {
					t.Fatal(err)
				}
			}
			args := []string{"canonicalize", "--cwd", root}
			input := ""
			if tt.Kind == "stream" {
				args = append(args, "--stream", "--css", filepath.Base(tt.Options.CSS), "--format", tt.Options.Format)
				input = tt.Options.Input
			} else {
				for _, arg := range tt.Options.Argv {
					if strings.HasSuffix(arg, ".css") {
						arg = filepath.Join(root, filepath.Base(arg))
					}
					args = append(args, arg)
				}
				if tt.Options.Stdin != nil {
					input = *tt.Options.Stdin
				}
			}
			var out, stderr bytes.Buffer
			modules := map[string]rocketwind.Module{"./plugin-with-logs.mjs": {Plugin: func(api *rocketwind.PluginAPI) error {
				fmt.Fprintln(&stderr, "hello from a plugin")
				return api.MatchUtility(rocketwind.Utility{Name: "plugin-color", Values: []rocketwind.NamedValue{{Name: "red", Value: "red"}}, Compile: func(value, modifier string) []*rocketwind.Node {
					fmt.Fprintln(&stderr, "compiling plugin-color-"+value)
					return []*rocketwind.Node{{Kind: "declaration", Name: "color", Value: value}}
				}})
			}}}
			code := Run(context.Background(), args, Options{Stdin: strings.NewReader(input), Stdout: &out, Stderr: &stderr, Modules: modules, StdinIsTTY: tt.Options.StdinIsTTY, StdoutIsTTY: tt.Options.StdoutIsTTY})
			if code != tt.Result.ExitCode {
				t.Fatalf("exit %d want %d: %s", code, tt.Result.ExitCode, stderr.String())
			}
			if strings.Contains(tt.Result.Stdout, "Usage:") {
				// The original assertion checks for Usage; Rocketwind has its own branding.
				if !strings.Contains(out.String(), "Usage:") {
					t.Fatal(out.String())
				}
			} else if out.String() != tt.Result.Stdout {
				t.Errorf("stdout:\n%s\nwant:\n%s", out.String(), tt.Result.Stdout)
			}
			errors := stderr.String()
			if strings.Contains(tt.Test, "plugin logs") {
				for _, line := range []string{"hello from a plugin\n", "compiling plugin-color-red\n"} {
					if !strings.Contains(errors, line) {
						t.Errorf("missing log %q", line)
					}
				}
				for _, line := range []string{"hello from a plugin\n", "compiling plugin-color-red\n"} {
					errors = strings.ReplaceAll(errors, line, "")
				}
			}
			if strings.TrimSuffix(errors, "\n") != tt.Result.Stderr {
				t.Errorf("stderr %q want %q", errors, tt.Result.Stderr)
			}
		})
	}
}

func TestCanonicalizeCommandOptionsAndErrors(t *testing.T) {
	root := t.TempDir()
	write(t, root, "theme.css", `@theme { --spacing: 1px; } @tailwind utilities;`)
	write(t, root, "broken.css", `@plugin "broken"; @tailwind utilities;`)
	write(t, root, "invalid.css", `@apply unknown;`)
	failure := errors.New("plugin failed")
	modules := map[string]rocketwind.Module{"broken": {Plugin: func(api *rocketwind.PluginAPI) error {
		return api.MatchUtility(rocketwind.Utility{Name: "broken", Values: []rocketwind.NamedValue{{Name: "x", Value: "x"}}, CompileWithError: func(string, string) ([]*rocketwind.Node, error) {
			return nil, failure
		}})
	}}}
	for _, tt := range []struct {
		args                 []string
		input, output, error string
	}{
		{args: []string{"--help"}, output: "Usage:"},
		{args: []string{"-h"}, output: "Usage:"},
		{args: []string{"--stream=false", "--help=false", "flex"}, output: "flex"},
		{args: []string{"--css=theme.css", "--", "-mt-2 flex"}, output: "-mt-2 flex"},
		{args: []string{"--format=jsonl", "flex", "grid"}, output: "{\"input\":\"flex\",\"output\":\"flex\",\"changed\":false}\n{\"input\":\"grid\",\"output\":\"grid\",\"changed\":false}"},
		{input: "\r\n flex\r\n  \n grid \n", output: "flex\ngrid"},
		{args: []string{"--format=yaml"}, error: "Invalid value for --format: yaml"},
		{args: []string{"--css"}, error: "--css requires a value"},
		{args: []string{"--stream=perhaps"}, error: "invalid value for --stream"},
		{args: []string{"--unknown"}, error: "unknown option"},
		{args: []string{"--css=missing.css", "flex"}, error: "no such file"},
		{args: []string{"--css=invalid.css", "flex"}, error: "unknown utility"},
		{args: []string{"--css=broken.css", "broken-x"}, error: "plugin failed"},
	} {
		t.Run(fmt.Sprint(tt.args), func(t *testing.T) {
			var output, stderr bytes.Buffer
			args := append([]string{"canonicalize", "--cwd", root, "--css", "theme.css"}, tt.args...)
			code := Run(context.Background(), args, Options{Stdin: strings.NewReader(tt.input), Stdout: &output, Stderr: &stderr, Modules: modules})
			if tt.error != "" {
				if code != 1 || !strings.Contains(stderr.String(), tt.error) {
					t.Fatalf("exit %d: %s", code, stderr.String())
				}
			} else if code != 0 || !strings.Contains(output.String(), tt.output) || stderr.Len() != 0 {
				t.Fatalf("exit %d: %q %s", code, output.String(), stderr.String())
			}
		})
	}
	var output, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"canonicalize"}, Options{Stdout: &output, Stderr: &stderr, StdoutIsTTY: true}); code != 0 || !strings.Contains(output.String(), "Usage:") {
		t.Fatal(code, output.String(), stderr.String())
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if code := Run(ctx, []string{"canonicalize", "flex"}, Options{Stdout: &output, Stderr: &stderr}); code != 1 || !strings.Contains(stderr.String(), "context canceled") {
		t.Fatal(code, stderr.String())
	}
	ctx, cancel = context.WithCancel(context.Background())
	input := &cancelReader{reader: strings.NewReader("flex"), cancel: cancel}
	stderr.Reset()
	if code := Run(ctx, []string{"canonicalize", "--css", filepath.Join(root, "theme.css")}, Options{Stdin: input, Stdout: &output, Stderr: &stderr}); code != 1 || !strings.Contains(stderr.String(), "context canceled") {
		t.Fatal(code, stderr.String())
	}
}

type cancelReader struct {
	reader *strings.Reader
	cancel context.CancelFunc
}

func (r *cancelReader) Read(p []byte) (int, error) { r.cancel(); return r.reader.Read(p) }

type failWrite struct{ at, count int }

func (w *failWrite) Write(p []byte) (int, error) {
	w.count++
	if w.count == w.at {
		return 0, errors.New("write failed")
	}
	return len(p), nil
}

type byteReader struct{ *strings.Reader }

func (r byteReader) Read(p []byte) (int, error) { return r.Reader.Read(p[:min(len(p), 1)]) }

func TestCanonicalizeStreams(t *testing.T) {
	process := func(s string) (candidateGroupResult, error) {
		return candidateGroupResult{Input: s, Output: s, Changed: false}, nil
	}
	for _, tt := range []struct{ input, format, want string }{
		{"", "json", "[]"}, {"", "text", ""}, {"a\rb\r\nc\n\nlast", "text", "a\nb\nc\n\nlast\n"}, {"a\r", "text", "a\n"},
		{strings.Repeat("x", 70000), "text", strings.Repeat("x", 70000) + "\n"},
	} {
		var out bytes.Buffer
		var input = byteReader{strings.NewReader(tt.input)}
		if err := streamCanonicalize(context.Background(), input, &out, tt.format, process); err != nil || out.String() != tt.want {
			t.Fatalf("format %s: output length %d want %d; %v", tt.format, out.Len(), len(tt.want), err)
		}
	}
	for _, tt := range []struct {
		input, format string
		failAt        int
	}{{"", "json", 1}, {"", "json", 2}, {"flex", "json", 2}, {"flex", "json", 3}, {"flex", "text", 1}, {"flex", "jsonl", 1}} {
		if err := streamCanonicalize(context.Background(), strings.NewReader(tt.input), &failWrite{at: tt.failAt}, tt.format, process); err == nil || err.Error() != "write failed" {
			t.Fatal(tt, err)
		}
	}
	var out bytes.Buffer
	if err := streamCanonicalize(context.Background(), failedIO{}, &out, "text", process); err == nil || err.Error() != "read failure" {
		t.Fatal(err)
	}
	if err := streamCanonicalize(context.Background(), strings.NewReader("flex"), &out, "text", func(string) (candidateGroupResult, error) {
		return candidateGroupResult{}, errors.New("process failed")
	}); err == nil || err.Error() != "process failed" {
		t.Fatal(err)
	}
	if got := canonicalJSON(candidateGroupResult{Input: "<&>", Output: "<&>"}, false); strings.Contains(got, `\u003`) {
		t.Fatal(got)
	}
}

func TestCanonicalizeIOFailures(t *testing.T) {
	root := t.TempDir()
	write(t, root, "theme.css", `@tailwind utilities;`)
	for _, options := range []Options{{Stdin: failedIO{}}, {Stdin: strings.NewReader("flex"), Stdout: failedIO{}}} {
		var stderr bytes.Buffer
		options.Stderr = &stderr
		if code := Run(context.Background(), []string{"canonicalize", "--css", filepath.Join(root, "theme.css")}, options); code != 1 || !strings.Contains(stderr.String(), "failure") {
			t.Fatal(code, stderr.String())
		}
	}
	f, err := os.Open(filepath.Join(root, "theme.css"))
	if err != nil {
		t.Fatal(err)
	}
	if terminalFile(f) {
		t.Fatal("regular file is not a terminal")
	}
	f.Close()
	if terminalFile(f) {
		t.Fatal("closed file is not a terminal")
	}
	dev, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer dev.Close()
	if !terminalFile(dev) {
		t.Fatal("character device")
	}
	t.Chdir(root)
	if err := os.Chmod(root, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0700)
	var out, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"canonicalize", "flex"}, Options{Stdout: &out, Stderr: &stderr}); code != 1 || stderr.Len() == 0 {
		t.Fatal(code, stderr.String())
	}
}

func TestCanonicalizeStreamCancellation(t *testing.T) {
	root := t.TempDir()
	write(t, root, "theme.css", `@tailwind utilities;`)
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var out, stderr lockedBuffer
	done := make(chan int, 1)
	go func() {
		done <- Run(ctx, []string{"canonicalize", "--css", filepath.Join(root, "theme.css"), "--stream"}, Options{Stdin: reader, Stdout: &out, Stderr: &stderr})
	}()
	if _, err := io.WriteString(writer, "flex\n"); err != nil {
		t.Fatal(err)
	}
	await(t, func() bool { return out.String() == "flex\n" })
	cancel()
	select {
	case code := <-done:
		if code != 1 || !strings.Contains(stderr.String(), "context canceled") {
			t.Fatal(code, stderr.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("cancellation blocked on stdin")
	}
	// A processing error also stops the reader before it can deliver more lines.
	var output bytes.Buffer
	if err := streamCanonicalize(context.Background(), strings.NewReader("one\ntwo\nthree\n"), &output, "text", func(string) (candidateGroupResult, error) { return candidateGroupResult{}, errors.New("stop") }); err == nil || err.Error() != "stop" {
		t.Fatal(err)
	}
}
