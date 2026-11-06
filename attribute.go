// Attribute selector parsing follows Tailwind CSS v4.3.4 under the MIT license.
package rocketwind

import "strings"

type attributeSelector struct {
	Attribute   string  `json:"attribute"`
	Operator    *string `json:"operator"`
	Quote       *string `json:"quote"`
	Value       *string `json:"value"`
	Sensitivity *string `json:"sensitivity"`
}

func parseAttribute(input string) *attributeSelector {
	if len(input) < 2 || input[0] != '[' || input[len(input)-1] != ']' {
		return nil
	}
	i, end := 1, len(input)-1
	space := func() {
		for i < end && strings.ContainsRune(" \t\n\r", rune(input[i])) {
			i++
		}
	}
	space()
	start := i
	for i < end {
		c := input[i]
		if c == '\\' {
			i += 2
			continue
		}
		if asciiLetter(c) || digit(c) || c == '-' || c == '_' || c >= 128 {
			i++
			continue
		}
		break
	}
	if start == i || i > end {
		return nil
	}
	result := &attributeSelector{Attribute: input[start:i]}
	space()
	if i == end {
		return result
	}
	op := ""
	if input[i] == '=' {
		op = "="
		i++
	} else if i+1 < end && input[i+1] == '=' && strings.ContainsRune("~|^$*", rune(input[i])) {
		op = input[i : i+2]
		i += 2
	} else {
		return nil
	}
	result.Operator = &op
	space()
	if i == end {
		return nil
	}
	value := ""
	if quote := input[i]; quote == '\'' || quote == '"' {
		q := string(quote)
		result.Quote = &q
		i++
		start = i
		for j := i; j < end; j++ {
			if input[j] == quote {
				i = j + 1
			} else if input[j] == '\\' {
				j++
			}
		}
		if i <= start {
			value = ""
		} else {
			value = input[start : i-1]
		}
	} else {
		start = i
		for i < end && !strings.ContainsRune(" \t\n\r", rune(input[i])) {
			i++
		}
		value = input[start:i]
	}
	result.Value = &value
	space()
	if i == end {
		return result
	}
	if !strings.ContainsRune("iIsS", rune(input[i])) {
		return nil
	}
	sensitivity := strings.ToLower(input[i : i+1])
	result.Sensitivity = &sensitivity
	i++
	space()
	if i != end {
		return nil
	}
	return result
}
