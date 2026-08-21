//go:build linux

package executor

import "testing"

// TestBuildSysProcAttr_Setsid_excludes_Setpgid asserts that enabling Setsid
// does not also set Setpgid. On Linux, setsid(2) makes the caller both a
// session leader and a process-group leader, so a subsequent setpgid(0,0)
// returns EPERM. Both flags must never be set together.
//
// Must fail against pre-fix code (which set Setpgid unconditionally unless
// noProcGroup was set, regardless of setsid).
func TestBuildSysProcAttr_Setsid_excludes_Setpgid(t *testing.T) {
	t.Parallel()

	attr := buildSysProcAttr(processConfig{setsid: true})

	if !attr.Setsid {
		t.Error("expected Setsid=true, got false")
	}
	if attr.Setpgid {
		t.Error("Setpgid must be false when Setsid is true: setting both causes EPERM on Linux")
	}
}

// TestBuildSysProcAttr_Default_setsSetpgid asserts the default (no flags)
// sets Setpgid=true and Setsid=false.
func TestBuildSysProcAttr_Default_setsSetpgid(t *testing.T) {
	t.Parallel()

	attr := buildSysProcAttr(processConfig{})

	if !attr.Setpgid {
		t.Error("expected Setpgid=true by default, got false")
	}
	if attr.Setsid {
		t.Error("expected Setsid=false by default, got true")
	}
}
