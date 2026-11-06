package rocketwind

import (
	"math/rand/v2"
	"strings"
	"testing"
)

func TestLegacyDFAMatchesOriginalExpressions(t *testing.T) {
	random := rand.New(rand.NewPCG(271828, 314159))
	inputs := []string{
		"", "[color:red]", "hover:w-[calc(100%-1rem)]", "[&>p]:!font-bold", "bg-[url('/a[b].png')]/50",
		strings.Repeat(":", 10000), strings.Repeat("[abc:def]", 400),
		"@[!]/!:[!::", "[!:![]:\":!",
	}
	alphabet := "azAZ09_!@:-/.$={}<>\\\"'`()#%; \t\v\n\r\f[]\x00\x7f"
	for range 32768 {
		var text strings.Builder
		for range random.IntN(160) {
			text.WriteByte(alphabet[random.IntN(len(alphabet))])
		}
		inputs = append(inputs, text.String())
	}
	for i, dfa := range v3ExtractDFAs {
		for _, input := range inputs {
			begin, end, handled := dfa.find(input)
			if !handled {
				continue
			}
			want := v3ExtractRegexps[i].FindStringIndex(input)
			if want == nil {
				if begin != -1 || end != -1 {
					t.Fatalf("pattern %d: matched %q at %d:%d", i, input, begin, end)
				}
			} else if begin != want[0] || end != want[1] {
				t.Fatalf("pattern %d: %q: %d:%d != %v", i, input, begin, end, want)
			}
		}
	}
}

func FuzzLegacyExtractorDFA(f *testing.F) {
	for _, input := range []string{"hover:w-[calc(100%-1rem)]", "[&>p]:!font-bold", "bg-[url('/a[b].png')]/50", "@[!]/!:[!::", "[!:![]:\":!"} {
		f.Add(input)
	}
	f.Fuzz(func(t *testing.T, input string) {
		if strings.IndexFunc(input, func(r rune) bool { return r >= 128 }) >= 0 {
			return
		}
		for i, dfa := range v3ExtractDFAs {
			begin, end, handled := dfa.find(input)
			if !handled {
				continue
			}
			want := v3ExtractRegexps[i].FindStringIndex(input)
			if want == nil && begin != -1 || want != nil && (begin != want[0] || end != want[1]) {
				t.Fatalf("pattern %d: %q: %d:%d != %v", i, input, begin, end, want)
			}
		}
	})
}
