package diagnose

import (
	"strings"
	"testing"
)

func TestEnforceBudget_ShouldPassthroughContent_WhenUnderSectionByteBudget(t *testing.T) {
	cfg := DefaultDiagnosticBundleConfig(1000) // yields a byte ceiling of 4000
	content := "well under budget"

	got := enforceBudget(BundleSectionDescription, content, cfg)

	if got != content {
		t.Fatalf("expected content passed through unchanged, got %q", got)
	}
}

func TestEnforceBudget_ShouldTruncateWithMarker_WhenContentExceedsSectionByteBudget(t *testing.T) {
	// bundleSectionAllocation gives Logs 24% of the byte ceiling; pick a
	// TokenBudget so Logs' MaxBytes lands at exactly 240,000, matching the
	// validation.md scenario (TokenBudget * 4 * 0.24 = 240,000 => TokenBudget = 250,000).
	cfg := DefaultDiagnosticBundleConfig(250000)
	logsBudget := sectionMaxBytes(BundleSectionLogs, cfg)
	if logsBudget != 240000 {
		t.Fatalf("test setup: expected Logs MaxBytes=240000, got %d", logsBudget)
	}

	content := strings.Repeat("x", 500000)

	got := enforceBudget(BundleSectionLogs, content, cfg)

	if len(got) > logsBudget {
		t.Fatalf("expected result <= %d bytes, got %d", logsBudget, len(got))
	}
	if !strings.Contains(got, "truncated") || !strings.HasSuffix(got, "bytes omitted for budget ...]") {
		t.Fatalf("expected result to end with a truncation marker, got suffix %q", got[len(got)-60:])
	}
}

func TestEnforceBudget_ShouldReturnUnchanged_WhenSectionHasNoConfiguredBudget(t *testing.T) {
	cfg := DiagnosticBundleConfig{} // sections deliberately left empty
	content := strings.Repeat("y", 100)

	got := enforceBudget(BundleSectionDescription, content, cfg)

	if got != content {
		t.Fatalf("expected content unchanged when section has no budget, got %q", got)
	}
}
