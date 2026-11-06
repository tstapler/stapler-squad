package gitconfig

import (
	"strconv"
	"strings"
)

// ParseKey splits a dotted key the way git_config_parse_key does: section and name are
// lowercased, the subsection (everything between the first and last dot) is verbatim. ok is
// false for a key git rejects.
func ParseKey(key string) (section, sub string, hasSub bool, name string, ok bool) {
	first, last := strings.IndexByte(key, '.'), strings.LastIndexByte(key, '.')
	if first <= 0 || last == len(key)-1 {
		return "", "", false, "", false
	}
	section, name = strings.ToLower(key[:first]), strings.ToLower(key[last+1:])
	if first != last {
		sub, hasSub = key[first+1:last], true
	}
	for _, r := range section {
		if !isKeyChar(int(r)) {
			return "", "", false, "", false
		}
	}
	for i, r := range name {
		if !isKeyChar(int(r)) || (i == 0 && !isAlpha(int(r))) {
			return "", "", false, "", false
		}
	}
	return section, sub, hasSub, name, true
}

// ParseBool is git_parse_maybe_bool: a bare key is true, the empty string is false,
// true/yes/on and false/no/off are case-insensitive, and anything else must be an integer
// (non-zero is true). ok is false for a value git rejects with "bad boolean config value".
func ParseBool(value string, hasValue bool) (b, ok bool) {
	if !hasValue {
		return true, true
	}
	switch strings.ToLower(value) {
	case "":
		return false, true
	case "true", "yes", "on":
		return true, true
	case "false", "no", "off":
		return false, true
	}
	n, ok := parseInt(value)
	return n != 0, ok
}

// parseInt is git_parse_int: an integer in any base strconv understands with an optional
// k, m or g suffix.
func parseInt(s string) (int64, bool) {
	mult := int64(1)
	if n := len(s); n > 0 {
		switch s[n-1] {
		case 'k', 'K':
			mult, s = 1<<10, s[:n-1]
		case 'm', 'M':
			mult, s = 1<<20, s[:n-1]
		case 'g', 'G':
			mult, s = 1<<30, s[:n-1]
		}
	}
	v, err := strconv.ParseInt(s, 0, 64)
	if err != nil {
		return 0, false
	}
	return v * mult, true
}

// envBool reads a boolean environment variable like git_env_bool. ok is false for a value git
// would die on.
func envBool(environ []string, key string) (value, ok bool) {
	prefix := key + "="
	for i := len(environ) - 1; i >= 0; i-- {
		if strings.HasPrefix(environ[i], prefix) {
			return ParseBool(environ[i][len(prefix):], true)
		}
	}
	return false, true
}

// Lookup returns the last entry for key (git's "last one wins"), and whether there is one.
func (r *Result) Lookup(key string) (Source, bool) {
	all := r.All(key)
	if len(all) == 0 {
		return Source{}, false
	}
	return all[len(all)-1], true
}

// All returns every entry for key in file order.
func (r *Result) All(key string) []Source {
	section, sub, hasSub, name, ok := ParseKey(key)
	if !ok {
		return nil
	}
	var out []Source
	for _, e := range r.Entries {
		if e.Section == section && e.HasSubsection == hasSub && e.Subsection == sub && e.Key == name {
			out = append(out, e)
		}
	}
	return out
}

// Bool evaluates key as a boolean. set is false when the key is absent; ok is false when the
// last value is not a boolean git accepts.
func (r *Result) Bool(key string) (value, set, ok bool) {
	e, found := r.Lookup(key)
	if !found {
		return false, false, true
	}
	value, ok = ParseBool(e.Value, e.HasValue)
	return value, true, ok
}

// InSection returns every entry whose section is name (case-insensitive), any subsection.
func (r *Result) InSection(name string) []Source {
	name = strings.ToLower(name)
	var out []Source
	for _, e := range r.Entries {
		if e.Section == name {
			out = append(out, e)
		}
	}
	return out
}
