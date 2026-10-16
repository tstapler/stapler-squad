package changelog

import (
	"strconv"
	"strings"
)

// Newer reports whether version a is strictly newer than b, comparing the
// numeric major.minor.patch parts (leading "v" and any "-pre"/"+build" suffix
// ignored). It returns false when either side is not a numeric version, so a
// "dev" build is never nudged to upgrade.
func Newer(a, b string) bool {
	pa, okA := numericParts(a)
	pb, okB := numericParts(b)
	if !okA || !okB {
		return false
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

// IsNumericVersion reports whether v is a release-style version ("v1.2.3");
// "dev", "" and "dev-abc123" are not.
func IsNumericVersion(v string) bool {
	_, ok := numericParts(v)
	return ok
}

func numericParts(v string) (parts [3]int, ok bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	fields := strings.Split(v, ".")
	if v == "" || len(fields) > 3 {
		return parts, false
	}
	for i, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil || n < 0 {
			return parts, false
		}
		parts[i] = n
	}
	return parts, true
}
