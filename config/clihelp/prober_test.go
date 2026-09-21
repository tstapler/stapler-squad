//go:build !windows

package clihelp

import (
	"bytes"
	"context"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeExec(t *testing.T, dir, name string, mode os.FileMode) string {
	t.Helper()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o600))
	require.NoError(t, os.Chmod(path, mode)) // chmod, not WriteFile perm: umask must not weaken the fixture
	return path
}

func hermeticProber(t *testing.T, dirs []string, opts ...Option) *Prober {
	t.Helper()
	all := append([]Option{WithLoginPath(func() []string { return dirs }), WithHome("/home/tyler")}, opts...)
	return NewProber(all...)
}

func TestProbe_should_ReturnNotFoundAndNeverRun_When_LookPathFails(t *testing.T) {
	p := hermeticProber(t, []string{t.TempDir()})
	for i := 0; i < 2; i++ {
		res := p.Probe(context.Background(), "nope", ProbeOpts{})
		assert.Equal(t, ProbeResult{Status: ProbeStatusNotFound}, res)
	}
}

func TestProbe_should_CallLookPathWithExpandedHome_When_TildeCommandAndWithHome(t *testing.T) {
	home := t.TempDir()
	want := writeExec(t, home, "tool", 0o755)
	var lookups int
	p := NewProber(WithHome(home), WithLookPath(func(string) (string, error) { lookups++; return "", fs.ErrNotExist }))

	res := p.Probe(context.Background(), `FOO=1 "~/tool" --x`, ProbeOpts{})

	assert.Equal(t, ProbeStatusFoundNoFlags, res.Status)
	assert.Equal(t, ResolvedPath(want), res.ResolvedPath)
	assert.Zero(t, lookups, "an expanded ~ path is absolute and needs no PATH lookup")
}

func TestProbe_should_TreatErrDotAsNotFound_When_LookPathReturnsErrDot(t *testing.T) {
	dir := t.TempDir()
	writeExec(t, dir, "tool", 0o755)
	p := hermeticProber(t, nil, WithLookPath(func(name string) (string, error) {
		return filepath.Join(dir, name), &exec.Error{Name: name, Err: exec.ErrDot}
	}))
	assert.Equal(t, ProbeStatusNotFound, p.Probe(context.Background(), "tool", ProbeOpts{}).Status)
}

func TestProbe_should_FindBinary_When_OnlyInLoginShellPathAndReportNotFoundWhen_AliasOnly(t *testing.T) {
	shellDir := t.TempDir()
	want := writeExec(t, shellDir, "proxy-tool", 0o755)
	p := hermeticProber(t, []string{shellDir})

	res := p.Probe(context.Background(), "proxy-tool --x", ProbeOpts{})
	assert.Equal(t, ProbeStatusFoundNoFlags, res.Status)
	assert.Equal(t, ResolvedPath(want), res.ResolvedPath)

	assert.Equal(t, ProbeStatusNotFound, p.Probe(context.Background(), "proxy-alias", ProbeOpts{}).Status)
}

func TestProbe_should_ReturnNotFound_When_DirectoryNonExecutableOrWorldWritable(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "adir"), 0o755))
	writeExec(t, dir, "plain", 0o644)
	writeExec(t, dir, "ww", 0o757)
	writeExec(t, dir, "good", 0o755)
	target := writeExec(t, dir, "target-ww", 0o777)
	require.NoError(t, os.Symlink(target, filepath.Join(dir, "link-to-ww")))
	p := hermeticProber(t, []string{dir})

	for _, cmd := range []string{"adir", "plain", "ww", filepath.Join(dir, "adir"), filepath.Join(dir, "plain"), "link-to-ww", filepath.Join(dir, "ww")} {
		assert.Equal(t, ProbeStatusNotFound, p.Probe(context.Background(), cmd, ProbeOpts{}).Status, cmd)
	}
	assert.Equal(t, ProbeStatusFoundNoFlags, p.Probe(context.Background(), "good", ProbeOpts{}).Status)
}

func TestProbe_should_NeverRun_When_WrapperCommand(t *testing.T) {
	dir := t.TempDir()
	envPath := writeExec(t, dir, "env", 0o755)
	writeExec(t, dir, "npx", 0o755)
	p := hermeticProber(t, []string{dir})

	res := p.Probe(context.Background(), "env -u X A=b claude", ProbeOpts{})
	assert.Equal(t, ProbeResult{Status: ProbeStatusFoundNoFlags, ResolvedPath: ResolvedPath(envPath), IsWrapper: true}, res)
	assert.True(t, p.Probe(context.Background(), "npx foo", ProbeOpts{}).IsWrapper)
}

func TestProbe_should_ResolveFirstTokenOnly_When_CommandHasDangerousArgs(t *testing.T) {
	dir := t.TempDir()
	want := writeExec(t, dir, "claude", 0o755)
	p := hermeticProber(t, []string{dir})

	res := p.Probe(context.Background(), "claude; rm -rf ~ && $(evil)", ProbeOpts{})
	assert.Equal(t, ProbeStatusNotFound, res.Status, "the token is the literal claude;")

	res = p.Probe(context.Background(), "claude --x $(evil)", ProbeOpts{})
	assert.Equal(t, ResolvedPath(want), res.ResolvedPath)
}

// captureLogs swaps slog's default handler; callers must not run in parallel.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestProbe_should_EmitExactlyOneProgramProbeLog_When_EachOutcome(t *testing.T) {
	dir := t.TempDir()
	writeExec(t, dir, "tool", 0o755)
	writeExec(t, dir, "env", 0o755)
	p := hermeticProber(t, []string{dir})

	for _, cmd := range []string{"tool", "nope", "", "env x", "-x"} {
		buf := captureLogs(t)
		p.Probe(context.Background(), cmd, ProbeOpts{})
		assert.Equal(t, 1, strings.Count(buf.String(), "msg=program_probe"), "command %q", cmd)
	}
}

func TestProbe_should_LogInfoForNotFound_When_Probed(t *testing.T) {
	buf := captureLogs(t)
	hermeticProber(t, nil).Probe(context.Background(), "nope", ProbeOpts{})
	line := buf.String()
	assert.Contains(t, line, "level=INFO")
	assert.Contains(t, line, "status=NOT_FOUND")
	for _, field := range []string{"command_token=nope", "resolved_path=", "is_wrapper=", "flags=", "duration_ms=", "cache_hit=", "truncated=", "confirmed=", "resolve_only="} {
		assert.Contains(t, line, field)
	}
}

func TestProbe_should_NotLogArgsOrEnvValues_When_CommandHasArgsAndAssignments(t *testing.T) {
	dir := t.TempDir()
	writeExec(t, dir, "claude", 0o755)
	buf := captureLogs(t)
	hermeticProber(t, []string{dir}).Probe(context.Background(), "FOO=secret claude --x argvalue123", ProbeOpts{})
	line := buf.String()
	assert.Contains(t, line, "command_token=claude")
	for _, leaked := range []string{"secret", "FOO", "--x", "argvalue123"} {
		assert.NotContains(t, line, leaked)
	}
}

func TestLookInDirs_should_SkipRelativeDirsAndNonExecutables_When_Searching(t *testing.T) {
	dir := t.TempDir()
	writeExec(t, dir, "tool", 0o644)
	_, err := lookInDirs("tool", []string{"relative/dir", dir})
	assert.Error(t, err)
	want := writeExec(t, dir, "tool2", 0o755)
	got, err := lookInDirs("tool2", []string{dir})
	require.NoError(t, err)
	assert.Equal(t, want, got)
}
