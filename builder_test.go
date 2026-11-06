package rocketwind

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"sync"
	"testing"
)

func TestIncrementalBuilder(t *testing.T) {
	c := New(Config{})
	builder := c.NewBuilder()
	first, err := builder.Build([]string{"flex"})
	if err != nil {
		t.Fatal(err)
	}
	again, err := builder.Build(nil)
	if err != nil || again != first {
		t.Fatalf("empty update: %s %v", again, err)
	}
	both, err := builder.Build([]string{"grid", "flex"})
	if err != nil || !strings.Contains(both, ".grid {") || !strings.Contains(both, ".flex {") {
		t.Fatalf("addition: %s %v", both, err)
	}
	want, err := c.Build([]string{"grid", "flex"})
	if err != nil || both != want {
		t.Fatal("incremental and full output differ")
	}
	invalid, err := builder.Build([]string{"not-a-utility"})
	if err != nil || invalid != both {
		t.Fatal("invalid candidate changed CSS")
	}
	grid, err := c.Build([]string{"grid"})
	if err != nil || strings.Contains(grid, ".flex {") {
		t.Fatal("stateless build leaked candidates")
	}
	builder.Reset()
	afterReset, err := builder.Build([]string{"grid"})
	if err != nil || afterReset != grid {
		t.Fatal("reset leaked candidates")
	}
	allocs := testing.AllocsPerRun(100, func() {
		css, err := builder.Build([]string{"grid", "grid"})
		if err != nil || css != grid {
			t.Fatal("cache changed CSS")
		}
	})
	if allocs != 0 {
		t.Fatalf("unchanged build allocated %g times", allocs)
	}
}

func TestIncrementalBuildOrderAndIndependence(t *testing.T) {
	c := New(Config{})
	classes := benchmarkCandidates()
	want, err := c.Build(classes)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func(reverse bool) {
			defer wg.Done()
			builder := c.NewBuilder()
			ordered := slices.Clone(classes)
			if reverse {
				slices.Reverse(ordered)
			}
			var css string
			for begin := 0; begin < len(ordered); begin += 37 {
				var err error
				css, err = builder.Build(ordered[begin:min(begin+37, len(ordered))])
				if err != nil {
					t.Error(err)
					return
				}
			}
			if css != want {
				t.Error("batch/order dependent CSS")
			}
		}(i%2 == 0)
	}
	wg.Wait()
}

func TestIncrementalSourcesAndStaticStylesheet(t *testing.T) {
	c, err := Compile(`@source inline("flex"); @source not inline("grid"); @tailwind utilities;`, Config{})
	if err != nil {
		t.Fatal(err)
	}
	builder := c.NewBuilder()
	css, err := builder.Build([]string{"grid"})
	if err != nil || !strings.Contains(css, ".flex {") || strings.Contains(css, ".grid {") {
		t.Fatalf("%s %v", css, err)
	}
	builder.Reset()
	fresh, err := builder.Build(nil)
	if err != nil || fresh != css {
		t.Fatal("reset lost inline sources")
	}
	static, err := Compile(`.card {display:block}`, Config{})
	if err != nil {
		t.Fatal(err)
	}
	builder = static.NewBuilder()
	css, err = builder.Build([]string{"flex"})
	if err != nil || css != ".card {display:block}" {
		t.Fatalf("%s %v", css, err)
	}
	if again, err := builder.Build([]string{"grid"}); err != nil || again != css {
		t.Fatal("static stylesheet changed")
	}
}

func TestClassOrder(t *testing.T) {
	c, err := Compile(`@utility flex {color: red;} @tailwind utilities;`, Config{ExistingClasses: ClassSet{"flex": {}}})
	if err != nil {
		t.Fatal(err)
	}
	got := c.ClassOrder([]string{"unknown", "flex", "grid", "flex", "unknown"})
	if !slices.Equal(got, []int{-1, 0, 1, 0, -1}) {
		t.Fatal(got)
	}
}

func TestUpstreamClassOrder(t *testing.T) {
	data, err := os.ReadFile("testdata/upstream/sort.test.ts.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Test, Kind string
		Args       []json.RawMessage
		Result     [][]json.RawMessage
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	if len(cases) != 14 {
		t.Fatalf("got %d class order calls, want 14", len(cases))
	}
	for _, tt := range cases {
		var css string
		var classes []string
		if err := json.Unmarshal(tt.Args[0], &css); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(tt.Args[2], &classes); err != nil {
			t.Fatal(err)
		}
		c, err := Compile(css, Config{})
		if err != nil {
			t.Fatal(err)
		}
		order := c.ClassOrder(classes)
		for i, pair := range tt.Result {
			var expected *int
			if err := json.Unmarshal(pair[1], &expected); err != nil {
				t.Fatal(err)
			}
			want := -1
			if expected != nil {
				want = *expected
			}
			if order[i] != want {
				t.Errorf("%s: %s order %d, want %d", tt.Test, classes[i], order[i], want)
			}
		}
	}
}
