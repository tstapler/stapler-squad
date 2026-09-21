package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"

	"github.com/tstapler/stapler-squad/config/clihelp"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func newProbeExecutable(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(path, []byte("#!/bin/sh\n"), 0o700))
	return path
}

func probe(t *testing.T, d *DefaultsService, command string) *sessionv1.ProbeProgramResponse {
	t.Helper()
	resp, err := d.ProbeProgram(context.Background(), connect.NewRequest(&sessionv1.ProbeProgramRequest{Command: command}))
	require.NoError(t, err)
	return resp.Msg
}

func TestProbeResultToProto_should_DeriveFoundFromStatus_When_EachProbeStatus(t *testing.T) {
	tests := []struct {
		in    clihelp.ProbeStatus
		want  sessionv1.ProbeStatus
		found bool
	}{
		{clihelp.ProbeStatusFoundParsed, sessionv1.ProbeStatus_PROBE_STATUS_FOUND_PARSED, true},
		{clihelp.ProbeStatusFoundNoFlags, sessionv1.ProbeStatus_PROBE_STATUS_FOUND_NO_FLAGS, true},
		{clihelp.ProbeStatusTimeout, sessionv1.ProbeStatus_PROBE_STATUS_TIMEOUT, true},
		{clihelp.ProbeStatusNeedsConfirm, sessionv1.ProbeStatus_PROBE_STATUS_NEEDS_CONFIRM, true},
		{clihelp.ProbeStatusNotFound, sessionv1.ProbeStatus_PROBE_STATUS_NOT_FOUND, false},
		{clihelp.ProbeStatusError, sessionv1.ProbeStatus_PROBE_STATUS_ERROR, false},
		{clihelp.ProbeStatusBusy, sessionv1.ProbeStatus_PROBE_STATUS_BUSY, false},
		{clihelp.ProbeStatusUnspecified, sessionv1.ProbeStatus_PROBE_STATUS_UNSPECIFIED, false},
	}
	for _, tt := range tests {
		t.Run(tt.in.String(), func(t *testing.T) {
			got := probeResultToProto(clihelp.ProbeResult{Status: tt.in})
			assert.Equal(t, tt.want, got.ProbeStatus)
			assert.Equal(t, tt.found, got.Found)
		})
	}
}

func TestProbeProgram_should_ReturnInvalidArgument_When_CommandEmpty(t *testing.T) {
	d := NewDefaultsService()
	for _, cmd := range []string{"", "   "} {
		_, err := d.ProbeProgram(context.Background(), connect.NewRequest(&sessionv1.ProbeProgramRequest{Command: cmd}))
		require.Error(t, err)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	}
}

func TestProbeProgram_should_ReturnNotFoundWithNilError_When_TildePathMissing(t *testing.T) {
	home := t.TempDir()
	d := NewDefaultsService()
	d.SetProber(clihelp.NewProber(clihelp.WithHome(home), clihelp.WithLoginPath(func() []string { return nil })))

	got := probe(t, d, "FOO=1 ~/bin/nope --x")
	assert.False(t, got.Found)
	assert.Equal(t, sessionv1.ProbeStatus_PROBE_STATUS_NOT_FOUND, got.ProbeStatus)
	assert.Empty(t, got.ResolvedPath)
}

func TestProbeProgram_should_ReturnFoundWithPath_When_CommandResolvesViaLoginPath(t *testing.T) {
	path := newProbeExecutable(t, "alpha")
	d := NewDefaultsService()
	d.SetProber(clihelp.NewProber(clihelp.WithLoginPath(func() []string { return []string{filepath.Dir(path)} })))

	got := probe(t, d, "alpha --dangerously-skip-permissions")
	assert.True(t, got.Found)
	assert.Equal(t, sessionv1.ProbeStatus_PROBE_STATUS_FOUND_NO_FLAGS, got.ProbeStatus)
	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	assert.Equal(t, resolved, got.ResolvedPath)
	assert.False(t, got.IsWrapper)
}

func TestProbeProgram_should_FlagWrapperAndNotRun_When_EnvCommand(t *testing.T) {
	path := newProbeExecutable(t, "env")
	d := NewDefaultsService()
	d.SetProber(clihelp.NewProber(clihelp.WithLoginPath(func() []string { return []string{filepath.Dir(path)} })))

	got := probe(t, d, "env -u FOO claude")
	assert.True(t, got.Found)
	assert.True(t, got.IsWrapper)
}

func TestNewDefaultsService_should_SpawnNoProcessAndNoGoroutine_When_Constructed(t *testing.T) {
	defer goleak.VerifyNone(t)
	d := NewDefaultsService()
	require.NotNil(t, d.prober)
	_ = probe(t, d, "definitely-not-a-real-command-xyz")
}
