package rocketwind

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestUpstreamIntellisense(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/intellisense.test.ts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []sourceMapFixture
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 33 {
		t.Fatal(len(cases))
	}
	var failures []map[string]any
	for _, tt := range cases {
		t.Run(tt.Test+"/"+tt.Kind, func(t *testing.T) {
			var input string
			var options struct {
				Base        string
				Modules     []upstreamModule
				Stylesheets []struct {
					Args   []string
					Result Stylesheet
				}
			}
			json.Unmarshal(tt.Args[0], &input)
			json.Unmarshal(tt.Args[1], &options)
			config := Config{Base: options.Base, LoadModule: upstreamModuleLoader(options.Modules)}
			config.LoadStylesheet = func(id, base string) (Stylesheet, error) {
				for _, loaded := range options.Stylesheets {
					if loaded.Args[0] == id && loaded.Args[1] == base {
						return loaded.Result, nil
					}
				}
				return Stylesheet{}, fmt.Errorf("uncaptured stylesheet %q from %q", id, base)
			}
			c, err := Compile(input, config)
			if err != nil {
				t.Fatal(err)
			}
			var state struct {
				Theme   [][2]json.RawMessage
				Invalid []string
			}
			json.Unmarshal(tt.Args[2], &state)
			c.config.NoDefaultTheme = true
			c.config.Theme = map[string]string{}
			c.themeOrder = nil
			c.themeFlags = map[string]string{}
			for _, pair := range state.Theme {
				var key string
				var value struct {
					Value   string
					Options int
				}
				json.Unmarshal(pair[0], &key)
				json.Unmarshal(pair[1], &value)
				c.config.Theme[key] = value.Value
				c.themeOrder = append(c.themeOrder, key)
				for i, flag := range []string{"inline", "reference", "default", "static", "used"} {
					if value.Options&(1<<i) != 0 {
						c.themeFlags[key] += " " + flag
					}
				}
			}
			if c.blocklist == nil {
				c.blocklist = map[string]bool{}
			}
			for _, name := range state.Invalid {
				c.blocklist[name] = true
			}
			var got any
			switch tt.Kind {
			case "design.getClassList":
				entries := make([]any, 0)
				for _, entry := range c.ClassList() {
					entries = append(entries, []any{entry.Name, map[string]any{"modifiers": append([]string{}, entry.Modifiers...)}})
				}
				got = entries
			case "design.getVariants":
				entries := make([]any, 0)
				for _, entry := range c.VariantList() {
					entries = append(entries, map[string]any{"name": entry.Name, "values": append([]string{}, entry.Values...), "isArbitrary": entry.Arbitrary, "hasDash": entry.HasDash})
				}
				got = entries
			case "design.selectors":
				var name string
				var value struct{ Value, Modifier string }
				json.Unmarshal(tt.Args[3], &name)
				if len(tt.Args) > 4 {
					json.Unmarshal(tt.Args[4], &value)
				}
				if value.Value != "" {
					if name != "@" {
						name += "-"
					}
					name += value.Value
				}
				if value.Modifier != "" {
					name += "/" + value.Modifier
				}
				got = append([]string{}, c.VariantSelectors(name)...)
			case "design.candidatesToCss", "design.candidatesToAst":
				var classes []string
				json.Unmarshal(tt.Args[3], &classes)
				css := c.CandidateCSS(classes)
				if tt.Kind == "design.candidatesToCss" {
					entries := make([]any, len(css))
					for i, value := range css {
						if value != "" {
							entries[i] = value
						}
					}
					got = entries
				} else {
					var want [][]upstreamNode
					json.Unmarshal(tt.Result, &want)
					got := c.CandidateAST(classes)
					for i, nodes := range want {
						expected := convertUpstreamNodes(nodes, "")
						if len(got[i]) == 0 && len(expected) == 0 {
							continue
						}
						if !reflect.DeepEqual(got[i], expected) {
							t.Errorf("AST %s differs", classes[i])
						}
					}
					return
				}
			default:
				t.Fatal(tt.Kind)
			}
			encoded, _ := json.Marshal(got)
			var a, b any
			json.Unmarshal(encoded, &a)
			json.Unmarshal(tt.Result, &b)
			if !reflect.DeepEqual(a, b) {
				failures = append(failures, map[string]any{"test": tt.Test, "kind": tt.Kind, "got": a, "want": b})
				t.Errorf("result differs; details in .cache/intellisense-failures.json")
			}
		})
	}
	if len(failures) > 0 {
		data, _ := json.MarshalIndent(failures, "", "  ")
		os.WriteFile(".cache/intellisense-failures.json", data, 0644)
	}
}

func TestCompletionPluginsAndIsolation(t *testing.T) {
	c, err := NewWithPlugins(Config{Prefix: "tw", ExistingClasses: ClassSet{"tw:flex": {}}}, func(api *PluginAPI) error {
		api.MatchVariant(Variant{Name: "mode", Values: []NamedValue{{"DEFAULT", "default"}, {"compact", "compact"}}, Match: func(value, modifier string) string { return "&[data-mode=" + value + "]" }})
		if err := api.MatchUtility(Utility{Name: "item", Values: []NamedValue{{"DEFAULT", "1"}, {"x", "2"}, {"/1", "3"}, {"__BARE_VALUE__", "hidden"}, {"__CSS_VALUES__", "hidden"}}, Modifiers: []NamedValue{{"narrow", "small"}}, SupportsNegative: true, Compile: func(value, modifier string) []*Node { return declarations("width", value) }}); err != nil {
			return err
		}
		return api.MatchUtility(Utility{Name: "item", Values: []NamedValue{{"x", "2"}}, Modifiers: []NamedValue{{"wide", "large"}}, SupportsNegative: true, Compile: func(value, modifier string) []*Node { return declarations("height", value) }})
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := c.ClassList()
	for _, name := range []string{"item", "-item", "item-x", "-item-x", "item-/1"} {
		i := slices.IndexFunc(entries, func(v ClassSuggestion) bool { return v.Name == name })
		want := []string{"narrow"}
		if strings.HasSuffix(name, "-x") {
			want = append(want, "wide")
		}
		if i < 0 || !slices.Equal(entries[i].Modifiers, want) {
			t.Fatalf("missing %s with modifier", name)
		}
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name, "__") {
			t.Fatal(entry.Name)
		}
	}
	variants := c.VariantList()
	i := slices.IndexFunc(variants, func(v VariantSuggestion) bool { return v.Name == "mode" })
	if i < 0 || !slices.Equal(variants[i].Values, []string{"compact"}) {
		t.Fatal(variants)
	}
	for _, name := range []string{"[>div]", "does-not-exist"} {
		if got := c.VariantSelectors(name); len(got) > 0 {
			t.Fatal(got)
		}
	}
	// Tailwind emits no selector preview when the pseudo-element contains a
	// content declaration instead of an empty leaf rule.
	if got := c.VariantSelectors("before"); len(got) != 0 {
		t.Fatal(got)
	}
	classes := []string{"tw:flex", "tw:text-xl", "tw:font-bold", "tw:does-not-exist"}
	before := c.CandidateCSS(classes)
	if before[0] != ".tw\\:flex {\n  display: flex;\n}\n" || strings.Contains(before[1], ":root") || strings.Contains(before[2], "@supports") || before[3] != "" {
		t.Fatal(before)
	}
	nodes := c.CandidateAST(classes)
	for i, nodes := range nodes {
		var text strings.Builder
		writeCSS(&text, nodes, 0)
		if text.String() != before[i] {
			t.Errorf("CSS and AST disagree for %s", classes[i])
		}
	}
	nodes[0][0].Nodes[0].Value = "broken"
	entries[0].Name = "broken"
	variants[0].Name = "broken"
	if !slices.Equal(before, c.CandidateCSS(classes)) {
		t.Fatal("AST mutation changed compiler")
	}
	var group sync.WaitGroup
	for range 4 {
		group.Go(func() {
			if !slices.Equal(before, c.CandidateCSS(classes)) {
				t.Error("concurrent preview differs")
			}
			if c.ClassList()[0].Name == "broken" || c.VariantList()[0].Name == "broken" {
				t.Error("suggestion mutation changed compiler")
			}
		})
	}
	group.Wait()
}

func TestMergedCustomCompletions(t *testing.T) {
	c, err := Compile(`@theme { --spacing-a: 1px; --spacing-b: 2px; } @utility ex-* { width: --value(--spacing-*); } @utility ex-* { height: --value(--spacing-*); }`, Config{})
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, entry := range c.ClassList() {
		if strings.HasPrefix(entry.Name, "ex-") {
			got = append(got, entry.Name)
		}
	}
	if !slices.Equal(got, []string{"ex-a", "ex-b"}) {
		t.Fatal(got)
	}
}

func BenchmarkCompletions(b *testing.B) {
	c := New(Config{})
	b.Run("Classes", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c.ClassList()
		}
	})
	b.Run("Variants", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c.VariantList()
		}
	})
	b.Run("PreviewCSS", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c.CandidateCSS([]string{"flex", "hover:text-xl", "font-bold"})
		}
	})
	b.Run("PreviewAST", func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			c.CandidateAST([]string{"flex", "hover:text-xl", "font-bold"})
		}
	})
}
