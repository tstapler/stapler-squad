//go:build linux

package telemetry

import "testing"

func TestParseCgroupEvents(t *testing.T) {
	data := []byte("low 0\nhigh 42\nmax 3\noom 1\noom_kill 1\noom_group_kill 0\n")
	got := parseCgroupEvents(data)

	want := map[string]int64{"low": 0, "high": 42, "max": 3, "oom": 1, "oom_kill": 1, "oom_group_kill": 0}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("parseCgroupEvents()[%q] = %d, want %d", k, got[k], v)
		}
	}
}

func TestParseCgroupPSIAvg10(t *testing.T) {
	cases := []struct {
		name    string
		line    string
		wantAvg float64
		wantOK  bool
	}{
		{"some line", "some avg10=12.34 avg60=5.00 avg300=1.00 total=987654", 12.34, true},
		{"full line zeroed", "full avg10=0.00 avg60=0.00 avg300=0.00 total=0", 0.00, true},
		{"malformed line", "not a psi line", 0, false},
		{"empty line", "", 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseCgroupPSIAvg10(tc.line)
			if ok != tc.wantOK {
				t.Fatalf("parseCgroupPSIAvg10(%q) ok = %v, want %v", tc.line, ok, tc.wantOK)
			}
			if ok && got != tc.wantAvg {
				t.Errorf("parseCgroupPSIAvg10(%q) = %v, want %v", tc.line, got, tc.wantAvg)
			}
		})
	}
}

func TestCgroupMemoryDir_should_ReturnUnifiedMountPath_When_RunningUnderCgroupV2(t *testing.T) {
	dir, err := cgroupMemoryDir()
	if err != nil {
		t.Skipf("skipping: /proc/self/cgroup unavailable in this environment: %v", err)
	}
	if dir == "" {
		t.Error("cgroupMemoryDir() returned empty string with no error")
	}
}
