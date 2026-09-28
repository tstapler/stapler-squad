package diagnose

import "testing"

func TestDefaultDiagnosticBundleConfig_ReturnsOneSectionBudgetPerBundleSection(t *testing.T) {
	cfg := DefaultDiagnosticBundleConfig(250000)

	sections := cfg.Sections()
	if len(sections) != len(bundleSectionOrder) {
		t.Fatalf("len(Sections()) = %d, want %d", len(sections), len(bundleSectionOrder))
	}

	seen := make(map[BundleSection]bool, len(sections))
	for _, sb := range sections {
		if seen[sb.Section] {
			t.Errorf("BundleSection %q appeared more than once", sb.Section)
		}
		seen[sb.Section] = true
	}
	for _, section := range bundleSectionOrder {
		if !seen[section] {
			t.Errorf("missing SectionBudget for %q", section)
		}
	}
}

func TestDefaultDiagnosticBundleConfig_SectionBudgetsSumWithinByteCeiling(t *testing.T) {
	const tokenBudget = 250000
	cfg := DefaultDiagnosticBundleConfig(tokenBudget)

	sum := 0
	for _, sb := range cfg.Sections() {
		sum += sb.MaxBytes
	}

	wantCeiling := tokenBudget * bytesPerToken
	if wantCeiling != 1000000 {
		t.Fatalf("test assumption broken: tokenBudget*bytesPerToken = %d, want 1000000", wantCeiling)
	}
	if cfg.ByteCeiling != wantCeiling {
		t.Errorf("ByteCeiling = %d, want %d", cfg.ByteCeiling, wantCeiling)
	}
	if sum > wantCeiling {
		t.Errorf("sum of SectionBudget.MaxBytes = %d, want <= %d", sum, wantCeiling)
	}
}

func TestDefaultDiagnosticBundleConfig_EverySectionIsNonZero(t *testing.T) {
	cfg := DefaultDiagnosticBundleConfig(250000)

	for _, sb := range cfg.Sections() {
		if sb.MaxBytes <= 0 {
			t.Errorf("SectionBudget for %q has non-positive MaxBytes: %d", sb.Section, sb.MaxBytes)
		}
	}
}

func TestDefaultDiagnosticBundleConfig_AllocatesExactPercentages(t *testing.T) {
	const tokenBudget = 250000
	cfg := DefaultDiagnosticBundleConfig(tokenBudget)
	ceiling := float64(cfg.ByteCeiling)

	want := map[BundleSection]float64{
		BundleSectionDescription:        0.08,
		BundleSectionAcceptanceCriteria: 0.04,
		BundleSectionHistory:            0.16,
		BundleSectionPriorVerdicts:      0.08,
		BundleSectionSessionSnapshot:    0.04,
		BundleSectionLogs:               0.24,
		BundleSectionDiff:               0.16,
		BundleSectionLinkedTranscript:   0.20,
	}

	for _, sb := range cfg.Sections() {
		wantPct, ok := want[sb.Section]
		if !ok {
			t.Errorf("unexpected BundleSection %q in Sections()", sb.Section)
			continue
		}
		wantBytes := int(ceiling * wantPct)
		if sb.MaxBytes != wantBytes {
			t.Errorf("section %q MaxBytes = %d, want %d (%.0f%% of %d)", sb.Section, sb.MaxBytes, wantBytes, wantPct*100, cfg.ByteCeiling)
		}
	}
}

func TestDefaultDiagnosticBundleConfig_ZeroTokenBudget(t *testing.T) {
	cfg := DefaultDiagnosticBundleConfig(0)

	if cfg.ByteCeiling != 0 {
		t.Errorf("ByteCeiling = %d, want 0", cfg.ByteCeiling)
	}
	for _, sb := range cfg.Sections() {
		if sb.MaxBytes != 0 {
			t.Errorf("section %q MaxBytes = %d, want 0 for zero token budget", sb.Section, sb.MaxBytes)
		}
	}
}
