package claimcheck

import (
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/hostclaim"
)

// flagGatedForeignClaimLookup makes a session.ForeignClaimLookup honor the
// cross_host_claim_dedup flag, so SyncOne is unchanged while the flag is off.
type flagGatedForeignClaimLookup struct {
	inner session.ForeignClaimLookup
}

// NewFlagGatedForeignClaimLookup wraps inner so it reports no claims while the
// cross_host_claim_dedup feature flag is off.
func NewFlagGatedForeignClaimLookup(inner session.ForeignClaimLookup) session.ForeignClaimLookup {
	return flagGatedForeignClaimLookup{inner: inner}
}

func (f flagGatedForeignClaimLookup) ForeignClaim(externalURL string) (hostclaim.ClaimRecord, bool) {
	if !config.LoadConfig().GetFeatureFlag(featureflags.CrossHostClaimDedupFlagName) {
		return hostclaim.ClaimRecord{}, false
	}
	return f.inner.ForeignClaim(externalURL)
}
