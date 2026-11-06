package cli

import (
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/andreyvit/rocketwind"
)

type integrationAction struct {
	Kind           string
	Args           []json.RawMessage
	Result         json.RawMessage
	Error, Message string
	Line, Column   int
}
type integrationFixture struct {
	File, Name string
	Actions    []integrationAction
}

// These four cases specifically test JavaScript module formats, JavaScript
// exports, or an arbitrary third-party JavaScript plugin. The other JavaScript
// test callbacks have explicit Go ports below. TypeScript's own tsc invocation
// is excluded, while its fixture's Go plugin and CSS assertions still run.
var integrationJSExceptions = map[string]string{
	"CLI > module resolution using CJS, ESM, CTS, and MTS":            "Node and TypeScript module loading",
	"Standalone CLI > module resolution using CJS, ESM, CTS, and MTS": "Node and TypeScript module loading",
	"includes js APIs for plugins":                                    "JavaScript exports and typeof checks",
	"builds the `tailwindcss-animate` plugin utilities":               "third-party JavaScript plugin execution",
}

func TestUpstreamCLIIntegration(t *testing.T) {
	files, err := filepath.Glob("../testdata/upstream/cli-integration/*.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 68 {
		t.Fatalf("got %d integration fixtures", len(files))
	}
	seen := map[string]bool{}
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		reader, err := gzip.NewReader(strings.NewReader(string(data)))
		if err != nil {
			t.Fatal(err)
		}
		var fixture integrationFixture
		if err := json.NewDecoder(reader).Decode(&fixture); err != nil {
			t.Fatal(err)
		}
		reader.Close()
		if seen[fixture.Name] {
			t.Fatalf("duplicate fixture %q", fixture.Name)
		}
		seen[fixture.Name] = true
		if reason, ok := integrationJSExceptions[fixture.Name]; ok {
			t.Logf("JavaScript host exception: %s: %s", fixture.Name, reason)
			continue
		}
		t.Run(fixture.Name, func(t *testing.T) { t.Parallel(); replayIntegration(t, fixture) })
	}
	for name := range integrationJSExceptions {
		if !seen[name] {
			t.Errorf("stale JavaScript exception: %s", name)
		}
	}
}

type integrationProcess struct {
	stdout, stderr lockedBuffer
	cancel         context.CancelFunc
	done           chan int
	stderrOffset   int
}

func replayIntegration(t *testing.T, fixture integrationFixture) {
	root := t.TempDir()
	var process *integrationProcess
	reads := map[string]string{}
	decode := func(raw json.RawMessage, target any) {
		t.Helper()
		if err := json.Unmarshal(raw, target); err != nil {
			t.Fatal(err)
		}
	}
	str := func(raw json.RawMessage) string { var result string; decode(raw, &result); return result }
	resolve := func(name string) string { return strings.ReplaceAll(name, "__ROOT__", root) }
	file := func(name string) string {
		if filepath.IsAbs(name) {
			return resolve(name)
		}
		return filepath.Join(root, name)
	}
	check := func(fn func() error) {
		t.Helper()
		end := time.Now().Add(3 * time.Second)
		for {
			err := fn()
			if err == nil {
				return
			}
			if time.Now().After(end) {
				if process != nil {
					t.Log(process.stderr.String())
				}
				t.Fatal(err)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	for index, action := range fixture.Actions {
		t.Logf("%d %s", index, action.Kind)
		switch action.Kind {
		case "write":
			name := file(str(action.Args[0]))
			var content string
			if len(action.Args[1]) > 0 && action.Args[1][0] == '{' {
				var binary struct {
					Base64 string `json:"$base64"`
				}
				decode(action.Args[1], &binary)
				data, err := base64.StdEncoding.DecodeString(binary.Base64)
				if err != nil {
					t.Fatal(err)
				}
				content = string(data)
			} else {
				content = resolve(str(action.Args[1]))
			}
			if len(action.Args) > 2 && str(action.Args[2]) == "utf16le" {
				var bytes []byte
				for _, r := range utf16.Encode([]rune(content)) {
					bytes = append(bytes, byte(r), byte(r>>8))
				}
				content = string(bytes)
			}
			write(t, filepath.Dir(name), filepath.Base(name), content)
		case "delete":
			if err := os.Remove(file(str(action.Args[0]))); err != nil {
				t.Fatal(err)
			}
		case "symlink":
			target, name := file(str(action.Args[0])), file(str(action.Args[1]))
			if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, name); err != nil {
				t.Fatal(err)
			}
		case "exec", "spawn":
			command := str(action.Args[0])
			cwd := root
			if len(action.Args) > 1 && string(action.Args[1]) != "null" {
				var options struct{ Cwd string }
				decode(action.Args[1], &options)
				if options.Cwd != "" {
					cwd = resolve(options.Cwd)
				}
			}
			if command == "pnpm install" {
				integrationWorkspace(t, root)
				continue
			}
			if command == "pnpm tsc -b" {
				continue
			} // JavaScript host step; the Go callback is compiled by go test.
			if command == "git init" {
				cmd := exec.Command("git", "init", "-q")
				cmd.Dir = cwd
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatal(err, string(out))
				}
				continue
			}
			args, err := integrationArguments(command)
			if err != nil {
				t.Fatal(err)
			}
			var input string
			if len(action.Args) > 2 {
				var options struct{ Stdin string }
				decode(action.Args[2], &options)
				input = options.Stdin
			}
			for i, arg := range args {
				if arg == "<" {
					data, err := os.ReadFile(filepath.Join(cwd, args[i+1]))
					if err != nil {
						t.Fatal(err)
					}
					input = string(data)
					args = args[:i]
					break
				}
			}
			if fixture.Name == "does not scan itself for candidates" {
				command := exec.Command("go", "build", "-cover", "-o", filepath.Join(root, "tailwindcss"), "../cmd/rocketwind")
				if output, err := command.CombinedOutput(); err != nil {
					t.Fatal(err, string(output))
				}
			}
			args = append([]string{"--cwd", cwd}, args...)
			if strings.Contains(command, " canonicalize ") {
				args = append([]string{"canonicalize", "--cwd", cwd}, args[3:]...)
			}
			p := &integrationProcess{done: make(chan int, 1)}
			ctx, cancel := context.WithCancel(context.Background())
			p.cancel = cancel
			options := Options{Stdin: strings.NewReader(input), Stdout: &p.stdout, Stderr: &p.stderr, ResolveModule: integrationModules(fixture.Name, &p.stderr)}
			if action.Kind == "spawn" {
				pipeR, pipeW := io.Pipe()
				options.Stdin = pipeR
				go func() { p.done <- Run(ctx, args, options) }()
				process = p
				t.Cleanup(func() {
					cancel()
					pipeW.Close()
					pipeR.Close()
					select {
					case code := <-p.done:
						if code != 0 {
							t.Error(code, p.stderr.String())
						}
					case <-time.After(time.Second):
						t.Error("watcher did not stop")
					}
				})
			} else {
				var code int
				if fixture.Name == "does not scan itself for candidates" {
					command := exec.CommandContext(ctx, filepath.Join(root, "tailwindcss"), args...)
					command.Dir, command.Stdin, command.Stdout, command.Stderr = cwd, options.Stdin, options.Stdout, options.Stderr
					coverDir := os.Getenv("ROCKETWIND_COVERDIR")
					if coverDir == "" {
						coverDir = t.TempDir()
					}
					command.Env = append(os.Environ(), "GOCOVERDIR="+coverDir)
					if err := command.Run(); err != nil {
						if exit, ok := err.(*exec.ExitError); ok {
							code = exit.ExitCode()
						} else {
							t.Fatal(err)
						}
					}
				} else {
					code = Run(ctx, args, options)
				}
				cancel()
				if action.Error != "" {
					if code == 0 {
						t.Fatal("expected command failure")
					}
					switch fixture.Name {
					case "CSS parse errors should include filename and line number":
						if !regexp.MustCompile("CssSyntaxError: .*input.css:3:3: Invalid declaration: `\\*\\/`").MatchString(p.stderr.String()) {
							t.Fatal(p.stderr.String())
						}
					case "fails when reading files with UTF-16 files with BOM":
						if !strings.Contains(p.stderr.String(), "Invalid declaration:") {
							t.Fatal(p.stderr.String())
						}
					case "fails when input file does not exist":
						if !strings.Contains(p.stderr.String(), "Specified input file `./index.css` does not exist.") {
							t.Fatal(p.stderr.String())
						}
					case "fails when input file and output file are the same":
						if !strings.Contains(p.stderr.String(), "Specified input file `./input.css` and output file `./input.css` are identical.") {
							t.Fatal(p.stderr.String())
						}
					default:
						t.Fatal("unported error assertion:", fixture.Name)
					}
					continue
				}
				if code != 0 {
					t.Fatal(code, p.stderr.String())
				}
				if len(action.Result) > 0 && action.Result[0] == '{' {
					var expected struct {
						Stdout, Stderr string
						ExitCode       int
					}
					decode(action.Result, &expected)
					if p.stdout.String() != expected.Stdout || strings.TrimSpace(p.stderr.String()) != strings.TrimSpace(expected.Stderr) || code != expected.ExitCode {
						t.Fatalf("streams:\n%s\n%s\nwant %#v", p.stdout.String(), p.stderr.String(), expected)
					}
				} else if strings.Contains(command, "--output -") || strings.Contains(command, "--output=-") {
					want := str(action.Result)
					want, _, _ = strings.Cut(want, "\n\n≈ tailwindcss")
					if integrationCSS(p.stdout.String()) != integrationCSS(resolve(want)) {
						t.Fatalf("stdout mismatch\ngot %s\nwant %s", p.stdout.String(), want)
					}
				}
			}
		case "onStderr":
			if process == nil {
				t.Fatal("missing watcher")
			}
			want := "Done in"
			if action.Message == "Error:" {
				want = "Error"
			}
			check(func() error {
				output := process.stderr.String()
				at := strings.Index(output[process.stderrOffset:], want)
				if at < 0 {
					return fmt.Errorf("waiting for %q", want)
				}
				process.stderrOffset += at + len(want)
				return nil
			})
		case "expectFileToContain", "expectFileNotToContain":
			name := file(str(action.Args[0]))
			var tests []json.RawMessage
			if action.Args[1][0] == '[' {
				decode(action.Args[1], &tests)
			} else {
				tests = []json.RawMessage{action.Args[1]}
			}
			check(func() error {
				data, err := os.ReadFile(name)
				if err != nil {
					return err
				}
				for _, test := range tests {
					var match bool
					var expected string
					if test[0] == '{' {
						var pattern struct {
							Regexp string `json:"$regexp"`
						}
						decode(test, &pattern)
						expected = pattern.Regexp
						match = regexp.MustCompile(expected).Match(data)
					} else {
						expected = resolve(str(test))
						match = strings.Contains(string(data), expected)
					}
					if match != (action.Kind == "expectFileToContain") {
						return fmt.Errorf("%s %s %q\n%s", name, action.Kind, expected, data)
					}
				}
				return nil
			})
		case "read":
			name := file(str(action.Args[0]))
			expected := str(action.Result)
			check(func() error {
				data, err := os.ReadFile(name)
				if err != nil {
					return err
				}
				actual := string(data)
				if !strings.HasSuffix(name, ".map") && integrationCSS(actual) != integrationCSS(resolve(expected)) {
					return fmt.Errorf("read %s differs\ngot %s\nwant %s", name, actual, expected)
				}
				reads[expected] = actual
				return nil
			})
		case "glob":
			var expected [][2]string
			decode(action.Result, &expected)
			check(func() error {
				names, err := filepath.Glob(file(str(action.Args[0])))
				if err != nil {
					return err
				}
				if len(names) != len(expected) {
					return fmt.Errorf("glob count %d != %d", len(names), len(expected))
				}
				for _, entry := range expected {
					data, err := os.ReadFile(file(entry[0]))
					if err != nil {
						return err
					}
					if integrationCSS(string(data)) != integrationCSS(resolve(entry[1])) {
						return fmt.Errorf("glob %s differs\ngot %s\nwant %s", entry[0], data, entry[1])
					}
					reads[entry[1]] = string(data)
				}
				return nil
			})
		case "map":
			integrationSourceMap(t, action, reads, root)
		default:
			t.Fatalf("unhandled integration action %q", action.Kind)
		}
	}
}

var integrationComments = regexp.MustCompile(`(?s)[\s\n]*/\*[!#].*?\*/[\s\n]*`)

func integrationCSS(css string) string {
	return strings.TrimSpace(integrationComments.ReplaceAllString(css, ""))
}

func integrationArguments(command string) ([]string, error) {
	var result []string
	var word strings.Builder
	var quote rune
	escaped := false
	started := false
	for _, r := range command {
		if escaped {
			word.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' && quote != '\'' {
			escaped = true
			started = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			started = true
			continue
		}
		if r == ' ' || r == '\n' || r == '\t' {
			if started {
				result = append(result, word.String())
				word.Reset()
				started = false
			}
			continue
		}
		word.WriteRune(r)
		started = true
	}
	if quote != 0 || escaped {
		return nil, fmt.Errorf("unclosed shell argument: %s", command)
	}
	if started {
		result = append(result, word.String())
	}
	for i, word := range result {
		if strings.Contains(filepath.Base(word), "tailwindcss") {
			return result[i+1:], nil
		}
	}
	return nil, fmt.Errorf("not a compiler command: %s", command)
}

func integrationWorkspace(t *testing.T, root string) {
	t.Helper()
	var packages []string
	if err := filepath.WalkDir(root, func(name string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && entry.Name() == "node_modules" {
			return filepath.SkipDir
		}
		if entry.Name() == "package.json" {
			packages = append(packages, name)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, name := range packages {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var pkg struct{ Name string }
		if err := json.Unmarshal(data, &pkg); err != nil {
			t.Fatal(err)
		}
		if pkg.Name == "" {
			continue
		}
		link := filepath.Join(root, "node_modules", pkg.Name)
		if err := os.MkdirAll(filepath.Dir(link), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Dir(name), link); err != nil && !os.IsExist(err) {
			t.Fatal(err)
		}
	}
}

func integrationModules(test string, stderr io.Writer) func(string, string, string, func(string)) (*rocketwind.Module, error) {
	return func(id, base, kind string, dependency func(string)) (*rocketwind.Module, error) {
		if strings.Contains(id, "tailwind.config.") {
			name := filepath.Join(base, id)
			dependency(name)
			data, err := os.ReadFile(name)
			if err != nil {
				return nil, err
			}
			source := string(data)
			module := &rocketwind.Module{Base: filepath.Dir(name)}
			switch {
			case strings.Contains(source, "typography:"):
				module.Legacy = rocketwind.ConfigObject{{Key: "theme", Value: map[string]any{"typography": map[string]any{"custom": map[string]any{"css": map[string]any{"hr": rocketwind.ConfigObject{{Key: "--x", Value: "1"}, {Key: "@variant sm:custom", Value: map[string]any{"--x": "2"}}}}}}}}}
			case strings.Contains(source, "content:"):
				if strings.Contains(source, "relative:") {
					module.Legacy = rocketwind.ConfigObject{{Key: "content", Value: map[string]any{"relative": true, "files": []any{"html/*.html"}}}}
				} else {
					module.Legacy = rocketwind.ConfigObject{{Key: "content", Value: []any{"../project-b/src/**/*.js"}}}
				}
			default:
				if strings.Contains(source, "myColor") {
					ext := filepath.Ext(name)
					colorFile := filepath.Join(filepath.Dir(name), "my-color"+ext)
					dependency(colorFile)
					value, err := os.ReadFile(colorFile)
					if err != nil {
						return nil, err
					}
					source = string(value)
				}
				color := "blue"
				if strings.Contains(source, "'red'") {
					color = "red"
				} else if !strings.Contains(source, "'blue'") {
					return nil, fmt.Errorf("unported fixture config: %s", source)
				}
				module.Legacy = rocketwind.ConfigObject{{Key: "theme", Value: map[string]any{"extend": map[string]any{"colors": map[string]any{"primary": color}}}}}
			}
			return module, nil
		}
		if kind == "plugin" && strings.Contains(id, "plugin.") {
			return &rocketwind.Module{Plugin: func(api *rocketwind.PluginAPI) error {
				if strings.Contains(test, "canonicalize writes plugin logs") {
					fmt.Fprintln(stderr, "hello from a plugin")
					return nil
				}
				if strings.Contains(test, "compiled from TypeScript") {
					return api.MatchUtility(rocketwind.Utility{Name: "test", Values: []rocketwind.NamedValue{{Name: "red", Value: "red"}}, Compile: func(value, modifier string) []*rocketwind.Node {
						return []*rocketwind.Node{{Kind: "declaration", Name: "color", Value: value}}
					}})
				}
				if err := api.AddVariantFormats("inverted", "@media (inverted-colors: inverted)"); err != nil {
					return err
				}
				return api.AddVariantFormats("hocus", "&:focus", "&:hover")
			}}, nil
		}
		return nil, nil
	}
}

func integrationSourceMap(t *testing.T, action integrationAction, reads map[string]string, root string) {
	t.Helper()
	var content, rawMap string
	if action.Args[0][0] == '"' {
		var before string
		if err := json.Unmarshal(action.Args[0], &before); err != nil {
			t.Fatal(err)
		}
		var found bool
		content, found = reads[before]
		if !found {
			t.Fatal("source map input was not read")
		}
		_, encoded, found := strings.Cut(content, "sourceMappingURL=data:application/json;base64,")
		if !found {
			t.Fatal("missing inline source map")
		}
		encoded, _, _ = strings.Cut(encoded, " ")
		data, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			t.Fatal(err)
		}
		rawMap = string(data)
		content = strings.TrimRight(content, "\r\n\t ")
		content = content[:strings.LastIndexByte(content, '\n')]
	} else {
		var options struct{ Map, Content string }
		if err := json.Unmarshal(action.Args[0], &options); err != nil {
			t.Fatal(err)
		}
		var found bool
		content, found = reads[options.Content]
		if !found {
			t.Fatal("mapped CSS was not read")
		}
		rawMap, found = reads[options.Map]
		if !found {
			t.Fatal("source map file was not read")
		}
	}
	var sourceMap rocketwind.SourceMap
	if err := json.Unmarshal([]byte(rawMap), &sourceMap); err != nil {
		t.Fatal(err)
	}
	var expected struct {
		Source              *string
		Original, Generated string
	}
	if err := json.Unmarshal(action.Result, &expected); err != nil {
		t.Fatal(err)
	}
	source, line, column := -1, 0, 0
	previousSource, previousLine, previousColumn := 0, 0, 0
	for rowIndex, row := range strings.Split(sourceMap.Mappings, ";") {
		if rowIndex+1 > action.Line {
			break
		}
		generated := 0
		for _, segment := range strings.Split(row, ",") {
			if segment == "" {
				continue
			}
			var values []int
			for segment != "" {
				value, shift := 0, 0
				for {
					digit := strings.IndexByte("ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/", segment[0])
					segment = segment[1:]
					if digit < 0 {
						t.Fatal("invalid VLQ")
					}
					value |= (digit & 31) << shift
					shift += 5
					if digit&32 == 0 {
						break
					}
					if segment == "" {
						t.Fatal("short VLQ")
					}
				}
				decoded := value >> 1
				if value&1 != 0 {
					decoded = -decoded
				}
				values = append(values, decoded)
			}
			generated += values[0]
			if len(values) > 1 {
				previousSource += values[1]
				previousLine += values[2]
				previousColumn += values[3]
			}
			if rowIndex+1 == action.Line && generated <= action.Column {
				if len(values) == 1 {
					source = -1
				} else {
					source, line, column = previousSource, previousLine, previousColumn
				}
			}
		}
	}
	original := "(none)"
	var sourceName string
	if source >= 0 {
		if source >= len(sourceMap.SourcesContent) {
			t.Fatal("source out of range")
		}
		sourceName = sourceMap.Sources[source]
		original = integrationSnippet(sourceMap.SourcesContent[source], line+1, column)
	}
	generated := integrationSnippet(content, action.Line, action.Column)
	if original != expected.Original || generated != expected.Generated {
		t.Fatalf("map at %d:%d: original=%q generated=%q; want original=%q generated=%q\n%s", action.Line, action.Column, original, generated, expected.Original, expected.Generated, content)
	}
	if expected.Source == nil {
		if source >= 0 {
			t.Fatalf("unexpected source: %s", sourceName)
		}
		return
	}
	want := strings.ReplaceAll(*expected.Source, "__ROOT__", root)
	if strings.Contains(want, "utilities-") {
		want = "tailwindcss/utilities.css"
	}
	if strings.Contains(want, "tailwindcss") {
		if filepath.Base(want) != filepath.Base(sourceName) {
			t.Fatalf("source %q; want %q", sourceName, want)
		}
	} else if filepath.Base(filepath.Dir(want)) != filepath.Base(filepath.Dir(sourceName)) || filepath.Base(want) != filepath.Base(sourceName) {
		t.Fatalf("source %q; want %q", sourceName, want)
	}
}

func integrationSnippet(content string, line, column int) string {
	text := utf16.Encode([]rune(content))
	offset := 0
	for current := 1; current < line && offset < len(text); offset++ {
		if text[offset] == '\n' {
			current++
		}
	}
	offset = min(offset+column, len(text))
	return strings.TrimSpace(string(utf16.Decode(text[offset:min(offset+10, len(text))]))) + "..."
}
