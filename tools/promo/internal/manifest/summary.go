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
	b.WriteString("| name | format | status | bytes | sha256 | form factor |\n|---|---|---|---:|---|---|\n")
	for _, e := range m.Entries {
		fmt.Fprintf(&b, "| %s | %s | %s | %d | %s | %s |\n", e.Name, e.Format, e.Status, e.Bytes, e.SHA256, e.FormFactor)
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
