package services

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// newIsolatedLauncherPresetsService creates a LauncherPresetsService backed by a fresh
// temporary directory, preventing config state from leaking between tests.
func newIsolatedLauncherPresetsService(t *testing.T) (*LauncherPresetsService, string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", dir)
	return NewLauncherPresetsService(), dir
}

func writePresetsFixture(t *testing.T, dir, contents string) {
	t.Helper()
	path := filepath.Join(dir, "launcher-presets.json")
	require.NoError(t, os.WriteFile(path, []byte(contents), 0644))
}

func TestGetLauncherPresets_should_ReturnPresetsWithEmptyLoadError_When_FileIsValid(t *testing.T) {
	svc, dir := newIsolatedLauncherPresetsService(t)
	writePresetsFixture(t, dir, `{
		"version": 1,
		"presets": [
			{"id": "codex-gpt5", "label": "Codex GPT-5", "argv": ["codex", "--model", "gpt-5"]}
		]
	}`)

	resp, err := svc.GetLauncherPresets(context.Background(), connect.NewRequest(&sessionv1.GetLauncherPresetsRequest{}))
	require.NoError(t, err)
	require.Empty(t, resp.Msg.LoadError)
	require.Len(t, resp.Msg.Presets, 1)
	require.Equal(t, "codex-gpt5", resp.Msg.Presets[0].Id)
	require.Equal(t, []string{"codex", "--model", "gpt-5"}, resp.Msg.Presets[0].Argv)
}

func TestGetLauncherPresets_should_ReturnEmptyPresetsWithLoadError_When_FileHasDuplicateID(t *testing.T) {
	svc, dir := newIsolatedLauncherPresetsService(t)
	writePresetsFixture(t, dir, `{
		"version": 1,
		"presets": [
			{"id": "codex", "label": "A", "argv": ["codex"]},
			{"id": "codex", "label": "B", "argv": ["codex"]}
		]
	}`)

	resp, err := svc.GetLauncherPresets(context.Background(), connect.NewRequest(&sessionv1.GetLauncherPresetsRequest{}))
	require.NoError(t, err, "malformed file must not surface as a Connect error")
	require.Empty(t, resp.Msg.Presets)
	require.Contains(t, resp.Msg.LoadError, "codex")
}

func TestGetLauncherPresets_should_ReturnEmptyPresetsNoError_When_FileIsMissing(t *testing.T) {
	svc, _ := newIsolatedLauncherPresetsService(t)

	resp, err := svc.GetLauncherPresets(context.Background(), connect.NewRequest(&sessionv1.GetLauncherPresetsRequest{}))
	require.NoError(t, err)
	require.Empty(t, resp.Msg.Presets)
	require.Empty(t, resp.Msg.LoadError)
}

func TestGetLauncherPresets_should_ReflectEditedFile_When_CalledTwiceAcrossAnEdit(t *testing.T) {
	svc, dir := newIsolatedLauncherPresetsService(t)
	writePresetsFixture(t, dir, `{"version": 1, "presets": [{"id": "a", "label": "A", "argv": ["a"]}]}`)

	first, err := svc.GetLauncherPresets(context.Background(), connect.NewRequest(&sessionv1.GetLauncherPresetsRequest{}))
	require.NoError(t, err)
	require.Len(t, first.Msg.Presets, 1)

	writePresetsFixture(t, dir, `{"version": 1, "presets": [{"id": "a", "label": "A", "argv": ["a"]}, {"id": "b", "label": "B", "argv": ["b"]}]}`)

	second, err := svc.GetLauncherPresets(context.Background(), connect.NewRequest(&sessionv1.GetLauncherPresetsRequest{}))
	require.NoError(t, err)
	require.Len(t, second.Msg.Presets, 2, "GetLauncherPresets must re-read the file on every call, not cache")
}
