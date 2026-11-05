package artifacts

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedact_ShouldReplaceSecretValues_WhenTokenInURLOrFlag(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"https://x.io/cb?token=abcd1234":             "https://x.io/cb?token=[redacted]",
		"https://x.io/cb?a=1&api_key=abcd1234&b=2":   "https://x.io/cb?a=1&api_key=[redacted]&b=2",
		"deploy --token abcd1234 --env prod":         "deploy --token [redacted] --env prod",
		"deploy --api-key=abcd1234":                  "deploy --api-key=[redacted]",
		"GITHUB_TOKEN=abcd1234 gh pr view 1":         "GITHUB_TOKEN=[redacted] gh pr view 1",
		"curl https://user:hunter2@host.io/x":        "curl https://[redacted]@host.io/x",
		"auth ghp_abcdefghijklmnopqrstuvwxyz0123456": "auth [redacted]",
		"no secrets here --title hello":              "no secrets here --title hello",
	} {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%q)\n got %q\nwant %q", in, got, want)
		}
	}
}

func TestScan_ShouldNotStoreSecrets_WhenCommandOrURLCarriesToken(t *testing.T) {
	t.Parallel()
	f := scanLines(
		toolUseLine(t, "2026-11-04T10:00:00Z", "a", `git commit -m "x" --token abcd1234`),
		toolResultLine(t, "2026-11-04T10:00:01Z", "a", "see https://x.io/cb?token=abcd1234 [main 1a2b3c4] x"),
	)
	blob := &SessionArtifactsBlob{ExternalURLs: f.externalURLs, Commands: f.commands, Records: f.records}
	raw, _ := json.Marshal(blob)
	if strings.Contains(string(raw), "abcd1234") {
		t.Fatalf("secret stored: %s", raw)
	}
	if !strings.Contains(string(raw), "[redacted]") {
		t.Fatalf("nothing redacted: %s", raw)
	}
}

func TestBlobRedacted_ShouldCleanLegacyBlobsForExport(t *testing.T) {
	t.Parallel()
	b := &SessionArtifactsBlob{
		ExternalURLs: []string{"https://x.io/cb?token=abcd1234"},
		Commands:     []CommandArtifact{{Command: "x --token abcd1234", Detail: "d"}},
		Records:      RecordList{CILinkRecord{URL: "https://x.io/actions/runs/1?token=abcd1234"}},
	}
	raw, _ := json.Marshal(b.Redacted())
	if strings.Contains(string(raw), "abcd1234") {
		t.Fatalf("export leaks: %s", raw)
	}
	orig, _ := json.Marshal(b)
	if !strings.Contains(string(orig), "abcd1234") {
		t.Fatal("Redacted must not mutate the receiver")
	}
}
