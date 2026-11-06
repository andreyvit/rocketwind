package rocketwind

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestUpstreamLegacyInternals(t *testing.T) {
	for _, suite := range []struct {
		name  string
		count int
	}{
		{"compat_config_resolve-config", 5}, {"compat_config_create-compat-config", 16},
		{"compat_apply-config-to-theme", 70}, {"compat_apply-keyframes-to-theme", 2}, {"compat_flatten-color-palette", 2},
	} {
		t.Run(suite.name, func(t *testing.T) {
			data, err := os.ReadFile("testdata/upstream/" + suite.name + ".test.ts.json")
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
			if len(cases) != suite.count {
				t.Fatalf("got %d calls, want %d", len(cases), suite.count)
			}
			var failures []any
			for i, tt := range cases {
				want, err := decodeConfigValue(tt.Result)
				if err != nil {
					t.Fatal(err)
				}
				var got any
				switch {
				case strings.HasSuffix(tt.Kind, ":resolve"):
					var meta struct {
						Theme  json.RawMessage
						Prefix string
					}
					json.Unmarshal(tt.Args[1], &meta)
					c := upstreamThemeCompiler(t, meta.Theme)
					c.config.Prefix = meta.Prefix
					files, _ := decodeConfigValue(tt.Args[0])
					var sources []legacyConfigSource
					compat := false
					for _, file := range files.([]any) {
						object := configObject(file)
						config := configObject(object.Get("config"))
						if config.Get("$compat") == true {
							compat = true
							continue
						}
						config, err = portLegacyFixtureConfig(config)
						if err != nil {
							t.Fatal(err)
						}
						sources = append(sources, legacyConfigSource{config, themeString(object.Get("base"))})
					}
					if !compat {
						c.legacyTheme = ConfigObject{}
						c.legacyResolved = map[string]bool{}
						c.legacyResolving = map[string]bool{}
					}
					if len(sources) == 0 {
						sources = append(sources, legacyConfigSource{})
					}
					err = c.configureLegacy(sources)
					var replaced []string
					for key := range c.legacyReplaced {
						replaced = append(replaced, key)
					}
					if compat {
						for _, entry := range legacyThemeDefaults {
							replaced = append(replaced, entry.Key)
						}
					}
					slices.Sort(replaced)
					var names []any
					for _, key := range slices.Compact(replaced) {
						names = append(names, key)
					}
					got = ConfigObject{{"config", c.legacyConfig}, {"replaced", names}}
					object := configObject(want)
					if values, ok := object.Get("replaced").([]any); ok {
						slices.SortFunc(values, func(a, b any) int { return strings.Compare(a.(string), b.(string)) })
					}
				case strings.HasSuffix(tt.Kind, ":key"):
					var path []string
					json.Unmarshal(tt.Args[0], &path)
					if key := legacyThemeProperty(path); key != "" {
						got = strings.TrimPrefix(key, "--")
					}
				case strings.Contains(tt.Kind, "apply-config-to-theme:apply"):
					c := upstreamThemeCompiler(t, tt.Args[0])
					config, _ := decodeConfigValue(tt.Args[1])
					c.legacyPendingTheme = configObject(configObject(config).Get("theme"))
					c.legacyReplaced = map[string]bool{}
					var replaced []string
					json.Unmarshal(tt.Args[2], &replaced)
					for _, key := range replaced {
						c.legacyReplaced[key] = true
					}
					c.legacyPending = true
					err = c.finishLegacy()
					entries := []any{}
					seen := map[string]bool{}
					for _, key := range c.themeKeys() {
						if seen[key] || c.config.Theme[key] == "" {
							continue
						}
						seen[key] = true
						entries = append(entries, []any{key, ConfigObject{{"value", c.config.Theme[key]}, {"options", c.themeOptionBits(key)}}})
					}
					got = entries
				case strings.Contains(tt.Kind, "apply-keyframes-to-theme:apply"):
					var before, after []upstreamNode
					json.Unmarshal(tt.Args[0], &before)
					json.Unmarshal(tt.Result, &after)
					c := New(Config{NoDefaultTheme: true})
					c.keyframes = convertUpstreamNodes(before, "")
					config, _ := decodeConfigValue(tt.Args[1])
					c.legacyPendingTheme = configObject(configObject(config).Get("theme"))
					c.legacyPending = true
					err = c.finishLegacy()
					var result, expected strings.Builder
					writeCSS(&result, c.keyframes, 0)
					writeCSS(&expected, convertUpstreamNodes(after, ""), 0)
					got, want = result.String(), expected.String()
				case strings.HasSuffix(tt.Kind, ":flatten"):
					colors, _ := decodeConfigValue(tt.Args[0])
					got = FlattenColors(configObject(colors))
				default:
					t.Fatal(tt.Kind)
				}
				got, want = normalizePluginValue(got), normalizePluginValue(want)
				if err != nil || !reflect.DeepEqual(got, want) {
					failures = append(failures, map[string]any{"index": i, "test": tt.Test, "kind": tt.Kind, "got": got, "want": want, "error": fmt.Sprint(err)})
				}
			}
			if len(failures) > 0 {
				data, _ := json.MarshalIndent(failures, "", "  ")
				os.WriteFile(".cache/"+suite.name+"-failures.json", data, 0644)
				t.Fatalf("%d/%d upstream calls match; see .cache/%s-failures.json", len(cases)-len(failures), len(cases), suite.name)
			}
		})
	}
}

func upstreamThemeCompiler(t *testing.T, raw json.RawMessage) *Compiler {
	t.Helper()
	var entries [][]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	c := New(Config{NoDefaultTheme: true, Theme: map[string]string{}})
	c.themeFlags = map[string]string{}
	for _, entry := range entries {
		var key string
		var value struct {
			Value   string
			Options int
		}
		json.Unmarshal(entry[0], &key)
		json.Unmarshal(entry[1], &value)
		c.config.Theme[key] = value.Value
		c.themeOrder = append(c.themeOrder, key)
		for i, name := range []string{"inline", "reference", "default", "static", "used"} {
			if value.Options&(1<<i) != 0 {
				c.themeFlags[key] += " " + name
			}
		}
	}
	return c
}
