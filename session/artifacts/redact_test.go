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

func TestRedact_ShouldReplaceSecretValues_WhenFormIsQuotedOrUncommon(t *testing.T) {
	t.Parallel()
	jwt := "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0NTYifQ.c2lnbmF0dXJlMTIz"
	for name, tc := range map[string]struct{ in, want string }{
		"flag double quoted":   {`curl --token "abc123secret" x`, `curl --token [redacted] x`},
		"flag equals single":   {`run --password='hunter2' go`, `run --password=[redacted] go`},
		"flag quoted w/ space": {`run --api-key "abc def" go`, `run --api-key [redacted] go`},
		"env double quoted":    {`GITHUB_TOKEN="ghs_x" gh pr view`, `GITHUB_TOKEN=[redacted] gh pr view`},
		"export single spaced": {`export API_KEY='abc def'`, `export API_KEY=[redacted]`},
		"postgres userinfo":    {`psql postgres://admin:s3cret@db:5432/x`, `psql postgres://[redacted]@db:5432/x`},
		"mysql userinfo":       {`mysql://root:pw@host/db`, `mysql://[redacted]@host/db`},
		"curl -u":              {`curl -u alice:pw123 https://x.io`, `curl -u [redacted] https://x.io`},
		"curl --user quoted":   {`curl --user "alice:pw 1" https://x.io`, `curl --user [redacted] https://x.io`},
		"auth basic":           {`-H "Authorization: Basic dXNlcjpwYXNz"`, `-H "Authorization: [redacted]"`},
		"auth custom scheme":   {`Authorization: Token abc123def`, `Authorization: [redacted]`},
		"x-api-key header":     {`-H 'X-Api-Key: abc123def'`, `-H 'X-Api-Key: [redacted]'`},
		"mysql -p":             {`mysql -u root -p hunter2 db`, `mysql -u root -p [redacted] db`},
		"mysql -ppw":           {`mysql -u root -phunter2 db`, `mysql -u root -p[redacted] db`},
		"sshpass -p":           {`sshpass -p hunter2 ssh host`, `sshpass -p [redacted] ssh host`},
		"aws lowercase spaced": {`aws_secret_access_key = wJalrXUtnFEMI/K7MDENG`, `aws_secret_access_key = [redacted]`},
		"npm authToken":        {`//registry.npmjs.org/:_authToken=abc123xyz`, `//registry.npmjs.org/:_authToken=[redacted]`},
		"npm token shape":      {`token npm_` + strings.Repeat("a1", 18), `token [redacted]`},
		"jwt":                  {`see ` + jwt + ` end`, `see [redacted] end`},
		"password colon":       {`password: hunter2`, `password: [redacted]`},
		"json quoted":          {`{"password": "hunter2"}`, `{"password": [redacted]}`},
		"idempotent":           {`--token [redacted]`, `--token [redacted]`},
	} {
		if got := Redact(tc.in); got != tc.want {
			t.Errorf("%s: Redact(%q)\n got %q\nwant %q", name, tc.in, got, tc.want)
		}
	}
}

func TestRedact_ShouldLeaveOrdinaryText_WhenNoSecretIsPresent(t *testing.T) {
	t.Parallel()
	for _, in := range []string{
		`git commit -m "fix password reset"`,
		`https://github.com/o/r/pull/12`,
		`https://example.com:8080/path?x=1`,
		`git push -u origin main`,
		`curl -u`,
		`mysql -u root -p`,
		`set max_tokens: 100`,
		`the author wrote this`,
		`deploy --token --env prod`,
	} {
		if got := Redact(in); got != in {
			t.Errorf("Redact(%q) = %q, want unchanged", in, got)
		}
	}
}

func TestRedactedRecords_ShouldScrubWithoutMutatingInput(t *testing.T) {
	t.Parallel()
	in := RecordList{CILinkRecord{URL: "https://x.io/r?token=abcd1234"}}
	out := RedactedRecords(in)
	if got := out[0].(CILinkRecord).URL; strings.Contains(got, "abcd1234") {
		t.Fatalf("leak: %s", got)
	}
	if !strings.Contains(in[0].(CILinkRecord).URL, "abcd1234") {
		t.Fatal("input mutated")
	}
}
