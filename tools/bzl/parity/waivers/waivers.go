// Package waivers validates the `waivers:` list of owner-attestations.yaml (Task 7.5f).
//
// Only the two waivable P10 rows can be waived; every other row, the cold row and T1-T7 are rejected.
package waivers

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	// MaxDays is the longest allowed granted-to-expires span (owner default `waiver_expiry_days`).
	MaxDays = 90
	// MinReason is the shortest allowed reason (owner default `waiver_reason_min_chars`).
	MinReason  = 20
	dateLayout = "2006-01-02"
)

// Waivable lists the only rows a waiver may name.
var Waivable = []string{"P10.hit_ratio_floor", "P10.disk_ceiling"}

// Waiver is one entry of `waivers:`.
type Waiver struct {
	Row     string `yaml:"row"`
	Reason  string `yaml:"reason"`
	Granted string `yaml:"granted"`
	Expires string `yaml:"expires"`
}

// Parse reads the waivers list from an attestations document; other keys are ignored. Absent means none.
func Parse(r io.Reader) ([]Waiver, error) {
	var doc struct {
		Waivers []Waiver `yaml:"waivers"`
	}
	if err := yaml.NewDecoder(r).Decode(&doc); err != nil && err != io.EOF {
		return nil, err
	}
	return doc.Waivers, nil
}

// Problem is one rejected waiver.
type Problem struct {
	Row    string
	Reason string
}

// Validate returns the valid waivers (sorted by row) and the rejected ones.
func Validate(ws []Waiver, today time.Time) (ok []Waiver, bad []Problem) {
	today = today.Truncate(24 * time.Hour)
	for _, w := range ws {
		if msg := check(w, today); msg != "" {
			bad = append(bad, Problem{Row: w.Row, Reason: msg})
			continue
		}
		ok = append(ok, w)
	}
	sort.Slice(ok, func(i, j int) bool { return ok[i].Row < ok[j].Row })
	return ok, bad
}

func check(w Waiver, today time.Time) string {
	if !isWaivable(w.Row) {
		return fmt.Sprintf("row %q is not waivable (only %s)", w.Row, strings.Join(Waivable, ", "))
	}
	if len([]rune(strings.TrimSpace(w.Reason))) < MinReason {
		return fmt.Sprintf("reason must be at least %d characters", MinReason)
	}
	g, err := time.Parse(dateLayout, w.Granted)
	if err != nil {
		return "granted is not a YYYY-MM-DD date"
	}
	e, err := time.Parse(dateLayout, w.Expires)
	if err != nil {
		return "expires is not a YYYY-MM-DD date"
	}
	if e.Before(g) || e.After(g.AddDate(0, 0, MaxDays)) {
		return fmt.Sprintf("expires must be within %d days after granted", MaxDays)
	}
	if e.Before(today) {
		return "waiver expired on " + w.Expires
	}
	return ""
}

func isWaivable(row string) bool {
	for _, r := range Waivable {
		if r == row {
			return true
		}
	}
	return false
}

// Active returns the waiver for a row, if one is in force.
func Active(ok []Waiver, row string) (Waiver, bool) {
	for _, w := range ok {
		if w.Row == row {
			return w, true
		}
	}
	return Waiver{}, false
}

// Block renders the report block: one line per waiver, empty when there are none.
func Block(ok []Waiver) string {
	var b strings.Builder
	for _, w := range ok {
		fmt.Fprintf(&b, "WAIVER %s: %s until %s\n", w.Row, strings.TrimSpace(w.Reason), w.Expires)
	}
	return b.String()
}

// Inline renders the `PARITY: PASS WITH WAIVERS (...)` list.
func Inline(ok []Waiver) string {
	parts := make([]string, len(ok))
	for i, w := range ok {
		parts[i] = fmt.Sprintf("%s: %s until %s", w.Row, strings.TrimSpace(w.Reason), w.Expires)
	}
	return strings.Join(parts, "; ")
}
