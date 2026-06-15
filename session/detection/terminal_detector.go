package detection

// TerminalDetector is the interface implemented by StatusDetector that
// consumer packages (session, server/services) depend on.
// Defined here (consumer-of-input pattern) so all call sites can use
// the interface without importing the concrete type.
type TerminalDetector interface {
	Detect(output []byte) DetectedStatus
	DetectWithContext(output []byte) (DetectedStatus, string)
	DetectWithContextFromLines(lines []string) (DetectedStatus, string)
	DetectFromLines(lines []string) DetectedStatus
	RecentEvents(n int) []DetectionEvent
	SetSessionID(id string)
}

// Compile-time assertion: *StatusDetector must satisfy TerminalDetector.
var _ TerminalDetector = (*StatusDetector)(nil)
