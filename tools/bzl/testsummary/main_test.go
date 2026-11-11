package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const sampleXML = `<testsuites><testsuite name="x">
<testcase name="TestA"/>
<testcase name="TestB"><failure message="boom"/></testcase>
<testcase name="TestC"><skipped/></testcase>
<testcase name="TestD"><error/></testcase>
</testsuite></testsuites>`

func TestParseXMLSplitsPassFailSkip(t *testing.T) {
	passed, failed, err := parseXML(strings.NewReader(sampleXML))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(passed, []string{"TestA"}) || !reflect.DeepEqual(failed, []string{"TestB", "TestD"}) {
		t.Fatalf("passed=%v failed=%v", passed, failed)
	}
}

func TestFromTestlogsKeysByPackageAndMergesShards(t *testing.T) {
	root := t.TempDir()
	write := func(rel, body string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("config/config_test/test.xml", sampleXML)
	write("server/services/services_test/shard_1_of_2/test.xml", `<testsuites><testsuite><testcase name="TestS1"/></testsuite></testsuites>`)
	write("server/services/services_test/shard_2_of_2/test.xml", `<testsuites><testsuite><testcase name="TestS2"/></testsuite></testsuites>`)
	got, err := fromTestlogs(root, "m", false)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"m/config/TestA", "m/server/services/TestS1", "m/server/services/TestS2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestFromGoJSONOnlyTestLevelEvents(t *testing.T) {
	in := `{"Action":"pass","Package":"m/a","Test":"TestX"}
{"Action":"pass","Package":"m/a"}
{"Action":"fail","Package":"m/b","Test":"TestY"}
not json
{"Action":"pass","Package":"m/a","Test":"TestX/sub"}
`
	got, err := fromGoJSON(strings.NewReader(in), false)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"m/a/TestX", "m/a/TestX/sub"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	failed, _ := fromGoJSON(strings.NewReader(in), true)
	if want := []string{"m/b/TestY"}; !reflect.DeepEqual(failed, want) {
		t.Fatalf("failed=%v", failed)
	}
}

func writeXML(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSummaryListsFailuresWithLogPaths(t *testing.T) {
	root := t.TempDir()
	writeXML(t, root, "config/config_test/test.xml", sampleXML)
	writeXML(t, root, "ok/ok_test/test.xml", `<testsuites><testsuite><testcase name="TestOK"/></testsuite></testsuites>`)
	writeXML(t, root, "slow/slow_test/test.xml", `<testsuites><testsuite failures="1"><testcase name="TestSlow"/></testsuite></testsuites>`)
	fails, err := collectFailures(root)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	writeSummary(&out, fails, 3, 60)
	want := `FAIL //config:config_test
  TestB
  TestD
  log: bazel-testlogs/config/config_test/test.log
FAIL //slow:slow_test
  (no failing test case recorded; timeout or crash)
  log: bazel-testlogs/slow/slow_test/test.log
(3 test targets cached; rerun with FORCE=1)
`
	if out.String() != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out.String(), want)
	}
}

func TestSummaryIsCappedAtMaxLines(t *testing.T) {
	var fails []failure
	for i := 0; i < 40; i++ {
		fails = append(fails, failure{pkg: "p", target: "t" + strings.Repeat("x", i), names: []string{"TestA", "TestB"}})
	}
	var out strings.Builder
	writeSummary(&out, fails, 1, 60)
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 60 {
		t.Fatalf("got %d lines, want 60", len(lines))
	}
	if !strings.HasPrefix(lines[58], "... ") || !strings.HasPrefix(lines[59], "(1 test targets cached") {
		t.Fatalf("tail lines: %q %q", lines[58], lines[59])
	}
}
