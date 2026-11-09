package session

import (
	"testing"
	"time"
)

func TestDefaultReviewQueuePollerConfig_StalenessEnvOverride(t *testing.T) {
	tests := []struct {
		name string
		env  string
		want time.Duration
	}{
		{"unset keeps default", "", 5 * time.Minute},
		{"valid override", "24h", 24 * time.Hour},
		{"garbage keeps default", "soon", 5 * time.Minute},
		{"non-positive keeps default", "-1m", 5 * time.Minute},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(StalenessThresholdEnv, tt.env)
			if got := DefaultReviewQueuePollerConfig().StalenessThreshold; got != tt.want {
				t.Fatalf("StalenessThreshold = %v, want %v", got, tt.want)
			}
		})
	}
}
