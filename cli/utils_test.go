package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestUpstreamCLIHelpers(t *testing.T) {
	for file, count := range map[string]int{"args": 15, "renderer": 7, "format-ns": 21} {
		data, err := os.ReadFile("../testdata/upstream/cli-" + file + ".test.ts.json")
		if err != nil {
			t.Fatal(err)
		}
		var cases []struct {
			Test, Kind string
			Args       []json.RawMessage
			Result     json.RawMessage
		}
		if err := json.Unmarshal(data, &cases); err != nil {
			t.Fatal(err)
		}
		if len(cases) != count {
			t.Fatal(file, len(cases))
		}
		for _, tt := range cases {
			t.Run(file+"/"+tt.Test, func(t *testing.T) {
				var actual any
				switch tt.Kind {
				case "args":
					var options map[string]map[string]any
					var argv []string
					json.Unmarshal(tt.Args[0], &options)
					json.Unmarshal(tt.Args[1], &argv)
					defs := make(map[string]argDefinition)
					for key, option := range options {
						def := argDefinition{Type: option["type"].(string)}
						def.Alias, _ = option["alias"].(string)
						def.Default, def.HasDefault = option["default"]
						defs[key] = def
					}
					raw, positional := rawArguments(argv)
					actual = typedArguments(defs, raw, positional)
				case "formatNanoseconds":
					var ns int64
					json.Unmarshal(tt.Args[0], &ns)
					actual = formatNanoseconds(ns)
				case "wordWrap":
					var text string
					var width int
					json.Unmarshal(tt.Args[0], &text)
					json.Unmarshal(tt.Args[1], &width)
					actual = wordWrap(text, width)
				case "relative":
					var to, from, cwd string
					var options struct{ PreferAbsoluteIfShorter bool }
					options.PreferAbsoluteIfShorter = true
					json.Unmarshal(tt.Args[0], &to)
					json.Unmarshal(tt.Args[1], &from)
					json.Unmarshal(tt.Args[2], &options)
					json.Unmarshal(tt.Args[3], &cwd)
					root := t.TempDir()
					t.Chdir(root)
					to, from = strings.ReplaceAll(to, cwd, root), strings.ReplaceAll(from, cwd, root)
					actual = filepath.ToSlash(relativePath(to, from, options.PreferAbsoluteIfShorter))
				default:
					t.Fatal(tt.Kind)
				}
				data, err := json.Marshal(actual)
				if err != nil {
					t.Fatal(err)
				}
				var got, want any
				json.Unmarshal(data, &got)
				json.Unmarshal(tt.Result, &want)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("got %s, want %s", data, tt.Result)
				}
			})
		}
	}
}

func TestArgumentConversions(t *testing.T) {
	for _, tt := range []struct {
		input any
		kind  string
		want  any
	}{
		{true, "boolean", true}, {false, "boolean", false}, {"false", "boolean", false}, {"true", "boolean", true},
		{"yes", "boolean", nil}, {"abc", "number", nil}, {false, "number", float64(0)}, {true, "number", float64(1)},
		{"  ", "number", float64(0)}, {"0Xff", "number", float64(255)}, {"0o10", "number", float64(8)}, {"0B11", "number", float64(3)},
		{"0x!", "number", nil}, {"12.5", "number | string", 12.5}, {"indefinitely", "number | string", "indefinitely"},
		{"true", "boolean | string", true}, {"always", "boolean | string", "always"},
		{"nope", "boolean | number", nil}, {"NaN", "number", nil}, {"Inf", "number", nil},
		{"Infinity", "number", math.Inf(1)}, {"+Infinity", "number", math.Inf(1)}, {"-Infinity", "number", math.Inf(-1)},
		{"012", "number", float64(12)}, {struct{}{}, "number", nil},
		{float64(0), "string", "0"}, {float64(1e-7), "string", "1e-7"}, {float64(1e20), "string", "100000000000000000000"},
		{float64(1e21), "string", "1e+21"}, {true, "string", "true"},
	} {
		got, valid := convertArgument(tt.input, tt.kind)
		if !reflect.DeepEqual(got, tt.want) || valid != (tt.want != nil) {
			t.Errorf("%#v as %s: %#v, %v; want %#v", tt.input, tt.kind, got, valid, tt.want)
		}
	}
	func() {
		defer func() {
			if got := recover(); got != "Unhandled type: object" {
				t.Fatal(got)
			}
		}()
		convertArgument("x", "object")
	}()
	defs := map[string]argDefinition{"--output": {Type: "string", Alias: "-o"}, "--flag": {Type: "boolean"}}
	argv := []string{"-o", "a", "--output", "long", "-o", "last", "--flag", "--no-flag", "--", "-", "10"}
	original := slices.Clone(argv)
	raw, positional := rawArguments(argv)
	got := typedArguments(defs, raw, positional)
	if got["--output"] != "long" || got["--flag"] != false || !slices.Equal(positional, []string{"__IO_DEFAULT_VALUE__", "10"}) || !slices.Equal(argv, original) {
		t.Fatal(got, argv)
	}
	raw, _ = rawArguments([]string{"-abc=7", "--empty=", "value", "--last=", "--input", "-", "0x10", "---xyz", "--no-flag"})
	for key, want := range map[string]any{"a": true, "b": true, "c": float64(7), "empty": "value", "last": true, "input": "-", "x": true, "y": true, "z": true, "flag": false} {
		if values := raw[key]; len(values) != 1 || values[0] != want {
			t.Error(key, values, want)
		}
	}
}

func TestCLIArgumentReferenceBoundaries(t *testing.T) {
	data, err := os.ReadFile("../testdata/upstream/cli-args-extra.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Type   string
		Argv   []string
		Result map[string]any
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 812 {
		t.Fatal(len(cases))
	}
	for _, tt := range cases {
		raw, pos := rawArguments(tt.Argv)
		actual := typedArguments(map[string]argDefinition{"--value": {Type: tt.Type, Alias: "-v"}}, raw, pos)
		for key, value := range actual {
			if n, ok := value.(float64); ok && math.IsInf(n, 0) {
				text := "Infinity"
				if n < 0 {
					text = "-Infinity"
				}
				actual[key] = map[string]string{"$number": text}
			}
		}
		data, err := json.Marshal(actual)
		if err != nil {
			t.Fatal(err)
		}
		var got any
		json.Unmarshal(data, &got)
		if !reflect.DeepEqual(got, tt.Result) {
			t.Errorf("%s %q: %s, want %#v", tt.Type, tt.Argv, data, tt.Result)
		}
	}
}

func TestTailwindCLIFlagRules(t *testing.T) {
	if _, err := parseFlags([]string{"unknown-command"}); err == nil {
		t.Fatal("unknown command accepted")
	}
	if _, err := parseFlags([]string{"build", "ignored-positional"}); err != nil {
		t.Fatal(err)
	}
	f, err := parseFlags([]string{"-i", "alias.css", "--input", "long.css", "-i", "late.css", "--no-silent", "--watch=false", "--poll=false", "--unknown", "--map=false"})
	if err != nil || f.input != "long.css" || f.watch != "" || f.silent || f.inlineMap {
		t.Fatal(f, err)
	}
	f, err = parseFlags([]string{"-wh", "--poll=1.5", "--content=a,b", "--content=c", "--watch=forever", "--no-help"})
	if err != nil || f.watch != "stdin" || f.help || f.poll != 1500*time.Microsecond || !slices.Equal(f.content, []string{"a", "b", "c"}) {
		t.Fatal(f, err)
	}
	for _, poll := range []string{"0", "-1", "Infinity", "1000000000000000000", "foo"} {
		if _, err := parseFlags([]string{"--poll=" + poll}); err == nil {
			t.Fatal(poll)
		}
	}
	if f, err := parseFlags([]string{"--poll=true", "--watch", "true"}); err != nil || f.watch != "stdin" || f.poll != 250*time.Millisecond {
		t.Fatal(f, err)
	}
}

func TestDiagnosticBoundaries(t *testing.T) {
	for _, tt := range []struct {
		text  string
		width int
		want  []string
	}{
		{"", 10, []string{}}, {"   ", 10, []string{}}, {"abcdefgh x", 3, []string{"abcdefgh", "x"}},
		{"a  b", 3, []string{"a ", "b"}}, {"😀 x y", 4, []string{"😀 x", "y"}},
		{"\x1b]8;;https://example.org\x07abc\x1b]8;;\x07 d", 4, []string{"\x1b]8;;https://example.org\x07abc\x1b]8;;\x07", "d"}},
	} {
		if got := wordWrap(tt.text, tt.width); !slices.Equal(got, tt.want) {
			t.Errorf("%q: %#v, want %#v", tt.text, got, tt.want)
		}
	}
	if got := formatError(fmt.Errorf("outer: %w", errors.New("inner\n\nlonger line")), 12); !strings.Contains(got, "Caused by:") || !strings.Contains(got, "│ \n") {
		t.Fatal(got)
	}
	if got := formatError(errors.New("test"), 0); got != "Error:\n┌\n│ test\n└" {
		t.Fatal(got)
	}
	e := &cycleError{}
	e.cause = e
	if got := formatError(e, 0); strings.Count(got, "Error:") != 1 || strings.Contains(got, "Caused by:") {
		t.Fatal(got)
	}
	e.cause = nil
	formatError(e, 0)
	if got := formatError(sliceError{1}, 0); strings.Count(got, "Caused by:") != 100 {
		t.Fatal("unbounded non-comparable errors")
	}
	root := t.TempDir()
	if got := relativePath(root, root, false); got != "."+string(filepath.Separator) {
		t.Fatal(got)
	}
	if got := relativePath(filepath.Dir(root), root, false); got != ".." {
		t.Fatal(got)
	}
	t.Chdir(root)
	if err := os.Chmod(root, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0700)
	if got := relativePath("a", root, false); got != "a" {
		t.Fatal(got)
	}
	if got := relativePath(root, "relative", false); got != root {
		t.Fatal(got)
	}
}

type cycleError struct{ cause error }

func (*cycleError) Error() string   { return "cycle" }
func (e *cycleError) Unwrap() error { return e.cause }

type sliceError []int

func (sliceError) Error() string   { return "slice" }
func (e sliceError) Unwrap() error { return e }
