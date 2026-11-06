package cli

import (
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// Tailwind's argument conversion and the no-options path through mri 1.2.0.
// See testdata/upstream/LICENSE and testdata/upstream/MRI-LICENSE.
type argDefinition struct {
	Type, Alias string
	Default     any
	HasDefault  bool
}

func rawArguments(argv []string) (map[string][]any, []string) {
	flags := make(map[string][]any)
	positional := make([]string, 0)
	argAt := func(i int) string {
		if argv[i] == "-" {
			return "__IO_DEFAULT_VALUE__"
		}
		return argv[i]
	}
	for i := 0; i < len(argv); i++ {
		arg := argAt(i)
		if arg == "--" {
			for i++; i < len(argv); i++ {
				positional = append(positional, argAt(i))
			}
			break
		}
		dashes := len(arg) - len(strings.TrimLeft(arg, "-"))
		if dashes == 0 {
			positional = append(positional, arg)
			continue
		}
		name := arg[dashes:]
		if strings.HasPrefix(name, "no-") {
			flags[name[3:]] = []any{false}
			continue
		}
		value := ""
		if len(name) > 1 {
			if eq := strings.IndexByte(name[1:], '='); eq >= 0 {
				name, value = name[:eq+1], name[eq+2:]
			}
		}
		var raw any = value
		if value == "" {
			if i+1 == len(argv) || strings.HasPrefix(argAt(i+1), "-") {
				raw = true
			} else {
				i++
				raw = argAt(i)
			}
		}
		if text, ok := raw.(string); ok {
			if n, ok := argumentNumber(text); ok && !math.IsInf(n, 0) {
				raw = n
			}
		}
		if dashes == 2 {
			flags[name] = append(flags[name], raw)
		} else {
			letters := []rune(name)
			for j, letter := range letters {
				value := raw
				if j+1 < len(letters) {
					value = true
				}
				key := string(letter)
				flags[key] = append(flags[key], value)
			}
		}
	}
	for _, values := range flags {
		for i, value := range values {
			if value == "__IO_DEFAULT_VALUE__" {
				values[i] = "-"
			}
		}
	}
	return flags, positional
}

func typedArguments(definitions map[string]argDefinition, raw map[string][]any, positional []string) map[string]any {
	result := map[string]any{"_": positional}
	for flag, def := range definitions {
		value := def.Default
		if !def.HasDefault && def.Type == "boolean" {
			value = false
		}
		result[flag] = value
		for _, key := range []string{strings.TrimPrefix(def.Alias, "-"), strings.TrimPrefix(flag, "--")} {
			if values := raw[key]; len(values) > 0 {
				value, valid := convertArgument(values[len(values)-1], def.Type)
				if valid {
					result[flag] = value
				} else {
					// JavaScript's undefined disappears when encoded as JSON.
					delete(result, flag)
				}
			}
		}
	}
	return result
}

func convertArgument(value any, kind string) (any, bool) {
	switch kind {
	case "boolean", "number", "string", "boolean | string", "number | string", "boolean | number", "boolean | number | string":
	default:
		panic(fmt.Sprintf("Unhandled type: %s", kind))
	}
	for _, kind := range strings.Split(kind, " | ") {
		switch kind {
		case "boolean":
			if value == true || value == false {
				return value, true
			}
			if value == "true" || value == "false" {
				return value == "true", true
			}
		case "number":
			if value, ok := argumentNumber(value); ok {
				return value, true
			}
		case "string":
			return argumentString(value), true
		}
	}
	return nil, false
}

func argumentString(value any) string {
	if n, ok := value.(float64); ok {
		if n == 0 {
			return "0"
		}
		if a := math.Abs(n); a >= 1e-6 && a < 1e21 {
			return strconv.FormatFloat(n, 'f', -1, 64)
		}
		text := strconv.FormatFloat(n, 'g', -1, 64)
		return strings.ReplaceAll(strings.ReplaceAll(text, "e-0", "e-"), "e+0", "e+")
	}
	return fmt.Sprint(value)
}

func argumentNumber(value any) (float64, bool) {
	switch value := value.(type) {
	case float64:
		return value, true
	case bool:
		if value {
			return 1, true
		}
		return 0, true
	case string:
		value = strings.TrimFunc(value, func(r rune) bool {
			return r >= 9 && r <= 13 || r == 32 || r == 160 || r == 5760 || r >= 8192 && r <= 8202 || r == 8232 || r == 8233 || r == 8239 || r == 8287 || r == 12288 || r == 65279
		})
		if value == "" {
			return 0, true
		}
		if len(value) > 2 && value[0] == '0' {
			var base int
			switch value[1] {
			case 'x', 'X':
				base = 16
			case 'o', 'O':
				base = 8
			case 'b', 'B':
				base = 2
			}
			if base != 0 {
				n, err := strconv.ParseUint(value[2:], base, 64)
				if errors.Is(err, strconv.ErrRange) {
					integer, ok := new(big.Int).SetString(value[2:], base)
					if !ok {
						return 0, false
					}
					n, _ := new(big.Float).SetInt(integer).Float64()
					return n, true
				}
				return float64(n), err == nil
			}
		}
		if value == "Infinity" || value == "+Infinity" {
			return math.Inf(1), true
		}
		if value == "-Infinity" {
			return math.Inf(-1), true
		}
		n, err := strconv.ParseFloat(value, 64)
		return n, (err == nil && !math.IsInf(n, 0) || errors.Is(err, strconv.ErrRange)) && !math.IsNaN(n) && !strings.ContainsAny(value, "_xXpP")
	}
	return 0, false
}
