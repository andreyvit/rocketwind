package rocketwind

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestUpstreamAssetURLs(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/urls.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct{ CSS, Base, Root, Result string }
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("got %d calls, want 2", len(cases))
	}
	for _, tt := range cases {
		got, err := RewriteURLs(tt.CSS, tt.Base, tt.Root)
		if err != nil || got != tt.Result {
			t.Fatalf("%v\ngot:\n%s\nwant:\n%s", err, got, tt.Result)
		}
	}
}

func TestFilesystemStylesheetsAndModules(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"src/index.css":                     `@import "tailwindcss/utilities"; @import "../components/card"; @import "ui/colors"; @config "../theme.json"; @plugin "@tailwindcss/forms" {strategy:"class";}`,
		"components/card.css":               `.card {background:url(./card.png)}`,
		"node_modules/ui/package.json":      `{"exports":{"./colors":{"style":"./styles/colors.css"}}}`,
		"node_modules/ui/styles/colors.css": `@theme {--color-brand: red;}`,
		"theme.json":                        `{"theme":{"extend":{"spacing":{"big":"27rem"}}}}`,
	}
	for name, text := range files {
		writeTestSource(t, filepath.Join(root, name), text)
	}
	var dependencies []string
	fs := Filesystem{Root: filepath.Join(root, "src"), OnDependency: func(path string) { dependencies = append(dependencies, path) }}
	sheet, err := fs.LoadStylesheet("./index.css", filepath.Join(root, "src"))
	if err != nil {
		t.Fatal(err)
	}
	c, err := Compile(sheet.Content, Config{Base: sheet.Base, LoadStylesheet: fs.LoadStylesheet, LoadModule: fs.LoadModule})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"text-brand", "p-big", "form-input"})
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{"url(../components/card.png)", "27rem", ".text-brand", ".form-input"} {
		if !strings.Contains(css, text) {
			t.Fatalf("missing %s:\n%s", text, css)
		}
	}
	for name := range files {
		path, _ := filepath.EvalSymlinks(filepath.Join(root, name))
		if !slices.Contains(dependencies, path) {
			t.Errorf("missing dependency %s", name)
		}
	}
	fs.Modules = map[string]Module{"custom": {Theme: map[string]string{"--color-brand": "blue"}}}
	module, err := fs.LoadModule("custom", root, "config")
	if err != nil || module.Theme["--color-brand"] != "blue" {
		t.Fatal(module, err)
	}
	for _, input := range []struct{ id, kind string }{{"missing.json", "config"}, {"test.js", "config"}, {"test.js", "plugin"}} {
		if _, err := fs.LoadModule(input.id, root, input.kind); err == nil {
			t.Fatal(input)
		}
	}
	writeTestSource(t, filepath.Join(root, "invalid.json"), "{")
	if _, err := fs.LoadModule("./invalid.json", root, "config"); err == nil {
		t.Fatal("invalid JSON accepted")
	}
	if _, err := fs.LoadStylesheet("./absent.css", root); err == nil {
		t.Fatal("missing import accepted")
	}
}

func writeTestSource(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestStylesheetPackageResolution(t *testing.T) {
	root := t.TempDir()
	for name, text := range map[string]string{
		"local.css":                              "local",
		"folder/index.css":                       "folder",
		"node_modules/plain/index.css":           "plain",
		"node_modules/styled/package.json":       `{"style":"./css/style.css"}`,
		"node_modules/styled/css/style.css":      "styled",
		"node_modules/@scope/ui/package.json":    `{"exports":{".":{"style":"./style.css"},"./colors/*":[false,{"style":"./colors/*.css"}],"./hidden":null}}`,
		"node_modules/@scope/ui/style.css":       "scoped",
		"node_modules/@scope/ui/colors/blue.css": "blue",
		"node_modules/ordered/package.json":      `{"exports":{"default":"./default.css","style":"./style.css"}}`,
		"node_modules/ordered/default.css":       "default",
		"node_modules/ordered/style.css":         "style",
		"node_modules/loop/package.json":         `{"style":"."}`,
		"node_modules/broken/package.json":       "{",
		"node_modules/broken/index.css":          "broken",
	} {
		writeTestSource(t, filepath.Join(root, name), text)
	}
	fs := Filesystem{}
	for id, want := range map[string]string{"./local": "local", "local": "local", filepath.Join(root, "local.css"): "local", "./folder": "folder", "plain": "plain", "styled": "styled", "@scope/ui": "scoped", "@scope/ui/colors/blue": "blue", "ordered": "default", "broken": "broken"} {
		sheet, err := fs.LoadStylesheet(id, root)
		if err != nil || sheet.Content != want {
			t.Errorf("%s: %+v %v", id, sheet, err)
		}
	}
	for _, id := range []string{"loop", "@scope/ui/hidden", "@scope/ui/unknown"} {
		if _, err := fs.LoadStylesheet(id, root); err == nil {
			t.Fatal(id)
		}
	}
	if _, err := fs.read(filepath.Join(root, "missing")); err == nil {
		t.Fatal("missing file read")
	}
}

func TestJSONGoPlugins(t *testing.T) {
	root := t.TempDir()
	writeTestSource(t, filepath.Join(root, "theme.json"), `{"presets":[{"plugins":["@tailwindcss/line-clamp"]}],"plugins":["@tailwindcss/forms","custom"]}`)
	fs := Filesystem{Modules: map[string]Module{"custom": {Theme: map[string]string{"--color-brand": "red"}, Legacy: ConfigObject{{"theme", ConfigObject{{"extend", ConfigObject{{"spacing", ConfigObject{{"special", "42rem"}}}}}}}}, Plugins: []Plugin{func(api *PluginAPI) error {
		return api.AddUtilities([]*Node{{Kind: "rule", Name: ".custom", Nodes: declarations("color", "blue")}})
	}}}}}
	c, err := Compile(`@config "./theme.json"; @tailwind utilities;`, Config{Base: root, LoadModule: fs.LoadModule})
	if err != nil {
		t.Fatal(err)
	}
	css, err := c.Build([]string{"form-input", "line-clamp-2", "custom", "bg-brand", "p-special"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".form-input {", ".line-clamp-2 {", ".custom {", "background-color: red", "42rem"} {
		if !strings.Contains(css, want) {
			t.Fatalf("missing %s", want)
		}
	}
	fs.Modules["failure"] = Module{Configure: func(map[string]any) (Plugin, error) { return nil, errors.New("bad options") }}
	for _, body := range []string{`{"plugins":42}`, `{"plugins":[{}]}`, `{"plugins":["missing"]}`, `{"plugins":["failure"]}`, `{"presets":[{"plugins":["missing"]}]}`} {
		writeTestSource(t, filepath.Join(root, "theme.json"), body)
		if _, err := fs.LoadModule("./theme.json", root, "config"); err == nil {
			t.Fatal(body)
		}
	}
}
