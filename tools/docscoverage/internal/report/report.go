// Package report renders the per-feature coverage report.
package report

import (
	"fmt"
	"io"
	"strings"

	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/check"
	"github.com/tstapler/stapler-squad/tools/docscoverage/internal/model"
)

// Options tune the report.
type Options struct {
	Enforce bool
	// Notices are run-level remarks, such as an unresolved base.
	Notices []string
	// Trend, when set, adds the one-line trend to the summary.
	Trend *Trend
}

// Write prints violations, ungrandfathered gaps and the counts. Covered,
// exempt and grandfathered counts always appear side by side, never as one
// percentage; grandfathered features are counted, not listed, to keep the
// summary readable.
func Write(w io.Writer, res check.Result, opt Options) {
	mode := "report-only"
	if opt.Enforce {
		mode = "enforcing"
	}
	fmt.Fprintf(w, "## Docs coverage (%s)\n\n", mode)
	c := res.Counts
	fmt.Fprintf(w, "In scope: %d | covered: %d | exempt: %d | grandfathered: %d | gap: %d\n\n",
		c.InScope, c.Covered, c.Exempt, c.Grandfathered, c.Gap)
	if opt.Trend != nil {
		fmt.Fprintf(w, "%s\n\n", opt.Trend.Line())
	}
	for _, n := range opt.Notices {
		fmt.Fprintf(w, "NOTICE: %s\n", n)
	}
	section(w, "Violations", res.Violations)
	section(w, "Missing docs (new or changed features)", filterEntries(res, func(e check.Entry) bool {
		return e.Status == model.StatusGap && e.Change != ""
	}, func(e check.Entry) string {
		return fmt.Sprintf("%s: missing %s", e.ID, strings.Join(e.Missing, ", "))
	}))
	section(w, "Changed, still grandfathered", filterEntries(res, func(e check.Entry) bool {
		return len(e.Notes) > 0
	}, func(e check.Entry) string { return string(e.ID) }))
	section(w, "Gaps outside the change (not grandfathered)", filterEntries(res, func(e check.Entry) bool {
		return e.Status == model.StatusGap && e.Change == ""
	}, func(e check.Entry) string {
		return fmt.Sprintf("%s: missing %s", e.ID, strings.Join(e.Missing, ", "))
	}))
	section(w, "Warnings", res.Warnings)
}

func filterEntries(res check.Result, keep func(check.Entry) bool, line func(check.Entry) string) []string {
	var out []string
	for _, e := range res.Entries {
		if keep(e) {
			out = append(out, line(e))
		}
	}
	return out
}

func section(w io.Writer, title string, lines []string) {
	if len(lines) == 0 {
		return
	}
	fmt.Fprintf(w, "### %s (%d)\n\n", title, len(lines))
	for _, l := range lines {
		fmt.Fprintf(w, "- %s\n", l)
	}
	fmt.Fprintln(w)
}
