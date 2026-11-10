package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const baseline = `{"specs":[{"spec":"a.spec.ts","project":"chromium","title":"known bad","class":"fail"}]}`

func report(failTitle string) string {
	one := func(title, status string) string {
		return `{"title":"` + title + `","file":"a.spec.ts","tests":[{"projectName":"chromium","results":[{"retry":0,"status":"failed"},{"retry":1,"status":"` + status + `"}]}]}`
	}
	return `{"suites":[{"title":"a.spec.ts","file":"a.spec.ts","specs":[` + one("known bad", "failed") + `,` + one("flaky ok", "passed") + `,` + one(failTitle, "failed") + `]}]}`
}

func fixtureRoot(t *testing.T) string {
	root := t.TempDir()
	dir := filepath.Join(root, "tests/e2e/baseline")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "next-e2e-baseline.json"), []byte(baseline), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func runReport(t *testing.T, body string) (int, string) {
	root := fixtureRoot(t)
	rp := filepath.Join(root, "r.json")
	if err := os.WriteFile(rp, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errw bytes.Buffer
	code := run([]string{"-root", root, "-report", rp, "-target", "t"}, &out, &errw)
	return code, errw.String()
}

func TestBaselineListedFailurePasses(t *testing.T) {
	// the third spec reuses a baseline title, so the only failures are baseline-listed
	if code, msg := runReport(t, report("known bad")); code != 0 {
		t.Fatalf("code %d: %s", code, msg)
	}
}

func TestNewFailureFails(t *testing.T) {
	code, msg := runReport(t, report("brand new failure"))
	if code != 1 || !strings.Contains(msg, "brand new failure") {
		t.Fatalf("code %d: %s", code, msg)
	}
}

func TestEmptyReportFails(t *testing.T) {
	if code, _ := runReport(t, `{"suites":[]}`); code != 1 {
		t.Fatal("empty report must not pass")
	}
}

func TestMissingBinaryHasRemedy(t *testing.T) {
	t.Setenv("TEST_SERVER_BINARY", "")
	var out, errw bytes.Buffer
	if code := run([]string{"-root", t.TempDir()}, &out, &errw); code != 1 || !strings.Contains(errw.String(), "Remedy:") {
		t.Fatalf("%d %s", code, errw.String())
	}
}
