package session

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/domain"
)

func TestMigrationToTmux_WiresInstanceEnvironment(t *testing.T) {
	t.Parallel()
	inst := &Instance{Title: "migration-env", UUID: "migration-proof", Backend: domain.BackendTymux, EnvVars: map[string]string{"MIGRATION_CUSTOM": "present"}}
	require.NoError(t, inst.updateProcessManagerForRestart(true, domain.BackendTmux, shellLaunch("bash")))
	sess := inst.GetTmuxSession()
	require.NotNil(t, sess)
	field := reflect.ValueOf(sess).Elem().FieldByName("extraEnv")
	var env []string
	for idx := 0; idx < field.Len(); idx++ {
		env = append(env, field.Index(idx).String())
	}
	assert.Contains(t, env, "STAPLER_SESSION_UUID=migration-proof")
	assert.Contains(t, env, "MIGRATION_CUSTOM=present")
}
