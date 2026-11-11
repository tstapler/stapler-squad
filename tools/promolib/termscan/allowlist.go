package termscan

import (
	"fmt"
	"strings"
)

// AllowEntry permits banned terms in one file, with a reason.
type AllowEntry struct {
	Path   string
	Reason string
}

func (a AllowEntry) String() string { return a.Path + ":" + a.Reason }

// ParseAllowlist parses `path:reason` lines. Blank lines and # comments are
// skipped; a line without both parts is an error.
func ParseAllowlist(data []byte) ([]AllowEntry, error) {
	var out []AllowEntry
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		p, reason, ok := strings.Cut(line, ":")
		p, reason = strings.TrimSpace(p), strings.TrimSpace(reason)
		if !ok || p == "" || reason == "" {
			return nil, fmt.Errorf("allow-list line %d %q: want path:reason", i+1, line)
		}
		out = append(out, AllowEntry{Path: p, Reason: reason})
	}
	return out, nil
}
