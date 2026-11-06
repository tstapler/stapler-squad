package gitconfig

import "strings"

// Wildmatch results, as in git's wildmatch.c.
const (
	wmMatch = iota
	wmNoMatch
	wmAbortAll
	wmAbortToStarStar
)

// Match reports whether text matches pattern under git's wildmatch with WM_PATHNAME: `*` and
// `?` never match '/', `**` is special only as a whole path component (`/**/` matches zero or
// more directories, a trailing `/**` matches the rest), and anywhere else `**` acts as `*`.
// fold makes the match ASCII case-insensitive (gitdir/i:).
//
// Bracket expressions and backslash escapes are not ported: a pattern containing `[` or `\`
// returns ok=false and the caller fails closed instead of guessing.
func Match(pattern, text string, fold bool) (matched, ok bool) {
	if strings.ContainsAny(pattern, "[\\") {
		return false, false
	}
	if fold {
		pattern, text = asciiLower(pattern), asciiLower(text)
	}
	return dowild(pattern, 0, text, 0) == wmMatch, true
}

func asciiLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}

// at returns the byte at i, or 0 past the end (git's NUL terminator).
func at(s string, i int) byte {
	if i < len(s) {
		return s[i]
	}
	return 0
}

func isGlobSpecial(c byte) bool { return c == '*' || c == '?' || c == '[' || c == '\\' }

// dowild is git's dowild() restricted to the ported syntax, with WM_PATHNAME always on.
func dowild(p string, pi int, text string, ti int) int {
	for ; pi < len(p); pi, ti = pi+1, ti+1 {
		pc, tc := p[pi], at(text, ti)
		if ti >= len(text) && pc != '*' {
			return wmAbortAll
		}
		switch pc {
		case '?':
			if tc == '/' {
				return wmNoMatch
			}
		case '*':
			return star(p, pi, text, ti)
		default:
			if tc != pc {
				return wmNoMatch
			}
		}
	}
	if ti < len(text) {
		return wmNoMatch
	}
	return wmMatch
}

// star handles a run of '*' starting at p[pi].
func star(p string, pi int, text string, ti int) int {
	pi++
	matchSlash := false
	if at(p, pi) == '*' {
		prev := pi - 2
		for pi++; at(p, pi) == '*'; pi++ {
		}
		wholeComponent := (prev < 0 || p[prev] == '/') && (pi >= len(p) || p[pi] == '/')
		if wholeComponent {
			// Assume "foo/" already matched and "**/" matches nothing, so foo/**/bar matches foo/bar.
			if at(p, pi) == '/' && dowild(p, pi+1, text, ti) == wmMatch {
				return wmMatch
			}
			matchSlash = true
		}
	}
	if pi >= len(p) {
		// A trailing "**" matches everything; a trailing "*" only if no slash remains.
		if !matchSlash && strings.Contains(text[ti:], "/") {
			return wmNoMatch
		}
		return wmMatch
	}
	if !matchSlash && p[pi] == '/' {
		// One '*' followed by '/' matches within the next segment only.
		slash := strings.IndexByte(text[ti:], '/')
		if slash < 0 {
			return wmNoMatch
		}
		return dowild(p, pi+1, text, ti+slash+1)
	}
	return starScan(p, pi, text, ti, matchSlash)
}

// starScan tries the rest of the pattern at each text position the '*' could end at.
func starScan(p string, pi int, text string, ti int, matchSlash bool) int {
	for {
		tc := at(text, ti)
		if tc == 0 && ti >= len(text) {
			break
		}
		if !isGlobSpecial(p[pi]) {
			// Skip ahead to the next occurrence of the literal that follows the star.
			for ti < len(text) && (matchSlash || text[ti] != '/') && text[ti] != p[pi] {
				ti++
			}
			if at(text, ti) != p[pi] || ti >= len(text) {
				return wmNoMatch
			}
			tc = text[ti]
		}
		if matched := dowild(p, pi, text, ti); matched != wmNoMatch {
			if !matchSlash || matched != wmAbortToStarStar {
				return matched
			}
		} else if !matchSlash && tc == '/' {
			return wmAbortToStarStar
		}
		ti++
	}
	return wmAbortAll
}
