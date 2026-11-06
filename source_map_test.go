package rocketwind

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
)

type sourceMapFixture struct {
	Test, Kind string
	Args       []json.RawMessage
	Result     json.RawMessage
}

func readSourceMapFixtures(t *testing.T, suite string, count int) []sourceMapFixture {
	t.Helper()
	data, err := os.ReadFile("testdata/upstream/source-maps_" + suite + ".test.ts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []sourceMapFixture
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != count {
		t.Fatalf("%d captured calls, want %d", len(cases), count)
	}
	return cases
}

func equalSourceJSON(t *testing.T, got any, want json.RawMessage) {
	t.Helper()
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	var a, b any
	json.Unmarshal(encoded, &a)
	json.Unmarshal(want, &b)
	if !reflect.DeepEqual(a, b) {
		t.Errorf("got %s\nwant %s", encoded, want)
	}
}

func TestUpstreamLineTable(t *testing.T) {
	for _, tt := range readSourceMapFixtures(t, "line-table", 46) {
		t.Run(tt.Test, func(t *testing.T) {
			var input string
			json.Unmarshal(tt.Args[0], &input)
			table := newLineTable(input)
			if strings.HasSuffix(tt.Kind, ":find") {
				var offset int
				json.Unmarshal(tt.Args[1], &offset)
				equalSourceJSON(t, table.find(offset), tt.Result)
			} else {
				var pos sourcePosition
				json.Unmarshal(tt.Args[1], &pos)
				equalSourceJSON(t, table.findOffset(pos), tt.Result)
			}
		})
	}
}

func TestUpstreamTranslationMap(t *testing.T) {
	for _, tt := range readSourceMapFixtures(t, "translation-map", 12) {
		t.Run(tt.Test, func(t *testing.T) {
			var options struct{ Original, Generated string }
			var node upstreamNode
			json.Unmarshal(tt.Args[0], &options)
			json.Unmarshal(tt.Args[1], &node)
			target := convertUpstreamNodes([]upstreamNode{node}, options.Original)[0]
			ast, err := ParseCSS(options.Original, "input.css")
			if err != nil {
				t.Fatal(err)
			}
			var out strings.Builder
			var mappings []cssMapping
			writeMappedCSS(&out, ast, 0, &mappings)
			if out.String() != options.Generated {
				t.Fatalf("got %q, want %q", out.String(), options.Generated)
			}
			original, generated := newLineTable(options.Original), newLineTable(out.String())
			for _, m := range mappings {
				if m.original.start == target.Start && m.original.end == target.End {
					equalSourceJSON(t, [][4]sourcePosition{{original.bytePosition(target.Start), original.bytePosition(target.End), generated.bytePosition(m.start), generated.bytePosition(m.end)}}, tt.Result)
					return
				}
			}
			t.Fatal("missing source location")
		})
	}
}

func TestUpstreamSourceMaps(t *testing.T) {
	for _, tt := range readSourceMapFixtures(t, "source-map", 24) {
		t.Run(tt.Test+"/"+tt.Kind, func(t *testing.T) {
			var input string
			var options struct {
				From, Base  string
				Modules     []upstreamModule
				Stylesheets []struct {
					Args   []string
					Result Stylesheet
				}
			}
			json.Unmarshal(tt.Args[0], &input)
			json.Unmarshal(tt.Args[1], &options)
			config := Config{From: options.From, Base: options.Base, LoadModule: upstreamModuleLoader(options.Modules)}
			config.LoadStylesheet = func(id, base string) (Stylesheet, error) {
				for _, s := range options.Stylesheets {
					if s.Args[0] == id && s.Args[1] == base {
						return s.Result, nil
					}
				}
				return Stylesheet{}, fmt.Errorf("uncaptured stylesheet %q from %q", id, base)
			}
			c, err := Compile(input, config)
			if err != nil {
				t.Fatal(err)
			}
			if tt.Kind == "compile.metadata" {
				var want string
				json.Unmarshal(tt.Result, &want)
				equalSourceJSON(t, struct {
					Sources  []Source    `json:"sources"`
					Root     *SourceRoot `json:"root"`
					Features Features    `json:"features"`
				}{append([]Source{}, c.Sources()...), c.Root(), c.Features()}, json.RawMessage(want))
				return
			}
			var builds [][]string
			json.Unmarshal(tt.Args[2], &builds)
			var candidates []string
			for _, build := range builds {
				candidates = append(candidates, build...)
			}
			css, decoded, err := c.build(candidates, true)
			if err != nil {
				t.Fatal(err)
			}
			if tt.Kind == "compile.build" {
				equalSourceJSON(t, css, tt.Result)
			} else {
				if t.Failed() {
					return
				}
				equalSourceJSON(t, decoded, tt.Result)
				if t.Failed() {
					got, _ := json.MarshalIndent(decoded, "", "  ")
					name := fmt.Sprint(c.Features())
					os.WriteFile(".cache/source-map-"+name+"-got.json", got, 0644)
					os.WriteFile(".cache/source-map-"+name+"-want.json", tt.Result, 0644)
				}
			}
		})
	}
}

func TestUpstreamSourceMapVisualization(t *testing.T) {
	for _, tt := range readSourceMapFixtures(t, "visualize-source-map", 7) {
		t.Run(tt.Test, func(t *testing.T) {
			var css, got string
			json.Unmarshal(tt.Args[1], &css)
			if strings.HasSuffix(tt.Kind, ":visualizeSourceMapRanges") {
				var sources map[string]string
				var ranges []visualizationRange
				json.Unmarshal(tt.Args[0], &sources)
				json.Unmarshal(tt.Args[2], &ranges)
				got = visualizeSourceMapRanges(sources, css, ranges)
			} else {
				var sourceMap SourceMap
				json.Unmarshal(tt.Args[0], &sourceMap)
				var err error
				got, err = sourceMap.Visualize(css)
				if err != nil {
					t.Fatal(err)
				}
			}
			equalSourceJSON(t, got, tt.Result)
		})
	}
}

func TestSourceMapEncoding(t *testing.T) {
	for _, suite := range []struct {
		file  string
		count int
	}{{"raw", 8}, {"extra", 12}} {
		data, err := os.ReadFile("testdata/upstream/source-map-" + suite.file + ".json")
		if err != nil {
			t.Fatal(err)
		}
		var cases []struct {
			Name, Input, CSS string
			Candidates       []string
			Decoded          decodedSourceMap
			Encoded          json.RawMessage
		}
		if err := json.Unmarshal(data, &cases); err != nil {
			t.Fatal(err)
		}
		if len(cases) != suite.count {
			t.Fatal(len(cases))
		}
		for _, tt := range cases {
			t.Run(suite.file+"/"+tt.Name, func(t *testing.T) {
				equalSourceJSON(t, tt.Decoded.encode(), tt.Encoded)
				if suite.file == "raw" {
					return
				}
				c, err := Compile(tt.Input, Config{From: "input.css"})
				if err != nil {
					t.Fatal(err)
				}
				css, decoded, err := c.build(tt.Candidates, true)
				if err != nil {
					t.Fatal(err)
				}
				if css != tt.CSS {
					t.Errorf("CSS got %q, want %q", css, tt.CSS)
				}
				want, _ := json.Marshal(tt.Decoded)
				equalSourceJSON(t, decoded, want)
				css, encoded, err := c.BuildWithSourceMap(tt.Candidates)
				if err != nil || css != tt.CSS {
					t.Fatal(css, err)
				}
				equalSourceJSON(t, encoded, tt.Encoded)
			})
		}
	}
}

func TestSourceMapBuilderAndConcurrency(t *testing.T) {
	c, err := Compile(`@import "tailwindcss"; .card { @apply hover:flex flex; }`, Config{From: "app.css"})
	if err != nil {
		t.Fatal(err)
	}
	wantCSS, wantMap, err := c.BuildWithSourceMap([]string{"flex", "ring-2"})
	if err != nil {
		t.Fatal(err)
	}
	builder := c.NewBuilder()
	if _, err := builder.Build([]string{"flex"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := builder.BuildWithSourceMap(nil); err != nil {
		t.Fatal(err)
	}
	css, sourceMap, err := builder.BuildWithSourceMap([]string{"ring-2"})
	if err != nil || css != wantCSS || !reflect.DeepEqual(sourceMap, wantMap) {
		t.Fatal("incremental map differs", err)
	}
	if allocs := testing.AllocsPerRun(50, func() {
		got, m, err := builder.BuildWithSourceMap([]string{"flex"})
		if err != nil || got != css || m != sourceMap {
			t.Fatal("cached result changed")
		}
	}); allocs != 0 {
		t.Fatalf("cached maps allocated %g times", allocs)
	}
	builder.Reset()
	css, sourceMap, err = builder.BuildWithSourceMap(nil)
	wantCSS, wantMap, _ = c.BuildWithSourceMap(nil)
	if err != nil || css != wantCSS || !reflect.DeepEqual(sourceMap, wantMap) {
		t.Fatal("reset retained old mappings", err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Go(func() {
			for j := 0; j < 5; j++ {
				css, m, err := c.BuildWithSourceMap(nil)
				if err != nil || css != wantCSS || !reflect.DeepEqual(m, wantMap) {
					t.Error("concurrent map changed")
				}
			}
		})
	}
	wg.Wait()
	static, _ := Compile(".a { color:red }", Config{From: "static.css"})
	staticCSS, staticMap, err := static.NewBuilder().BuildWithSourceMap([]string{"grid"})
	if err != nil || staticCSS != ".a { color:red }" || staticMap.Sources[0] != "static.css" {
		t.Fatal(staticCSS, staticMap, err)
	}
	withoutSources := New(Config{})
	_, empty, err := withoutSources.BuildWithSourceMap([]string{"flex"})
	if err != nil || len(empty.Sources) != 0 || empty.Mappings != "" {
		t.Fatal(empty, err)
	}
	broken, err := NewWithPlugins(Config{}, func(api *PluginAPI) error {
		return api.MatchUtility(Utility{Name: "broken", Values: []NamedValue{{"x", "1"}}, Compile: func(string, string) []*Node {
			return []*Node{{Kind: "at-rule", Name: "@apply", Value: "broken-x"}}
		}})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, m, err := broken.BuildWithSourceMap([]string{"broken-x"}); err == nil || m != nil {
		t.Fatal("missing build error")
	}
	if _, m, err := broken.NewBuilder().BuildWithSourceMap([]string{"broken-x"}); err == nil || m != nil {
		t.Fatal("missing incremental build error")
	}
}

func TestSourceMapValidation(t *testing.T) {
	for _, mapping := range []string{"!", "g", "ggggggggggggg", "AA", "AAA", "AAAAAA", "D", "ACAA", "AADA", "AAAD", "AAAAD", "AAAAC"} {
		m := SourceMap{Version: 3, Sources: []string{"input.css"}, Names: []string{"color"}, Mappings: mapping}
		if _, err := m.Visualize(""); err == nil {
			t.Errorf("accepted invalid map %q", mapping)
		}
	}
	if _, err := (&SourceMap{Version: 2}).Visualize(""); err == nil {
		t.Fatal("accepted wrong version")
	}
	m := SourceMap{Version: 3, Sources: []string{"input.css"}, SourcesContent: []string{"ab"}, Names: []string{"a"}, Mappings: "AAAAA,CAAC,CAAD;A;"}
	points, err := m.decode()
	if err != nil || len(points) != 4 || points[0].Name == nil || *points[0].Name != "a" || points[2].Original.Column != 0 || points[3].Original != nil {
		t.Fatal(points, err)
	}
	if _, err := m.Visualize("ab\n "); err != nil {
		t.Fatal(err)
	}
	for _, value := range []int{-1048576, -1, 0, 31, 32, 1048576} {
		var out strings.Builder
		writeVLQ(&out, value)
		got, rest, err := readVLQ(out.String() + "A")
		if err != nil || got != value || rest != "A" {
			t.Fatal(value, got, rest, err)
		}
	}
}

func TestSourceMapComments(t *testing.T) {
	// Original @tailwindcss/node/src/source-maps.test.ts assertion.
	m := &SourceMap{Version: 3, Sources: []string{}, Names: []string{}}
	if got := m.Comment("app.css.map"); got != "/*# sourceMappingURL=app.css.map */\n" {
		t.Fatal(got)
	}
	inline := m.Inline()
	encoded := strings.TrimSuffix(strings.TrimPrefix(inline, "/*# sourceMappingURL=data:application/json;base64,"), " */\n")
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := json.Marshal(m)
	if string(data) != string(want) {
		t.Fatal(string(data))
	}
}

func BenchmarkSourceMaps(b *testing.B) {
	classes := benchmarkCandidates()
	c, err := Compile(`@import "tailwindcss";`, Config{From: "app.css"})
	if err != nil {
		b.Fatal(err)
	}
	for _, track := range []bool{false, true} {
		b.Run(fmt.Sprint(track), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if track {
					_, _, err = c.BuildWithSourceMap(classes)
				} else {
					_, err = c.Build(classes)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestSourceMapSparseAndAnonymousSources(t *testing.T) {
	if got := createSourceMap("", []cssMapping{{}, {original: cssLocation{source: &cssSource{}}}}); len(got.Mappings) != 0 || len(got.Sources) != 0 {
		t.Fatal(got)
	}
	code := "text"
	source := &decodedSource{Content: &code}
	m := emptySourceMap()
	file := "output.css"
	m.File = &file
	m.Mappings = []decodedMapping{
		{Generated: sourcePosition{1, 0}, Original: &originalPosition{source, sourcePosition{1, 0}}},
		{Generated: sourcePosition{1, 0}, Original: &originalPosition{source, sourcePosition{1, 0}}},
		{Generated: sourcePosition{1, 4}, Original: &originalPosition{source, sourcePosition{1, 4}}},
		{Generated: sourcePosition{2, 0}},
	}
	raw := m.encode()
	if raw.File != file || raw.Mappings != "AAAA,IAAI;A" || !reflect.DeepEqual(raw.Sources, []string{"<unknown 1>"}) {
		t.Fatal(raw)
	}
	if compareMappings(decodedMapping{}, decodedMapping{}) != 0 {
		t.Fatal("unmapped positions differ")
	}
}

func TestSourceMapLongAndMissingLines(t *testing.T) {
	long := ".😀" + strings.Repeat("x", 120)
	ranges := []visualizationRange{
		{Original: visualOriginal{"one.css", visualRange{visualPosition{1, 0}, visualPosition{1, 1000}}}, Generated: &visualRange{visualPosition{1, 0}, visualPosition{1, 1000}}},
		{Original: visualOriginal{"one.css", visualRange{visualPosition{3, 0}, visualPosition{3, 1}}}},
		{Original: visualOriginal{"two.css", visualRange{visualPosition{1, 0}, visualPosition{3, 1}}}},
		{Original: visualOriginal{"missing.css", visualRange{visualPosition{1, 0}, visualPosition{1, 1}}}},
		{Original: visualOriginal{"one.css", visualRange{visualPosition{100, 0}, visualPosition{100, 1}}}},
	}
	got := visualizeSourceMapRanges(map[string]string{"one.css": long + "\ncontext\nx", "two.css": "a\nb\nc"}, long, ranges)
	for _, want := range []string{"... A @ 1:0-1000", "context", "unmapped B", "--- two.css ---", "unmapped D"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	for _, tt := range []struct {
		s    string
		n    int
		want string
	}{{"😀x", 1, "�"}, {"😀x", 2, "😀"}, {"😀", 10, "😀"}} {
		if got := visualPrefix(tt.s, tt.n); got != tt.want {
			t.Fatal(got, tt.want)
		}
	}
	if got := fitColumn("123456789 @ long label", 8, false); got != "12345..." {
		t.Fatal(got)
	}
	if got := withoutLineNumber(2, ""); got != "" {
		t.Fatal(got)
	}
}

func FuzzSourceMapDecoder(f *testing.F) {
	for _, input := range []string{"", "AAAA;AACA", "AAAAD", "ggg", "!"} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		m := SourceMap{Version: 3, Sources: []string{"input.css"}, Mappings: input}
		m.decode()
	})
}
