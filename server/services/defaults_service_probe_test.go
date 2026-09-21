package services

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
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

func elfHead(string) ([]byte, error) { return []byte{0x7f, 'E', 'L', 'F'}, nil }

// runnerProber is a real Prober whose runner returns out/err and counts calls.
func runnerProber(dir string, calls *atomic.Int32, out clihelp.RunOutput, extra ...clihelp.Option) *clihelp.Prober {
	run := func(context.Context, clihelp.ResolvedPath, clihelp.Limits) (clihelp.RunOutput, error) {
		calls.Add(1)
		return out, nil
	}
	opts := make([]clihelp.Option, 0, 3+len(extra))
	opts = append(opts,
		clihelp.WithLoginPath(func() []string { return []string{dir} }),
		clihelp.WithRun(run),
		clihelp.WithReadHead(elfHead),
	)
	return clihelp.NewProber(append(opts, extra...)...)
}

func TestProbeResultToProto_should_MapFlagsAliasesAndTruncated_When_ResultHasFlags(t *testing.T) {
	got := probeResultToProto(clihelp.ProbeResult{
		Status:    clihelp.ProbeStatusFoundParsed,
		Truncated: true,
		Flags: []clihelp.Flag{
			{Name: "--allowedTools", Short: "-a", Aliases: []string{"--allowed-tools"}, TakesValue: true, Description: "tools"},
			{Name: "--verbose"},
		},
	})
	assert.True(t, got.Truncated)
	require.Len(t, got.Flags, 2)
	assert.Equal(t, "--allowedTools", got.Flags[0].Name)
	assert.Equal(t, "-a", got.Flags[0].Short)
	assert.Equal(t, []string{"--allowed-tools"}, got.Flags[0].Aliases)
	assert.True(t, got.Flags[0].TakesValue)
	assert.Equal(t, "tools", got.Flags[0].Description)
	assert.Equal(t, "--verbose", got.Flags[1].Name)
	assert.Empty(t, probeResultToProto(clihelp.ProbeResult{Status: clihelp.ProbeStatusNotFound}).Flags)
}

func TestProbeProgram_should_ReachProberOptsUnchanged_When_ConfirmAndResolveOnlySet(t *testing.T) {
	path := newProbeExecutable(t, "script") // #! head, so non-native
	var calls atomic.Int32
	d := NewDefaultsService()
	d.SetProber(clihelp.NewProber(
		clihelp.WithLoginPath(func() []string { return []string{filepath.Dir(path)} }),
		clihelp.WithRun(func(context.Context, clihelp.ResolvedPath, clihelp.Limits) (clihelp.RunOutput, error) {
			calls.Add(1)
			return clihelp.RunOutput{Text: "  --x  y\n"}, nil
		}),
	))
	call := func(confirm, resolveOnly bool) *sessionv1.ProbeProgramResponse {
		resp, err := d.ProbeProgram(context.Background(), connect.NewRequest(&sessionv1.ProbeProgramRequest{
			Command: "script", ConfirmExecute: confirm, ResolveOnly: resolveOnly,
		}))
		require.NoError(t, err)
		return resp.Msg
	}

	blur := call(false, false)
	assert.Equal(t, sessionv1.ProbeStatus_PROBE_STATUS_NEEDS_CONFIRM, blur.ProbeStatus)
	assert.True(t, blur.Found)
	assert.Equal(t, sessionv1.ProbeStatus_PROBE_STATUS_NEEDS_CONFIRM, call(true, true).ProbeStatus, "resolve_only wins")
	assert.Zero(t, calls.Load())

	checked := call(true, false)
	assert.Equal(t, sessionv1.ProbeStatus_PROBE_STATUS_FOUND_PARSED, checked.ProbeStatus)
	assert.EqualValues(t, 1, calls.Load())
}

func TestProbeProgram_should_PreserveStatusAndReportNotFound_When_TimeoutOrRunnerBusy(t *testing.T) {
	path := newProbeExecutable(t, "slow")
	var calls atomic.Int32
	d := NewDefaultsService()
	d.SetProber(runnerProber(filepath.Dir(path), &calls, clihelp.RunOutput{TimedOut: true}))

	got := probe(t, d, "slow")
	assert.True(t, got.Found, "TIMEOUT is found: the binary exists")
	assert.Equal(t, sessionv1.ProbeStatus_PROBE_STATUS_TIMEOUT, got.ProbeStatus)
	assert.Empty(t, got.Flags)
}

// The aider fixture is the shape of a real --help; a fake runner feeds it
// through the real Prober, parser and handler.
func TestProbeProgram_should_ReturnAiderFlags_When_RealProberRunsFakeAiderHelp(t *testing.T) {
	help, err := os.ReadFile(filepath.Join("..", "..", "config", "clihelp", "testdata", "help", "aider-0.78.0.txt"))
	require.NoError(t, err)
	path := newProbeExecutable(t, "aider")
	var calls atomic.Int32
	d := NewDefaultsService()
	d.SetProber(runnerProber(filepath.Dir(path), &calls, clihelp.RunOutput{Text: clihelp.HelpText(help)}))

	got := probe(t, d, "aider --model x")

	resolved, err := filepath.EvalSymlinks(path)
	require.NoError(t, err)
	assert.True(t, got.Found)
	assert.Equal(t, resolved, got.ResolvedPath)
	assert.Equal(t, sessionv1.ProbeStatus_PROBE_STATUS_FOUND_PARSED, got.ProbeStatus)
	assert.False(t, got.Truncated)
	assert.False(t, got.IsWrapper)
	byName := map[string]*sessionv1.FlagInfo{}
	for _, f := range got.Flags {
		byName[f.Name] = f
	}
	require.Contains(t, byName, "--model")
	assert.True(t, byName["--model"].TakesValue)
	assert.NotEmpty(t, byName["--model"].Description)
	assert.Greater(t, len(got.Flags), 20)
	assert.EqualValues(t, 1, calls.Load())
}
