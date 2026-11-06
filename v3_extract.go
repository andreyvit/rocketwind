package rocketwind

import (
	"regexp"
	"strings"
)

var v3ExtractNumber = regexp.MustCompile(`^(?:[+-]?(?:Infinity|(?:[0-9]+\.?[0-9]*|\.[0-9]+)(?:[eE][+-]?[0-9]+)?)|0[xX][0-9a-fA-F]+|0[bB][01]+|0[oO][0-7]+)$`)

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

// Without an opening bracket, all arbitrary-value alternatives are impossible.
// Keep the two upstream variant forms, including their different quote rules.
var v3SimpleRegexps = []*regexp.Regexp{
	regexp.MustCompile("(?:[^\\s\"'`\\[\\\\]+:)*!?(?:-?\\w+|@\\w+)(?:[-/][^\\s'\"`\\\\$={><]*)?"),
	regexp.MustCompile("(?:[^\\s`\\[\\\\]+:)*!?(?:-?\\w+|@\\w+)(?:[-/][^\\s'\"`\\\\$={><]*)?"),
	v3ExtractRegexps[2],
}

func appendExtractedV3(dst []Extracted, input string) []Extracted {
	start := len(dst)
	patterns := v3ExtractRegexps
	if !strings.ContainsAny(input, "[:") && strings.IndexFunc(input, func(r rune) bool { return r >= 128 }) < 0 {
		dst = appendV3PlainTokens(dst, input)
		patterns = nil
	} else if !strings.ContainsRune(input, '[') {
		patterns = v3SimpleRegexps
	}
	for _, pattern := range patterns {
		for _, span := range pattern.FindAllStringIndex(input, -1) {
			value := v3ClipCandidate(input[span[0]:span[1]])
			dst = append(dst, Extracted{Kind: "candidate", Value: value, Start: span[0], End: span[0] + len(value)})
		}
	}
	end := len(dst)
	for i := start; i < end; i++ {
		token := dst[i]
		if !strings.ContainsRune(token.Value, '.') {
			continue
		}
		parts := appendSegments(nil, token.Value, '.')
		offset := 0
		for j := 0; j < len(parts); j++ {
			part := parts[j]
			if j+1 < len(parts) {
				next := strings.TrimSpace(parts[j+1])
				if next == "" || v3ExtractNumber.MatchString(next) {
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

// Without variants or bracket values, both detailed extractors reduce to this
// ASCII word scanner. The broad fallback preserves the old extractor's tokens.
func appendV3PlainTokens(dst []Extracted, input string) []Extracted {
	word := func(ch byte) bool { return asciiLetter(ch) || digit(ch) || ch == '_' }
	for i := 0; i < len(input); {
		start := i
		if input[i] == '!' {
			i++
		}
		if i < len(input) && (input[i] == '-' || input[i] == '@') {
			i++
		}
		if i == len(input) || !word(input[i]) {
			i = start + 1
			continue
		}
		for i < len(input) && word(input[i]) {
			i++
		}
		if i < len(input) && (input[i] == '-' || input[i] == '/') {
			i++
			for i < len(input) && !cssSpace(input[i]) && !strings.ContainsRune("\"'`\\$={><", rune(input[i])) {
				i++
			}
		}
		dst = append(dst, Extracted{Kind: "candidate", Value: input[start:i], Start: start, End: i})
	}
	// Third upstream expression: two or more characters, with stricter edges.
	for i := 0; i < len(input); {
		for i < len(input) && (cssSpace(input[i]) || strings.ContainsRune("<>\"'`(){}[]#=%$", rune(input[i]))) {
			i++
		}
		start := i
		for i < len(input) && !cssSpace(input[i]) && !strings.ContainsRune("<>\"'`(){}[]#=%$", rune(input[i])) {
			i++
		}
		end := i
		for start < end && input[start] == '.' {
			start++
		}
		for end > start && strings.ContainsRune(".:", rune(input[end-1])) {
			end--
		}
		if end-start >= 2 {
			dst = append(dst, Extracted{Kind: "candidate", Value: input[start:end], Start: start, End: end})
		}
	}
	return dst
}
