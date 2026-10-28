package dbobs

import "strings"

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
	return truncate(strings.Join(strings.Fields(b.String()), " "))
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

func skipNumber(q string, i int) int {
	for i < len(q) && (isDigit(q[i]) || q[i] == '.') {
		i++
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

func isIdentByte(c byte) bool {
	return c == '_' || c == '`' || c == '"' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || isDigit(c)
}

func truncate(s string) string {
	if len(s) <= maxStatementLen {
		return s
	}
	return s[:maxStatementLen] + "..."
}

// operation is the leading SQL verb, used as a low-cardinality span attribute.
func operation(q string) string {
	f := strings.Fields(q)
	if len(f) == 0 {
		return "unknown"
	}
	return strings.ToUpper(f[0])
}
