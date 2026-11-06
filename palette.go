package rocketwind

// FlattenColors flattens a legacy color namespace for a Go utility matcher.
// Nested keys become hyphenated names; DEFAULT names the containing color.
// CSS values retain their precedence over legacy defaults.
func FlattenColors(colors ConfigObject) ConfigObject {
	result := configObjectBuilder{object: ConfigObject{}}
	for _, entry := range colors {
		if entry.Key == "__CSS_VALUES__" {
			continue
		}
		if children := configObject(entry.Value); children != nil {
			for _, child := range FlattenColors(children) {
				key := entry.Key
				if child.Key != "DEFAULT" {
					key += "-" + child.Key
				}
				result.set(key, child.Value)
			}
		} else {
			result.set(entry.Key, entry.Value)
		}
	}
	for _, entry := range configObject(colors.Get("__CSS_VALUES__")) {
		var flags int
		switch value := entry.Value.(type) {
		case int:
			flags = value
		case float64:
			flags = int(value)
		}
		if flags&4 == 0 {
			result.set(entry.Key, colors.Get(entry.Key))
		}
	}
	return result.object
}
