package session

// DiagnosticBundleTokenBudget is the approximate token ceiling for a
// diagnostic context bundle (Diagnose & Nudge, AC1). session/tokens is a
// Claude-usage cost/analytics parser, not a tokenizer, so there is no
// existing "count tokens in this string" utility to reuse — this package
// estimates instead of counting exactly, consistent with how
// HandoffSummaryGenerator already budgets by byte/line caps rather than
// exact tokens.
const DiagnosticBundleTokenBudget = 250_000

// bytesPerTokenEstimate is the byte/token ratio EstimateTokens divides by.
// English prose and source code both average close to 4 bytes per GPT-style
// token; this is a heuristic ceiling check, not a billing-accurate count, so
// a single ratio for all content is an intentional simplification.
const bytesPerTokenEstimate = 4

// EstimateTokens returns a rough token-count estimate for s, used only to
// decide whether a diagnostic bundle needs compaction before it is over
// DiagnosticBundleTokenBudget. Not accurate enough for billing or exact
// context-window accounting.
func EstimateTokens(s string) int {
	return len(s) / bytesPerTokenEstimate
}

// ExceedsDiagnosticBundleBudget reports whether s's estimated token count
// exceeds DiagnosticBundleTokenBudget.
func ExceedsDiagnosticBundleBudget(s string) bool {
	return EstimateTokens(s) > DiagnosticBundleTokenBudget
}
