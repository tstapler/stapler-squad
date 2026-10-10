package tokens

import (
	"testing"

	"github.com/stretchr/testify/assert"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestContextHealthLevelToProto(t *testing.T) {
	tests := []struct {
		in   ContextHealthLevel
		want sessionv1.ContextHealth
	}{
		{HealthUnknown, sessionv1.ContextHealth_CONTEXT_HEALTH_UNSPECIFIED},
		{HealthGreen, sessionv1.ContextHealth_CONTEXT_HEALTH_GREEN},
		{HealthAmber, sessionv1.ContextHealth_CONTEXT_HEALTH_AMBER},
		{HealthRed, sessionv1.ContextHealth_CONTEXT_HEALTH_RED},
		{ContextHealthLevel(99), sessionv1.ContextHealth_CONTEXT_HEALTH_UNSPECIFIED},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, ContextHealthLevelToProto(tc.in), tc.in.String())
	}
}
