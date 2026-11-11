package termscan

import "testing"

func TestLeakPatternsMatchKnownBadStrings(t *testing.T) {
	t.Parallel()
	bad := []string{"/home/alice/x", "/Users/alice/x", "/tmp/tmp.abc", "ghp_abc123", "box.internal", "a@b.com", "runneradmin"}
	for _, s := range bad {
		found := false
		for _, p := range LeakPatterns() {
			if p.Find(s) != "" {
				found = true
			}
		}
		if !found {
			t.Errorf("no pattern matches %q", s)
		}
	}
}

func TestLeakPatternsPassBenignText(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"Approve the push", "3m ago", "internal review", "test runner", "services/payments"} {
		for _, p := range LeakPatterns() {
			if m := p.Find(s); m != "" {
				t.Errorf("%s matched %q in %q", p.Name, m, s)
			}
		}
	}
}

func TestScanLeaksReportsLineAndHonoursIgnore(t *testing.T) {
	t.Parallel()
	src := MemSource{
		"dom-text/a.txt": []byte("ok\nsee /home/bob/repo\n"),
		"dom-text/b.txt": []byte("/Users/dev/services/x\n"),
		"dom-text/c.bin": []byte("a\x00/home/"),
	}
	got, err := ScanLeaks(src, []string{"/Users/dev/"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "dom-text/a.txt" || got[0].Line != 2 {
		t.Fatalf("findings = %v", got)
	}
}
