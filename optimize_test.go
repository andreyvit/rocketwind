package rocketwind

import (
	"compress/gzip"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestOptimize(t *testing.T) {
	for _, test := range []struct{ input, want string }{
		{"", ""},
		{"/* ordinary */ /*! keep */ .a { display: flex; color: red; }", "/*! keep */.a{display:flex;color:red}"},
		{".a {color:red;&:hover {color:blue} color:green}", ".a{color:red}.a:hover{color:blue}.a{color:green}"},
		{".a, #b { color:red; .child { display:flex } }", ".a,#b{color:red}:is(.a,#b) .child{display:flex}"},
		{".a { @media print {color: red; &:hover {color:blue}} }", "@media print{.a{color:red}.a:hover{color:blue}}"},
		{"@layer first {} @layer second; @unknown foo {}", "@layer first{}@layer second;@unknown foo{}"},
		{".a{display:-webkit-box;display:flex;--blank: ;--blank2:  ;color:red!important}", ".a{display:-webkit-box;display:flex;--blank: ;--blank2: ;color:red!important}"},
		{`.a {--x:foo\( bar;--y:\31  a;content:"a \\";font-family:Foo\ Bar, serif}`, `.a{--x:foo\( bar;--y:\31  a;content:"a \\";font-family:Foo\ Bar,serif}`},
		{`.a{--tw-sort: keep;--x:var(--missing, );width:calc(100% - 1px);content:'a  b'}`, `.a{--tw-sort:keep;--x:var(--missing,);width:calc(100% - 1px);content:'a  b'}`},
		{`.a{font-family:\31  a;color:r/**/ed}`, `.a{font-family:\31  a;color:r/**/ed}`},
		{`.\31  a{color:red;&:hover{color:blue}}`, `.\31  a{color:red}.\31  a:hover{color:blue}`},
		{`.\31 a{--x:;--y:!important;--z: !important}`, `.\31 a{--x:;--y:!important;--z: !important}`},
		{"@media (hover: hover) {.a{display:block}}", "@media (hover: hover){.a{display:block}}"},
	} {
		t.Run(test.input, func(t *testing.T) {
			got, err := Optimize(test.input, OptimizeOptions{Minify: true})
			if err != nil || got != test.want {
				t.Fatalf("got %q, %v; want %q", got, err, test.want)
			}
			again, err := Optimize(got, OptimizeOptions{Minify: true})
			if err != nil || again != got {
				t.Fatalf("not idempotent: %q, %v", again, err)
			}
		})
	}
	input := "/*! keep */ @layer first; .a{ color:red !important; display:flex } .b{color:blue} @media print{.c{display:block}}"
	want := "/*! keep */\n@layer first;\n.a {\n  color: red !important;\n  display: flex;\n}\n\n.b {\n  color: blue;\n}\n\n@media print {\n  .c {\n    display: block;\n  }\n}\n"
	got, err := Optimize(input, OptimizeOptions{})
	if err != nil || got != want {
		t.Fatalf("pretty: %q, %v", got, err)
	}
	if _, err := Optimize(".a{", OptimizeOptions{From: "broken.css"}); err == nil || !strings.Contains(err.Error(), "broken.css") {
		t.Fatalf("diagnostic: %v", err)
	}
}

func TestOptimizeValues(t *testing.T) {
	for input, want := range map[string]string{
		" \t\n\r\f ": " ", "": "", " foo   bar ": "foo bar", "f( x , y )": "f(x,y)",
		`"unterminated`: `"unterminated`, `"trailing\`: `"trailing\`, `\`: `\`,
		`foo\, bar`: `foo\, bar`, `foo\( bar`: `foo\( bar`, `f(\31  a)`: `f(\31  a)`,
		`"a \\"   b`: `"a \\" b`,
	} {
		if got := compactCSSValue(input); got != want {
			t.Errorf("%q: got %q, want %q", input, got, want)
		}
	}
}

// The distinct stylesheet outputs of the complete upstream core suite exercise
// both optimizer modes. The companion differential script checks their meaning
// with the pinned Lightning CSS parser and optimizer, independently of Go.
func TestOptimizeCoreStylesheets(t *testing.T) {
	file, err := os.Open("testdata/upstream/optimize.json.gz")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	reader, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	var data struct{ Cases []struct{ Test, CSS string } }
	if err := json.NewDecoder(reader).Decode(&data); err != nil {
		t.Fatal(err)
	}
	if len(data.Cases) != 849 {
		t.Fatalf("got %d cases", len(data.Cases))
	}
	for _, test := range data.Cases {
		t.Run(test.Test, func(t *testing.T) {
			for _, compact := range []bool{false, true} {
				options := OptimizeOptions{Minify: compact}
				got, err := Optimize(test.CSS, options)
				if err != nil {
					t.Fatal(err)
				}
				again, err := Optimize(got, options)
				if err != nil || again != got {
					t.Fatalf("not idempotent (%v): %v\n%s\n%s", compact, err, got, again)
				}
			}
		})
	}
}

func TestOptimizeSourceMap(t *testing.T) {
	input := ".a {\n  color: red;\n  &:hover {\n    color: blue;\n  }\n}\n"
	for _, compact := range []bool{false, true} {
		css, m, err := OptimizeWithSourceMap(input, OptimizeOptions{From: "theme.css", Minify: compact}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(m.Sources, []string{"theme.css"}) || !reflect.DeepEqual(m.SourcesContent, []string{input}) {
			t.Fatalf("map: %#v", m)
		}
		points, err := m.decode()
		if err != nil || len(points) == 0 {
			t.Fatalf("mappings: %v, %v", points, err)
		}
		if _, err := m.Visualize(css); err != nil {
			t.Fatal(err)
		}
	}
	compiler, err := Compile("@theme {--color-test: red} @tailwind utilities; .a{@apply flex}", Config{From: "original.css"})
	if err != nil {
		t.Fatal(err)
	}
	css, before, err := compiler.BuildWithSourceMap([]string{"bg-test", "hover:flex"})
	if err != nil {
		t.Fatal(err)
	}
	before.File = "app.css"
	saved, _ := json.Marshal(before)
	output, after, err := OptimizeWithSourceMap(css, OptimizeOptions{Minify: true}, before)
	if err != nil {
		t.Fatal(err)
	}
	if output == "" || !reflect.DeepEqual(after.Sources, before.Sources) || !reflect.DeepEqual(after.SourcesContent, before.SourcesContent) || after.File != "app.css" {
		t.Fatalf("composed: %#v", after)
	}
	again, _ := json.Marshal(before)
	if string(saved) != string(again) {
		t.Fatal("input map mutated")
	}
	if _, _, err := OptimizeWithSourceMap("", OptimizeOptions{}, &SourceMap{Version: 2}); err == nil {
		t.Fatal("invalid map accepted")
	}
	if _, m, err := OptimizeWithSourceMap("", OptimizeOptions{}, nil); err != nil || len(m.Sources) != 0 {
		t.Fatalf("empty: %#v, %v", m, err)
	}
	if _, m, err := OptimizeWithSourceMap(".x{color:red}", OptimizeOptions{}, &SourceMap{Version: 3}); err != nil || len(m.Sources) != 0 {
		t.Fatalf("unmapped: %#v, %v", m, err)
	}
}
