package session

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestExtractTestClassesFromLog(t *testing.T) {
	tests := []struct {
		name string
		log  string
		want []string
	}{
		{"gradle", "OutlinerRegressionTest > testBasicOutlinerFlow FAILED", []string{"OutlinerRegressionTest"}},
		{"gradle with job prefix", "jvmTest\t2026-10-15T00:00:00Z OutlinerRegressionTest > testX FAILED", []string{"OutlinerRegressionTest"}},
		{"go", "--- FAIL: TestCreateSession (0.03s)", []string{"TestCreateSession"}},
		{"pytest", "FAILED tests/test_foo.py::TestBar::test_baz", []string{"test_foo"}},
		{"dedup", "ATest > a FAILED\nATest > b FAILED", []string{"ATest"}},
		{"pom error", "Error: could not read pom.xml", nil},
		{"exit value", "Process 'command ...' finished with non-zero exit value 1", nil},
		{"build failed", "BUILD FAILED in 2m 13s", nil},
		{"empty", "", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := extractTestClassesFromLog(tc.log); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTestFileInDiff(t *testing.T) {
	tests := []struct {
		name  string
		class string
		files []string
		want  bool
	}{
		{"exact kt", "OutlinerRegressionTest", []string{"src/jvmTest/OutlinerRegressionTest.kt"}, true},
		{"not in diff", "OutlinerRegressionTest", []string{"src/Foo.kt", "src/Bar.kt"}, false},
		{"go func, no test file changed", "TestCreateSession", []string{"session/instance.go"}, false},
		{"go func, test file changed (conservative)", "TestCreateSession", []string{"session/x_test.go"}, true},
		{"java", "SomeTest", []string{"src/SomeTest.java", "src/Bar.kt"}, true},
		{"pytest module", "test_foo", []string{"tests/test_foo.py"}, true},
		{"no files", "SomeTest", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := testFileInDiff(tc.class, tc.files); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

type fakeCIRerun struct {
	files    []string
	filesErr error
	logs     map[int64]string
	logErr   error
	rerunErr error
	reruns   []int64
}

func (f *fakeCIRerun) ChangedFiles(context.Context, string, int) ([]string, error) {
	return f.files, f.filesErr
}
func (f *fakeCIRerun) FailedLogs(_ context.Context, _ string, id int64) (string, error) {
	return f.logs[id], f.logErr
}
func (f *fakeCIRerun) RerunFailed(_ context.Context, _ string, id int64) error {
	if f.rerunErr != nil {
		return f.rerunErr
	}
	f.reruns = append(f.reruns, id)
	return nil
}

const flakyLog = "OutlinerRegressionTest > testBasicOutlinerFlow FAILED"

func TestMaybeFlakyCheckRerun(t *testing.T) {
	tests := []struct {
		name       string
		fake       *fakeCIRerun
		runIDs     []int64
		preAttempt bool
		want       bool
		wantReruns []int64
	}{
		{"out of diff reruns", &fakeCIRerun{files: []string{"Foo.kt"}, logs: map[int64]string{1: flakyLog}}, []int64{1}, false, true, []int64{1}},
		{"already rerun falls through", &fakeCIRerun{files: []string{"Foo.kt"}, logs: map[int64]string{1: flakyLog}}, []int64{1}, true, false, nil},
		{"in diff falls through", &fakeCIRerun{files: []string{"OutlinerRegressionTest.kt"}, logs: map[int64]string{1: flakyLog}}, []int64{1}, false, false, nil},
		{"no test class falls through", &fakeCIRerun{files: []string{"Foo.kt"}, logs: map[int64]string{1: "BUILD FAILED"}}, []int64{1}, false, false, nil},
		{"log error falls through", &fakeCIRerun{files: []string{"Foo.kt"}, logErr: errors.New("boom")}, []int64{1}, false, false, nil},
		{"changed files error falls through", &fakeCIRerun{filesErr: errors.New("boom")}, []int64{1}, false, false, nil},
		{"rerun error falls through, not recorded", &fakeCIRerun{files: []string{"Foo.kt"}, logs: map[int64]string{1: flakyLog}, rerunErr: errors.New("403")}, []int64{1}, false, false, nil},
		{"no run ids", &fakeCIRerun{}, nil, false, false, nil},
		{"one run in diff blocks all", &fakeCIRerun{files: []string{"BTest.kt"}, logs: map[int64]string{1: flakyLog, 2: "BTest > x FAILED"}}, []int64{1, 2}, false, false, nil},
		{"two runs out of diff", &fakeCIRerun{files: []string{"Foo.kt"}, logs: map[int64]string{1: flakyLog, 2: "BTest > x FAILED"}}, []int64{1, 2}, false, true, []int64{1, 2}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			l := &BacklogLifecycleListener{ciRerunner: tc.fake}
			if tc.preAttempt {
				l.ciReruns.record("item", 1)
			}
			got := l.maybeFlakyCheckRerun(context.Background(), "item", "/repo", 7, tc.runIDs)
			if got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			if !reflect.DeepEqual(tc.fake.reruns, tc.wantReruns) {
				t.Fatalf("reruns %v, want %v", tc.fake.reruns, tc.wantReruns)
			}
			if tc.fake.rerunErr != nil && l.ciReruns.attempted("item", 1) {
				t.Fatal("failed rerun must not be recorded")
			}
		})
	}
}

func TestMaybeFlakyCheckRerun_SecondTickFallsThrough(t *testing.T) {
	fake := &fakeCIRerun{files: []string{"Foo.kt"}, logs: map[int64]string{1: flakyLog}}
	l := &BacklogLifecycleListener{ciRerunner: fake}
	if !l.maybeFlakyCheckRerun(context.Background(), "item", "/r", 1, []int64{1}) {
		t.Fatal("first encounter should rerun")
	}
	if l.maybeFlakyCheckRerun(context.Background(), "item", "/r", 1, []int64{1}) {
		t.Fatal("second encounter should fall through to the fix session")
	}
	l.ciReruns.clear("item")
	if l.ciReruns.attempted("item", 1) {
		t.Fatal("clear should reset the ledger")
	}
}
