package session

import (
	"net/url"
	"strings"
)

// NormalizeClaimURL is the single canonical form of an external URL used as a
// claim key: lowercase scheme and host, http upgraded to https for github.com,
// no fragment, no trailing slash. Every claim write, lookup and peer query goes
// through it so cosmetic URL differences cannot dodge a claim. An unparseable
// value is returned trimmed but otherwise unchanged.
func NormalizeClaimURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	u, err := url.Parse(trimmed)
	if err != nil || u.Host == "" {
		return trimmed
	}
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	if u.Scheme == "http" && (u.Host == "github.com" || u.Host == "www.github.com") {
		u.Scheme = "https"
	}
	u.Fragment = ""
	u.RawFragment = ""
	u.Path = strings.TrimRight(u.Path, "/")
	u.RawPath = strings.TrimRight(u.RawPath, "/")
	return u.String()
}
