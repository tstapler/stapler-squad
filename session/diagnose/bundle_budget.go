package diagnose

import (
	"fmt"
	"strings"
)

// enforceBudget truncates content to fit section's MaxBytes (per cfg.Sections()),
// appending a trailing marker noting how many bytes were omitted -- Story
// 2.1.3's AC is explicit that a cut must never be silent. Content already
// within budget is returned unchanged. A section with no configured budget
// (MaxBytes <= 0) is returned unchanged too, since there is nothing to
// enforce against.
func enforceBudget(section BundleSection, content string, cfg DiagnosticBundleConfig) string {
	budget := sectionMaxBytes(section, cfg)
	if budget <= 0 || len(content) <= budget {
		return content
	}

	// The marker's own length depends on the digit count of "omitted", which
	// depends on how much of content is kept -- so converge by shrinking keep
	// until content[:keep]+marker actually fits within budget, rather than
	// computing the split in one pass.
	keep := budget
	for {
		if keep < 0 {
			keep = 0
		}
		omitted := len(content) - keep
		if omitted < 0 {
			omitted = 0
		}
		marker := truncationMarker(omitted)
		if keep+len(marker) <= budget || keep == 0 {
			return strings.ToValidUTF8(content[:keep], "") + marker
		}
		keep--
	}
}

func truncationMarker(omittedBytes int) string {
	return fmt.Sprintf("[... truncated, %d bytes omitted for budget ...]", omittedBytes)
}

// sectionMaxBytes looks up section's byte ceiling in cfg.Sections(), returning
// 0 if the section isn't present (defensive -- every BundleSection is always
// present per DefaultDiagnosticBundleConfig, but a hand-built
// DiagnosticBundleConfig in a test might omit one).
func sectionMaxBytes(section BundleSection, cfg DiagnosticBundleConfig) int {
	for _, sb := range cfg.Sections() {
		if sb.Section == section {
			return sb.MaxBytes
		}
	}
	return 0
}
