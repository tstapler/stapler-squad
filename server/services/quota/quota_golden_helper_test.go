package quota

import (
	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/session/tokens"
)

func newGoldenGate(cfgFn func() config.QuotaConfig, store tokens.TokenStoreReader, poller InstancePoller,
	ctrl featureflags.FeatureController, bus *events.EventBus) *QuotaGate {
	return NewQuotaGate(cfgFn, store, poller, ctrl, bus, testPorts)
}

// testPorts copies notify.DerivePriority, which a leaf test cannot import. The
// composition root's wiring of the real function is checked in package server.
var testPorts = Ports{DerivePriority: func(urgent, important bool) int32 {
	switch {
	case urgent && important:
		return int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_URGENT)
	case important:
		return int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_HIGH)
	case urgent:
		return int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_MEDIUM)
	default:
		return int32(sessionv1.NotificationPriority_NOTIFICATION_PRIORITY_LOW)
	}
}}
