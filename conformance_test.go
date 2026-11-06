package rocketwind

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestUpstreamUtilities(t *testing.T) {
	checkCompilerSuite(t, "utilities", 749)
}

func TestUpstreamVariants(t *testing.T) {
	checkCompilerSuite(t, "variants", 1009)
}

func TestUpstreamStylesheets(t *testing.T) {
	for _, suite := range []struct {
		name  string
		count int
	}{{"index", 252}, {"prefix", 14}, {"important", 4}, {"css-functions", 75}, {"at-import", 27}, {"compiler-lifetime", 1}, {"compat_legacy-utilities", 13}} {
		t.Run(suite.name, func(t *testing.T) { checkCompilerSuite(t, suite.name, suite.count) })
	}
}

func checkCompilerSuite(t *testing.T, suite string, count int) {
	t.Helper()
	data, err := os.ReadFile("testdata/upstream/" + suite + ".test.ts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Test, Kind, Result, Error string
		Args                      []json.RawMessage
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != count {
		t.Fatalf("got %d calls, want %d", len(cases), count)
	}
	var failures []map[string]any
	passed := 0
	for i, tt := range cases {
		var input string
		json.Unmarshal(tt.Args[0], &input)
		config := Config{}
		var options struct {
			Polyfills   *Polyfills
			Modules     []upstreamModule
			Base        string
			Stylesheets []struct {
				Args   []string
				Result Stylesheet
				Error  string
			}
		}
		if len(tt.Args) > 1 {
			if err := json.Unmarshal(tt.Args[1], &options); err != nil {
				t.Fatal(err)
			}
		}
		config.Base = options.Base
		if options.Polyfills != nil {
			config.DisablePolyfills = PolyfillAll &^ *options.Polyfills
		}
		if len(options.Modules) > 0 {
			config.LoadModule = upstreamModuleLoader(options.Modules)
		}
		if options.Stylesheets != nil {
			config.LoadStylesheet = func(id, base string) (Stylesheet, error) {
				for _, loaded := range options.Stylesheets {
					if len(loaded.Args) >= 2 && loaded.Args[0] == id && loaded.Args[1] == base {
						if loaded.Error != "" {
							return Stylesheet{}, fmt.Errorf("%s", loaded.Error)
						}
						return loaded.Result, nil
					}
				}
				return Stylesheet{}, fmt.Errorf("uncaptured stylesheet %q from %q", id, base)
			}
		}
		if suite == "variants" && tt.Test == "matchVariant sorts deterministically" {
			// Port the upstream test's plugin callback to Go; retain its reference CSS.
			config.Plugins = map[string]Plugin{"./plugin.js": func(api *PluginAPI) error {
				api.MatchVariant(Variant{Name: "is-data", Values: []NamedValue{{"", "default"}, {"foo", "foo"}, {"bar", "bar"}}, Match: func(value, modifier string) string { return "&:is([data-" + value + "])" }})
				return nil
			}}
		}
		c, err := Compile(input, config)
		got := ""
		if err == nil && tt.Kind == "compile.build" {
			var builds [][]string
			json.Unmarshal(tt.Args[2], &builds)
			for _, classes := range builds {
				got, err = c.Build(classes)
				if err != nil {
					break
				}
			}
		}
		ok := err == nil && tt.Error == "" && strings.TrimSpace(got) == strings.TrimSpace(tt.Result) || err != nil && err.Error() == tt.Error
		if !ok {
			failures = append(failures, map[string]any{"index": i, "test": tt.Test, "input": input, "got": got, "want": tt.Result, "error": fmt.Sprint(err), "wantError": tt.Error, "args": tt.Args})
		} else {
			passed++
		}
	}
	if len(failures) > 0 {
		os.MkdirAll(".cache", 0755)
		report, _ := json.MarshalIndent(failures, "", "  ")
		os.WriteFile(".cache/"+suite+"-failures.json", report, 0644)
		t.Fatalf("%d/%d upstream calls match; %d failures in .cache/%s-failures.json (first: %s)", passed, len(cases), len(failures), suite, failures[0]["test"])
	}
}
