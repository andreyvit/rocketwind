package rocketwind

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

type scannerFixtureFile struct {
	Path, Kind, Content, Target string
	Bytes                       []byte
	Mtime                       int
}
type scannerFixture struct {
	Test   string
	Roots  int
	States [][]scannerFixtureFile
	Steps  []struct {
		Method       string
		Args, Result json.RawMessage
		State        int
	}
}

func TestUpstreamScanner(t *testing.T) {
	checkScannerFixtures(t, "scanner", 89, 382)
}

func TestUpstreamScannerInternals(t *testing.T) {
	checkScannerFixtures(t, "scanner-internals", 27, 29)
}

func checkScannerFixtures(t *testing.T, name string, instances, calls int) {
	t.Helper()
	data, err := os.ReadFile("testdata/upstream/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []scannerFixture
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != instances {
		t.Fatalf("got %d instances, want %d", len(cases), instances)
	}
	var failures []any
	count := 0
	for index, tt := range cases {
		root := t.TempDir()
		root, err = filepath.EvalSymlinks(root)
		if err != nil {
			t.Fatal(err)
		}
		var pairs []string
		for i := range tt.Roots {
			pairs = append(pairs, "$ROOT"+strconv.Itoa(i), filepath.Join(root, strconv.Itoa(i)))
		}
		replace := strings.NewReplacer(pairs...)
		var reverse []string
		for i := 0; i < len(pairs); i += 2 {
			reverse = append(reverse, pairs[i+1], pairs[i])
		}
		normalize := strings.NewReplacer(reverse...)
		var scanner *Scanner
		var previous []scannerFixtureFile
		for stepIndex, step := range tt.Steps {
			count++
			state := tt.States[step.State]
			applyScannerState(t, replace, previous, state)
			previous = state
			var got any
			switch step.Method {
			case "prepare_sources":
				var sources []Source
				decodeScannerArg(t, replace, step.Args, &sources)
				prepared, _ := prepareScanSources(sources)
				items := []any{}
				for _, source := range prepared {
					items = append(items, map[string]any{"base": source.base, "pattern": source.pattern, "kind": source.kind})
				}
				got = items
			case "normalize_source":
				var source Source
				decodeScannerArg(t, replace, step.Args, &source)
				v := normalizeScanSource(source)
				got = map[string]any{"base": v.base, "pattern": v.pattern, "negated": source.Negated}
			case "posix_path":
				var input string
				decodeScannerArg(t, replace, step.Args, &input)
				got = filepath.ToSlash(input)
			case "new":
				var sources []Source
				decodeScannerArg(t, replace, step.Args, &sources)
				scanner = NewScanner(sources)
			case "scan":
				got = scanner.Scan()
			case "files":
				got = scanner.Files()
			case "scanned_files":
				got = scanner.ScannedFiles()
			case "globs", "normalized_sources", "optimize_globs":
				var globs []Glob
				if step.Method == "optimize_globs" {
					decodeScannerArg(t, replace, step.Args, &globs)
					globs = optimizeScanGlobs(globs)
				} else if step.Method == "globs" {
					globs = scanner.Globs()
				} else {
					globs = scanner.Sources()
				}
				items := []any{}
				for _, g := range globs {
					items = append(items, map[string]any{"base": g.Base, "pattern": g.Pattern})
				}
				got = items
			case "scan_content":
				var inputs []struct{ File, Content, Extension string }
				decodeScannerArg(t, replace, step.Args, &inputs)
				var content []Content
				for _, v := range inputs {
					content = append(content, Content{Path: v.File, Text: v.Content, Extension: v.Extension})
				}
				got = scanner.ScanContent(content)
			case "positions":
				var input struct{ File, Content, Extension string }
				decodeScannerArg(t, replace, step.Args, &input)
				items := []any{}
				for _, v := range scanner.Positions(Content{Path: input.File, Text: input.Content, Extension: input.Extension}) {
					items = append(items, []any{v.Value, v.Start})
				}
				got = items
			default:
				t.Fatal(step.Method)
			}
			encoded, _ := json.Marshal(got)
			if string(encoded) == "null" && step.Method != "new" {
				encoded = []byte("[]")
			}
			if err := json.Unmarshal([]byte(normalize.Replace(string(encoded))), &got); err != nil {
				t.Fatal(err)
			}
			var want any
			if err := json.Unmarshal(step.Result, &want); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, want) {
				failures = append(failures, map[string]any{"index": index, "step": stepIndex, "test": tt.Test, "method": step.Method, "got": got, "want": want})
			}
		}
	}
	if count != calls {
		t.Fatalf("got %d calls, want %d", count, calls)
	}
	if len(failures) > 0 {
		data, _ := json.MarshalIndent(failures, "", "  ")
		os.MkdirAll(".cache", 0755)
		file := ".cache/" + name + "-failures.json"
		os.WriteFile(file, data, 0644)
		t.Fatalf("%d/%d upstream scanner calls match; see %s", count-len(failures), count, file)
	}
}

func decodeScannerArg(t *testing.T, replace *strings.Replacer, data []byte, out any) {
	t.Helper()
	if err := json.Unmarshal([]byte(replace.Replace(string(data))), out); err != nil {
		t.Fatal(err)
	}
}

func applyScannerState(t *testing.T, replace *strings.Replacer, previous, current []scannerFixtureFile) {
	t.Helper()
	for _, file := range previous {
		if !slices.ContainsFunc(current, func(v scannerFixtureFile) bool { return v.Path == file.Path && v.Kind == file.Kind }) {
			if err := os.RemoveAll(replace.Replace(file.Path)); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, file := range current {
		name := replace.Replace(file.Path)
		if !strings.Contains(name, string(filepath.Separator)+"TestUpstreamScanner") {
			t.Fatalf("fixture path outside test tree: %s", name)
		}
		if slices.ContainsFunc(previous, func(v scannerFixtureFile) bool { return reflect.DeepEqual(v, file) }) {
			continue
		}
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			t.Fatal(err)
		}
		var err error
		switch file.Kind {
		case "dir":
			err = os.MkdirAll(name, 0755)
		case "link":
			os.Remove(name)
			err = os.Symlink(replace.Replace(file.Target), name)
		case "file":
			data := []byte(file.Content)
			if file.Bytes != nil {
				data = file.Bytes
			}
			err = os.WriteFile(name, data, 0644)
			if err == nil {
				when := time.Unix(1700000000+int64(file.Mtime), 0)
				err = os.Chtimes(name, when, when)
			}
		default:
			err = fmt.Errorf("unknown fixture kind %s", file.Kind)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
}

// Ported from crates/oxide/src/cursor.rs::test_cursor.
func TestUpstreamExtractionCursor(t *testing.T) {
	c := scanCursor{input: "hello world"}
	for _, step := range []struct {
		advance, pos     int
		prev, curr, next byte
	}{{0, 0, 0, 'h', 'e'}, {1, 1, 'h', 'e', 'l'}, {10, 11, 'd', 0, 0}, {1, 11, 'd', 0, 0}} {
		c.move(c.pos + step.advance)
		if c.pos != step.pos || c.prev() != step.prev || c.curr() != step.curr || c.next() != step.next {
			t.Fatalf("cursor at %d: %d %d %d", c.pos, c.prev(), c.curr(), c.next())
		}
	}
}

func TestStylesheetScanner(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{"outside.gox": "flex", "views/widget.gox": "grid", "extra/template.pug": ".block.p-4"} {
		name = filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, tt := range []struct {
		css  string
		want []string
	}{
		{`@tailwind utilities;`, []string{"block", "flex", "grid", "p-4"}},
		{`@tailwind utilities source(none); @source "./views";`, []string{"grid"}},
		{`@tailwind utilities source("./views"); @source "./extra";`, []string{"block", "grid", "p-4"}},
	} {
		c, err := Compile(tt.css, Config{Base: root})
		if err != nil {
			t.Fatal(err)
		}
		s := c.NewScanner(root)
		if got := s.Scan(); !slices.Equal(got, tt.want) {
			t.Fatalf("%s: %v", tt.css, got)
		}
		if got := s.Scan(); !slices.Equal(got, tt.want) || len(s.ScannedFiles()) != 0 {
			t.Fatalf("unchanged scan: %v (%v)", got, s.ScannedFiles())
		}
	}
	s := NewScanner(nil)
	if got := s.ScanContent([]Content{{Path: filepath.Join(root, "missing.html")}, {Path: filepath.Join(root, "outside.gox")}, {Text: "flex", Extension: "html"}}); !slices.Equal(got, []string{"flex"}) {
		t.Fatal(got)
	}
	if got := s.Positions(Content{Path: root}); len(got) != 0 {
		t.Fatal(got)
	}
	s.addContent(Content{Path: filepath.Join(root, "missing.html")}, false, nil)
	if got := s.Scan(); !slices.Equal(got, []string{"flex"}) {
		t.Fatal(got)
	}
}

func TestScannerGlobMatching(t *testing.T) {
	for _, tt := range []struct {
		pattern, path string
		want          bool
	}{
		{"*.html", "a/b.html", false}, {"**/*.html", "a/b.html", true}, {"**/*.html", "index.html", true},
		{"a/**", "a/b/c", true}, {"a/?x.html", "a/ax.html", true}, {"a/?x.html", "a/abx.html", false},
		{"a/[ab].html", "a/b.html", true}, {"a/[!ab].html", "a/c.html", true}, {"a/[!ab].html", "a/b.html", false},
		{"a/[a-z].html", "a/b.html", true}, {"a/[[", "a/[[", true}, {"a/[[", "a/other", false},
		{`a/\*.html`, "a/*.html", true}, {`a/\*.html`, "a/b.html", false}, {"a/\\", "a/\\", true}, {"[z-a]", "x", false},
	} {
		if got := newScanGlob(tt.pattern).match(tt.path); got != tt.want {
			t.Errorf("%q matches %q: %v", tt.pattern, tt.path, got)
		}
	}
	rules := newScanIgnores("", "# comment\n\n*.tmp   \r\n!keep.tmp\n\\#file\nname\\ \n{unbalanced\n", false)
	for path, want := range map[string]int{"a.tmp": 1, "keep.tmp": -1, "#file": 1, "name ": 1, "{unbalanced": 1, "normal": 0} {
		if got := scanIgnoreMatch(rules, path, false); got != want {
			t.Errorf("%q: %d, want %d", path, got, want)
		}
	}
}

func BenchmarkScanner(b *testing.B) {
	b.Run("content-cached", func(b *testing.B) {
		s := NewScanner(nil)
		input := []Content{{Text: `<div class="flex p-4 hover:text-red-500" style="color:var(--color-brand)"></div>`, Extension: "html"}}
		s.ScanContent(input)
		b.ReportAllocs()
		for b.Loop() {
			s.ScanContent(input)
		}
	})
	b.Run("files-cached", func(b *testing.B) {
		root := b.TempDir()
		for i := range 1000 {
			name := filepath.Join(root, fmt.Sprint(i/50), fmt.Sprint(i)+".html")
			if err := os.MkdirAll(filepath.Dir(name), 0755); err != nil {
				b.Fatal(err)
			}
			if err := os.WriteFile(name, []byte(`<div class="flex p-4 hover:text-red-500"></div>`), 0644); err != nil {
				b.Fatal(err)
			}
		}
		s := NewScanner([]Source{{Base: root, Pattern: "**/*"}})
		s.Scan()
		b.ReportAllocs()
		for b.Loop() {
			s.Scan()
		}
	})
}

func BenchmarkScannerLegacyBundledJavaScript(b *testing.B) {
	var source strings.Builder
	for i := range 1000 {
		fmt.Fprintf(&source, `const component%d={view:"<div class='flex p-4 hover:text-red-500 w-[var(--card-width)]'> Card </div>",data:{id:%d,kind:"card"}}; `, i, i)
	}
	input := []Content{{Text: source.String(), Extension: "js"}}
	b.SetBytes(int64(source.Len()))
	b.ReportAllocs()
	for b.Loop() {
		scanner := NewScanner(nil, ScannerOptions{Compatibility: Tailwind3})
		scanner.ScanContent(input)
	}
}
