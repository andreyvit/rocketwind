package rocketwind

import (
	"regexp"
	"strconv"
	"strings"
)

var v3ExtractRegexps = func() []*regexp.Regexp {
	var result []*regexp.Regexp
	for _, source := range v3ExtractPatterns {
		if strings.HasPrefix(source, "((?=((") {
			variant, utility, _ := strings.Cut(source[6:], ")+))\\2)?")
			source = "(?:" + variant + ")*" + utility
			source = strings.ReplaceAll(source, "(?![{([]])", "")
		}
		result = append(result, regexp.MustCompile(source))
	}
	return result
}()

func appendExtractedV3(dst []Extracted, input string) []Extracted {
	start := len(dst)
	for _, pattern := range v3ExtractRegexps {
		for _, span := range pattern.FindAllStringIndex(input, -1) {
			value := v3ClipCandidate(input[span[0]:span[1]])
			dst = append(dst, Extracted{Kind: "candidate", Value: value, Start: span[0], End: span[0] + len(value)})
		}
	}
	end := len(dst)
	for i := start; i < end; i++ {
		token := dst[i]
		parts := appendSegments(nil, token.Value, '.')
		offset := 0
		for j := 0; j < len(parts); j++ {
			part := parts[j]
			if j+1 < len(parts) {
				next := strings.TrimSpace(parts[j+1])
				if _, err := strconv.ParseFloat(next, 64); next == "" || err == nil {
					offset += len(part) + len(parts[j+1]) + 2
					j++
					continue
				}
			}
			dst = append(dst, Extracted{Kind: "candidate", Value: part, Start: token.Start + offset, End: token.Start + offset + len(part)})
			offset += len(part) + 1
		}
	}
	return dst
}

func v3ClipCandidate(input string) string {
	if !strings.Contains(input, "-[") {
		return input
	}
	depth := 0
	var quotes []byte
	for i := 0; i < len(input); i++ {
		if !strings.ContainsRune("[]'\"`", rune(input[i])) {
			continue
		}
		end := min(i+2, len(input))
		for ; i < end; i++ {
			ch := input[i]
			inString := len(quotes) > 0
			if inString && ch == quotes[len(quotes)-1] {
				quotes = quotes[:len(quotes)-1]
			} else if ch == '\'' || ch == '"' || ch == '`' {
				quotes = append(quotes, ch)
			}
			if inString {
				continue
			}
			if ch == '[' {
				depth++
				continue
			}
			if ch == ']' {
				depth--
				continue
			}
			if depth < 0 {
				return input[:i-1]
			}
			if depth == 0 && (cssSpace(ch) || strings.ContainsRune("\"'`<>]", rune(ch))) {
				return input[:i]
			}
		}
		i--
	}
	return input
}
