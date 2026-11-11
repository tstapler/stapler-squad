package termcheck_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/tools/promo/internal/termcheck"
	"github.com/tstapler/stapler-squad/internal/testenv"
	"github.com/tstapler/stapler-squad/tools/promolib/gitbase"
	"github.com/tstapler/stapler-squad/tools/promolib/gitbase/gitbasetest"
	"github.com/tstapler/stapler-squad/tools/promolib/termscan"
)

func term() string { return strings.Join([]string{"wh", "40", "k"}, "") }

func run(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := termcheck.Run(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestRun_SourceMode(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.LinearRepo(t, gitbasetest.Files{
		"web-app/legacy.ts": []byte("id = " + term()),
		"server/other.go":   []byte(term()),
	})
	code, out, _ := run("-repo", dir)
	assert.Equal(t, termcheck.ExitFindings, code)
	assert.Contains(t, out, "server/other.go:1")
	assert.Contains(t, out, "web-app/legacy.ts:1")
}

func TestRun_AllowlistFromWorkingTreeAndStale(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.LinearRepo(t, gitbasetest.Files{"web-app/legacy.ts": []byte(term()), "ok.go": []byte("x")})
	list := filepath.Join(dir, "tools", "promo", "termcheck")
	require.NoError(t, os.MkdirAll(list, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(list, "allowlist.txt"), []byte("web-app/legacy.ts:legacy alias\nok.go:stale\n"), 0o644))

	code, out, _ := run("-repo", dir)
	assert.Equal(t, termcheck.ExitFindings, code)
	assert.Contains(t, out, "stale allow-list entry")
	assert.Contains(t, out, "ok.go:stale")
	assert.NotContains(t, out, "web-app/legacy.ts:1")
}

func TestRun_AbsentBaseAllowlistIsEmptyAndPRCopyIgnored(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t,
		gitbasetest.Files{"a.go": []byte("x")},
		gitbasetest.Files{"web-app/legacy.ts": []byte(term()), termcheck.AllowlistPath: []byte("web-app/legacy.ts:self-allowed\n")})
	code, out, _ := run("-repo", dir, "-base", "HEAD^1")
	assert.Equal(t, termcheck.ExitFindings, code)
	assert.Contains(t, out, "web-app/legacy.ts:1")
}

func TestRun_MalformedAllowlistIsExit2(t *testing.T) {
	t.Parallel()
	dir := gitbasetest.PRRepo(t,
		gitbasetest.Files{termcheck.AllowlistPath: []byte("no-reason-here\n")},
		gitbasetest.Files{"b.go": []byte("x")})
	code, _, errOut := run("-repo", dir, "-base", "HEAD^1")
	assert.Equal(t, termcheck.ExitError, code)
	assert.Contains(t, errOut, "path:reason")
}

func TestRun_DistMode(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "tours", "x"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "tours", "x", "index.html"), []byte(`<img alt="`+term()+`">`), 0o644))
	code, out, _ := run("-mode", "dist", "-dir", dir)
	assert.Equal(t, termcheck.ExitFindings, code)
	assert.Contains(t, out, "tours/x/index.html:1")
}

func TestRun_UsageErrors(t *testing.T) {
	t.Parallel()
	for _, args := range [][]string{{"-mode", "dist"}, {"-mode", "bogus"}, {"-nope"}} {
		code, _, _ := run(args...)
		assert.Equal(t, termcheck.ExitError, code, args)
	}
}

func TestRun_LeakPatterns(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("ok\nsee /home/bob/x\n"), 0o644))
	code, out, _ := run("-patterns", "leak", "-dir", dir)
	assert.Equal(t, termcheck.ExitFindings, code)
	assert.Contains(t, out, "a.txt:2")
	clean := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(clean, "a.txt"), []byte("3m ago"), 0o644))
	code, _, _ = run("-patterns", "leak", "-dir", clean)
	assert.Equal(t, termcheck.ExitOK, code)
	code, _, _ = run("-patterns", "leak")
	assert.Equal(t, termcheck.ExitError, code)
	code, _, _ = run("-patterns", "nope")
	assert.Equal(t, termcheck.ExitError, code)
}

const ocrHeader = "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n"

func ocrRow(conf, text string) string {
	return "5\t1\t1\t1\t1\t1\t0\t0\t10\t10\t" + conf + "\t" + text + "\n"
}

func mediaDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644))
	}
	return dir
}

func TestRun_MediaTextOCRConfidenceAndAllowlist(t *testing.T) {
	t.Parallel()
	banned := strings.Join([]string{"war", "hammer"}, "")
	dir := mediaDir(t, map[string]string{
		"sure.tsv": ocrHeader + ocrRow("90.0", banned),
		"weak.tsv": ocrHeader + ocrRow("30.0", banned),
	})
	budgets := filepath.Join(t.TempDir(), "budgets.json")
	require.NoError(t, os.WriteFile(budgets, []byte(`{"clipMaxBytes":1,"imageMaxBytes":1,"distMaxBytes":1,"recordWallClockMaxMinutes":1,"clipMaxDurationMs":1,"ocrMinConfidence":60,"ocrFixtureMinRecall":0.8,"recordInputPaths":["x"]}`), 0o644))
	allow := filepath.Join(t.TempDir(), "allow.txt")

	code, out, _ := run("-mode=media-text", "-dir", dir, "-budgets", budgets, "-ocr-allowlist", allow)
	assert.Equal(t, termcheck.ExitFindings, code)
	assert.Contains(t, out, "sure: banned term")
	assert.Contains(t, out, "warning: weak: banned term")
	assert.NotContains(t, out, "\nweak:")

	require.NoError(t, os.WriteFile(allow, []byte("sure:confirmed false positive\n"), 0o644))
	code, out, _ = run("-mode=media-text", "-dir", dir, "-budgets", budgets, "-ocr-allowlist", allow)
	assert.Equal(t, termcheck.ExitOK, code, out)
	assert.Contains(t, out, "warning: weak")
}

func TestRun_MediaTextDOMTextBlocksAndAbsentConfigDefaults(t *testing.T) {
	t.Parallel()
	dir := mediaDir(t, map[string]string{"dom.txt": "x " + term()})
	missing := filepath.Join(t.TempDir(), "nope")
	code, out, _ := run("-mode=media-text", "-dir", dir, "-budgets", missing, "-ocr-allowlist", missing)
	assert.Equal(t, termcheck.ExitFindings, code)
	assert.Contains(t, out, "dom.txt:1")
}

func TestRun_MediaTextRequiresDir(t *testing.T) {
	t.Parallel()
	code, _, errOut := run("-mode=media-text")
	assert.Equal(t, termcheck.ExitError, code)
	assert.Contains(t, errOut, "-dir")
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found above the test directory")
		dir = parent
	}
}

// The allow-list must name exactly the files that contain a banned term: no
// missing entry (a leak) and no extra one (a stale exemption).
func TestAllowlistEqualsLegacyIdFileSet(t *testing.T) {
	t.Parallel()
	testenv.SkipUnderBazel(t, "scans the real git checkout")
	root := repoRoot(t)
	r, err := gitbase.Open(root)
	require.NoError(t, err)
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(termcheck.AllowlistPath)))
	require.NoError(t, err)
	allow, err := termscan.ParseAllowlist(data)
	require.NoError(t, err)

	res, err := termscan.Scan(termscan.GitSource{Repo: r, Root: root}, termscan.Options{})
	require.NoError(t, err)

	actual := map[string]bool{}
	for _, f := range res.Findings {
		actual[f.Path] = true
	}
	listed := map[string]bool{}
	for _, a := range allow {
		listed[a.Path] = true
	}
	assert.Equal(t, listed, actual, "allow-list entries vs files that actually contain a banned term")
}
