package detection

import "testing"

type fakeTerminalDetector struct{}

func (f *fakeTerminalDetector) Detect(output []byte) DetectedStatus { return StatusUnknown }
func (f *fakeTerminalDetector) DetectWithContext(output []byte) (DetectedStatus, string) {
	return StatusUnknown, ""
}
func (f *fakeTerminalDetector) DetectWithContextFromLines(lines []string) (DetectedStatus, string) {
	return StatusUnknown, ""
}
func (f *fakeTerminalDetector) DetectFromLines(lines []string) DetectedStatus { return StatusUnknown }
func (f *fakeTerminalDetector) RecentEvents(n int) []DetectionEvent           { return nil }
func (f *fakeTerminalDetector) SetSessionID(id string)                        {}

func TestFakeTerminalDetector_should_satisfyInterface(t *testing.T) {
	var _ TerminalDetector = (*fakeTerminalDetector)(nil)
	// If this compiles, the interface is satisfied.
}
