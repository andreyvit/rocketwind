package rocketwind

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Source identifies files to scan, relative to the stylesheet which declared it.
type Source struct {
	Base, Pattern string
	Negated       bool
}

func (c *Compiler) Sources() []Source { return slices.Clone(c.sources) }

func (c *Compiler) addSource(node *Node) error {
	path := node.Value
	negated := strings.HasPrefix(path, "not ")
	if negated {
		path = path[4:]
	}
	inline := strings.HasPrefix(path, "inline(")
	if inline {
		path = strings.TrimSpace(path[7 : len(path)-1])
	}
	if len(path) < 2 || path[0] != '"' && path[0] != '\'' || path[len(path)-1] != path[0] {
		return fmt.Errorf("`@source` paths must be quoted.")
	}
	path = path[1 : len(path)-1]
	if !inline {
		c.sources = append(c.sources, Source{node.base, path, negated})
		return nil
	}
	for pattern := range Segments(path, ' ') {
		candidates, err := Expand(pattern)
		if err != nil {
			return err
		}
		if negated {
			if c.excludedCandidates == nil {
				c.excludedCandidates = make(map[string]bool)
			}
			for _, candidate := range candidates {
				c.excludedCandidates[candidate] = true
			}
		} else {
			c.inlineCandidates = append(c.inlineCandidates, candidates...)
		}
	}
	return nil
}

// Expand applies Tailwind's brace expansion, including numerical sequences.
func Expand(pattern string) ([]string, error) {
	start := strings.IndexByte(pattern, '{')
	if start < 0 {
		return []string{pattern}, nil
	}
	end, depth := -1, 0
	for i := start; i < len(pattern); i++ {
		switch pattern[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 {
		return nil, fmt.Errorf("The pattern `%s` is not balanced.", pattern)
	}
	inside := pattern[start+1 : end]
	parts := slices.Collect(Segments(inside, ','))
	if seq := strings.Split(inside, ".."); len(seq) == 2 || len(seq) == 3 {
		first, e1 := strconv.Atoi(seq[0])
		last, e2 := strconv.Atoi(seq[1])
		step := 1
		var e3 error
		if len(seq) == 3 {
			step, e3 = strconv.Atoi(seq[2])
		}
		if e1 == nil && e2 == nil && e3 == nil {
			if step == 0 {
				return nil, fmt.Errorf("Step cannot be zero in sequence expansion.")
			}
			if step < 0 {
				step = -step
			}
			if first > last {
				step = -step
			}
			parts = nil
			for i := first; step > 0 && i <= last || step < 0 && i >= last; i += step {
				parts = append(parts, strconv.Itoa(i))
				if i == last {
					break
				}
			}
		}
	}
	var expanded []string
	for _, part := range parts {
		values, err := Expand(part)
		if err != nil {
			return nil, err
		}
		expanded = append(expanded, values...)
	}
	tails, err := Expand(pattern[end+1:])
	if err != nil {
		return nil, err
	}
	var result []string
	for _, tail := range tails {
		for _, part := range expanded {
			result = append(result, pattern[:start]+part+tail)
		}
	}
	return result, nil
}
