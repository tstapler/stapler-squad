package session

// ScrollForwardCapability describes what a ScrollAdapter's scroll-forward
// strategy has been verified to do. Pure value object populated by each
// adapter's constructor -- no logic here. Claude's entry (verified by Story
// 1.2.1's live spike) is built in NewClaudeGestureForwardStrategy.
type ScrollForwardCapability struct {
	// KeySequencesUp are the raw bytes sent to scroll the underlying CLI's
	// own transcript upward, in the order they must be written.
	KeySequencesUp [][]byte
	// VerifiedAgainstVersion records the CLI version the spike (Story 1.2.1)
	// confirmed this strategy against.
	VerifiedAgainstVersion string
	// KnownFailureModes documents situations where forwarding is expected to
	// silently do nothing or behave unexpectedly, for UX/support reference.
	KnownFailureModes []string
}
