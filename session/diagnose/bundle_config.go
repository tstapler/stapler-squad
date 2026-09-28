package diagnose

// BundleSection identifies one budgeted slice of a DiagnosticBundle. A closed
// enum -- drives per-section truncation rather than one flat cap.
type BundleSection string

const (
	BundleSectionDescription        BundleSection = "description"
	BundleSectionAcceptanceCriteria BundleSection = "acceptance_criteria"
	BundleSectionHistory            BundleSection = "history"
	BundleSectionPriorVerdicts      BundleSection = "prior_verdicts"
	BundleSectionSessionSnapshot    BundleSection = "session_snapshot"
	BundleSectionLogs               BundleSection = "logs"
	BundleSectionDiff               BundleSection = "diff"
	BundleSectionLinkedTranscript   BundleSection = "linked_transcript"
)

// SectionBudget pairs a BundleSection with its byte ceiling. Immutable value
// object.
type SectionBudget struct {
	Section  BundleSection
	MaxBytes int
}

// bundleSectionAllocation is the fraction of the overall byte ceiling given to
// each BundleSection. Percentages chosen so Logs+Diff+LinkedTranscript (the
// three variable-size, evidence-heavy sections) get 60% of the budget between
// them, per research/build-vs-buy.md's per-section-budget recommendation.
// Sums to 1.0 (8+4+16+8+4+24+16+20 = 100%).
var bundleSectionAllocation = map[BundleSection]float64{
	BundleSectionDescription:        0.08,
	BundleSectionAcceptanceCriteria: 0.04,
	BundleSectionHistory:            0.16,
	BundleSectionPriorVerdicts:      0.08,
	BundleSectionSessionSnapshot:    0.04,
	BundleSectionLogs:               0.24,
	BundleSectionDiff:               0.16,
	BundleSectionLinkedTranscript:   0.20,
}

// bundleSectionOrder fixes Sections()'s iteration order (map iteration in Go
// is unordered), matching the Domain Glossary's listing order.
var bundleSectionOrder = []BundleSection{
	BundleSectionDescription,
	BundleSectionAcceptanceCriteria,
	BundleSectionHistory,
	BundleSectionPriorVerdicts,
	BundleSectionSessionSnapshot,
	BundleSectionLogs,
	BundleSectionDiff,
	BundleSectionLinkedTranscript,
}

// bytesPerToken is the heuristic conversion this feature uses to derive a
// byte ceiling from a token budget (mirrors HandoffSummaryConfig's shape).
const bytesPerToken = 4

// DiagnosticBundleConfig holds the per-section byte budgets derived from a
// token budget, plus the overall byte ceiling they were allocated from.
type DiagnosticBundleConfig struct {
	TokenBudget int
	ByteCeiling int
	sections    []SectionBudget
}

// Sections returns the per-section byte budgets, one per BundleSection, in
// bundleSectionOrder.
func (c DiagnosticBundleConfig) Sections() []SectionBudget {
	return c.sections
}

// DefaultDiagnosticBundleConfig derives a DiagnosticBundleConfig from
// tokenBudget using the ~4 bytes/token heuristic (byte ceiling =
// tokenBudget * bytesPerToken), then splits that ceiling across all 8
// BundleSections per bundleSectionAllocation.
func DefaultDiagnosticBundleConfig(tokenBudget int) DiagnosticBundleConfig {
	byteCeiling := tokenBudget * bytesPerToken

	sections := make([]SectionBudget, 0, len(bundleSectionOrder))
	for _, section := range bundleSectionOrder {
		maxBytes := int(float64(byteCeiling) * bundleSectionAllocation[section])
		sections = append(sections, SectionBudget{Section: section, MaxBytes: maxBytes})
	}

	return DiagnosticBundleConfig{
		TokenBudget: tokenBudget,
		ByteCeiling: byteCeiling,
		sections:    sections,
	}
}
