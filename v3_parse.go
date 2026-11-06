package rocketwind

import (
	"iter"
	"strings"
)

func (c *Compiler) parseV3(parser candidateParser, class string) iter.Seq[Candidate] {
	return func(yield func(Candidate) bool) {
		var local [8]string
		parts := appendSegments(local[:0], class, ':')
		base := parts[len(parts)-1]
		if strings.HasSuffix(base, "!") {
			return
		}
		important := strings.HasPrefix(base, "!")
		base = strings.TrimPrefix(base, "!")
		if prefix := c.v3Prefix; prefix != "" && !strings.HasPrefix(base, "[") {
			negative := strings.HasPrefix(base, "-")
			base = strings.TrimPrefix(base, "-")
			var ok bool
			base, ok = strings.CutPrefix(base, prefix)
			if !ok || strings.HasPrefix(base, "[") {
				return
			}
			if negative {
				base = "-" + base
			}
		}
		for segment := range Segments(base, '/') {
			if strings.HasPrefix(segment, "(") || strings.HasSuffix(segment, ")") && !strings.HasSuffix(segment, "]") {
				return
			}
		}
		if important {
			base = "!" + base
		}
		parts[len(parts)-1] = base
		for candidate := range parser.parse(strings.Join(parts, ":")) {
			candidate.Raw = class
			if !yield(candidate) {
				return
			}
		}
	}
}
