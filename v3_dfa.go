package rocketwind

// The bounded tables retain regexp alternative priority and greedy matching.
// Rare states and searches exceeding a linear work budget use Go's regexp
// engine instead. Callers use the original engine for non-ASCII input too.
type extractDFA struct {
	classes     [128]uint8
	width       int
	accepting   []bool
	transitions []uint16
}

func (d *extractDFA) find(input string) (begin, end int, handled bool) {
	budget := len(input)*32 + 128
	for start := range len(input) {
		state := uint16(1)
		matched := -1
		for i := start; i < len(input); i++ {
			budget--
			if budget == 0 {
				return 0, 0, false
			}
			state = d.transitions[int(state)*d.width+int(d.classes[input[i]])]
			if state == 65535 {
				return 0, 0, false
			}
			if state == 0 {
				break
			}
			if d.accepting[state] {
				matched = i + 1
			}
		}
		if matched >= 0 {
			return start, matched, true
		}
	}
	return -1, -1, true
}
