package rocketwind

import (
	"iter"
	"slices"
	"strconv"
	"strings"
)

// intersection preserves the iteration order of the smaller set, as the native
// ECMAScript Set method does. Inputs already contain unique values.
func intersection[T comparable](a, b []T) []T {
	if len(a) > len(b) {
		a, b = b, a
	}
	result := make([]T, 0)
	set := make(map[T]bool, len(b))
	for _, item := range b {
		set[item] = true
	}
	for _, item := range a {
		if set[item] {
			result = append(result, item)
		}
	}
	return result
}

func canonicalCombinations(input []int) iter.Seq[[]int] {
	// Increasing index vectors in colexicographic order match Gosper's bit masks
	// without a fixed-width integer limit.
	return func(yield func([]int) bool) {
		for size := len(input); size >= 2; size-- {
			indices := make([]int, size)
			for i := range indices {
				indices[i] = i
			}
			for {
				combo := make([]int, size)
				for i, index := range indices {
					combo[i] = input[index]
				}
				if !yield(combo) {
					return
				}
				at := 0
				for at < size && (at+1 < size && indices[at]+1 == indices[at+1] || at+1 == size && indices[at]+1 == len(input)) {
					at++
				}
				if at == size {
					break
				}
				indices[at]++
				for i := 0; i < at; i++ {
					indices[i] = i
				}
			}
		}
	}
}

func (k *Canonicalizer) collapse(candidates []string) []string {
	type group struct {
		before, after string
		classes       []string
	}
	var groups []*group
	var beforeOrder []string
	byBefore := map[string][]*group{}
	for _, candidate := range candidates {
		parts := slices.Collect(Segments(candidate, ':'))
		utility := parts[len(parts)-1]
		after := ""
		if strings.HasSuffix(utility, "!") {
			utility = utility[:len(utility)-1]
			after = "!"
		}
		before := strings.Join(parts[:len(parts)-1], ":")
		if before != "" {
			before += ":"
		}
		var found *group
		for _, g := range byBefore[before] {
			if g.after == after {
				found = g
				break
			}
		}
		if found == nil {
			found = &group{before: before, after: after}
			if _, ok := byBefore[before]; !ok {
				beforeOrder = append(beforeOrder, before)
			}
			byBefore[before] = append(byBefore[before], found)
		}
		found.classes = appendUnique(found.classes, k.prefixed(utility))
	}
	for _, before := range beforeOrder {
		groups = append(groups, byBefore[before]...)
	}
	var result []string
	for _, g := range groups {
		for _, candidate := range k.collapseGroup(g.classes) {
			if p := k.compiler.config.Prefix; p != "" {
				candidate = strings.TrimPrefix(candidate, p+":")
			}
			result = appendUnique(result, g.before+candidate+g.after)
		}
	}
	return result
}

type cachedCollapse struct {
	generation uint64
	classes    []string
}

func (k *Canonicalizer) collapseGroup(candidates []string) (output []string) {
	var keyBuilder strings.Builder
	var number [20]byte
	for _, name := range candidates {
		keyBuilder.Write(strconv.AppendInt(number[:0], int64(len(name)), 10))
		keyBuilder.WriteByte(':')
		keyBuilder.WriteString(name)
	}
	key := keyBuilder.String()
	if cached, ok := k.collapseCache[key]; ok && cached.generation == k.propertyGeneration {
		return cached.classes
	}
	defer func() { k.collapseCache[key] = cachedCollapse{k.propertyGeneration, output} }()
	props := make([]map[string][]string, len(candidates))
	for i, name := range candidates {
		props[i] = k.utilityProperties(name)
	}
	var fontNames []string
	for _, key := range k.compiler.themeKeys() {
		if name, ok := strings.CutPrefix(key, "--text-"); ok && !strings.Contains(name, "--") {
			fontNames = append(fontNames, name)
		}
	}
	if len(fontNames) > 0 {
		var lineHeights []string
		seen := map[string]bool{}
		for _, p := range props {
			for _, value := range p["line-height"] {
				if seen[value] {
					continue
				}
				seen[value] = true
				if bare, ok := k.spacing(value); ok {
					modifier := "[" + value + "]"
					if quarterNumber(jsFloat(bare)) {
						modifier = jsFloat(bare)
					}
					lineHeights = appendUnique(lineHeights, modifier)
					for _, font := range fontNames {
						k.utilityProperties("text-" + font + "/" + modifier)
					}
				}
			}
		}
		for _, p := range props {
			for _, size := range p["font-size"] {
				for _, line := range lineHeights {
					k.utilityProperties("text-[" + size + "]/" + line)
				}
			}
		}
	}
	dynamicCache := map[string]map[string]map[string][]string{}
	dynamic := func(name string) map[string]map[string][]string {
		if cached, ok := dynamicCache[name]; ok {
			return cached
		}
		result := map[string]map[string][]string{}
		dynamicCache[name] = result
		relevant := k.utilityProperties(name)
		for parsed := range k.parse(name) {
			if parsed.Kind != "functional" || parsed.Value == nil {
				continue
			}
			for _, root := range k.roots {
				if root == parsed.Root {
					continue
				}
				copy := parsed
				copy.Root = root
				replacement := k.unprefixed(copy)
				values := k.utilityProperties(replacement)
				for _, property := range k.propertyKeys[replacement] {
					if _, ok := relevant[property]; !ok {
						continue
					}
					if result[property] == nil {
						result[property] = map[string][]string{}
					}
					for _, value := range values[property] {
						result[property][value] = appendUnique(result[property][value], replacement)
					}
				}
			}
			break
		}
		return result
	}
	other := make([][]string, len(candidates))
	for i, name := range candidates {
		for j, property := range k.propertyKeys[name] {
			var matches []string
			for _, value := range k.staticValueKeys[property] {
				for _, class := range k.staticProperties[property][value] {
					matches = appendUnique(matches, class)
				}
			}
			lookup := dynamic(name)
			for _, value := range props[i][property] {
				for _, class := range lookup[property][value] {
					matches = appendUnique(matches, class)
				}
			}
			if j == 0 {
				other[i] = matches
			} else {
				other[i] = intersection(other[i], matches)
			}
		}
	}
	counts := map[string]int{}
	for _, list := range other {
		for _, name := range list {
			counts[name]++
		}
	}
	covered := map[string]map[string]bool{}
	for i, list := range other {
		for _, name := range list {
			if counts[name] < 2 {
				continue
			}
			if covered[name] == nil {
				covered[name] = map[string]bool{}
			}
			for property := range props[i] {
				covered[name][property] = true
			}
		}
	}
	possible := map[string]bool{}
	checked := map[string]bool{}
	for i, list := range other {
		filtered := make([]string, 0, len(list))
		for _, name := range list {
			if counts[name] < 2 {
				continue
			}
			if !checked[name] {
				checked[name] = true
				possible[name] = true
				for property := range k.utilityProperties(name) {
					if !covered[name][property] {
						possible[name] = false
						break
					}
				}
			}
			if possible[name] {
				filtered = append(filtered, name)
			}
		}
		other[i] = filtered
	}
	linked := map[int][]int{}
	var linkedOrder []int
	link := func(i, j int) {
		if _, ok := linked[i]; !ok {
			linked[i] = []int{i}
			linkedOrder = append(linkedOrder, i)
		}
		linked[i] = append(linked[i], j)
	}
	for i := range other {
		for j := i + 1; j < len(other); j++ {
			if len(intersection(other[i], other[j])) > 0 {
				link(i, j)
				link(j, i)
			}
		}
	}
	if len(linked) == 0 {
		return candidates
	}
	var groups [][]int
	for _, i := range linkedOrder {
		group := linked[i]
		slices.Sort(group)
		duplicate := false
		for _, existing := range groups {
			if slices.Equal(group, existing) {
				duplicate = true
				break
			}
		}
		if !duplicate {
			groups = append(groups, group)
		}
	}
	var orderable []string
	for _, i := range linkedOrder {
		if _, err := k.compiler.classOrder(candidates[i : i+1]); err == nil {
			orderable = append(orderable, candidates[i])
		}
	}
	orders := k.compiler.ClassOrder(orderable)
	order := map[string]int{}
	for _, i := range linkedOrder {
		order[candidates[i]] = -1
	}
	for i, name := range orderable {
		order[name] = orders[i]
	}
	result := slices.Clone(candidates)
	drop := map[string]bool{}
	for _, group := range groups {
		for combo := range canonicalCombinations(group) {
			skip := false
			for _, i := range combo {
				if drop[candidates[i]] {
					skip = true
					break
				}
			}
			if skip {
				continue
			}
			potential := other[combo[0]]
			for _, i := range combo[1:] {
				potential = intersection(potential, other[i])
			}
			if len(potential) == 0 {
				continue
			}
			names := make([]string, len(combo))
			for i, index := range combo {
				names[i] = candidates[index]
			}
			slices.SortFunc(names, localeCompare)
			collapsed := k.signature(strings.Join(names, " "))
			if collapsed.css == "" {
				continue
			}
			for i, index := range combo {
				names[i] = candidates[index]
			}
			slices.SortStableFunc(names, func(a, b string) int {
				return canonicalCollapseOrder(order[a], order[b])
			})
			for _, name := range potential {
				names = appendUnique(names, name)
			}
			for _, name := range names {
				sig := k.signature(name)
				if sig.css != collapsed.css || !sameThemeVariables(collapsed.variables, sig.variables) {
					continue
				}
				result = appendUnique(result, name)
				for _, i := range combo {
					if candidates[i] != name {
						drop[candidates[i]] = true
					}
				}
				break
			}
		}
	}
	return slices.DeleteFunc(result, func(name string) bool { return drop[name] })
}

func canonicalCollapseOrder(a, b int) int {
	if a == b {
		return 0
	}
	if a < 0 {
		return -1
	}
	if b < 0 {
		return 1
	}
	return b - a
}

func sameThemeVariables(original, replacement []string) bool {
	if original == nil || replacement == nil || len(original) != len(replacement) {
		return false
	}
	for i, value := range original {
		if value != "" && value != replacement[i] {
			return false
		}
	}
	return true
}
