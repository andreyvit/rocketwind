package rocketwind

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestUpstreamAST(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/ast.test.ts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Kind, Test, Result string
		Args               []json.RawMessage
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 604 {
		t.Fatalf("got %d calls, want 604", len(cases))
	}
	var failures []any
	for i, tt := range cases {
		var ast []upstreamNode
		json.Unmarshal(tt.Args[0], &ast)
		nodes := convertUpstreamNodes(ast, "")
		var out strings.Builder
		switch tt.Kind {
		case "internal:ast:print":
			writeCSS(&out, nodes, 0)
		case "internal:ast:nesting":
			writeCSS(&out, flattenRules(nodes), 0)
		case "internal:ast:optimize":
			b := buildContext{compiler: New(Config{NoDefaultTheme: true}), used: map[string]bool{}}
			out.WriteString(b.render(nodes))
		default:
			t.Fatal(tt.Kind)
		}
		if out.String() != tt.Result {
			failures = append(failures, map[string]any{"index": i, "test": tt.Test, "kind": tt.Kind, "input": tt.Args, "got": out.String(), "want": tt.Result})
		}
	}
	if len(failures) > 0 {
		data, _ := json.MarshalIndent(failures, "", "  ")
		os.WriteFile(".cache/ast-failures.json", data, 0644)
		t.Fatalf("%d/%d upstream calls match; %d failures in .cache/ast-failures.json", len(cases)-len(failures), len(cases), len(failures))
	}
}
