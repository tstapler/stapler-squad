package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestUpdateQuotaConfig_NilSettings_InvalidArgument(t *testing.T) {
	withIsolatedStateDir(t)
	_, err := NewQuotaSettingsService().UpdateQuotaConfig(context.Background(),
		connect.NewRequest(&sessionv1.UpdateQuotaConfigRequest{}))
	require.Error(t, err)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
