//go:build !windows

package clihelp

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProbe_should_NotRunScript_When_ImplicitOrResolveOnlyProbe(t *testing.T) {
	dir, path := toolIn(t, "script")
	f := newFakeRun(twoFlagHelp)
	p := runProber(t, []string{dir}, f, shebangHd())

	for _, opts := range []ProbeOpts{{}, {}, {ResolveOnly: true}, {ResolveOnly: true, ConfirmExecute: true}} {
		res := p.Probe(context.Background(), "script", opts)
		assert.Equal(t, ProbeResult{Status: ProbeStatusNeedsConfirm, ResolvedPath: ResolvedPath(path)}, res)
	}
	assert.Zero(t, f.calls.Load())
}

func TestProbe_should_RunOnceAndRememberConsent_When_ConfirmExecuteThenExpiredCache(t *testing.T) {
	dir, path := toolIn(t, "script")
	f := newFakeRun(twoFlagHelp)
	clock := &fakeClock{t: time.Unix(1000, 0)}
	p := runProber(t, []string{dir}, f, shebangHd(), WithClock(clock.Now))

	assert.Equal(t, ProbeStatusNeedsConfirm, p.Probe(context.Background(), "script", ProbeOpts{}).Status, "NEEDS_CONFIRM is not cached")
	res := p.Probe(context.Background(), "script", ProbeOpts{ConfirmExecute: true})
	assert.Equal(t, ProbeStatusFoundParsed, res.Status)
	assert.EqualValues(t, 1, f.calls.Load())

	clock.Advance(11 * time.Minute)
	assert.Equal(t, ProbeStatusFoundParsed, p.Probe(context.Background(), "script", ProbeOpts{}).Status,
		"a confirmed target runs implicitly after cache expiry")
	assert.EqualValues(t, 2, f.calls.Load())

	require.NoError(t, os.Chtimes(path, time.Now(), time.Now().Add(time.Hour)))
	assert.Equal(t, ProbeStatusNeedsConfirm, p.Probe(context.Background(), "script", ProbeOpts{}).Status,
		"a changed mtime needs a fresh confirmation")
	assert.EqualValues(t, 2, f.calls.Load())
}

func TestProbe_should_RunNativeImplicitly_When_HeadIsELF(t *testing.T) {
	dir, _ := toolIn(t, "bin")
	f := newFakeRun(twoFlagHelp)
	assert.Equal(t, ProbeStatusFoundParsed, runProber(t, []string{dir}, f, elfHead()).Probe(context.Background(), "bin", ProbeOpts{}).Status)
	assert.EqualValues(t, 1, f.calls.Load())
}

func TestProbe_should_RunOnlyNativeMagics_When_HeadsVary(t *testing.T) {
	for head, native := range map[string]bool{
		"\x7fELF": true, "\xcf\xfa\xed\xfe": true, "\xce\xfa\xed\xfe": true,
		"\xfe\xed\xfa\xce": true, "\xfe\xed\xfa\xcf": true, "\xca\xfe\xba\xbe": true,
		"#!/b": false, "hell": false, "": false,
	} {
		assert.Equal(t, native, isNative([]byte(head)), "%q", head)
	}
}

func TestProbe_should_TreatAsNonNative_When_ReadHeadFails(t *testing.T) {
	dir, _ := toolIn(t, "bin")
	f := newFakeRun(twoFlagHelp)
	p := hermeticProber(t, []string{dir}, WithRun(f.run), WithReadHead(func(string) ([]byte, error) { return nil, errors.New("eio") }))
	assert.Equal(t, ProbeStatusNeedsConfirm, p.Probe(context.Background(), "bin", ProbeOpts{}).Status)
	assert.Zero(t, f.calls.Load())
}

func TestProbe_should_ServeCachedFlags_When_ResolveOnlyHitsCache(t *testing.T) {
	dir, _ := toolIn(t, "bin")
	f := newFakeRun(twoFlagHelp)
	p := runProber(t, []string{dir}, f, elfHead())
	p.Probe(context.Background(), "bin", ProbeOpts{})

	res := p.Probe(context.Background(), "bin", ProbeOpts{ResolveOnly: true})
	assert.Equal(t, ProbeStatusFoundParsed, res.Status)
	assert.True(t, res.CacheHit)
	assert.EqualValues(t, 1, f.calls.Load())
}

func TestProbe_should_ReturnNeedsConfirm_When_ResolveOnlyOnNativeWithoutCache(t *testing.T) {
	dir, _ := toolIn(t, "bin")
	f := newFakeRun(twoFlagHelp)
	res := runProber(t, []string{dir}, f, elfHead()).Probe(context.Background(), "bin", ProbeOpts{ResolveOnly: true})
	assert.Equal(t, ProbeStatusNeedsConfirm, res.Status)
	assert.Zero(t, f.calls.Load())
}

func TestProbe_should_HonourConfirmedSet_When_ResolveOnlyAfterExpiryOrMtimeChangeOrRestart(t *testing.T) {
	dir, path := toolIn(t, "script")
	f := newFakeRun(twoFlagHelp)
	clock := &fakeClock{t: time.Unix(1000, 0)}
	p := runProber(t, []string{dir}, f, shebangHd(), WithClock(clock.Now))
	p.Probe(context.Background(), "script", ProbeOpts{ConfirmExecute: true})

	clock.Advance(11 * time.Minute)
	res := p.Probe(context.Background(), "script", ProbeOpts{ResolveOnly: true})
	assert.Equal(t, ProbeStatusFoundParsed, res.Status, "a confirmed key never regresses to NEEDS_CONFIRM on resolve-only")
	assert.EqualValues(t, 2, f.calls.Load())

	restarted := runProber(t, []string{dir}, newFakeRun(twoFlagHelp), shebangHd())
	assert.Equal(t, ProbeStatusNeedsConfirm, restarted.Probe(context.Background(), "script", ProbeOpts{ResolveOnly: true}).Status,
		"consent is in memory only")

	require.NoError(t, os.Chtimes(path, time.Now(), time.Now().Add(time.Hour)))
	assert.Equal(t, ProbeStatusNeedsConfirm, p.Probe(context.Background(), "script", ProbeOpts{ResolveOnly: true}).Status)
}

func TestProbe_should_OnlyExecuteScriptOnConfirm_When_RealRunnerAndMarkerFile(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "marker")
	script := filepath.Join(dir, "tool")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\n: > "+marker+"\necho '  --alpha  first flag'\n"), 0o700))
	p := hermeticProber(t, []string{dir}, WithDefaultRunner())

	res := p.Probe(context.Background(), script, ProbeOpts{})
	assert.Equal(t, ProbeStatusNeedsConfirm, res.Status)
	_, err := os.Stat(marker)
	assert.ErrorIs(t, err, os.ErrNotExist)

	res = p.Probe(context.Background(), script, ProbeOpts{ConfirmExecute: true})
	assert.Equal(t, ProbeStatusFoundParsed, res.Status)
	_, err = os.Stat(marker)
	assert.NoError(t, err)
}
