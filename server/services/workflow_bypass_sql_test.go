package services

import (
	"regexp"
	"strconv"
	"strings"
)

// Best-effort heuristic: a small tokenizer plus regexes over one SQL string
// literal. It is not a SQL parser and guarantees nothing about dialect forms it
// does not enumerate.

var (
	clsSQLUpdate    = regexp.MustCompile(`(?s)\bupdate\s+(?:or\s+\w+\s+)?(?:\w+\.)?backlog_items\b(?:\s+(?:as\s+)?\w+)?(?:\s+not\s+indexed|\s+indexed\s+by\s+\w+)?\s+set\s+(.*)$`)
	clsSQLInsert    = regexp.MustCompile(`(?s)\b(?:insert\s+(?:or\s+\w+\s+)?into|replace\s+into)\s+(?:\w+\.)?backlog_items\b\s*(\(([^)]*)\))?(.*)$`)
	clsSQLSetEnd    = regexp.MustCompile(`\b(where|returning|from)\b`)
	clsSQLUpsertSet = regexp.MustCompile(`(?s)\bdo\s+update\s+set\s+(.*)$`)
	clsSQLStatusCol = regexp.MustCompile(`\bstatus\b`)
	clsSQLStatusSet = regexp.MustCompile(`^\s*(?:\w+\.)?status\s*=`)
	clsSQLRowValue  = regexp.MustCompile(`\x00(\d+)\x00\s*=`)
)

// clsNormalizeSQL lowercases s, blanks '...' string literals, drops comments,
// and unquotes "ident" and `ident` so only structure and identifiers remain.
func clsNormalizeSQL(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\'':
			i = clsSkipSQLString(s, i)
			b.WriteString("''")
		case strings.HasPrefix(s[i:], "/*"):
			end := strings.Index(s[i+2:], "*/")
			if end < 0 {
				return b.String()
			}
			i += end + 3
			b.WriteByte(' ')
		case strings.HasPrefix(s[i:], "--"):
			end := strings.IndexByte(s[i:], '\n')
			if end < 0 {
				return b.String()
			}
			i += end
			b.WriteByte(' ')
		case s[i] == '"' || s[i] == '`' || s[i] == '[' || s[i] == ']':
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// clsSkipSQLString returns the index of the closing quote of the '...' literal
// opening at s[i] (two quotes in a row are an escaped quote), or len(s) if unterminated.
func clsSkipSQLString(s string, i int) int {
	for i++; i < len(s); i++ {
		if s[i] != '\'' {
			continue
		}
		if i+1 < len(s) && s[i+1] == '\'' {
			i++
			continue
		}
		return i
	}
	return i
}

// clsSQLTopLevel returns s with every parenthesised group replaced by a
// \x00<n>\x00 placeholder (contents in groups) so keywords inside subqueries
// cannot end or pollute the clause.
func clsSQLTopLevel(s string) (top string, groups []string) {
	var b strings.Builder
	depth, start := 0, 0
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '(':
			if depth == 0 {
				start = i + 1
			}
			depth++
		case s[i] == ')' && depth > 0:
			depth--
			if depth == 0 {
				b.WriteString("\x00" + strconv.Itoa(len(groups)) + "\x00")
				groups = append(groups, s[start:i])
			}
		case depth == 0:
			b.WriteByte(s[i])
		}
	}
	return b.String(), groups
}

// clsSQLSetListWritesStatus analyses the text after SET (or DO UPDATE SET): the
// list ends at the first top-level where/returning/from; status counts as
// `status =` or as a column of a `(cols) =` row-value target.
func clsSQLSetListWritesStatus(rest string) bool {
	top, groups := clsSQLTopLevel(rest)
	if loc := clsSQLSetEnd.FindStringIndex(top); loc != nil {
		top = top[:loc[0]]
	}
	for _, assignment := range strings.Split(top, ",") {
		if clsSQLStatusSet.MatchString(assignment) {
			return true
		}
	}
	for _, m := range clsSQLRowValue.FindAllStringSubmatch(top, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil && clsSQLStatusCol.MatchString(groups[n]) {
			return true
		}
	}
	return false
}

// clsSQLWritesBacklogStatus reports whether sql appears to write
// backlog_items.status: in an UPDATE SET list, an INSERT/REPLACE column list
// (or any INSERT/REPLACE with no column list, which writes every column), or an
// upsert DO UPDATE SET list.
func clsSQLWritesBacklogStatus(sql string) bool {
	for _, stmt := range strings.Split(clsNormalizeSQL(sql), ";") {
		if m := clsSQLUpdate.FindStringSubmatch(stmt); m != nil && clsSQLSetListWritesStatus(m[1]) {
			return true
		}
		if m := clsSQLInsert.FindStringSubmatch(stmt); m != nil {
			if m[1] == "" || clsSQLStatusCol.MatchString(m[2]) {
				return true
			}
			if u := clsSQLUpsertSet.FindStringSubmatch(m[3]); u != nil && clsSQLSetListWritesStatus(u[1]) {
				return true
			}
		}
	}
	return false
}
