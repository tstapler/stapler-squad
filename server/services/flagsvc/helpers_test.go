package flagsvc

import (
	"context"
	"sync"

	"connectrpc.com/connect"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
)

// testFlagRegistry is the registry the tests in this package share; it is immutable, so parallel tests may use it.
var testFlagRegistry = sync.OnceValue(func() *featureflags.Registry { return FlagRegistry() })

func flagUpdate(svc *FeatureFlagService, name string, enabled bool) error {
	_, err := svc.UpdateFeatureFlag(context.Background(),
		connect.NewRequest(&sessionv1.UpdateFeatureFlagRequest{Name: name, Enabled: enabled}))
	return err
}
