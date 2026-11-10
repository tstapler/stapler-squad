package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tstapler/stapler-squad/tools/bzl/soak"
)

const ledger = `# header comment
schema: 1
soak_runs_n: 30
soak_consecutive_green_k: 10
queue_timeout_seconds: 900
runs: []
`

func TestAppendKeepsCommentsAndValidates(t *testing.T) {
	t.Parallel()
	out, added, err := Append([]byte(ledger), RowFor("nightly-1-1", 0))
	if err != nil || !added {
		t.Fatal(err, added)
	}
	out, _, err = Append(out, RowFor("nightly-2-1", 3))
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.HasPrefix(s, "# header comment") {
		t.Errorf("header comment lost:\n%s", s)
	}
	l, err := soak.Parse(bytes.NewReader(out))
	if err != nil || len(l.Runs) != 2 || l.Runs[1].Result != "fail" || l.Runs[1].Attribution != "" {
		t.Fatalf("%v %+v\n%s", err, l, s)
	}
	if errs := l.Validate(); len(errs) != 0 {
		t.Fatal(errs)
	}
}

func TestRowForClassifiesInfraOnly(t *testing.T) {
	t.Parallel()
	if r := RowFor("x", 36); r.Attribution != soak.Infra || r.Result != "fail" {
		t.Errorf("%+v", r)
	}
	for _, code := range []int{1, 3, 4} {
		if r := RowFor("x", code); r.Attribution != "" || r.Result != "fail" {
			t.Errorf("exit %d must stay unattributed: %+v", code, r)
		}
	}
}

func TestAppendIsIdempotentAndRejectsConflicts(t *testing.T) {
	t.Parallel()
	once, _, _ := Append([]byte(ledger), RowFor("a", 0))
	again, added, err := Append(once, RowFor("a", 0))
	if err != nil || added || !bytes.Equal(once, again) {
		t.Fatal(err, added)
	}
	if _, _, err := Append(once, RowFor("a", 3)); err == nil {
		t.Fatal("conflicting row must error")
	}
}

func TestAppendFile(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "ledger.yaml")
	if err := os.WriteFile(p, []byte(ledger), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := appendFile(p, "nightly-9-1", 0, &out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "nightly-9-1") {
		t.Fatalf("%s", b)
	}
	if err := appendFile(p, "", 0, &out); err == nil {
		t.Fatal("missing run id must error")
	}
}
