package rocketwind

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestEmbeddedCompilationAndManifest(t *testing.T) {
	files := fstest.MapFS{
		"assets/main.css": {Data: []byte(`@import "tailwindcss"; @import "../parts"; @config "../theme.json"; @source inline("grid flex"); @source not inline("hidden");`)},
		"parts/index.css": {Data: []byte(`.card { background:url(./card.png) }`)},
		"theme.json":      {Data: []byte(`{"theme":{"extend":{"colors":{"brand":"red"}}},"blocklist":["block"],"plugins":["@tailwindcss/forms"]}`)},
	}
	loader := Filesystem{FS: files, Root: "assets"}
	sheet, err := loader.LoadStylesheet("./main.css", "assets")
	if err != nil {
		t.Fatal(err)
	}
	compiler, err := Compile(sheet.Content, Config{Base: sheet.Base, LoadStylesheet: loader.LoadStylesheet, LoadModule: loader.LoadModule, ExistingClasses: ClassSet{"existing": {}}})
	if err != nil {
		t.Fatal(err)
	}
	input := []string{"flex", "text-brand", "grid", "flex", "hidden", "block", "", "existing", "unknown"}
	before := slices.Clone(input)
	manifest := compiler.Candidates(input)
	if !slices.Equal(manifest, []string{"flex", "grid", "text-brand", "unknown"}) || !slices.Equal(input, before) {
		t.Fatal(manifest, input)
	}
	css, err := compiler.Build(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{".flex", ".grid", ".text-brand", "url(../parts/card.png)"} {
		if !strings.Contains(css, want) {
			t.Fatal(want, css)
		}
	}
	if _, err := loader.LoadStylesheet("../../escape.css", "assets"); err == nil {
		t.Fatal("escaped FS root")
	}
	files["parts/item.css"] = &fstest.MapFile{Data: []byte(".item {}")}
	if _, err := loader.LoadStylesheet("parts/item", ""); err != nil {
		t.Fatal(err)
	}
}

func TestResolveStylesheetURLs(t *testing.T) {
	input := `.a {background: image-set("../media/a.png?q=1#v" 1x, url(./b.png) 2x); mask:url(#icon); x:url(/root.png); y:url(data:image/png;base64,abc); z:url(https://other.test/a); color:red; broken:url(./bad%path)}`
	css, err := ResolveURLs(input, "https://app.example/static/css/admin.css")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`url("https://app.example/static/media/a.png?q=1#v")`, "url(https://app.example/static/css/b.png)", "url(#icon)", "url(/root.png)", "url(data:image/png;base64,abc)", "url(https://other.test/a)", "url(./bad%path)"} {
		if !strings.Contains(css, want) {
			t.Fatal(want, css)
		}
	}
	for _, base := range []string{"relative.css", "https://bad%host/a"} {
		if _, err := ResolveURLs(input, base); err == nil {
			t.Fatal(base)
		}
	}
	if _, err := ResolveURLs(`.broken {`, "https://app.test/a"); err == nil {
		t.Fatal("invalid CSS accepted")
	}
}

func TestVirtualFilesystemIgnoresHostSymlinks(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "other"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(root, "other"), filepath.Join(root, "assets")); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	loader := Filesystem{Root: "assets", FS: fstest.MapFS{"assets/main.css": {Data: []byte(`.image {background:url(./icon.png);mask:url(../../media/icon.svg)}`)}}}
	sheet, err := loader.LoadStylesheet("./main.css", "assets")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sheet.Content, "url(./icon.png)") || !strings.Contains(sheet.Content, "url(../../media/icon.svg)") {
		t.Fatal(sheet.Content)
	}
}

func TestVirtualFilesystemRejectsMismatchedRoot(t *testing.T) {
	loader := Filesystem{Root: "/absolute", FS: fstest.MapFS{"assets/main.css": {Data: []byte(`.image {background:url(./icon.png)}`)}}}
	if _, err := loader.LoadStylesheet("./main.css", "assets"); err == nil {
		t.Fatal("absolute virtual root accepted")
	}
}
