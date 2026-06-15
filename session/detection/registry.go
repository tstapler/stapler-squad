package detection

import "github.com/tstapler/stapler-squad/session/detection/binaries"

// DefaultRegistry returns a DetectorRegistry pre-populated with all built-in binary detectors.
func DefaultRegistry() *DetectorRegistry {
	r := NewDetectorRegistry()
	r.Register(binaries.NewClaudeDetector())
	r.Register(binaries.NewGeminiDetector())
	r.Register(binaries.NewAiderDetector())
	r.Register(binaries.NewOpencodeDetector())
	r.Register(binaries.NewAgyDetector())
	return r
}
