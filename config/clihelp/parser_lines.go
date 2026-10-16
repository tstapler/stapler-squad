package clihelp

import "strings"

const (
	maxNameLen  = 100
	noBracket   = "--[no-]"
	noPrefix    = "--no-"
	specGapSpan = "  "
)

// parseFlagLine parses one trimmed candidate line such as
// "-a, --assignee string   Filter by assignee". Any doubt yields ok=false.
func parseFlagLine(line string) (Flag, bool) {
	spec, desc := splitSpec(line)
	var names []string
	takesValue := false
	for _, form := range strings.Split(spec, ", ") {
		formNames, hint, ok := parseForm(form)
		if !ok {
			return Flag{}, false
		}
		names = append(names, formNames...)
		takesValue = takesValue || hint
	}
	f, ok := assemble(names)
	if !ok {
		return Flag{}, false
	}
	f.TakesValue = takesValue
	f.Description = desc
	return f, true
}

// splitSpec cuts the flag spec from its same-line description at the first
// run of two spaces or a tab.
func splitSpec(line string) (spec, desc string) {
	cut := len(line)
	if i := strings.Index(line, specGapSpan); i >= 0 {
		cut = i
	}
	if i := strings.IndexByte(line, '\t'); i >= 0 && i < cut {
		cut = i
	}
	return strings.TrimSpace(line[:cut]), strings.TrimSpace(line[cut:])
}

// parseForm parses one comma-separated form: a flag token plus at most one
// value hint ("-e PATTERN", "--regexp=PATTERN", "--color[=WHEN]", "--[no-]x").
func parseForm(form string) (names []string, hasHint, ok bool) {
	form = strings.TrimSuffix(strings.TrimSpace(form), "...")
	fields := strings.Fields(form)
	if len(fields) == 0 || len(fields) > 2 {
		return nil, false, false
	}
	head, hint := splitNameHint(fields[0])
	if len(fields) == 2 {
		if hint != "" || !plausibleHint(fields[1]) {
			return nil, false, false
		}
		hint = fields[1]
	}
	names, ok = expandNames(head)
	return names, hint != "", ok
}

// splitNameHint separates "--regexp=PATTERN" or "--color[=WHEN]" into name
// and hint, leaving the "--[no-]" prefix intact.
func splitNameHint(tok string) (name, hint string) {
	start := 0
	if strings.HasPrefix(tok, noBracket) {
		start = len(noBracket)
	}
	i := strings.IndexAny(tok[start:], "=[")
	if i < 0 {
		return tok, ""
	}
	return tok[:start+i], strings.TrimLeft(tok[start+i:], "=")
}

// plausibleHint rejects tokens that look like the end of a prose sentence.
func plausibleHint(tok string) bool {
	if strings.HasSuffix(tok, "...") {
		return true
	}
	return !strings.ContainsAny(tok[len(tok)-1:], ".,;:")
}

// expandNames validates a flag token and expands "--[no-]x" into "--x" and
// its negation.
func expandNames(tok string) ([]string, bool) {
	if rest, found := strings.CutPrefix(tok, noBracket); found {
		if !validFlagName("--" + rest) {
			return nil, false
		}
		return []string{"--" + rest, noPrefix + rest}, true
	}
	if !validFlagName(tok) {
		return nil, false
	}
	return []string{tok}, true
}

// validFlagName matches ^--?[A-Za-z0-9][A-Za-z0-9_-]*$ with a length bound.
func validFlagName(s string) bool {
	if len(s) > maxNameLen || len(s) < 2 || s[0] != '-' {
		return false
	}
	body := strings.TrimPrefix(s[1:], "-")
	if body == "" || !isAlnum(body[0]) {
		return false
	}
	for i := 1; i < len(body); i++ {
		if c := body[i]; !isAlnum(c) && c != '_' && c != '-' {
			return false
		}
	}
	return true
}

func isAlnum(c byte) bool {
	return c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// assemble picks Name (first long form, else first single-dash form), Short
// (single-letter form when a long exists) and Aliases (remaining long forms).
func assemble(names []string) (Flag, bool) {
	var longs []string
	short := ""
	for _, n := range names {
		if isShortForm(n) {
			if short == "" {
				short = n
			}
			continue
		}
		longs = append(longs, n)
	}
	switch {
	case len(longs) > 0:
		f := Flag{Name: longs[0], Short: short}
		if len(longs) > 1 {
			f.Aliases = longs[1:]
		}
		return f, true
	case short != "":
		return Flag{Name: short}, true
	default:
		return Flag{}, false
	}
}

// isShortForm reports whether n is "-x" (single dash, single character).
func isShortForm(n string) bool {
	return len(n) == 2 && n[0] == '-'
}
