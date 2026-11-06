package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpdateConfig_CorruptFileNotOverwritten(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	dir, err := GetConfigDir()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(dir, 0o750))
	path := filepath.Join(dir, ConfigFileName)
	corrupt := []byte(`{"half": `)
	require.NoError(t, os.WriteFile(path, corrupt, 0o644))

	called := false
	err = UpdateConfig(func(*Config) error { called = true; return nil })
	require.Error(t, err)
	assert.False(t, called)
	got, rerr := os.ReadFile(path)
	require.NoError(t, rerr)
	assert.Equal(t, corrupt, got)
}

func TestUpdateConfig_MissingFileStartsFromDefaults(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())
	require.NoError(t, UpdateConfig(func(*Config) error { return nil }))
	dir, err := GetConfigDir()
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, ConfigFileName))
	require.NoError(t, err)
}
