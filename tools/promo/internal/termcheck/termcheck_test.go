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
	"github.com/tstapler/stapler-squad/tools/promolib/gitbase/gitbasetest"
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
