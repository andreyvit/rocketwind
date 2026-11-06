package rocketwind

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestUpstreamSyntax(t *testing.T) {
	for _, suite := range []struct {
		name  string
		count int
	}{
		{"value-parser", 34}, {"selector-parser", 118}, {"attribute-selector-parser", 21},
		{"utils_segment", 15}, {"utils_escape", 19}, {"utils_brace-expansion", 164},
		{"utils_compare", 954}, {"utils_to-key-path", 8}, {"utils_replace-shadow-colors", 1239},
	} {
		t.Run(suite.name, func(t *testing.T) {
			data, err := os.ReadFile("testdata/upstream/" + suite.name + ".test.ts.json")
			if err != nil {
				t.Fatal(err)
			}
			var cases []struct {
				Test, Kind, Error string
				Args              []json.RawMessage
				Result            json.RawMessage
			}
			if err := json.Unmarshal(data, &cases); err != nil {
				t.Fatal(err)
			}
			if len(cases) != suite.count {
				t.Fatalf("got %d calls, want %d", len(cases), suite.count)
			}
			for i, tt := range cases {
				t.Run(fmt.Sprintf("%d_%s", i, tt.Test), func(t *testing.T) {
					var s, z string
					json.Unmarshal(tt.Args[0], &s)
					if len(tt.Args) > 1 {
						json.Unmarshal(tt.Args[1], &z)
					}
					var got any
					var err error
					switch tt.Kind {
					case "internal:value-parser:parse":
						got = encodeValues(parseValue(s))
					case "internal:value-parser:toCss":
						var nodes []map[string]any
						json.Unmarshal(tt.Args[0], &nodes)
						got = valueCSS(decodeValues(nodes))
					case "internal:selector-parser:parse":
						got = encodeSelectors(parseSelector(s))
					case "internal:selector-parser:toCss":
						var nodes []*selectorNode
						json.Unmarshal(tt.Args[0], &nodes)
						minify := false
						if len(tt.Args) > 1 {
							json.Unmarshal(tt.Args[1], &minify)
						}
						got = selectorCSSMode(nodes, minify)
					case "internal:attribute-selector-parser:parse":
						got = parseAttribute(s)
					case "internal:utils/segment:segment":
						got = slices.Collect(Segments(s, z[0]))
					case "internal:utils/escape:escape":
						got = Escape(s)
					case "internal:utils/escape:unescape":
						got = Unescape(s)
					case "internal:utils/brace-expansion:expand":
						got, err = Expand(s)
					case "internal:utils/compare:compare":
						got = cmp.Compare(naturalCompare(s, z), 0)
					case "internal:utils/to-key-path:toKeyPath":
						got = themeKeyPath(s)
					case "internal:utils/replace-shadow-colors:replaceShadowColors":
						alpha := strings.Contains(tt.Test, "with replacer >")
						got = replaceShadowColors(s, func(color string) string {
							if alpha {
								color = "oklab(from " + color + " l a b / 50%)"
							}
							return "var(--tw-shadow-color, " + color + ")"
						})
					default:
						t.Fatalf("unhandled call %s", tt.Kind)
					}
					if tt.Error != "" {
						if err == nil || err.Error() != tt.Error {
							t.Fatalf("got %v, want %s", err, tt.Error)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					var want, normalized any
					json.Unmarshal(tt.Result, &want)
					encoded, _ := json.Marshal(got)
					json.Unmarshal(encoded, &normalized)
					if tt.Kind == "internal:utils/compare:compare" {
						want = float64(cmp.Compare(want.(float64), 0))
					}
					if !reflect.DeepEqual(normalized, want) {
						t.Fatalf("input %s: got %s\nwant %s", tt.Args, encoded, tt.Result)
					}
				})
			}
		})
	}
}

func encodeValues(nodes []*valueNode) []map[string]any {
	result := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		kind := map[byte]string{'w': "word", 's': "separator", 'f': "function"}[n.kind]
		r := map[string]any{"kind": kind, "value": n.value}
		if n.kind == 'f' {
			r["nodes"] = encodeValues(n.nodes)
		}
		result = append(result, r)
	}
	return result
}

func decodeValues(nodes []map[string]any) []*valueNode {
	result := make([]*valueNode, 0, len(nodes))
	for _, n := range nodes {
		r := &valueNode{kind: map[string]byte{"word": 'w', "separator": 's', "function": 'f'}[n["kind"].(string)], value: n["value"].(string)}
		if r.kind == 'f' {
			encoded, _ := json.Marshal(n["nodes"])
			var children []map[string]any
			json.Unmarshal(encoded, &children)
			r.nodes = decodeValues(children)
		}
		result = append(result, r)
	}
	return result
}

func encodeSelectors(nodes []*selectorNode) []map[string]any {
	result := make([]map[string]any, 0, len(nodes))
	for _, n := range nodes {
		r := map[string]any{"kind": n.Kind}
		switch n.Kind {
		case "selector", "value", "combinator":
			r["value"] = n.Value
		case "function":
			r["value"] = n.Value
			r["nodes"] = encodeSelectors(n.Nodes)
		default:
			r["nodes"] = encodeSelectors(n.Nodes)
		}
		result = append(result, r)
	}
	return result
}
