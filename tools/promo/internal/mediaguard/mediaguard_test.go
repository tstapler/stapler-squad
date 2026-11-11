package mediaguard_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/tools/promo/internal/codeowners"
	"github.com/tstapler/stapler-squad/tools/promo/internal/mediaguard"
	"github.com/tstapler/stapler-squad/tools/promolib/gitbase/gitbasetest"
)

type files = gitbasetest.Files

func guard(t *testing.T, dir string, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	code := mediaguard.RunEnv(append([]string{"-repo", dir}, args...), &out, &errb, func(k string) string { return env[k] })
	return code, out.String(), errb.String()
}

func bytesOf(n int) []byte { return bytes.Repeat([]byte("x"), n) }

func event(kind, login, typ, at string) string {
	return fmt.Sprintf(`{"event":%q,"label":{"name":"allow-media"},"actor":{"login":%q,"type":%q},"created_at":%q}`, kind, login, typ, at)
}

func writeEvents(t *testing.T, evs ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "events.json")
	require.NoError(t, os.WriteFile(p, []byte("["+strings.Join(evs, ",")+"]"), 0o644))
	return p
}

func TestRun_AddedMediaOutsideAllowlistFails(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t, files{"a": []byte("a"), mediaguard.AllowlistPath: []byte("web-app/public/icon-*.png\n")},
		files{"website/public/clips/new.webm": bytesOf(10)})
	code, out, _ := guard(t, dir, nil)
	assert.Equal(t, mediaguard.ExitFindings, code)
	assert.Contains(t, out, "website/public/clips/new.webm")
	assert.Contains(t, out, mediaguard.AllowlistPath)
}

func TestRun_AllowedDeletedAndUnrelatedPass(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t,
		files{"assets/demo.gif": bytesOf(5), mediaguard.AllowlistPath: []byte("# icons\nweb-app/public/icon-*.png\n")},
		files{"web-app/public/icon-192.png": bytesOf(5), "assets/demo.gif": nil, "README.md": []byte("hi")})
	code, out, _ := guard(t, dir, nil)
	assert.Equal(t, mediaguard.ExitOK, code, out)
}

func TestRun_SizeBackstop(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t, files{"a": []byte("a")}, files{"data.json": bytesOf(900 * 1024)})
	code, out, _ := guard(t, dir, nil)
	assert.Equal(t, mediaguard.ExitFindings, code)
	assert.Contains(t, out, "data.json")
}

func TestRun_ModifiedLargeFilePasses(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t, files{"big.json": bytesOf(900 * 1024)}, files{"big.json": bytesOf(900*1024 + 1)})
	code, out, _ := guard(t, dir, nil)
	assert.Equal(t, mediaguard.ExitOK, code, out)
}

func TestRun_OverrideAuthority(t *testing.T) {
	t.Parallel()
	base := func(owners string) files {
		return files{mediaguard.CodeownersPath: []byte(".github/promo-flags.json " + owners + "\n")}
	}
	pr := files{"website/public/demo.webm": bytesOf(8)}
	tests := []struct {
		name     string
		owners   string
		events   []string
		labelled bool
		want     int
		contains string
	}{
		{"no label", "@alice", nil, false, 1, ""},
		{"listed user", "@alice", []string{event("labeled", "alice", "User", "2026-01-01T00:00:00Z")}, true, 0, "allow-media"},
		{"unlisted user", "@alice", []string{event("labeled", "mallory", "User", "2026-01-01T00:00:00Z")}, true, 1, "mallory"},
		{"newest event decides: re-added by other actor", "@alice", []string{
			event("labeled", "alice", "User", "2026-01-01T00:00:00Z"),
			event("unlabeled", "alice", "User", "2026-01-02T00:00:00Z"),
			event("labeled", "mallory", "User", "2026-01-03T00:00:00Z")}, true, 1, "mallory"},
		{"newest event is unlabeled", "@alice", []string{
			event("labeled", "alice", "User", "2026-01-01T00:00:00Z"),
			event("unlabeled", "alice", "User", "2026-01-02T00:00:00Z")}, true, 1, "removed"},
		{"bot never counts even if listed", "@dependabot[bot]", []string{event("labeled", "dependabot[bot]", "Bot", "2026-01-01T00:00:00Z")}, true, 1, "bot"},
		{"team only entry fails closed", "@org/maintainers", []string{event("labeled", "alice", "User", "2026-01-01T00:00:00Z")}, true, 1, "teams cannot be resolved with GITHUB_TOKEN"},
		{"team plus user is honoured", "@org/maintainers @alice", []string{event("labeled", "alice", "User", "2026-01-01T00:00:00Z")}, true, 0, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := gitbasetest.PRRepo(t, base(tt.owners), pr)
			args := []string{}
			if tt.labelled {
				args = append(args, "-allow-media")
			}
			if tt.events != nil {
				args = append(args, "-events-json", writeEvents(t, tt.events...))
			}
			code, out, _ := guard(t, dir, nil, args...)
			assert.Equal(t, tt.want, code, out)
			assert.Contains(t, out, tt.contains)
		})
	}
}

func TestRun_OverrideWritesJobSummaryWithByteTotal(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t, files{mediaguard.CodeownersPath: []byte(".github/promo-flags.json @alice\n")},
		files{"website/public/demo.webm": bytesOf(1234)})
	summary := filepath.Join(t.TempDir(), "summary.md")
	code, out, _ := guard(t, dir, map[string]string{"GITHUB_STEP_SUMMARY": summary},
		"-allow-media", "-events-json", writeEvents(t, event("labeled", "alice", "User", "2026-01-01T00:00:00Z")))
	require.Equal(t, mediaguard.ExitOK, code, out)
	got, err := os.ReadFile(summary)
	require.NoError(t, err)
	assert.Contains(t, string(got), "1234 bytes")
	assert.Contains(t, out, "1234 bytes")
}

func TestRun_GateInputsComeFromBaseNotPR(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t,
		files{"a": []byte("a"), ".github/promo-flags.json": []byte(`{"guardsEnforce": true}`)},
		files{
			".github/promo-flags.json":    []byte(`{"guardsEnforce": false}`),
			mediaguard.AllowlistPath:      []byte("website/public/clips/x.webm\n"),
			mediaguard.CodeownersPath:     []byte(".github/promo-flags.json @mallory\n"),
			"website/public/clips/x.webm": bytesOf(4),
		})
	code, out, _ := guard(t, dir, nil, "-allow-media",
		"-events-json", writeEvents(t, event("labeled", "mallory", "User", "2026-01-01T00:00:00Z")))
	assert.Equal(t, mediaguard.ExitFindings, code)
	assert.Contains(t, out, "website/public/clips/x.webm")
	assert.Contains(t, out, "base has no .github/CODEOWNERS")
}

func TestRun_AbsentInputsTakeSafeDefaults(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t, files{"a": []byte("a")},
		files{"website/public/clips/x.webm": bytesOf(4), "web-app/public/icon.png": bytesOf(4)})
	code, out, _ := guard(t, dir, nil, "-allow-media",
		"-events-json", writeEvents(t, event("labeled", "alice", "User", "2026-01-01T00:00:00Z")))
	assert.Equal(t, mediaguard.ExitFindings, code)
	assert.Contains(t, out, "web-app/public/icon.png")
	assert.Contains(t, out, "nobody holds override authority")
}

func TestRun_MalformedAllowlistIsExit2(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t, files{mediaguard.AllowlistPath: []byte("[unclosed\n")}, files{"b": []byte("b")})
	code, _, errOut := guard(t, dir, nil)
	assert.Equal(t, mediaguard.ExitError, code)
	assert.Contains(t, errOut, "invalid glob")
}

func TestRun_WrongBaseFailsLoudly(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.LinearRepo(t, files{"a": []byte("a")})
	code, _, errOut := guard(t, dir, nil)
	assert.Equal(t, mediaguard.ExitError, code)
	assert.Contains(t, errOut, "-base")
}

func TestRealGateInputsParse(t *testing.T) {
	t.Parallel()
	root := filepath.Join("..", "..", "..", "..")
	al, err := os.ReadFile(filepath.Join(root, mediaguard.AllowlistPath))
	require.NoError(t, err)
	list, err := mediaguard.ParseAllowlist(al)
	require.NoError(t, err)
	assert.True(t, list.Allows("web-app/public/icons/icon-192.png"))
	assert.True(t, list.Allows("project_plans/x/design/mock.png"))
	assert.False(t, list.Allows("website/public/clips/new.webm"))

	co, err := os.ReadFile(filepath.Join(root, mediaguard.CodeownersPath))
	require.NoError(t, err)
	owners := codeowners.Parse(co)
	for _, p := range []string{".github/promo-flags.json", "tools/promo/cmd/promoguard/main.go", "tools/promolib/gitbase/gitbase.go",
		"tools/docscoverage/x.go", mediaguard.AllowlistPath, "tools/promo/termcheck/allowlist.txt",
		".github/workflows/backlog-scaffolding-guard.yml", ".github/workflows/deploy-website.yml"} {
		users, _ := codeowners.Split(owners.Owners(p))
		assert.NotEmpty(t, users, p)
	}
}
