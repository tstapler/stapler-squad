package omnibar

import (
	"sync"
)

// Detector is the interface that all input type detectors must implement
type Detector interface {
	// Name returns the name of the detector for debugging/logging
	Name() string

	// Priority returns the priority of this detector (lower = higher priority)
	// Detectors are tried in priority order, first match wins
	Priority() int

	// Detect attempts to detect the input type from the given input string
	// Returns nil if this detector doesn't match the input
	Detect(input string) *DetectionResult

	// Validate validates the detected input (e.g., path exists, URL is reachable)
	// This may involve I/O operations and should be called asynchronously
	Validate(result *DetectionResult) *ValidationResult
}

// Registry manages the collection of detectors and orchestrates detection
type Registry struct {
	detectors []Detector
	mu        sync.RWMutex
}

// NewRegistry creates a new detector registry
func NewRegistry() *Registry {
	return &Registry{
		detectors: make([]Detector, 0),
	}
}

// Register adds a detector to the registry, maintaining priority order
func (r *Registry) Register(detector Detector) {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Insert in priority order (lower priority value = higher priority)
	inserted := false
	for i, existing := range r.detectors {
		if detector.Priority() < existing.Priority() {
			// Insert at this position
			r.detectors = append(r.detectors[:i], append([]Detector{detector}, r.detectors[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		r.detectors = append(r.detectors, detector)
	}
}

// Detect runs all detectors in priority order and returns the first match
func (r *Registry) Detect(input string) *DetectionResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, detector := range r.detectors {
		if result := detector.Detect(input); result != nil {
			return result
		}
	}

	return &DetectionResult{
		Type:       InputTypeUnknown,
		Confidence: 0,
	}
}

// DetectAll runs all detectors and returns all matching results
// Useful for debugging or showing alternatives to the user
func (r *Registry) DetectAll(input string) []*DetectionResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	results := make([]*DetectionResult, 0)
	for _, detector := range r.detectors {
		if result := detector.Detect(input); result != nil {
			results = append(results, result)
		}
	}
	return results
}

// Validate validates a detection result using the appropriate detector
func (r *Registry) Validate(result *DetectionResult) *ValidationResult {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, detector := range r.detectors {
		// Find the detector that can handle this input type
		testResult := detector.Detect(result.ParsedValue)
		if testResult != nil && testResult.Type == result.Type {
			return detector.Validate(result)
		}
	}

	return &ValidationResult{
		Valid:        false,
		ErrorMessage: "No validator found for this input type",
	}
}

// GetDetectors returns a copy of the registered detectors
func (r *Registry) GetDetectors() []Detector {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]Detector, len(r.detectors))
	copy(result, r.detectors)
	return result
}

// DefaultRegistry is the default registry with all built-in detectors
var DefaultRegistry *Registry

// init initializes the default registry with all built-in detectors
func init() {
	DefaultRegistry = NewRegistry()

	// Register detectors in priority order
	// Lower priority number = higher priority (checked first)
	DefaultRegistry.Register(&GitHubPRDetector{})         // Priority 10
	DefaultRegistry.Register(&GitHubBranchDetector{})     // Priority 20
	DefaultRegistry.Register(&GitHubRepoDetector{})       // Priority 30
	DefaultRegistry.Register(&GitHubShorthandDetector{})  // Priority 40
	DefaultRegistry.Register(&PathWithBranchDetector{})   // Priority 50
	DefaultRegistry.Register(&LocalPathDetector{})        // Priority 100 (catch-all)
}

// Detect is a convenience function that uses the default registry
func Detect(input string) *DetectionResult {
	return DefaultRegistry.Detect(input)
}

// Validate is a convenience function that uses the default registry
func Validate(result *DetectionResult) *ValidationResult {
	return DefaultRegistry.Validate(result)
}
