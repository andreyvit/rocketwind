package rocketwind

import (
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"
)

func TestUpstreamCanonicalize(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/canonicalize-candidates.test.ts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []sourceMapFixture
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 3353 {
		t.Fatal(len(cases))
	}
	compilers := map[int]*Compiler{}
	canonicalizers := map[string]*Canonicalizer{}
	var failures []map[string]any
	for index, tt := range cases {
		var got any
		if tt.Kind == "internal:canonicalize-candidates:intersection" {
			var a, b []any
			json.Unmarshal(tt.Args[0], &a)
			json.Unmarshal(tt.Args[1], &b)
			got = intersection(a, b)
		} else {
			var ref struct {
				ID         int `json:"$design"`
				Definition *struct {
					CSS     string
					Options struct {
						Base        string
						Modules     []upstreamModule
						Stylesheets []struct {
							Args   []string
							Result Stylesheet
						}
					}
				}
			}
			if err := json.Unmarshal(tt.Args[0], &ref); err != nil {
				t.Fatal(err)
			}
			if d := ref.Definition; d != nil {
				config := Config{Base: d.Options.Base, LoadModule: upstreamModuleLoader(d.Options.Modules)}
				config.LoadStylesheet = func(id, base string) (Stylesheet, error) {
					for _, entry := range d.Options.Stylesheets {
						if entry.Args[0] == id && entry.Args[1] == base {
							return entry.Result, nil
						}
					}
					return Stylesheet{}, fmt.Errorf("uncaptured stylesheet %q from %q", id, base)
				}
				c, err := Compile(d.CSS, config)
				if err != nil {
					t.Errorf("design %d %s: %v", ref.ID, tt.Test, err)
					continue
				}
				compilers[ref.ID] = c
			}
			c := compilers[ref.ID]
			if c == nil {
				t.Errorf("missing design %d", ref.ID)
				continue
			}
			var classes []string
			json.Unmarshal(tt.Args[1], &classes)
			switch tt.Kind {
			case "design.canonicalize":
				var options CanonicalizeOptions
				key := fmt.Sprint(ref.ID)
				if len(tt.Args) > 2 {
					json.Unmarshal(tt.Args[2], &options)
					key += "/" + string(tt.Args[2])
				}
				k := canonicalizers[key]
				if k == nil {
					k = c.NewCanonicalizer(options)
					canonicalizers[key] = k
				}
				got = k.Canonicalize(classes)
			case "design.order":
				order := c.ClassOrder(classes)
				pairs := make([]any, len(classes))
				for i, name := range classes {
					var rank any
					if order[i] >= 0 {
						rank = order[i]
					}
					pairs[i] = []any{name, rank}
				}
				got = pairs
			default:
				t.Fatalf("unknown kind %s", tt.Kind)
			}
		}
		var want any
		json.Unmarshal(tt.Result, &want)
		actual, _ := json.Marshal(got)
		var normalized any
		json.Unmarshal(actual, &normalized)
		if !reflect.DeepEqual(normalized, want) {
			failures = append(failures, map[string]any{"index": index, "test": tt.Test, "kind": tt.Kind, "args": tt.Args, "got": got, "want": want})
			if len(failures) <= 10 {
				t.Errorf("%d %s: got %s, want %s", index, tt.Test, actual, tt.Result)
			}
		}
	}
	if len(failures) > 0 {
		os.MkdirAll(".cache", 0755)
		output, _ := json.MarshalIndent(failures, "", "  ")
		os.WriteFile(".cache/canonicalize-failures.json", output, 0644)
		t.Fatalf("%d of %d captured calls differ; see .cache/canonicalize-failures.json", len(failures), len(cases))
	}
}

func TestCanonicalizeReferenceBoundaries(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/canonicalize-extra.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		CSS           string
		Options       CanonicalizeOptions
		Input, Output []string
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	cache := map[string]*Canonicalizer{}
	for _, tt := range cases {
		key, _ := json.Marshal([]any{tt.CSS, tt.Options})
		k := cache[string(key)]
		if k == nil {
			c, err := Compile(tt.CSS, Config{})
			if err != nil {
				t.Fatal(err)
			}
			k = c.NewCanonicalizer(tt.Options)
			cache[string(key)] = k
		}
		if got := k.Canonicalize(tt.Input); !reflect.DeepEqual(got, tt.Output) {
			t.Errorf("%v %+v: got %v, want %v", tt.Input, tt.Options, got, tt.Output)
		}
	}
}
