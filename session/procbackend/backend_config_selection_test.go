package procbackend

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/instance"
)

// The configured backend (not opts.Backend, not defaultBackend) decides when the
// caller expresses no preference.
func TestNewProcessManager_ShouldHonourConfiguredBackend_WhenOptsBackendIsUnset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, config.ConfigFileName), []byte(`{"process_manager_backend":"tymux"}`), 0o600))

	pm, err := NewProcessManager(context.Background(), domain.BackendTmux, instance.ProcessManagerOptions{})
	require.NoError(t, err)
	_, ok := pm.(*TymuxBackend)
	assert.True(t, ok, "expected *TymuxBackend from the configured backend, got %T", pm)
}
