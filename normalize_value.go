// Value normalization follows Tailwind CSS v4.3.4 under the MIT license.
package rocketwind

import (
	"math"
	"strconv"
	"strings"
)

func canonicalizeCalcExpressions(input string) string {
	nodes := parseValue(input)
	if canonicalizeCalcNodes(nodes) {
		return valueCSS(nodes)
	}
	return input
}

func canonicalizeCalcNodes(nodes []*valueNode) bool {
	changed := false
	for _, n := range nodes {
		if canonicalizeCalcNodes(n.nodes) {
			changed = true
		}
		if !binaryCalculation(n) || n.nodes[2].value != "*" && n.nodes[2].value != "+" {
			continue
		}
		if swapCalcOperands(n.nodes[0], n.nodes[4]) {
			n.nodes[0], n.nodes[4] = n.nodes[4], n.nodes[0]
			changed = true
		}
	}
	return changed
}

type cssDimension struct {
	number float64
	unit   string
	valid  bool
}

func nodeDimension(n *valueNode) cssDimension {
	if n.kind != 'w' {
		return cssDimension{}
	}
	number, unit, ok := dimension(n.value)
	return cssDimension{number, unit, ok}
}

func swapCalcOperands(a, b *valueNode) bool {
	lhs, rhs := nodeDimension(a), nodeDimension(b)
	if lhs.valid != rhs.valid {
		return lhs.valid
	}
	if lhs.valid {
		if (lhs.unit == "") != (rhs.unit == "") {
			return lhs.unit == ""
		}
		if lhs.number != rhs.number {
			return lhs.number > rhs.number
		}
		if lhs.unit != rhs.unit {
			return localeCompare(lhs.unit, rhs.unit) > 0
		}
	}
	return localeCompare(valueCSS([]*valueNode{a}), valueCSS([]*valueNode{b})) > 0
}

func binaryCalculation(n *valueNode) bool {
	return n.kind == 'f' && (n.value == "calc" || n.value == "") && len(n.nodes) == 5 && n.nodes[2].kind == 'w'
}

func constantFoldDeclaration(input string, rem *float64, normalize bool) string {
	nodes := parseValue(input)
	if constantFoldNodes(nodes, false, rem, normalize) {
		return valueCSS(nodes)
	}
	return input
}

func constantFoldNodes(nodes []*valueNode, parentFunction bool, rem *float64, normalize bool) bool {
	changed := false
	for i, n := range nodes {
		if constantFoldNodes(n.nodes, n.kind == 'f', rem, normalize) {
			changed = true
		}
		if n.kind == 'w' && n.value != "0" {
			canonical, ok := canonicalizeDimension(n.value, rem, normalize)
			if !ok || canonical == n.value {
				continue
			}
			if canonical == "0" && parentFunction {
				canonical, _ = canonicalizeDimension(n.value, rem, false)
			}
			n.value, changed = canonical, true
		} else if binaryCalculation(n) {
			if replacement := foldCalculation(n, parentFunction); replacement != nil {
				nodes[i], changed = replacement, true
			}
		}
	}
	return changed
}

func foldCalculation(n *valueNode, parentFunction bool) *valueNode {
	lhsNode, rhsNode, op := n.nodes[0], n.nodes[4], n.nodes[2].value
	lhs, rhs := nodeDimension(lhsNode), nodeDimension(rhsNode)
	word := func(s string) *valueNode { return &valueNode{kind: 'w', value: s} }
	if op == "*" {
		if lhs.valid && lhs.number == 0 && lhs.unit == "" || rhs.valid && rhs.number == 0 && rhs.unit == "" {
			return word("0")
		}
		if lhs.valid && rhs.valid && (lhs.number == 0 && rhs.unit == "" || rhs.number == 0 && lhs.unit == "") {
			unit := lhs.unit
			if unit == "" {
				unit = rhs.unit
			}
			if parentFunction {
				return word("0" + unit)
			}
			return word("0")
		}
		if lhs.valid && lhs.number == 1 && lhs.unit == "" {
			return rhsNode
		}
		if rhs.valid && rhs.number == 1 && rhs.unit == "" {
			return lhsNode
		}
	}
	if (op == "*" || op == "+") && lhs.valid != rhs.valid {
		constant, nested := lhs, rhsNode
		if !lhs.valid {
			constant, nested = rhs, lhsNode
		}
		if binaryCalculation(nested) && nested.nodes[2].value == op {
			a, b := nodeDimension(nested.nodes[0]), nodeDimension(nested.nodes[4])
			if a.valid != b.valid {
				known, unknown := a, nested.nodes[4]
				if !a.valid {
					known, unknown = b, nested.nodes[0]
				}
				if op == "*" && constant.unit != "" && known.unit != "" || op == "+" && constant.unit != known.unit {
					return nil
				}
				value := constant.number * known.number
				if op == "+" {
					value = constant.number + known.number
				}
				unit := constant.unit
				if unit == "" {
					unit = known.unit
				}
				combined := jsFloat(value) + unit
				if op == "*" && combined == "1" {
					return unknown
				}
				n.nodes[0], n.nodes[4] = word(combined), unknown
				return n
			}
		}
	}
	if !lhs.valid || !rhs.valid {
		return nil
	}
	switch op {
	case "*":
		if lhs.unit == rhs.unit || lhs.unit == "" || rhs.unit == "" {
			unit := lhs.unit
			if unit == "" {
				unit = rhs.unit
			}
			return word(jsFloat(lhs.number*rhs.number) + unit)
		}
	case "+":
		if lhs.unit == rhs.unit {
			return word(jsFloat(lhs.number+rhs.number) + lhs.unit)
		}
	case "-":
		if lhs.unit == rhs.unit {
			return word(jsFloat(lhs.number-rhs.number) + lhs.unit)
		}
	case "/":
		if rhs.number != 0 && rhs.unit == "" {
			computed := lhs.number / rhs.number
			if math.Floor(computed*100+0.5)/100 == computed {
				return word(jsFloat(computed) + lhs.unit)
			}
		}
	}
	return nil
}

func canonicalizeDimension(input string, rem *float64, normalize bool) (string, bool) {
	value, unit, ok := dimension(input)
	if !ok {
		return "", false
	}
	if unit == "" {
		return jsFloat(value), true
	}
	if value == 0 && isLength(input) {
		if normalize {
			return "0", true
		}
		return "0" + unit, true
	}
	if !normalize {
		return input, true
	}
	switch strings.ToLower(unit) {
	case "in":
		value, unit = value*96, "px"
	case "cm":
		value, unit = value*96/2.54, "px"
	case "mm":
		value, unit = value*96/2.54/10, "px"
	case "q":
		value, unit = value*96/2.54/10/4, "px"
	case "pc":
		value, unit = value*96/6, "px"
	case "pt":
		value, unit = value*96/72, "px"
	case "rem":
		if rem == nil {
			return "", false
		}
		value, unit = value**rem, "px"
	case "grad":
		value, unit = value*0.9, "deg"
	case "rad":
		value, unit = value*180/math.Pi, "deg"
	case "turn":
		value, unit = value*360, "deg"
	case "ms":
		value, unit = value/1000, "s"
	case "khz":
		value, unit = value*1000, "hz"
	}
	return jsFloat(value) + unit, true
}

func jsFloat(value float64) string {
	if value == 0 {
		return "0"
	}
	if math.IsInf(value, 1) {
		return "Infinity"
	}
	if math.IsInf(value, -1) {
		return "-Infinity"
	}
	if v := math.Abs(value); v < 1e-6 || v >= 1e21 {
		text := strconv.FormatFloat(value, 'e', -1, 64)
		mantissa, exponent, _ := strings.Cut(text, "e")
		return mantissa + "e" + exponent[:1] + strings.TrimLeft(exponent[1:], "0")
	}
	return strconv.FormatFloat(value, 'f', -1, 64)
}
