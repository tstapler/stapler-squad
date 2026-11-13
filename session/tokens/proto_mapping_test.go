package tokens

import (
	"testing"

	"github.com/tstapler/stapler-squad/session/domain"

	"github.com/stretchr/testify/assert"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

func TestContextHealthLevelToProto(t *testing.T) {
	tests := []struct {
		in   domain.ContextHealthLevel
		want sessionv1.ContextHealth
	}{
		{domain.HealthUnknown, sessionv1.ContextHealth_CONTEXT_HEALTH_UNSPECIFIED},
		{domain.HealthGreen, sessionv1.ContextHealth_CONTEXT_HEALTH_GREEN},
		{domain.HealthAmber, sessionv1.ContextHealth_CONTEXT_HEALTH_AMBER},
		{domain.HealthRed, sessionv1.ContextHealth_CONTEXT_HEALTH_RED},
		{domain.ContextHealthLevel(99), sessionv1.ContextHealth_CONTEXT_HEALTH_UNSPECIFIED},
	}
	for _, tc := range tests {
		assert.Equal(t, tc.want, ContextHealthLevelToProto(tc.in), tc.in.String())
	}
}
