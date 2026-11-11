package manifest

import (
	"fmt"
	"strings"
)

// RenderSummary writes a Markdown job-summary table. prevDistBytes < 0 omits the trend.
func RenderSummary(m MediaManifest, distBytes, prevDistBytes int64) string {
	counts := map[Status]int{}
	for _, e := range m.Entries {
		counts[e.Status]++
	}
	var b strings.Builder
	fmt.Fprintf(&b, "### Promo media\n\nrecorded %d, failed %d, skipped %d\n\n", counts[StatusRecorded], counts[StatusFailed], counts[StatusSkipped])
	b.WriteString("| name | format | status | bytes | sha256 | form factor | attempts |\n|---|---|---|---:|---|---|---|\n")
	for _, e := range m.Entries {
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %s | %s | %s |\n", e.Name, e.Format, e.Status, e.Bytes, e.SHA256, e.FormFactor, attemptsCell(e))
	}
	if distBytes > 0 {
		fmt.Fprintf(&b, "\ndist size: %d bytes", distBytes)
		if prevDistBytes >= 0 {
			fmt.Fprintf(&b, " (previous run %d, %+d)", prevDistBytes, distBytes-prevDistBytes)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// attemptsCell shows the attempt count, marking a scene that needed a retry as
// flaky (Story 5.1.2's soak criterion reads these rows). Zero means not recorded.
func attemptsCell(e MediaEntry) string {
	switch {
	case e.Attempts > 1:
		return fmt.Sprintf("%d flaky", e.Attempts)
	case e.Attempts == 1:
		return "1"
	default:
		return "-"
	}
}
