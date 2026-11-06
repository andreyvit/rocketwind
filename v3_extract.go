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
	ascii := strings.IndexFunc(input, func(r rune) bool { return r >= 128 }) < 0
	if !strings.ContainsAny(input, "[:") && ascii {
		dst = appendV3PlainTokens(dst, input)
		patterns = nil
	} else if !strings.ContainsRune(input, '[') || !strings.ContainsRune(input, ':') && !strings.Contains(input, "-[") {
		dst = appendV3VariantTokens(dst, input, false)
		dst = appendV3VariantTokens(dst, input, true)
		patterns = v3SimpleRegexps[2:]
	}
	for _, pattern := range patterns {
		var dfa *extractDFA
		if ascii {
			if pattern == v3ExtractRegexps[0] {
				dfa = &v3ExtractDFAs[0]
			} else if pattern == v3ExtractRegexps[1] {
				dfa = &v3ExtractDFAs[1]
			}
		}
		// Only whole-match offsets are needed. FindAllStringIndex collects
		// every match and pads unused capture groups in the legacy patterns.
		for offset := 0; offset < len(input); {
			begin, end, handled := 0, 0, false
			if dfa != nil {
				begin, end, handled = dfa.find(input[offset:])
			}
			if !handled {
				span := pattern.FindStringIndex(input[offset:])
				if span == nil {
					break
				}
				begin, end = span[0], span[1]
			}
			if begin < 0 {
				break
			}
			begin, end = offset+begin, offset+end
			value := v3ClipCandidate(input[begin:end])
			dst = append(dst, Extracted{Kind: "candidate", Value: value, Start: begin, End: begin + len(value)})
			offset = end
		}
	}
	end := len(dst)
	for i := start; i < end; i++ {
		token := dst[i]
		if !strings.ContainsRune(token.Value, '.') {
			continue
		}
		var local [8]string
		parts := appendSegments(local[:0], token.Value, '.')
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

// A bracket-free variant prefix greedily ends at the last colon followed by a
// utility. Inspect that boundary once, then scan the remaining plain words.
func appendV3VariantTokens(dst []Extracted, input string, quotedVariants bool) []Extracted {
	separator := func(ch byte) bool {
		return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' || ch == '\f' || ch == '`' || ch == '\\' || !quotedVariants && (ch == '\'' || ch == '"')
	}
	for at := 0; at < len(input); {
		if separator(input[at]) {
			at++
			continue
		}
		end := at + 1
		for end < len(input) && !separator(input[end]) {
			end++
		}
		for colon := strings.LastIndexByte(input[at:end], ':'); colon > 0; colon = strings.LastIndexByte(input[at:at+colon], ':') {
			if stop := v3UtilityEnd(input, at+colon+1); stop > at+colon+1 {
				dst = append(dst, Extracted{Kind: "candidate", Value: input[at:stop], Start: at, End: stop})
				at = stop
				break
			}
		}
		for at < end {
			stop := v3UtilityEnd(input, at)
			if stop == at {
				at++
				continue
			}
			dst = append(dst, Extracted{Kind: "candidate", Value: input[at:stop], Start: at, End: stop})
			at = stop
		}
	}
	return dst
}

func v3UtilityEnd(input string, start int) int {
	i := start
	if i < len(input) && input[i] == '!' {
		i++
	}
	if i < len(input) && (input[i] == '-' || input[i] == '@') {
		i++
	}
	word := func(ch byte) bool { return asciiLetter(ch) || digit(ch) || ch == '_' }
	if i == len(input) || !word(input[i]) {
		return start
	}
	for i < len(input) && word(input[i]) {
		i++
	}
	if i < len(input) && (input[i] == '-' || input[i] == '/') {
		i++
		for i < len(input) && !strings.ContainsRune(" \t\n\r\f'\"`\\$={><", rune(input[i])) {
			i++
		}
	}
	return i
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
