package termscan

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Pattern is one leak pattern: text that must never reach published media.
type Pattern struct {
	// Name says what the pattern guards against.
	Name string
	re   *regexp.Regexp
}

// Find returns the first match in text, or "" if none.
func (p Pattern) Find(text string) string { return p.re.FindString(text) }

// LeakPatterns returns freshly compiled leak patterns (R14): developer and CI
// paths, hostnames, tokens and email addresses. It is a function, not a package
// variable, because gochecknoglobals covers this package; callers that loop
// should call it once.
func LeakPatterns() []Pattern {
	def := []struct{ name, expr string }{
		{"linux home directory", `/home/`},
		{"macOS home directory", `/Users/`},
		{"mktemp directory", `/tmp/tmp\.`},
		{"GitHub token", `ghp_[A-Za-z0-9]`},
		{"internal hostname", `\.internal\b`},
		{"CI runner user", `\brunneradmin\b`},
		{"email address", `[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`},
	}
	out := make([]Pattern, len(def))
	for i, d := range def {
		out[i] = Pattern{Name: d.name, re: regexp.MustCompile(d.expr)}
	}
	return out
}

// LeakFinding is one leak pattern match.
type LeakFinding struct {
	Path    string
	Line    int
	Pattern string
	Match   string
}

func (f LeakFinding) String() string {
	return fmt.Sprintf("%s:%d: leak (%s): %q", f.Path, f.Line, f.Pattern, f.Match)
}

// ScanLeaks checks every readable text file of src against LeakPatterns.
// Lines containing an entry of ignore (for example a documented fake path
// prefix) have that entry removed before matching.
func ScanLeaks(src Source, ignore []string) ([]LeakFinding, error) {
	files, err := src.Files()
	if err != nil {
		return nil, err
	}
	pats := LeakPatterns()
	var out []LeakFinding
	for _, p := range files {
		data, err := src.Read(p)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", p, err)
		}
		if data == nil || isBinary(data) {
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, ig := range ignore {
				line = strings.ReplaceAll(line, ig, "")
			}
			for _, pat := range pats {
				if m := pat.Find(line); m != "" {
					out = append(out, LeakFinding{Path: p, Line: i + 1, Pattern: pat.Name, Match: m})
				}
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	return out, nil
}
