package services

import "github.com/tstapler/stapler-squad/config"

// sessionMultiPRFlagAuditPolicies audits the multi-PR flags like the
// notification-actions flags: turning one on loosens (it opens a new reader or
// scan path), so the flip is audit-fsynced and refused while the sink is down.
func sessionMultiPRFlagAuditPolicies() map[string]FlagAuditPolicy {
	return map[string]FlagAuditPolicy{
		config.SessionMultiPRFlag: {
			Loosening: notificationActionsFlagAuditPolicy.Loosening,
			// The refresher kill switch is how an operator stops the refresher when the sink is the fault.
			EscapeHatchScopes: []string{config.SessionMultiPRScopeRefresherOff},
		},
		config.ArtifactScanV2Flag: notificationActionsFlagAuditPolicy,
	}
}
