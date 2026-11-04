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
