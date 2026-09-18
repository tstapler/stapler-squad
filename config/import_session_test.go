package config

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestImportSessionEnabled_ReturnsTrue_When_EnvVarIsTrue(t *testing.T) {
	orig, wasSet := os.LookupEnv("STAPLER_SQUAD_ENABLE_SESSION_IMPORT")
	os.Setenv("STAPLER_SQUAD_ENABLE_SESSION_IMPORT", "true")
	defer restoreEnv(t, "STAPLER_SQUAD_ENABLE_SESSION_IMPORT", orig, wasSet)

	assert.True(t, ImportSessionEnabled())
}

func TestImportSessionEnabled_ReturnsFalse_When_EnvVarUnset(t *testing.T) {
	orig, wasSet := os.LookupEnv("STAPLER_SQUAD_ENABLE_SESSION_IMPORT")
	os.Unsetenv("STAPLER_SQUAD_ENABLE_SESSION_IMPORT")
	defer restoreEnv(t, "STAPLER_SQUAD_ENABLE_SESSION_IMPORT", orig, wasSet)

	assert.False(t, ImportSessionEnabled())
}

func TestImportSessionEnabled_ReturnsFalse_When_EnvVarIsNotExactlyTrue(t *testing.T) {
	orig, wasSet := os.LookupEnv("STAPLER_SQUAD_ENABLE_SESSION_IMPORT")
	os.Setenv("STAPLER_SQUAD_ENABLE_SESSION_IMPORT", "1")
	defer restoreEnv(t, "STAPLER_SQUAD_ENABLE_SESSION_IMPORT", orig, wasSet)

	assert.False(t, ImportSessionEnabled())
}

func restoreEnv(t *testing.T, key, orig string, wasSet bool) {
	t.Helper()
	if wasSet {
		os.Setenv(key, orig)
	} else {
		os.Unsetenv(key)
	}
}
