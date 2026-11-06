package rocketwind

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

func collationFixture(t *testing.T, name string) *gzip.Reader {
	t.Helper()
	f, err := os.Open("testdata/collation/" + name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { f.Close() })
	r, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close() })
	return r
}

func TestCollationNode(t *testing.T) {
	var cases []struct {
		A, B string
		Want int
	}
	if err := json.NewDecoder(collationFixture(t, "node-26.json.gz")).Decode(&cases); err != nil {
		t.Fatal(err)
	}
	failures := 0
	for _, tt := range cases {
		if got := localeCompare(tt.A, tt.B); got != tt.Want {
			failures++
			if failures <= 20 {
				t.Errorf("%q vs %q: got %d, want %d", tt.A, tt.B, got, tt.Want)
			}
		}
	}
	if failures != 0 {
		t.Fatalf("%d mismatches in %d comparisons", failures, len(cases))
	}
}

func TestCollationCLDR(t *testing.T) {
	expected, err := io.ReadAll(collationFixture(t, "cldr-node.bin.gz"))
	if err != nil {
		t.Fatal(err)
	}
	scanner := bufio.NewScanner(collationFixture(t, "cldr-48.txt.gz"))
	previous, count, failures := "", 0, 0
	for scanner.Scan() {
		line, _, _ := strings.Cut(scanner.Text(), "#")
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		var runes []rune
		for _, field := range fields {
			n, err := strconv.ParseInt(field, 16, 32)
			if err != nil {
				t.Fatal(err)
			}
			runes = append(runes, rune(n))
		}
		current := string(runes)
		if count > 0 && localeCompare(previous, current) != int(expected[count-1])-1 {
			failures++
			if failures <= 20 {
				t.Errorf("line %d: %q (%X) vs %q (%X): got %d, want %d", count+1, previous, []rune(previous), current, runes, localeCompare(previous, current), int(expected[count-1])-1)
			}
		}
		previous = current
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if count != len(expected)+1 {
		t.Fatal(count, len(expected))
	}
	if failures != 0 {
		t.Fatalf("%d mismatches in %d inputs", failures, count)
	}
}

func BenchmarkLocaleCompare(b *testing.B) {
	for _, tt := range []struct{ name, a, z string }{
		{"ASCII", "calc(var(--spacing) * 4)", "calc(var(--spacing) * 8)"},
		{"Case", "var(--color-a)", "var(--color-A)"},
		{"Unicode", "var(--café)", "var(--cafe\u0301)"},
		{"Emoji", "var(--🤖)", "var(--🫨)"},
	} {
		b.Run(tt.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				localeCompare(tt.a, tt.z)
			}
		})
	}
}
