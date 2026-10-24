package services

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestUpdateQuotaConfig_UpperBounds(t *testing.T) {
	withIsolatedStateDir(t)
	i32 := func(v int32) *int32 { return &v }
	cases := []struct {
		name string
		set  func(*sessionv1.QuotaSettings, int32)
		max  int32
	}{
		{"ticks_pause", func(s *sessionv1.QuotaSettings, v int32) { s.ConsecutiveTicksToPause = i32(v) }, 1000},
		{"ticks_resume", func(s *sessionv1.QuotaSettings, v int32) { s.ConsecutiveTicksToResume = i32(v) }, 1000},
		{"window_min", func(s *sessionv1.QuotaSettings, v int32) { s.RateLimitWindowMinutes = i32(v) }, 10080},
		{"grace_min", func(s *sessionv1.QuotaSettings, v int32) { s.ManualOverrideGraceMinutes = i32(v) }, 10080},
		{"throttle_sec", func(s *sessionv1.QuotaSettings, v int32) { s.ForegroundThrottleDelaySeconds = i32(v) }, 86400},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			ok := &sessionv1.QuotaSettings{}
			c.set(ok, c.max)
			_, err := updateQuota(t, ok)
			require.NoError(t, err)
			for _, v := range []int32{c.max + 1, 2147483647} {
				bad := &sessionv1.QuotaSettings{}
				c.set(bad, v)
				_, err = updateQuota(t, bad)
				require.Error(t, err)
				require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
			}
		})
	}
}

func TestUpdateQuotaConfig_NilSettings_InvalidArgument(t *testing.T) {
	withIsolatedStateDir(t)
	_, err := NewQuotaSettingsService().UpdateQuotaConfig(context.Background(),
		connect.NewRequest(&sessionv1.UpdateQuotaConfigRequest{}))
	require.Error(t, err)
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
