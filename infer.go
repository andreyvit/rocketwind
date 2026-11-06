// CSS type inference follows Tailwind CSS v4.3.4. See testdata/upstream/LICENSE.
package rocketwind

import (
	"regexp"
	"strings"
)

var cssNumberPattern = regexp.MustCompile(`^[+-]?\d*\.?\d+(?:[eE][+-]?\d+)?$`)

func hasMath(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '(' {
			j := i
			for j > 0 && (asciiLetter(s[j-1]) || digit(s[j-1])) {
				j--
			}
			if mathFunction(s[j:i]) {
				return true
			}
		}
	}
	return false
}
func isNumber(s string) bool { return cssNumberPattern.MatchString(s) || hasMath(s) }
func isPercentage(s string) bool {
	return strings.HasSuffix(s, "%") && cssNumberPattern.MatchString(s[:len(s)-1]) || hasMath(s)
}
func isDimension(s string, units string) bool {
	for unit := range strings.FieldsSeq(units) {
		if strings.HasSuffix(s, unit) && cssNumberPattern.MatchString(s[:len(s)-len(unit)]) {
			return true
		}
	}
	return false
}
func isLength(s string) bool {
	return strings.HasPrefix(strings.ToLower(s), "--spacing(") || hasMath(s) || isDimension(s, "cm mm Q in pc pt px em ex ch cap ic rem rex rch rcap ric lh rlh vw vh vmin vmax vb vi svw svh svi svb svmin svmax lvw lvh lvi lvb lvmin lvmax dvw dvh dvi dvb dvmin dvmax cqw cqh cqi cqb cqmin cqmax")
}
func isColor(s string) bool {
	s = strings.ToLower(s)
	if strings.HasPrefix(s, "#") || namedColors[s] {
		return true
	}
	name, _, found := strings.Cut(s, "(")
	return found && containsWord("rgb rgba hsl hsla hwb color lab lch oklab oklch light-dark color-mix --alpha", name)
}
func containsWord(words, s string) bool {
	for word := range strings.FieldsSeq(words) {
		if word == s {
			return true
		}
	}
	return false
}
func isURL(s string) bool {
	return strings.HasPrefix(s, "url(") && strings.HasSuffix(s, ")") && !strings.Contains(s, "\n")
}
func inferType(s string, types ...string) string {
	if strings.HasPrefix(s, "var(") {
		return ""
	}
	for _, t := range types {
		if matchesType(s, t) {
			return t
		}
	}
	return ""
}
func matchesType(s, t string) bool {
	switch t {
	case "color":
		return isColor(s)
	case "number":
		return isNumber(s)
	case "integer":
		return positiveInteger(s)
	case "length":
		return isLength(s)
	case "percentage":
		return isPercentage(s)
	case "angle":
		return isDimension(s, "deg rad grad turn")
	case "url":
		return isURL(s)
	case "ratio":
		a, z, ok := strings.Cut(s, "/")
		return ok && cssNumberPattern.MatchString(strings.TrimSpace(a)) && cssNumberPattern.MatchString(strings.TrimSpace(z)) || hasMath(s)
	case "vector":
		parts := strings.Fields(s)
		return len(parts) == 3 && cssNumberPattern.MatchString(parts[0]) && cssNumberPattern.MatchString(parts[1]) && cssNumberPattern.MatchString(parts[2])
	case "generic-name":
		return containsWord("serif sans-serif monospace cursive fantasy system-ui ui-serif ui-sans-serif ui-monospace ui-rounded math emoji fangsong", s)
	case "absolute-size":
		return containsWord("xx-small x-small small medium large x-large xx-large xxx-large", s)
	case "relative-size":
		return s == "larger" || s == "smaller"
	case "line-width":
		for part := range Segments(s, ' ') {
			if !isLength(part) && !isNumber(part) && !containsWord("thin medium thick", part) {
				return false
			}
		}
		return true
	case "family-name":
		count := 0
		for part := range Segments(s, ',') {
			if len(part) > 0 && digit(part[0]) {
				return false
			}
			if !strings.HasPrefix(part, "var(") {
				count++
			}
		}
		return count > 0
	case "image":
		count := 0
		for part := range Segments(s, ',') {
			if strings.HasPrefix(part, "var(") {
				continue
			}
			name, _, _ := strings.Cut(part, "(")
			if !isURL(part) && !containsWord("element image cross-fade image-set conic-gradient linear-gradient radial-gradient repeating-conic-gradient repeating-linear-gradient repeating-radial-gradient", name) {
				return false
			}
			count++
		}
		return count > 0
	case "position":
		count := 0
		for part := range Segments(s, ' ') {
			if strings.HasPrefix(part, "var(") {
				continue
			}
			if !containsWord("center top right bottom left", part) && !isLength(part) && !isPercentage(part) {
				return false
			}
			count++
		}
		return count > 0
	case "bg-size":
		count := 0
		for size := range Segments(s, ',') {
			if size == "cover" || size == "contain" {
				count++
				continue
			}
			n := 0
			valid := true
			for part := range Segments(size, ' ') {
				n++
				if part != "auto" && !isLength(part) && !isPercentage(part) {
					valid = false
				}
			}
			if n < 1 || n > 2 {
				return false
			}
			if valid {
				count++
			}
		}
		return count > 0
	}
	return false
}
