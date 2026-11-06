package rocketwind

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInvalidCandidateEncoding(t *testing.T) {
	c := New(Config{})
	for _, raw := range []string{"w-[\xff]", "[color:\xfe]", "[&\xff]:flex"} {
		for parsed := range c.Parse(raw) {
			t.Fatalf("parsed invalid UTF-8: %#v", parsed)
		}
		css, err := c.Build([]string{raw})
		if err != nil || css != "" {
			t.Fatalf("%q: %v %s", raw, err, css)
		}
	}
}

func TestEmptyPluginUtility(t *testing.T) {
	c, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		api.AddUtility("empty", nil)
		api.AddUtility("empty", declarations("color", "red"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"empty"})
	if err != nil || css != ".empty {\n  color: red;\n}\n" {
		t.Fatalf("%v\n%s", err, css)
	}
}

func TestNegativeExtensionOfPositiveBuiltin(t *testing.T) {
	c, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		return api.MatchUtility(Utility{Name: "blur", SupportsNegative: true, Compile: func(value, modifier string) []*Node {
			return declarations("--extension", value)
		}})
	})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"-blur-[2px]"})
	if err != nil || !strings.Contains(css, "--extension: calc(2px * -1)") || strings.Contains(css, "filter:") {
		t.Fatalf("%v\n%s", err, css)
	}
}

func TestDefaultVariantOrder(t *testing.T) {
	c := New(Config{Variants: []Variant{{Name: "state", Values: []NamedValue{{"DEFAULT", "default"}, {"open", "open"}}, Match: func(value, modifier string) string { return "&[data-state=" + value + "]" }}}})
	css, err := c.Build([]string{"state-open:flex", "state:flex"})
	if err != nil || !strings.Contains(css, "data-state=default") || strings.Index(css, ".state\\:flex") > strings.Index(css, ".state-open\\:flex") {
		t.Fatalf("%v\n%s", err, css)
	}
}

func TestDefaultContainerBreakpoints(t *testing.T) {
	css, err := New(Config{}).Build([]string{"container"})
	if err != nil || strings.Count(css, "@media") != 5 || !strings.Contains(css, "width >= 40rem") || !strings.Contains(css, "width >= 96rem") {
		t.Fatalf("%v\n%s", err, css)
	}
}

func TestThemeVariableWithMalformedReference(t *testing.T) {
	c := New(Config{Theme: map[string]string{"--color-brand": "var(--unclosed"}})
	css, err := c.Build([]string{"bg-brand"})
	if err != nil || !strings.Contains(css, "--color-brand: var(--unclosed;") {
		t.Fatalf("%v\n%s", err, css)
	}
}

func TestPluginContextsKeepFollowingNodes(t *testing.T) {
	c, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		api.AddUtility("demo", []*Node{
			{Kind: "context", Nodes: declarations("color", "red")},
			{Kind: "declaration", Name: "display", Value: "flex"},
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"demo"})
	if err != nil || !strings.Contains(css, "color: red") || !strings.Contains(css, "display: flex") {
		t.Fatalf("%v\n%s", err, css)
	}
}

func TestRelativeAssetDirectories(t *testing.T) {
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	css, err := RewriteURLs(".a {background:url(a.png)}", ".", filepath.Dir(cwd))
	if err != nil || !strings.Contains(css, "url(./"+filepath.Base(cwd)+"/a.png)") {
		t.Fatalf("%v %s", err, css)
	}
	// Rel also fails for different volumes on Windows. Preserve the original
	// asset URL whenever a filesystem-relative URL cannot be represented.
	if value, ok := rewriteAssetURL("a.png", "relative", filepath.Dir(cwd)); ok || value != "" {
		t.Fatalf("invalid relative path: %q %v", value, ok)
	}
}
