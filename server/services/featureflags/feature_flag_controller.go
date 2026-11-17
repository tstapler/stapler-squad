package featureflags

import (
	"context"
)

// FeatureController is implemented by components that can be enabled/disabled at runtime.
// Used by GetFeatureFlags/UpdateFeatureFlag to toggle named subsystems.
type FeatureController interface {
	Enable(ctx context.Context) error
	Disable() error
	IsEnabled() bool
}
