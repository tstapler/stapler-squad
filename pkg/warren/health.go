package warren

import "time"

// HealthReport is the aggregate result of running all registered health checks.
type HealthReport struct {
	// Healthy is true only when every registered check passed.
	Healthy bool
	// Checks contains one result per registered health check, in registration order.
	Checks []CheckResult
}

// CheckResult is the outcome of a single named health check.
type CheckResult struct {
	Name    string
	Healthy bool
	// Err is non-nil when the check failed.
	Err     error
	Latency time.Duration
}
