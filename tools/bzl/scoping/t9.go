package main

import (
	"fmt"
	"io"
)

// T9Targets are the ADR-010 proposal values (owner-confirmed at Task 13.1f; the 8-line improvement.sh is untouched).
type T9Targets struct {
	TestLOCShareMax      float64
	UniversalTouch500Max float64
	LibLOCShareMax       float64
	LowerLibMin          float64
	LargestPkgLOCMax     int
	TargetShareMax       float64
}

// DefaultT9Targets are the "at close" values.
var DefaultT9Targets = T9Targets{
	TestLOCShareMax: 0.45, UniversalTouch500Max: 0.61, LibLOCShareMax: 0.395, LowerLibMin: 0.12,
	LargestPkgLOCMax: 72000, TargetShareMax: 0.30,
}

func verdict(ok bool) string {
	if ok {
		return "PASS"
	}
	return "FAIL"
}

// PrintT9 writes `T9a: PASS|FAIL|UNMEASURED` lines and reports whether none failed. T9f needs the Story 12.1 slice
// manifest, so it stays UNMEASURED, which is not a failure before Story 13.7.
func PrintT9(w io.Writer, r *Report, t T9Targets) bool {
	rows := []struct {
		name string
		ok   bool
		note string
	}{
		{"T9a", r.T9aTestLOCShare500 <= t.TestLOCShareMax, fmt.Sprintf("test_loc_share=%.4f max=%.2f", r.T9aTestLOCShare500, t.TestLOCShareMax)},
		{"T9b", r.T9bUniversalTouch500 <= t.UniversalTouch500Max, fmt.Sprintf("universal_touch_500=%.4f max=%.2f", r.T9bUniversalTouch500, t.UniversalTouch500Max)},
		{"T9c", r.T9cMeanLibLOCShare <= t.LibLOCShareMax && r.T9cChangesWithLowerLib >= t.LowerLibMin,
			fmt.Sprintf("lib_loc_share=%.4f max=%.3f lower=%.3f min=%.2f", r.T9cMeanLibLOCShare, t.LibLOCShareMax, r.T9cChangesWithLowerLib, t.LowerLibMin)},
		{"T9d", r.T9dServicesLOC <= t.LargestPkgLOCMax && r.T9dSessionLOC <= t.LargestPkgLOCMax && r.T9dTestInversions == 0,
			fmt.Sprintf("services=%d session=%d inversions=%d max=%d", r.T9dServicesLOC, r.T9dSessionLOC, r.T9dTestInversions, t.LargestPkgLOCMax)},
		{"T9e", r.T9eTargetModelShare <= t.TargetShareMax, fmt.Sprintf("target_share=%.4f max=%.2f", r.T9eTargetModelShare, t.TargetShareMax)},
	}
	pass := true
	for _, row := range rows {
		fmt.Fprintf(w, "%s: %s (%s)\n", row.name, verdict(row.ok), row.note)
		pass = pass && row.ok
	}
	fmt.Fprintln(w, "T9f: UNMEASURED (no tests/e2e/slice-manifest.json)")
	return pass
}
