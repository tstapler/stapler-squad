// Package changelog parses the release-please-generated CHANGELOG.md that is
// embedded in the binary, so the web UI can show release notes offline.
package changelog

import (
	"regexp"
	"strconv"
	"strings"
)

// Entry is one bullet under a section heading.
type Entry struct {
	Scope   string   `json:"scope,omitempty"`
	Text    string   `json:"text"`
	PRs     []int    `json:"prs,omitempty"`
	Commits []string `json:"commits,omitempty"`
}

// Section groups entries under a release-please heading.
type Section struct {
	Title   string  `json:"title"` // e.g. "Features", "Bug Fixes"
	Entries []Entry `json:"entries"`
}

// Release is one "## [x.y.z](compare-url) (date)" block.
type Release struct {
	Version    string    `json:"version"`
	Date       string    `json:"date,omitempty"` // YYYY-MM-DD as written
	CompareURL string    `json:"compare_url,omitempty"`
	Sections   []Section `json:"sections"`
}

var (
	releaseRe = regexp.MustCompile(`^##\s+\[?v?([0-9][^\]\s)]*)\]?(?:\(([^)]*)\))?(?:\s+\((\d{4}-\d{2}-\d{2})\))?`)
	sectionRe = regexp.MustCompile(`^###\s+(.+?)\s*$`)
	bulletRe  = regexp.MustCompile(`^[*-]\s+(.*)$`)
	scopeRe   = regexp.MustCompile(`^\*\*([^*]+?):\*\*\s*`)
	prRe      = regexp.MustCompile(`\s*\(\[#(\d+)\]\([^)]*\)\)`)
	commitRe  = regexp.MustCompile(`\s*\(\[([0-9a-f]{7,40})\]\([^)]*\)\)`)
	linkRe    = regexp.MustCompile(`\[([^\]]+)\]\([^)]*\)`)
)

// Parse reads release-please markdown. It never fails: lines that don't
// match the expected shape are skipped, and an unparseable document yields
// zero releases.
func Parse(content []byte) []Release {
	var p parser
	for _, line := range strings.Split(string(content), "\n") {
		p.line(strings.TrimRight(line, "\r"))
	}
	p.flushRelease()
	return p.releases
}

type parser struct {
	releases  []Release
	cur       *Release
	sec       *Section
	lastEntry *Entry
	lastRaw   string
}

func (p *parser) flushSection() {
	if p.cur != nil && p.sec != nil && len(p.sec.Entries) > 0 {
		p.cur.Sections = append(p.cur.Sections, *p.sec)
	}
	p.sec, p.lastEntry = nil, nil
}

func (p *parser) flushRelease() {
	p.flushSection()
	if p.cur != nil {
		p.releases = append(p.releases, *p.cur)
	}
	p.cur = nil
}

func (p *parser) line(line string) {
	if m := releaseRe.FindStringSubmatch(line); m != nil && !strings.HasPrefix(line, "###") {
		p.flushRelease()
		p.cur = &Release{Version: m[1], CompareURL: m[2], Date: m[3]}
		return
	}
	if p.cur == nil {
		return
	}
	if m := sectionRe.FindStringSubmatch(line); m != nil {
		p.flushSection()
		p.sec = &Section{Title: m[1]}
		return
	}
	if p.sec == nil {
		return
	}
	if m := bulletRe.FindStringSubmatch(line); m != nil {
		p.lastRaw = m[1]
		p.sec.Entries = append(p.sec.Entries, parseEntry(p.lastRaw))
		p.lastEntry = &p.sec.Entries[len(p.sec.Entries)-1]
		return
	}
	// Indented continuation of the previous bullet.
	if p.lastEntry != nil && strings.HasPrefix(line, "  ") && strings.TrimSpace(line) != "" {
		p.lastRaw += " " + strings.TrimSpace(line)
		*p.lastEntry = parseEntry(p.lastRaw)
	}
}

func parseEntry(raw string) Entry {
	var e Entry
	if m := scopeRe.FindStringSubmatch(raw); m != nil {
		e.Scope = m[1]
		raw = raw[len(m[0]):]
	}
	for _, m := range prRe.FindAllStringSubmatch(raw, -1) {
		if n, err := strconv.Atoi(m[1]); err == nil {
			e.PRs = append(e.PRs, n)
		}
	}
	raw = prRe.ReplaceAllString(raw, "")
	for _, m := range commitRe.FindAllStringSubmatch(raw, -1) {
		e.Commits = append(e.Commits, m[1])
	}
	raw = commitRe.ReplaceAllString(raw, "")
	e.Text = strings.TrimSpace(linkRe.ReplaceAllString(raw, "$1"))
	return e
}

// FindVersion returns the release matching v (a leading "v" is ignored).
func FindVersion(releases []Release, v string) (Release, bool) {
	v = strings.TrimPrefix(v, "v")
	for _, r := range releases {
		if r.Version == v {
			return r, true
		}
	}
	return Release{}, false
}
