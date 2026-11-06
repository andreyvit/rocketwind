package rocketwind

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"unicode/utf8"
)

func TestUpstreamCSSParser(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/css-parser.test.ts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Test   string
		Args   []json.RawMessage
		Result []upstreamNode
		Error  string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 222 {
		t.Fatalf("captured %d calls, want 222", len(cases))
	}
	for _, tt := range cases {
		t.Run(tt.Test, func(t *testing.T) {
			var input string
			var opt struct{ From string }
			if err := json.Unmarshal(tt.Args[0], &input); err != nil {
				t.Fatal(err)
			}
			if len(tt.Args) > 1 {
				if err := json.Unmarshal(tt.Args[1], &opt); err != nil {
					t.Fatal(err)
				}
			}
			got, err := ParseCSS(input, opt.From)
			if tt.Error != "" {
				if err == nil || err.Error() != tt.Error {
					t.Fatalf("got %v, want %s", err, tt.Error)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := convertUpstreamNodes(tt.Result, input)
			if !reflect.DeepEqual(got, want) {
				a, _ := json.Marshal(got)
				e, _ := json.Marshal(want)
				t.Fatalf("got %s\nwant %s", a, e)
			}
		})
	}
}

type upstreamNode struct {
	Context                                       map[string]any
	Kind, Selector, Name, Params, Property, Value string
	Important                                     bool
	Nodes                                         []upstreamNode
	Src                                           []json.RawMessage
}

func convertUpstreamNodes(nodes []upstreamNode, input string) []*Node {
	result := make([]*Node, 0, len(nodes))
	for _, n := range nodes {
		v := &Node{Kind: n.Kind, Name: n.Name, Value: n.Value, Important: n.Important}
		v.context = n.Context
		switch n.Kind {
		case "rule":
			v.Name = n.Selector
			v.Nodes = convertUpstreamNodes(n.Nodes, input)
		case "at-rule":
			v.Value = n.Params
			v.Nodes = convertUpstreamNodes(n.Nodes, input)
		case "declaration":
			v.Name = n.Property
		case "context", "at-root":
			v.Nodes = convertUpstreamNodes(n.Nodes, input)
		}
		if len(n.Src) > 0 {
			var source struct{ File, Code string }
			json.Unmarshal(n.Src[0], &source)
			v.source = &cssSource{file: source.File, code: source.Code}
			var start, end int
			json.Unmarshal(n.Src[1], &start)
			json.Unmarshal(n.Src[2], &end)
			v.Start = utf16Offset(input, start)
			v.End = utf16Offset(input, end)
		}
		result = append(result, v)
	}
	return result
}

func utf16Offset(s string, n int) int {
	i := 0
	for i < len(s) && n > 0 {
		r, size := utf8.DecodeRuneInString(s[i:])
		i += size
		n--
		if r > 0xffff {
			n--
		}
	}
	return i
}

func TestCSSDiagnosticsAndRecovery(t *testing.T) {
	for _, tt := range [][2]string{
		{"😀 }", "input.css:1:4: Missing opening {"},
		{"/* open", "input.css:1:1: Unterminated comment"},
		{".foo { --bar: /* open", "input.css:1:15: Unterminated comment"},
		{".foo { bad; }", "input.css:1:8: Invalid declaration: `bad`"},
		{".foo)", "input.css:1:5: Missing opening ("},
		{"@media print {", "input.css:1:1: Missing closing } at @media print"},
	} {
		_, err := ParseCSS(tt[0], "input.css")
		if err == nil || err.Error() != tt[1] {
			t.Errorf("%q: got %v, want %s", tt[0], err, tt[1])
		}
	}
	got, err := ParseCSS(`\31 st { content: "unfinished }`, "")
	if err != nil || len(got) != 1 || got[0].Name != `\31 st` {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func FuzzCSSParser(f *testing.F) {
	for _, s := range []string{"", ".a{color:red}", "@media(width>10px){.a{--foo:{a:b};}}", ".a{content:'}';}", "/* comment */", ".a{color:red!important}"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) { ParseCSS(s, "fuzz.css") })
}
