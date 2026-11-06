package rocketwind

import (
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

func TestTailwind3OriginalEdges(t *testing.T) {
	data, err := os.ReadFile("testdata/v3/edges.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases struct {
		Normalize, Pseudos []struct{ Input, Output string }
		Sheets             []struct {
			Name, CSS, Classes, Result string
			Config                     ConfigObject
		}
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tt := range cases.Normalize {
		if got := v3Normalize(tt.Input, ""); got != tt.Output {
			t.Errorf("normalize %q: got %q, want %q", tt.Input, got, tt.Output)
		}
	}
	for _, tt := range cases.Pseudos {
		if got := v3MovePseudos(tt.Input); strings.ReplaceAll(got, ", ", ",") != strings.ReplaceAll(tt.Output, ", ", ",") {
			t.Errorf("pseudos %q: got %q, want %q", tt.Input, got, tt.Output)
		}
	}
	for _, tt := range cases.Sheets {
		t.Run(tt.Name, func(t *testing.T) {
			c, err := Compile(tt.CSS, Config{Compatibility: Tailwind3})
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.Build(NewScanner(nil, ScannerOptions{Compatibility: Tailwind3}).ScanContent([]Content{{Text: tt.Classes}}))
			if err != nil {
				t.Fatal(err)
			}
			if normalizeV3CSS(t, got) != normalizeV3CSS(t, tt.Result) {
				t.Fatalf("got %s\nwant %s", got, tt.Result)
			}
		})
	}
}

func TestTailwind3Errors(t *testing.T) {
	for _, input := range []string{
		"a {", `@import "missing.css";`, `@plugin "missing";`,
		`@layer utilities {.x { @screen sm { @apply flex; }}}`,
		`.x {@media print {@apply flex;}}`, `.x {@screen missing {color:red}}`,
		`@layer components {@screen missing {.x {color:red}}}`,
		`@tailwind utilities; @layer utilities {h1 {color:theme(colors.no-such-color)}}`,
		`@tailwind utilities; .x {color:theme(colors.no-such-color)} @layer utilities {h1 {color:red}}`,
		`@tailwind utilities; @layer utilities {h1 {@apply no-such-utility;}}`,
		`.x {@apply no-such-utility;}`, `.x {color:theme(colors.no-such-color)}`,
	} {
		if _, err := Compile(input, Config{Compatibility: Tailwind3}); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
	for _, pattern := range []any{42, "[", (*regexp.Regexp)(nil)} {
		_, err := NewWithPlugins(Config{Compatibility: Tailwind3}, Legacy(ConfigObject{{"safelist", []any{ConfigObject{{"pattern", pattern}}}}}))
		if err == nil {
			t.Errorf("accepted %v", pattern)
		}
	}
	for _, p := range []Plugin{Forms(FormsOptions{Version: "unknown"}), Typography(TypographyOptions{Version: "unknown"})} {
		if _, err := NewWithPlugins(Config{}, p); err == nil {
			t.Fatal("accepted unknown version")
		}
	}
	failure := errors.New("broken import")
	_, err := Compile(`@import "broken.css";`, Config{Compatibility: Tailwind3, LoadStylesheet: func(string, string) (Stylesheet, error) { return Stylesheet{}, failure }})
	if !errors.Is(err, failure) {
		t.Fatal(err)
	}
}

func TestTailwind3PrintRoundTrip(t *testing.T) {
	for _, prefix := range []string{"", "tw-"} {
		c, err := NewWithPlugins(Config{Compatibility: Tailwind3}, Legacy(ConfigObject{{"prefix", prefix}}))
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range []string{"hover:" + prefix + "text-[length:var(--size)]/[var(--leading)]", "!" + prefix + "flex", "-" + prefix + "mt-2", prefix + "w-[var(--width)]", "hover:![color:red]", "supports-[display:grid]:" + prefix + "flex", "group-hover/item:" + prefix + "flex"} {
			candidates := slices.Collect(c.Parse(raw))
			if len(candidates) == 0 {
				t.Fatalf("parse %q: %+v", raw, candidates)
			}
			printed := c.Print(candidates[0])
			reparsed := slices.Collect(c.Parse(printed))
			if len(reparsed) == 0 {
				t.Fatalf("print %q: %q", raw, printed)
			}
			want, err := c.Build([]string{raw})
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.Build([]string{printed})
			if err != nil {
				t.Fatal(err)
			}
			// A normalized spelling may differ, but declarations and variants must survive.
			if strings.ReplaceAll(got, v3Escape(printed), "CLASS") != strings.ReplaceAll(want, v3Escape(raw), "CLASS") {
				t.Fatalf("%q -> %q:\n%s\n%s", raw, printed, want, got)
			}
		}
		for range c.Parse(prefix + "flex") {
			break
		}
	}
}

func TestTailwind3SafelistAndCompletions(t *testing.T) {
	c, err := NewWithPlugins(Config{Compatibility: Tailwind3}, Legacy(ConfigObject{
		{"blocklist", []any{"flex", 7}},
		{"safelist", []any{ConfigObject{{"pattern", regexp.MustCompile(`^!font-bold$`)}, {"variants", []any{"hover"}}}}}}), func(api *PluginAPI) error {
		api.AddUtility("custom", declarations("display", "grid"))
		return api.MatchUtility(Utility{Name: "custom-size", Values: []NamedValue{{"2", "2px"}}, Modifiers: []NamedValue{{"small", "1px"}}, SupportsNegative: true, Compile: func(v, m string) []*Node { return declarations("width", v) }})
	})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"flex"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(css, "display: flex") || !strings.Contains(css, "font-weight: 700 !important") {
		t.Fatal(css)
	}
	var names []string
	for _, v := range c.ClassList() {
		names = append(names, v.Name)
	}
	for _, name := range []string{"custom", "custom-size-2", "-custom-size-2"} {
		if !slices.Contains(names, name) {
			t.Fatal(name)
		}
	}
	input := ConfigObject{{"a", 1}, {"4294967295", 2}, {"10", 3}, {"02", 4}, {"2", 5}}
	var keys []string
	for _, v := range v3ObjectEntries(input) {
		keys = append(keys, v.Key)
	}
	if !slices.Equal(keys, []string{"2", "10", "a", "4294967295", "02"}) {
		t.Fatal(keys)
	}
}

func TestTailwind3ScannerState(t *testing.T) {
	s := NewScanner(nil, ScannerOptions{Compatibility: Tailwind3})
	first := s.ScanContent([]Content{{Text: "\n flex p-2\nflex p-2\n"}})
	if !slices.Contains(first, "flex") || !slices.Contains(first, "p-2") {
		t.Fatal(first)
	}
	if got := s.ScanContent([]Content{{Text: " flex p-2 \n"}}); len(got) != 0 {
		t.Fatal(got)
	}
	tokens := s.Positions(Content{Text: `<div class="hover:bg-red-500">`})
	found := false
	for _, v := range tokens {
		if v.Value == "hover:bg-red-500" {
			found = true
			if v.Start != 12 || v.End != 28 {
				t.Fatal(v)
			}
		}
	}
	if !found {
		t.Fatal(tokens)
	}
}

func TestTailwind3NamedOutputAndPluginColors(t *testing.T) {
	c, err := NewWithPlugins(Config{Compatibility: Tailwind3}, func(api *PluginAPI) error {
		return api.MatchUtility(Utility{Name: "paint", Types: []ValueType{TypeColor}, Values: []NamedValue{{"brand", "rgb(1 2 3 / <alpha-value>)"}}, Compile: func(v, m string) []*Node { return declarations("color", v) }})
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, class := range []string{"paint-brand", "paint-brand/50", "paint-brand/[.7]", "paint-brand/unknown"} {
		css, err := c.Build([]string{class})
		if err != nil {
			t.Fatal(err)
		}
		if (css == "") != strings.HasSuffix(class, "unknown") {
			t.Fatal(class, css)
		}
	}
	tmpl, err := c.Prepare("p-{gap} text-{size} text-{color} hover:font-bold", TemplateOptions{TypeOf: templateTypes(map[string]ValueType{"gap": TypeLength, "size": TypeLength, "color": TypeColor})})
	if err != nil {
		t.Fatal(err)
	}
	css, err := tmpl.CSS(".card", func(p Parameter) (Value, error) {
		v := map[string]string{"gap": "3px", "size": "18px", "color": "red"}[p.Expression]
		return Value{CSS: v}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"padding: 3px;", "font-size: 18px;", "color: rgb(255 0 0 / var(--tw-text-opacity, 1));", ".card:hover"} {
		if !strings.Contains(css, want) {
			t.Fatal(css)
		}
	}
	ast := c.CandidateAST([]string{"flex"})
	if len(ast) != 1 || len(ast[0]) != 1 {
		t.Fatal(ast)
	}
	_, sourceMap, err := c.BuildWithSourceMap([]string{"flex"})
	if err != nil || sourceMap == nil {
		t.Fatal(sourceMap, err)
	}
	if reflect.DeepEqual(ast, c.CandidateAST([]string{"grid"})) {
		t.Fatal("AST lost declarations")
	}
}

// Boundary cases which do not need a filesystem or JavaScript host.
func TestTailwind3ValueContracts(t *testing.T) {
	if got := (&PluginAPI{compiler: New(Config{Compatibility: Tailwind3, Prefix: "tw-"})}).Prefix(".card"); got != ".tw-card" {
		t.Fatal(got)
	}
	if got := v3ClipCandidate("bg-[red]]'tail"); got != "bg-[red]" {
		t.Fatal(got)
	}

	c := New(Config{Compatibility: Tailwind3})
	b := buildContext{compiler: c}
	for _, tt := range []struct {
		value, mod, want string
		arbitrary        bool
	}{
		{"rgb(1 2 3 / <alpha-value>)", "", "rgb(1 2 3 / 1)", false},
		{"#123456", "50", "rgb(18 52 86 / 0.5)", false},
		{"#123456", "missing", "", false},
	} {
		if got := b.colorAlpha(tt.value, tt.mod, tt.arbitrary); got != tt.want {
			t.Fatalf("%+v -> %q", tt, got)
		}
	}
	if got := b.v3Functional("-bg", "black", "", "", false, false); len(got) != 0 {
		t.Fatal("color accepted a negative sign")
	}
	if got := b.v3Functional("p", "2", "3", "", false, false); len(got) != 0 {
		t.Fatal("padding accepted a modifier")
	}
	if !v3MatchesType("1px 2px red, 0 0 blue", "shadow") {
		t.Fatal("rejected valid shadow")
	}
	ambiguous := []v3UtilityDefinition{{Namespace: "a", Types: []string{"length"}}, {Namespace: "b", Types: []string{"length"}}}
	if len(v3SelectDefinitions(ambiguous, "2px", "")) != 0 {
		t.Fatal("guessed ambiguous length")
	}
	if v3ValidValue("}") {
		t.Fatal("accepted unmatched delimiter")
	}
	if got := v3ClipCandidate("bg-[red]'tail"); got != "bg-[red]" {
		t.Fatal(got)
	}
	if got := v3Normalize("calc([foo+2)", ""); got != "calc([foo+2)" {
		t.Fatal(got)
	}
	if v3ParsableNodes([]*Node{{Kind: "rule", Name: "a", Nodes: declarations("color", "red; display: none")}}) {
		t.Fatal("accepted a second declaration inside a value")
	}
	if got := v3FilterRules([]*Node{{Kind: "v3-rule", Name: ".other", Nodes: declarations("color", "red")}}); len(got) != 0 {
		t.Fatal("retained a selector outside the applied class")
	}
	for _, v := range []*parsedVariant{{root: "group"}, {root: "unknown", value: "value"}} {
		if _, ok := b.applyV3Variant(declarations("display", "flex"), v, 0); ok {
			t.Fatal("accepted incomplete variant")
		}
	}
	css, err := c.Build([]string{"[>_&]:flex", "visited:before:text-red-500", "before:visited:text-red-500"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(css, "--tw-text-opacity") || !strings.Contains(css, ":visited::before") {
		t.Fatal(css)
	}
	input := []*Node{{Kind: "rule", Name: "a", Nodes: []*Node{{Kind: "comment", Value: "fallback"}, {Kind: "declaration", Name: "height", Value: "1px"}, {Kind: "declaration", Name: "height", Value: "2px"}}}}
	collapsed := v3Collapse(input)
	if len(collapsed[0].Nodes) != 2 || collapsed[0].Nodes[0].Kind != "comment" || collapsed[0].Nodes[1].Value != "2px" {
		t.Fatal(collapsed[0].Nodes)
	}
}

func TestTailwind3ProgrammaticBase(t *testing.T) {
	configured, err := NewWithPlugins(Config{Compatibility: Tailwind3}, Legacy(ConfigObject{{"prefix", "custom-"}}))
	if err != nil {
		t.Fatal(err)
	}
	if got := (&PluginAPI{compiler: configured}).Prefix(".card"); got != ".custom-card" {
		t.Fatal(got)
	}
	plugin := func(api *PluginAPI) error {
		if got := api.Prefix(".card"); got != ".tw-card" {
			t.Fatal(got)
		}
		api.AddBase([]*Node{{Kind: "rule", Name: "body", Nodes: declarations("color", "red")}})
		return nil
	}
	c, err := Compile(`@plugin "base"; @tailwind base; --custom: red;`, Config{Compatibility: Tailwind3, Prefix: "tw-", LoadModule: func(string, string, string) (Module, error) { return Module{Plugin: plugin}, nil }})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build(nil)
	if err != nil || !strings.Contains(css, "body {\n  color: red;") || !strings.Contains(css, "--custom: red;") {
		t.Fatal(css, err)
	}
	// A global declaration inside a wrapper remains in that wrapper.
	got := (&buildContext{compiler: c}).renderV3([]*Node{{Kind: "at-rule", Name: "@media", Value: "print", Nodes: declarations("--custom", "blue")}})
	if !strings.Contains(got, "@media print {\n  --custom: blue;") {
		t.Fatal(got)
	}
	fs := Filesystem{Compatibility: Tailwind3}
	module, err := fs.LoadModule("@tailwindcss/typography", "", "plugin")
	if err != nil || module.Legacy == nil {
		t.Fatal(module, err)
	}
}
