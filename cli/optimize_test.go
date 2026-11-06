package cli

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/andreyvit/rocketwind"
)

func TestOptimizationFlags(t *testing.T) {
	for _, test := range []struct {
		args             []string
		minify, optimize bool
	}{
		{[]string{"-m"}, true, false},
		{[]string{"--minify", "--optimize"}, true, true},
		{[]string{"--minify=false", "--optimize"}, false, true},
		{[]string{"--optimize=false", "--minify=true"}, true, false},
		{[]string{"--no-minify", "--no-optimize"}, false, false},
	} {
		flags, err := parseFlags(test.args)
		if err != nil || flags.minify != test.minify || flags.optimize != test.optimize {
			t.Fatalf("%v: %#v, %v", test.args, flags, err)
		}
	}
}

func TestOptimizedCommand(t *testing.T) {
	for _, flag := range []string{"--minify", "--optimize"} {
		for _, mapped := range []bool{false, true} {
			t.Run(flag+map[bool]string{true: "/map", false: "/css"}[mapped], func(t *testing.T) {
				var out, stderr bytes.Buffer
				args := []string{"--cwd", t.TempDir(), "-i", "-", flag, "--silent", "--classes", "flex hover:flex"}
				if mapped {
					args = append(args, "--map")
				}
				input := `/*! kept */ @tailwind utilities; .a{color:red;&:hover{color:blue}}`
				if code := Run(context.Background(), args, Options{Stdin: strings.NewReader(input), Stdout: &out, Stderr: &stderr}); code != 0 {
					t.Fatal(code, stderr.String())
				}
				got := out.String()
				if !strings.Contains(got, "/*! kept */") || !strings.Contains(got, ".a:hover") || strings.Contains(got, "&:hover") {
					t.Fatal(got)
				}
				if flag == "--minify" && !strings.Contains(got, ".flex{display:flex}") {
					t.Fatal(got)
				}
				if flag == "--optimize" && !strings.Contains(got, ".flex {\n  display: flex;\n}") {
					t.Fatal(got)
				}
				if mapped {
					_, encoded, ok := strings.Cut(got, "sourceMappingURL=data:application/json;base64,")
					if !ok {
						t.Fatal("missing source map")
					}
					encoded, _, _ = strings.Cut(encoded, " ")
					data, err := base64.StdEncoding.DecodeString(encoded)
					if err != nil {
						t.Fatal(err)
					}
					var m rocketwind.SourceMap
					if err := json.Unmarshal(data, &m); err != nil {
						t.Fatal(err)
					}
					if len(m.Sources) != 1 || m.Sources[0] != "<stdin>" || m.SourcesContent[0] != input {
						t.Fatalf("map: %#v", m)
					}
					if _, err := m.Visualize(got); err != nil {
						t.Fatal(err)
					}
				}
			})
		}
	}
	modules := map[string]rocketwind.Module{"broken": {Plugin: func(api *rocketwind.PluginAPI) error {
		return api.AddUtilities([]*rocketwind.Node{{Kind: "rule", Name: ".broken", Nodes: []*rocketwind.Node{{Kind: "declaration", Name: "color", Value: ")"}}}})
	}}}
	for _, mapped := range []bool{false, true} {
		var out, stderr bytes.Buffer
		args := []string{"--cwd", t.TempDir(), "-i", "-", "--minify", "--classes", "broken"}
		if mapped {
			args = append(args, "--map")
		}
		code := Run(context.Background(), args, Options{Stdin: strings.NewReader(`@plugin "broken";@tailwind utilities;`), Stdout: &out, Stderr: &stderr, Modules: modules})
		if code != 1 || out.Len() != 0 || !strings.Contains(stderr.String(), "Missing opening (") {
			t.Fatal(code, out.String(), stderr.String())
		}
	}
}

func TestOptimizationCache(t *testing.T) {
	c := optimizationCache{options: rocketwind.OptimizeOptions{Minify: true}}
	input := ".a{ color: red }"
	for i := 0; i < 2; i++ {
		got, m, err := c.run(input, nil)
		if err != nil || m != nil || got != ".a{color:red}" {
			t.Fatal(got, m, err)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() { _, _, _ = c.run(input, nil) }); allocations != 0 {
		t.Fatalf("cached: %g allocations", allocations)
	}
	_, source, err := rocketwind.OptimizeWithSourceMap(input, rocketwind.OptimizeOptions{From: "a.css"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, mapped, err := c.run(input, source)
	if err != nil {
		t.Fatal(err)
	}
	_, again, err := c.run(input, source)
	if err != nil || again != mapped {
		t.Fatalf("map cache: %#v, %v", again, err)
	}
	if _, _, err := c.run(".broken{", nil); err == nil {
		t.Fatal("bad CSS succeeded")
	}
	if _, _, err := c.run(input, &rocketwind.SourceMap{Version: 2}); err == nil {
		t.Fatal("bad map succeeded")
	}
	_, again, err = c.run(input, source)
	if err != nil || again != mapped {
		t.Fatal("failure invalidated good cache")
	}
	replacement := *source
	replacement.Sources = []string{"b.css"}
	_, next, err := c.run(input, &replacement)
	if err != nil || next == mapped || len(next.Sources) != 1 || next.Sources[0] != "b.css" {
		t.Fatalf("changed map: %#v, %v", next, err)
	}
}
