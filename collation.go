package rocketwind

import (
	_ "embed"
	"strings"
	"unicode/utf8"
)

// Unicode 17 / ICU 78.3 en-US data, matching the pinned Tailwind test runtime.
// See data/collation/LICENSE and scripts/collation.py. No ICU runtime is used.
//
//go:embed data/collation/unicode.bin
var collationData string

var collationSingleCount = int(collationUint32(0))
var collationNormCount = int(collationUint32(4))
var collationContractCount = int(collationUint32(8))
var collationPool = int(collationUint32(12))
var collationNorm = 16 + collationSingleCount*8
var collationContract = collationNorm + collationNormCount*12

var collationASCII = func() [128]struct {
	weights     string
	contraction bool
} {
	var result [128]struct {
		weights     string
		contraction bool
	}
	for cp := range result {
		i := collationLookup(rune(cp), 16, collationSingleCount, 8)
		result[cp].weights = collationWeights(collationUint32(i + 4))
	}
	for i := range collationContractCount {
		if cp := collationKey(i)[0]; cp < utf8.RuneSelf {
			result[cp].contraction = true
		}
	}
	return result
}()

func collationUint32(i int) uint32 {
	_ = collationData[i+3]
	return uint32(collationData[i]) | uint32(collationData[i+1])<<8 | uint32(collationData[i+2])<<16 | uint32(collationData[i+3])<<24
}

func collationLookup(cp rune, start, count, stride int) int {
	lo, hi := 0, count
	for lo < hi {
		mid := (lo + hi) >> 1
		if rune(collationUint32(start+mid*stride)) < cp {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	i := start + lo*stride
	if lo < count && rune(collationUint32(i)) == cp {
		return i
	}
	return -1
}

func combiningClass(cp rune) uint8 {
	if cp < 0x300 {
		return 0
	}
	if i := collationLookup(cp, collationNorm, collationNormCount, 12); i >= 0 {
		return uint8(collationUint32(i+8) >> 16)
	}
	return 0
}

// normalizeCollation performs canonical decomposition and combining-mark order.
// Already normalized short inputs keep their original string without allocation.
func normalizeCollation(input string) string {
	ascii := true
	for i := range len(input) {
		if input[i] >= utf8.RuneSelf {
			ascii = false
			break
		}
	}
	if ascii {
		return input
	}
	var local [128]rune
	chars := local[:0]
	changed := false
	add := func(cp rune) {
		chars = append(chars, cp)
		ccc := combiningClass(cp)
		if ccc != 0 {
			for i := len(chars) - 1; i > 0 && combiningClass(chars[i-1]) > ccc; i-- {
				chars[i], chars[i-1] = chars[i-1], chars[i]
				changed = true
			}
		}
	}
	for offset, cp := range input {
		if cp >= 0xac00 && cp <= 0xd7a3 {
			// ICU reserves FDD1 + 가 as an alphabetic-index marker. It is
			// deliberately distinct from FDD1 followed by decomposed Hangul.
			if cp == 0xac00 && offset >= 3 && input[offset-3:offset] == "\ufdd1" {
				add(cp)
				continue
			}
			// Algorithmic Hangul decomposition (Unicode Standard, section 3.12).
			index := cp - 0xac00
			add(0x1100 + index/588)
			add(0x1161 + index%588/28)
			if tail := index % 28; tail != 0 {
				add(0x11a7 + tail)
			}
			changed = true
			continue
		}
		if i := collationLookup(cp, collationNorm, collationNormCount, 12); i >= 0 {
			length := int(collationUint32(i+8) & 0xffff)
			if length != 0 {
				start := collationPool + int(collationUint32(i+4))
				for _, decomposed := range collationData[start : start+length] {
					add(decomposed)
				}
				changed = true
				continue
			}
		}
		add(cp)
	}
	if !changed {
		return input
	}
	return string(chars)
}

func localeCompare(a, b string) int {
	if a == b {
		return 0
	}
	a, b = normalizeCollation(a), normalizeCollation(b)
	if a == b {
		return 0
	}
	for level := 0; level < 3; level++ {
		left, right := collationIterator{text: a}, collationIterator{text: b}
		for {
			x, y := left.next(level), right.next(level)
			if x < y {
				return -1
			}
			if x > y {
				return 1
			}
			if x == 0 {
				break
			}
		}
	}
	return 0
}

type collationIterator struct {
	text               string
	weights            string
	implicit, trailing uint32
}

func (it *collationIterator) next(level int) uint32 {
	for {
		var element uint32
		if it.weights != "" {
			w := it.weights
			element = uint32(w[0]) | uint32(w[1])<<8 | uint32(w[2])<<16 | uint32(w[3])<<24
			it.weights = w[4:]
		} else if it.implicit != 0 {
			element, it.implicit, it.trailing = it.implicit, it.trailing, 0
		} else {
			if it.text == "" {
				return 0
			}
			it.advance()
			continue
		}
		var weight uint32
		switch level {
		case 0:
			weight = element >> 16
		case 1:
			weight = element >> 8 & 0xff
		case 2:
			// en-US uses caseFirst=off: discard the separate case bits.
			weight = element & 0x3f
		}
		if weight != 0 {
			return weight
		}
	}
}

func collationWeights(packed uint32) string {
	start := collationPool + int(packed&0xffffff)
	return collationData[start : start+int(packed>>24)*4]
}
func collationKey(index int) string {
	i := collationContract + index*12
	start, length := collationPool+int(collationUint32(i)), int(collationUint32(i+4))
	return collationData[start : start+length]
}

func (it *collationIterator) advance() {
	if cp := it.text[0]; cp < utf8.RuneSelf && (!collationASCII[cp].contraction || len(it.text) == 1 || it.text[1] < utf8.RuneSelf) {
		it.weights = collationASCII[cp].weights
		it.text = it.text[1:]
		return
	}
	cp, size := utf8.DecodeRuneInString(it.text)
	first := it.text[:size]
	lo, hi := 0, collationContractCount
	for lo < hi {
		mid := (lo + hi) >> 1
		if collationKey(mid) < first {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	best, consumed, keyLength, skipped := -1, size, 0, ""
	for i := lo; i < collationContractCount; i++ {
		key := collationKey(i)
		if !strings.HasPrefix(key, first) {
			break
		}
		if n, rest := matchCollationContraction(it.text, key); n > 0 && len(key) > keyLength {
			best, consumed, keyLength, skipped = i, n, len(key), rest
		}
	}
	if best >= 0 {
		it.weights = collationWeights(collationUint32(collationContract + best*12 + 8))
		it.text = skipped + it.text[consumed:]
		return
	}
	it.text = it.text[size:]
	if i := collationLookup(cp, 16, collationSingleCount, 8); i >= 0 {
		it.weights = collationWeights(collationUint32(i + 4))
		return
	}
	// ICU's implicit primary weights (icu4c/source/i18n/collation.cpp).
	x := uint32(cp) + 1
	primary := 2 + x%18*14
	x /= 18
	primary |= (2 + x%254) << 8
	x /= 254
	primary |= (4+x%251)<<16 | 0xfe000000
	it.implicit = primary&0xffff0000 | 0x0505
	it.trailing = primary<<16 | 0xc0
}

func matchCollationContraction(text, key string) (int, string) {
	if strings.HasPrefix(text, key) {
		return len(key), ""
	}
	index := 0
	var skipped strings.Builder
	for _, target := range key {
		ccc := combiningClass(target)
		for {
			if index == len(text) {
				return 0, ""
			}
			cp, size := utf8.DecodeRuneInString(text[index:])
			if cp == target {
				index += size
				break
			}
			other := combiningClass(cp)
			if other == 0 || ccc == 0 || other >= ccc {
				return 0, ""
			}
			skipped.WriteString(text[index : index+size])
			index += size
		}
	}
	return index, skipped.String()
}
