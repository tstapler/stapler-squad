package backend

import "testing"

func TestParseCohort_RoundTripsEveryCohort(t *testing.T) {
	for _, c := range AllCohorts() {
		got, ok := ParseCohort(c.String())
		if !ok || got != c {
			t.Errorf("ParseCohort(%q) = %v, %v; want %v, true", c.String(), got, ok, c)
		}
	}
	if len(AllCohorts()) != 5 {
		t.Errorf("AllCohorts() has %d entries, want 5", len(AllCohorts()))
	}
	if _, ok := ParseCohort("bogus"); ok {
		t.Error("ParseCohort(bogus) ok = true")
	}
}

func TestParseBackendMode(t *testing.T) {
	for name, want := range map[string]BackendMode{"cli": BackendCLI, "gogit": BackendGoGit, "shadow": BackendShadow} {
		got, ok := ParseBackendMode(name)
		if !ok || got != want || got.String() != name {
			t.Errorf("ParseBackendMode(%q) = %v, %v", name, got, ok)
		}
	}
	if m, ok := ParseBackendMode("GoGit"); ok || m != BackendCLI {
		t.Errorf("ParseBackendMode(GoGit) = %v, %v; want cli, false (case-sensitive)", m, ok)
	}
}

func TestCohortMap_ZeroValueIsAllCLI(t *testing.T) {
	var m CohortMap
	for _, c := range AllCohorts() {
		if m.Mode(c) != BackendCLI {
			t.Errorf("zero CohortMap.Mode(%v) = %v, want cli", c, m.Mode(c))
		}
	}
	if m.Mode(Cohort(200)) != BackendCLI {
		t.Error("out-of-range cohort must resolve to cli")
	}
}

func TestCohortMap_WithCoercesIllegalStatesToCLI(t *testing.T) {
	m := CohortMap{}.With(CohortRefs, BackendShadow).
		With(CohortLocalWrite, BackendShadow).
		With(CohortNetwork, BackendMode(99)).
		With(Cohort(200), BackendGoGit)
	if m.Mode(CohortRefs) != BackendShadow {
		t.Errorf("refs = %v, want shadow", m.Mode(CohortRefs))
	}
	if m.Mode(CohortLocalWrite) != BackendCLI {
		t.Errorf("localwrite = %v, want cli", m.Mode(CohortLocalWrite))
	}
	if m.Mode(CohortNetwork) != BackendCLI {
		t.Errorf("network = %v, want cli", m.Mode(CohortNetwork))
	}
}

func TestCohort_HasReadOperations(t *testing.T) {
	for _, c := range AllCohorts() {
		if want := c != CohortLocalWrite; c.HasReadOperations() != want {
			t.Errorf("%v.HasReadOperations() = %v, want %v", c, c.HasReadOperations(), want)
		}
	}
}
