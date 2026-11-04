package dbobs

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	maxStatementLen = 240
	sqlQuote        = '\''
)

// NormalizeStatement returns a log- and span-safe form of a SQL statement:
// quoted literals and bare numbers become "?", whitespace is collapsed and
// the result is truncated. ent binds every value as an argument so statements
// normally carry no literals already; this is defence in depth, and bind
// arguments are never passed here.
func NormalizeStatement(q string) string {
	var b strings.Builder
	b.Grow(len(q))
	for i := 0; i < len(q); {
		c := q[i]
		switch {
		case c == sqlQuote:
			i = skipQuoted(q, i)
			b.WriteByte('?')
		case isDigit(c) && !isIdentByte(prevByte(q, i)):
			i = skipNumber(q, i)
			b.WriteByte('?')
		default:
			b.WriteByte(c)
			i++
		}
	}
	return truncate(collapsePlaceholderLists(strings.Join(strings.Fields(b.String()), " ")))
}

var (
	placeholderList = regexp.MustCompile(`\?(?:, \?)+`)
	valuesRows      = regexp.MustCompile(`\(\?, \.\.\.\?\)(?:, \(\?, \.\.\.\?\))+`)
)

// collapsePlaceholderLists folds "?, ?, ?" to "?, ...?" and repeated identical
// multi-row VALUES tuples to one, so statements that differ only in how many
// values they bind aggregate under one key.
func collapsePlaceholderLists(s string) string {
	s = placeholderList.ReplaceAllString(s, "?, ...?")
	return valuesRows.ReplaceAllString(s, "(?, ...?), ...")
}

func skipQuoted(q string, i int) int {
	for i++; i < len(q); i++ {
		if q[i] != sqlQuote {
			continue
		}
		if i+1 < len(q) && q[i+1] == sqlQuote { // escaped quote
			i++
			continue
		}
		return i + 1
	}
	return len(q)
}

// skipNumber consumes a decimal, hex (0x..) or exponent (1e10, 1.5E-3) literal.
func skipNumber(q string, i int) int {
	if q[i] == '0' && i+1 < len(q) && (q[i+1] == 'x' || q[i+1] == 'X') {
		for i += 2; i < len(q) && isHexDigit(q[i]); i++ {
		}
		return i
	}
	for i < len(q) && (isDigit(q[i]) || q[i] == '.') {
		i++
	}
	if i < len(q) && (q[i] == 'e' || q[i] == 'E') {
		j := i + 1
		if j < len(q) && (q[j] == '+' || q[j] == '-') {
			j++
		}
		if j < len(q) && isDigit(q[j]) {
			for i = j; i < len(q) && isDigit(q[i]); i++ {
			}
		}
	}
	return i
}

func prevByte(q string, i int) byte {
	if i == 0 {
		return ' '
	}
	return q[i-1]
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '`' || c == '"' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || isDigit(c)
}

func truncate(s string) string {
	if len(s) <= maxStatementLen {
		return s
	}
	cut := maxStatementLen
	for cut > 0 && !utf8.RuneStart(s[cut]) { // never split a multibyte rune: span attributes must be valid UTF-8
		cut--
	}
	return s[:cut] + "..."
}

// operation is the leading SQL verb, used as a low-cardinality span attribute.
func operation(q string) string {
	f := strings.Fields(q)
	if len(f) == 0 {
		return "unknown"
	}
	return strings.ToUpper(f[0])
}
