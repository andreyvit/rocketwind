package rocketwind

import (
	"errors"
	"fmt"
	"math/bits"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestCanonicalizeSharedCompiler(t *testing.T) {
	c, err := Compile(`@theme inline { --spacing: .25rem; --color-brand: #123456; } @tailwind utilities;`, Config{})
	if err != nil {
		t.Fatal(err)
	}
	classes := []string{"[color:var(--color-brand)]", "mt-4", "mb-4", "custom", "custom"}
	baseline, err := c.Build(classes)
	if err != nil {
		t.Fatal(err)
	}
	rem := 16.0
	options := CanonicalizeOptions{Rem: &rem, Collapse: true, LogicalToPhysical: true}
	want := []string{"text-brand", "custom", "my-4"}
	if got := c.Canonicalize(classes, options); !slices.Equal(got, want) {
		t.Fatalf("%v", got)
	}
	k := c.NewCanonicalizer(options)
	rem = 32
	if got := k.Canonicalize([]string{"w-[16px]"}); !slices.Equal(got, []string{"w-4"}) {
		t.Fatal(got)
	}
	first := k.Canonicalize(classes)
	first[0] = "edited"
	if got := k.Canonicalize(classes); !slices.Equal(got, want) {
		t.Fatal(got)
	}
	cached := k.Canonicalize(classes)
	cached[0] = "edited again"
	if got := k.Canonicalize(classes); !slices.Equal(got, want) {
		t.Fatal(got)
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			local := c.NewCanonicalizer(options)
			for range 3 {
				if got := local.Canonicalize(classes); !slices.Equal(got, want) {
					t.Errorf("%v", got)
				}
				if got, err := c.Build(classes); err != nil || got != baseline {
					t.Errorf("compiler changed: %v", err)
				}
			}
		})
	}
	wg.Wait()
}

func TestCanonicalizePluginFailure(t *testing.T) {
	failure := errors.New("invalid icon")
	c, err := NewWithPlugins(Config{NoDefaultTheme: true}, func(api *PluginAPI) error {
		return api.MatchUtility(Utility{Name: "icon", Values: []NamedValue{{"valid", "red"}, {"invalid", "broken"}}, CompileWithError: func(value, modifier string) ([]*Node, error) {
			if value == "broken" {
				return nil, failure
			}
			return declarations("color", value), nil
		}})
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Build([]string{"icon-invalid"}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if got := c.Canonicalize([]string{"icon-invalid", "flex"}, CanonicalizeOptions{Collapse: true}); !slices.Equal(got, []string{"icon-invalid", "flex"}) {
		t.Fatal(got)
	}
	if css, err := c.Build([]string{"icon-valid"}); err != nil || !strings.Contains(css, "color: red") {
		t.Fatalf("%s %v", css, err)
	}
}

func TestCanonicalCombinations(t *testing.T) {
	for n := 0; n <= 7; n++ {
		input := make([]int, n)
		for i := range input {
			input[i] = i
		}
		var want [][]int
		for size := n; size >= 2; size-- {
			for mask := 1; mask < 1<<n; mask++ {
				if bits.OnesCount(uint(mask)) != size {
					continue
				}
				var combo []int
				for i := range n {
					if mask&(1<<i) != 0 {
						combo = append(combo, i)
					}
				}
				want = append(want, combo)
			}
		}
		if got := slices.Collect(canonicalCombinations(input)); !reflect.DeepEqual(got, want) {
			t.Fatalf("n=%d: %v != %v", n, got, want)
		}
	}
	input := make([]int, 70)
	for i := range input {
		input[i] = i
	}
	count := 0
	for combo := range canonicalCombinations(input) {
		count++
		if count == 1 && !slices.Equal(combo, input) {
			t.Fatal(combo)
		}
		if count == 2 {
			if !slices.Equal(combo, input[:69]) {
				t.Fatal(combo)
			}
			break
		}
	}
}

func TestCanonicalSignatureBoundaries(t *testing.T) {
	for _, tt := range []struct{ a, b, want int }{{-1, -1, 0}, {2, 2, 0}, {-1, 1, -1}, {1, -1, 1}, {3, 1, -2}, {1, 3, 2}} {
		if got := canonicalCollapseOrder(tt.a, tt.b); got != tt.want {
			t.Fatal(tt, got)
		}
	}
	k := New(Config{NoDefaultTheme: true, Theme: map[string]string{"--spacing": "1px"}}).NewCanonicalizer(CanonicalizeOptions{Collapse: true})
	for _, invalid := range []string{"flex missing", "[@media(width>--theme(--missing))]:flex"} {
		if got := k.signature(invalid); got.css != "" {
			t.Errorf("%s: %s", invalid, got.css)
		}
	}
	nodes := []*Node{{Kind: "rule", Name: ".x", Nodes: []*Node{{Kind: "comment", Value: "test"}, {Kind: "declaration", Name: "width", Value: "0px"}}}}
	got := k.canonicalAST(nodes, nil)
	var css strings.Builder
	writeCSS(&css, got, 0)
	if css.String() != ".x {\n  width: 0;\n}\n" {
		t.Fatal(css.String())
	}
	for _, tt := range []struct {
		a, b []string
		want bool
	}{{nil, nil, false}, {[]string{}, []string{}, true}, {[]string{"a"}, nil, false}, {[]string{"a"}, []string{}, false}, {[]string{"a"}, []string{"b"}, false}, {[]string{""}, []string{"b"}, true}} {
		if got := sameThemeVariables(tt.a, tt.b); got != tt.want {
			t.Fatalf("%v %v: %v", tt.a, tt.b, got)
		}
	}
	for _, tt := range []struct {
		value string
		stop  int
		want  CandidateValue
	}{{"20%", 2, CandidateValue{Kind: "named", Value: "20"}}, {"16/9", 2, CandidateValue{Kind: "named", Value: "16", Fraction: "16/9"}}} {
		index := 0
		var got CandidateValue
		for value := range canonicalBareValues(tt.value, map[string]bool{}) {
			index++
			if index == tt.stop {
				got = value
				break
			}
		}
		if got != tt.want {
			t.Fatal(got)
		}
	}
}

func BenchmarkCanonicalize(b *testing.B) {
	classes := []string{"[display:flex]", "mt-4", "mb-4", "hover:bg-[#fff]", "text-[14px]", "leading-6", "custom"}
	for _, full := range []bool{false, true} {
		config := Config{}
		if !full {
			config = Config{NoDefaultTheme: true, Theme: map[string]string{"--spacing": ".25rem", "--color-white": "#fff", "--text-sm": ".875rem", "--text-sm--line-height": "1.25rem"}}
		}
		c := New(config)
		rem := 16.0
		options := CanonicalizeOptions{Rem: &rem, Collapse: true, LogicalToPhysical: true}
		b.Run(fmt.Sprintf("FullTheme=%v/Cold", full), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				c.Canonicalize(classes, options)
			}
		})
		b.Run(fmt.Sprintf("FullTheme=%v/Cached", full), func(b *testing.B) {
			k := c.NewCanonicalizer(options)
			k.Canonicalize(classes)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				k.Canonicalize(classes)
			}
		})
	}
}
