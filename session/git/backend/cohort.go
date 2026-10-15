package backend

import "fmt"

// Cohort is a group of git operations that flip backend together (ADR-004).
// The zero value is CohortRefs; use ParseCohort at the config boundary, never a raw string.
type Cohort uint8

const (
	CohortRefs Cohort = iota
	CohortDiffStatus
	CohortLocalWrite
	CohortNetwork
	CohortWorktree

	cohortCount = iota
)

var cohortNames = [cohortCount]string{"refs", "diffstatus", "localwrite", "network", "worktree"}

// AllCohorts returns every cohort in promotion order (refs first, worktree last).
func AllCohorts() []Cohort {
	all := make([]Cohort, cohortCount)
	for i := range all {
		all[i] = Cohort(i)
	}
	return all
}

// ParseCohort resolves a config key to a Cohort; ok is false for an unknown name.
func ParseCohort(name string) (c Cohort, ok bool) {
	for i, n := range cohortNames {
		if n == name {
			return Cohort(i), true
		}
	}
	return 0, false
}

// String returns the config/metric-label name; "unknown" for an out-of-range value.
func (c Cohort) String() string {
	if int(c) >= cohortCount {
		return "unknown"
	}
	return cohortNames[c]
}

// HasReadOperations reports whether the cohort contains any read-only operation that may
// run in shadow mode. localwrite is all mutations, so it never shadows. Per-operation
// enforcement inside a mixed cohort (network, worktree) belongs to the router.
func (c Cohort) HasReadOperations() bool {
	return c != CohortLocalWrite
}

// ValidateMode reports why mode is illegal for c, or nil. It is the single owner of the
// shadow-needs-read-operations rule, shared by CohortMap.With (coercion) and config parsing (WARN).
func (c Cohort) ValidateMode(mode BackendMode) error {
	if mode == BackendShadow && !c.HasReadOperations() {
		return fmt.Errorf("shadow mode is not allowed for the %s cohort (no read operations)", c)
	}
	return nil
}

// BackendMode selects the implementation serving a cohort. The zero value is BackendCLI,
// so an unset or zeroed mode fails safe to the git CLI.
type BackendMode uint8

const (
	BackendCLI BackendMode = iota
	BackendGoGit
	// BackendShadow runs both implementations for read-only operations and returns the CLI result.
	BackendShadow

	backendModeCount = iota
)

var backendModeNames = [backendModeCount]string{"cli", "gogit", "shadow"}

// ParseBackendMode resolves a config value to a BackendMode; ok is false for an unknown name.
func ParseBackendMode(name string) (m BackendMode, ok bool) {
	for i, n := range backendModeNames {
		if n == name {
			return BackendMode(i), true
		}
	}
	return BackendCLI, false
}

// String returns the config/metric-label name; "unknown" for an out-of-range value.
func (m BackendMode) String() string {
	if int(m) >= backendModeCount {
		return "unknown"
	}
	return backendModeNames[m]
}

// CohortMap assigns a BackendMode to every Cohort. The zero value routes everything to
// the CLI, and Mode is total: an out-of-range Cohort also resolves to BackendCLI.
type CohortMap struct {
	modes [cohortCount]BackendMode
}

// Mode returns the mode for c, or BackendCLI for an out-of-range cohort.
func (m CohortMap) Mode(c Cohort) BackendMode {
	if int(c) >= cohortCount {
		return BackendCLI
	}
	return m.modes[c]
}

// With returns a copy of m with c set to mode. Illegal combinations (shadow on a cohort
// without read operations, out-of-range values) are coerced to BackendCLI, so a
// CohortMap can never hold one.
func (m CohortMap) With(c Cohort, mode BackendMode) CohortMap {
	if int(c) >= cohortCount {
		return m
	}
	if int(mode) >= backendModeCount || c.ValidateMode(mode) != nil {
		mode = BackendCLI
	}
	m.modes[c] = mode
	return m
}
