package main

import (
	"fmt"
	"regexp"
	"strings"
)

// scannerTests names the go_test rule in each scanner package whose data must list every leaf.
var scannerTests = map[string]string{"session": "session_test", "server/services": "services_test"}

var strRe = regexp.MustCompile(`"([^"]*)"`)

// visibleTo returns "" when some visibility entry grants pkg access, else why not.
func visibleTo(vis [][]string, pkg string) string {
	unsupported := ""
	for _, m := range vis {
		v := m[1]
		switch {
		case v == "//visibility:public":
			return ""
		case strings.HasPrefix(v, "//") && strings.HasSuffix(v, ":__pkg__"):
			if strings.TrimSuffix(strings.TrimPrefix(v, "//"), ":__pkg__") == pkg {
				return ""
			}
		case strings.HasPrefix(v, "//") && strings.HasSuffix(v, ":__subpackages__"):
			p := strings.TrimSuffix(strings.TrimPrefix(v, "//"), ":__subpackages__")
			if p == "" || p == pkg || strings.HasPrefix(pkg, p+"/") {
				return ""
			}
		case v == "//visibility:private":
		default:
			unsupported = v
		}
	}
	if unsupported != "" {
		return fmt.Sprintf("has unsupported visibility entry %q (use //pkg:__pkg__ or //visibility:public); not verified visible to", unsupported)
	}
	return "not visible to"
}

// stripComments drops # comments outside string literals.
func stripComments(src string) string {
	var b strings.Builder
	inStr, inComment := false, false
	for i := 0; i < len(src); i++ {
		c := src[i]
		switch {
		case inComment:
			if c == '\n' {
				inComment = false
				b.WriteByte(c)
			}
		case inStr:
			b.WriteByte(c)
			if c == '\\' && i+1 < len(src) {
				i++
				b.WriteByte(src[i])
			} else if c == '"' {
				inStr = false
			}
		case c == '"':
			inStr = true
			b.WriteByte(c)
		case c == '#':
			inComment = true
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// findCall returns the argument text of the top-level rule kind(name = "name", ...).
func findCall(src, kind, name string) (string, bool) {
	re := regexp.MustCompile(`(?m)^` + kind + `\(`)
	for _, loc := range re.FindAllStringIndex(src, -1) {
		body, ok := balanced(src, loc[1]-1)
		if ok && strings.TrimSpace(attr(body, "name")) == `"`+name+`"` {
			return body, true
		}
	}
	return "", false
}

// scan walks src from i, tracking string and bracket depth, and calls fn on every
// character outside strings with the current depth; fn returns true to stop.
func scan(src string, from int, fn func(i, depth int) bool) {
	depth, inStr := 0, false
	for i := from; i < len(src); i++ {
		c := src[i]
		switch {
		case inStr:
			if c == '\\' {
				i++
			} else if c == '"' {
				inStr = false
			}
			continue
		case c == '"':
			inStr = true
			continue
		case c == '(' || c == '[' || c == '{':
			depth++
		case c == ')' || c == ']' || c == '}':
			depth--
		}
		if fn(i, depth) {
			return
		}
	}
}

// balanced returns the text inside the bracket opening at src[open].
func balanced(src string, open int) (string, bool) {
	end := -1
	scan(src, open, func(i, depth int) bool {
		if depth == 0 {
			end = i
			return true
		}
		return false
	})
	if end < 0 {
		return "", false
	}
	return src[open+1 : end], true
}

// attr returns the value expression of a top-level `key = value,` argument.
func attr(body, key string) string {
	start := 0
	val := ""
	check := func(end int) bool {
		if k, v, found := strings.Cut(strings.TrimSpace(body[start:end]), "="); found && strings.TrimSpace(k) == key {
			val = v
			return true
		}
		start = end + 1
		return false
	}
	scan(body, 0, func(i, depth int) bool {
		return depth == 0 && body[i] == ',' && check(i)
	})
	if val == "" && start <= len(body) {
		check(len(body))
	}
	return val
}

var unsafeDataRe = regexp.MustCompile(`\b(select|exclude|for)\b`)

// unsafeData returns the construct that makes a data expression's string literals unreliable, or "".
func unsafeData(expr string) string {
	return unsafeDataRe.FindString(strRe.ReplaceAllString(expr, `""`))
}

// globsGoFiles reports whether srcs is exactly glob(["*.go"]).
func globsGoFiles(srcs string) bool {
	return strings.Join(strings.Fields(srcs), "") == `glob(["*.go"])`
}

func stringSet(expr string) map[string]bool {
	out := map[string]bool{}
	for _, m := range strRe.FindAllStringSubmatch(expr, -1) {
		out[m[1]] = true
	}
	return out
}
