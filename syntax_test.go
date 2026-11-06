package rocketwind

import (
	"slices"
	"strings"
	"testing"
)

// Cases adapted from Tailwind CSS v4.3.4 utils/segment.test.ts and
// utils/escape.test.ts. See testdata/upstream/LICENSE and manifest.json.
func TestSegments(t *testing.T) {
	for _, tt := range []struct {
		input string
		sep   byte
		want  []string
	}{
		{"foo", ':', []string{"foo"}},
		{"foo:bar:baz", ':', []string{"foo", "bar", "baz"}},
		{"a:(b:c):d", ':', []string{"a", "(b:c)", "d"}},
		{"a:[b:c]:d", ':', []string{"a", "[b:c]", "d"}},
		{"a:{b:c}:d", ':', []string{"a", "{b:c}", "d"}},
		{`a:"b:c":d`, ':', []string{"a", `"b:c"`, "d"}},
		{`a:'b:c':d`, ':', []string{"a", `'b:c'`, "d"}},
		{`a:"b:c:d`, ':', []string{"a", `"b:c:d`}},
		{`a:'b:c:d`, ':', []string{"a", `'b:c:d`}},
		{`a:"b:c\":d":e`, ':', []string{"a", `"b:c\":d"`, "e"}},
		{`a:'b:c\':d':e`, ':', []string{"a", `'b:c\':d'`, "e"}},
		{`a\b\c\d`, '\\', []string{"a", "b", "c", "d"}},
		{`a\(b\c)\d`, '\\', []string{"a", `(b\c)`, "d"}},
		{`a\[b\c]\d`, '\\', []string{"a", `[b\c]`, "d"}},
		{`a\{b\c}\d`, '\\', []string{"a", `{b\c}`, "d"}},
		{`a\:b:c`, ':', []string{`a\:b`, "c"}},
		{`a:):b`, ':', []string{"a", ")", "b"}},
		{"", ':', []string{""}},
		{"a::", ':', []string{"a", "", ""}},
		{strings.Repeat("(", 300) + ":" + strings.Repeat(")", 300) + ":b", ':', []string{strings.Repeat("(", 300) + ":" + strings.Repeat(")", 300), "b"}},
	} {
		if got := slices.Collect(Segments(tt.input, tt.sep)); !slices.Equal(got, tt.want) {
			t.Errorf("%q: got %#v, want %#v", tt.input, got, tt.want)
		}
	}
	for part := range Segments("a:b:c", ':') {
		if part != "a" {
			t.Fatal(part)
		}
		break
	}
}

func TestEscape(t *testing.T) {
	for _, tt := range [][2]string{
		{"", ""}, {"plain-name_2", "plain-name_2"}, {"-", `\-`}, {"\x00", "�"},
		{"1st", `\31 st`}, {"-2", `-\32 `}, {"\x01\x7f", `\1 \7f `},
		{"red-1/2", `red-1\/2`}, {"sm:text-[14px]!", `sm\:text-\[14px\]\!`}, {"é🐈", "é🐈"},
	} {
		if got := Escape(tt[0]); got != tt[1] {
			t.Errorf("%q: got %q, want %q", tt[0], got, tt[1])
		}
	}
}

func TestUnescape(t *testing.T) {
	for _, tt := range [][2]string{
		{"plain", "plain"}, {`\0`, "�"}, {`\1`, "\x01"}, {`\9`, "\t"}, {`\a`, "\n"}, {`\A`, "\n"},
		{`\c`, "\f"}, {`\C`, "\f"}, {`\f`, "\x0f"}, {`\F`, "\x0f"}, {`\a b`, "\nb"}, {`\00000a`, "\n"},
		{`\41`, "A"}, {`\0 `, "�"}, {`\!`, "!"}, {`\g`, "g"}, {`\\`, `\`}, {`red-1\/2`, "red-1/2"},
		{`--Coding-Projects-CharacterMapper-Master-Workspace\d8819554-4725-4235-9d22-2d0ed572e924`, "--Coding-Projects-CharacterMapper-Master-Workspace�54-4725-4235-9d22-2d0ed572e924"},
		{`\d800`, "�"}, {`\110000`, "�"}, {`\1f408`, "🐈"}, {"trailing\\", "trailing\\"}, {"\\é", "é"},
	} {
		if got := Unescape(tt[0]); got != tt[1] {
			t.Errorf("%q: got %q, want %q", tt[0], got, tt[1])
		}
	}
}

func BenchmarkSegments(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		for range Segments(`sm:group-hover/item:[&>a]:text-[length:var(--text_size)]`, ':') {
		}
	}
}

func BenchmarkEscape(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		Escape("text-xl")
	}
}
