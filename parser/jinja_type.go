package parser

import "strings"

type jinjaTypeResolver func(string) (string, bool)

// inferJinjaExpressionType recognizes types that are provable from a pure
// Jinja expression without evaluating variables or filters.
func inferJinjaExpressionType(value string, filterTypes map[string]string, resolver jinjaTypeResolver) (string, bool) {
	expr, ok := unwrapPureJinjaExpression(value)
	if !ok {
		return "", false
	}

	return inferJinjaExpression(expr, filterTypes, resolver)
}

func unwrapPureJinjaExpression(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if len(trimmed) >= 2 && (trimmed[0] == '"' || trimmed[0] == '\'') && trimmed[len(trimmed)-1] == trimmed[0] {
		trimmed = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	}
	if len(trimmed) < 4 || !strings.HasPrefix(trimmed, "{{") || !strings.HasSuffix(trimmed, "}}") {
		return "", false
	}

	return strings.TrimSpace(trimmed[2 : len(trimmed)-2]), true
}

func inferJinjaExpression(expr string, filterTypes map[string]string, resolver jinjaTypeResolver) (string, bool) {
	expr = strings.TrimSpace(expr)
	for isWholeBalancedCollection(expr, '(', ')') {
		expr = strings.TrimSpace(expr[1 : len(expr)-1])
	}
	if trueExpr, falseExpr, ok := splitTopLevelConditional(expr); ok {
		trueType, trueOK := inferJinjaExpression(trueExpr, filterTypes, resolver)
		falseType, falseOK := inferJinjaExpression(falseExpr, filterTypes, resolver)
		if !trueOK || !falseOK || trueType != falseType {
			return "", false
		}
		return trueType, true
	}
	for _, keyword := range []string{"or", "and"} {
		if leftExpr, rightExpr, ok := splitTopLevelKeyword(expr, keyword); ok {
			leftType, leftOK := inferJinjaExpression(leftExpr, filterTypes, resolver)
			rightType, rightOK := inferJinjaExpression(rightExpr, filterTypes, resolver)
			if leftOK && rightOK && leftType == rightType {
				return leftType, true
			}
			return "", false
		}
	}
	if leftExpr, rightExpr, ok := splitTopLevelBinary(expr, '+'); ok {
		leftType, leftOK := inferJinjaExpression(leftExpr, filterTypes, resolver)
		rightType, rightOK := inferJinjaExpression(rightExpr, filterTypes, resolver)
		if leftOK && rightOK && leftType == rightType {
			return leftType, true
		}
		return "", false
	}
	if filterName, ok := terminalJinjaFilter(expr); ok {
		if typ, known := standardJinjaFilterTypes[filterName]; known {
			return typ, true
		}
		if typ, known := filterTypes[filterName]; known && typ != "" {
			return typ, true
		}
	}
	if isWholeBalancedCollection(expr, '{', '}') {
		return Dict, true
	}
	if isWholeBalancedCollection(expr, '[', ']') {
		return List, true
	}
	if resolver != nil {
		if typ, ok := resolver(expr); ok {
			return typ, true
		}
	}
	return "", false
}

func splitTopLevelKeyword(expr, keyword string) (string, string, bool) {
	stack := make([]byte, 0, 4)
	var quote byte
	escaped := false
	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}

		switch ch {
		case '\'', '"':
			quote = ch
		case '(', '[', '{':
			stack = append(stack, ch)
		case ')', ']', '}':
			if len(stack) == 0 || !matchingDelimiters(stack[len(stack)-1], ch) {
				return "", "", false
			}
			stack = stack[:len(stack)-1]
		default:
			if len(stack) == 0 && keywordAt(expr, i, keyword) {
				left := strings.TrimSpace(expr[:i])
				right := strings.TrimSpace(expr[i+len(keyword):])
				return left, right, left != "" && right != ""
			}
		}
	}
	return "", "", false
}

func splitTopLevelBinary(expr string, operator byte) (string, string, bool) {
	stack := make([]byte, 0, 4)
	var quote byte
	escaped := false
	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}

		switch ch {
		case '\'', '"':
			quote = ch
		case '(', '[', '{':
			stack = append(stack, ch)
		case ')', ']', '}':
			if len(stack) == 0 || !matchingDelimiters(stack[len(stack)-1], ch) {
				return "", "", false
			}
			stack = stack[:len(stack)-1]
		default:
			if ch == operator && len(stack) == 0 {
				left := strings.TrimSpace(expr[:i])
				right := strings.TrimSpace(expr[i+1:])
				return left, right, left != "" && right != ""
			}
		}
	}
	return "", "", false
}

var standardJinjaFilterTypes = map[string]string{
	"bool":       Bool,
	"combine":    Dict,
	"dict2items": List,
	"float":      "float",
	"int":        Int,
	"items2dict": Dict,
	"list":       List,
	"string":     String,
}

func terminalJinjaFilter(expr string) (string, bool) {
	stack := make([]byte, 0, 4)
	var quote byte
	escaped := false
	lastPipe := -1
	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}

		switch ch {
		case '\'', '"':
			quote = ch
		case '(', '[', '{':
			stack = append(stack, ch)
		case ')', ']', '}':
			if len(stack) == 0 || !matchingDelimiters(stack[len(stack)-1], ch) {
				return "", false
			}
			stack = stack[:len(stack)-1]
		case '|':
			if len(stack) == 0 {
				lastPipe = i
			}
		}
	}
	if quote != 0 || len(stack) != 0 || lastPipe < 0 {
		return "", false
	}

	filter := strings.TrimSpace(expr[lastPipe+1:])
	end := 0
	for end < len(filter) && (filter[end] == '_' || filter[end] >= 'a' && filter[end] <= 'z' || filter[end] >= 'A' && filter[end] <= 'Z' || end > 0 && filter[end] >= '0' && filter[end] <= '9') {
		end++
	}
	if end == 0 {
		return "", false
	}
	return filter[:end], true
}

func isWholeBalancedCollection(expr string, opening, closing byte) bool {
	expr = strings.TrimSpace(expr)
	if len(expr) < 2 || expr[0] != opening || expr[len(expr)-1] != closing {
		return false
	}

	stack := make([]byte, 0, 4)
	var quote byte
	escaped := false
	for i := range len(expr) {
		ch := expr[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}

		switch ch {
		case '\'', '"':
			quote = ch
		case '(', '[', '{':
			stack = append(stack, ch)
		case ')', ']', '}':
			if len(stack) == 0 || !matchingDelimiters(stack[len(stack)-1], ch) {
				return false
			}
			stack = stack[:len(stack)-1]
			if len(stack) == 0 && i != len(expr)-1 {
				return false
			}
		}
	}

	return quote == 0 && !escaped && len(stack) == 0
}

func splitTopLevelConditional(expr string) (string, string, bool) {
	stack := make([]byte, 0, 4)
	var quote byte
	escaped := false
	ifPos := -1

	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}

		switch ch {
		case '\'', '"':
			quote = ch
		case '(', '[', '{':
			stack = append(stack, ch)
		case ')', ']', '}':
			if len(stack) == 0 || !matchingDelimiters(stack[len(stack)-1], ch) {
				return "", "", false
			}
			stack = stack[:len(stack)-1]
		default:
			if len(stack) != 0 {
				continue
			}
			if ifPos < 0 && keywordAt(expr, i, "if") {
				ifPos = i
				i++
				continue
			}
			if ifPos >= 0 && keywordAt(expr, i, "else") {
				trueExpr := strings.TrimSpace(expr[:ifPos])
				falseExpr := strings.TrimSpace(expr[i+len("else"):])
				return trueExpr, falseExpr, trueExpr != "" && falseExpr != ""
			}
		}
	}

	return "", "", false
}

func keywordAt(expr string, index int, keyword string) bool {
	if index+len(keyword) > len(expr) || expr[index:index+len(keyword)] != keyword {
		return false
	}
	return index > 0 && isWhitespaceByte(expr[index-1]) &&
		index+len(keyword) < len(expr) && isWhitespaceByte(expr[index+len(keyword)])
}

func isWhitespaceByte(ch byte) bool {
	return ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r'
}

func matchingDelimiters(opening, closing byte) bool {
	return opening == '(' && closing == ')' ||
		opening == '[' && closing == ']' ||
		opening == '{' && closing == '}'
}
