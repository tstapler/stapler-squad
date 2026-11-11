package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func spawn(label, mn, out, hash string) string {
	return `{"targetLabel":"` + label + `","mnemonic":"` + mn + `","listedOutputs":["` + out + `"],"cacheable":true,"actualOutputs":[{"path":"` + out + `","digest":{"hash":"` + hash + `"}}]}` + "\n"
}

func TestReadLogAndCompare(t *testing.T) {
	t.Parallel()
	a, err := ReadLog(strings.NewReader(spawn("//a", "GoCompilePkg", "o1", "aa") + spawn("//b", "BufGenTs", "o2", "11")))
	if err != nil {
		t.Fatal(err)
	}
	b, _ := ReadLog(strings.NewReader(spawn("//a", "GoCompilePkg", "o1", "aa") + spawn("//b", "BufGenTs", "o2", "22")))
	rep := Compare(a, b, nil)
	if !rep.Failed() || len(rep.Mismatches) != 1 || rep.Mismatches[0].Mnemonic != "BufGenTs" || rep.Compared != 2 {
		t.Fatalf("%+v", rep)
	}
	rep = Compare(a, b, map[string]bool{"BufGenTs": true})
	if rep.Failed() || len(rep.Mismatches) != 1 || !rep.Mismatches[0].Accepted {
		t.Fatalf("accepted downgrade must not fail: %+v", rep)
	}
}

func TestAcceptedActionThatMatchesIsFlagged(t *testing.T) {
	t.Parallel()
	a, _ := ReadLog(strings.NewReader(spawn("//b", "BufGenTs", "o2", "11")))
	rep := Compare(a, a, map[string]bool{"BufGenTs": true})
	if rep.Failed() || len(rep.NowMatching) != 1 || rep.NowMatching[0] != "BufGenTs" {
		t.Fatalf("%+v", rep)
	}
}

func TestUncacheableAndUnmatchedAreNotMismatches(t *testing.T) {
	t.Parallel()
	a, _ := ReadLog(strings.NewReader(`{"targetLabel":"//x","mnemonic":"M","listedOutputs":["o"],"actualOutputs":[{"path":"o","digest":{"hash":"1"}}]}` + "\n" + spawn("//only", "M", "p", "1")))
	b, _ := ReadLog(strings.NewReader(`{"targetLabel":"//x","mnemonic":"M","listedOutputs":["o"],"actualOutputs":[{"path":"o","digest":{"hash":"2"}}]}` + "\n"))
	rep := Compare(a, b, nil)
	if rep.Failed() || rep.OnlyA != 1 || rep.Compared != 0 {
		t.Fatalf("%+v", rep)
	}
}

func TestOutputPresentInOnlyOneBuildIsMismatch(t *testing.T) {
	t.Parallel()
	a, _ := ReadLog(strings.NewReader(`{"targetLabel":"//x","mnemonic":"M","listedOutputs":["o"],"cacheable":true,"actualOutputs":[{"path":"o","digest":{"hash":"1"}}]}`))
	b, _ := ReadLog(strings.NewReader(`{"targetLabel":"//x","mnemonic":"M","listedOutputs":["o"],"cacheable":true}`))
	if rep := Compare(a, b, nil); !rep.Failed() {
		t.Fatalf("%+v", rep)
	}
}

func TestReadLogRejectsGarbage(t *testing.T) {
	t.Parallel()
	if _, err := ReadLog(strings.NewReader("{not json")); err == nil {
		t.Fatal("want error")
	}
}

func TestParseDowngrades(t *testing.T) {
	t.Parallel()
	rows, err := ParseDowngrades(strings.NewReader("# c\n\nBufGenTs | buf | .bun-version | reason | pending\n"))
	if err != nil || len(rows) != 1 || rows[0].Action != "BufGenTs" || rows[0].Accepted != "pending" {
		t.Fatalf("%v %+v", err, rows)
	}
	for _, bad := range []string{"a | b | c | d\n", "a | b | | d | e\n"} {
		if _, err := ParseDowngrades(strings.NewReader(bad)); err == nil {
			t.Fatalf("want error for %q", bad)
		}
	}
}

func TestRunExitCodes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	a := write("a.json", spawn("//a", "M", "o", "1"))
	same := write("same.json", spawn("//a", "M", "o", "1"))
	diff := write("diff.json", spawn("//a", "M", "o", "2"))
	none := write("none.txt", "# no downgrades\n")
	accept := write("accept.txt", "M | tool | pin | why | 2026-11-06\n")
	var out, errb bytes.Buffer
	cases := []struct {
		args []string
		want int
	}{
		{[]string{"-a", a, "-b", same, "-downgrades", none}, 0},
		{[]string{"-a", a, "-b", diff, "-downgrades", none}, 1},
		{[]string{"-a", a, "-b", diff, "-downgrades", accept}, 0},
		{[]string{"-a", a, "-downgrades", none}, 2},
		{[]string{"-a", a, "-b", filepath.Join(dir, "missing"), "-downgrades", none}, 2},
	}
	for _, c := range cases {
		out.Reset()
		if got := run(c.args, &out, &errb); got != c.want {
			t.Errorf("%v: exit %d want %d\n%s%s", c.args, got, c.want, out.String(), errb.String())
		}
	}
	out.Reset()
	run(cases[1].args, &out, &errb)
	if !strings.Contains(out.String(), "MISMATCH M //a o") {
		t.Errorf("mismatch line must name the mnemonic and label: %s", out.String())
	}
}
