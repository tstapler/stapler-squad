package services

import "github.com/tstapler/stapler-squad/config"

// sessionMultiPRFlagAuditPolicies audits the multi-PR flags like the
// notification-actions flags: turning one on loosens (it opens a new reader or
// scan path), so the flip is audit-fsynced and refused while the sink is down.
func sessionMultiPRFlagAuditPolicies() map[string]FlagAuditPolicy {
	return map[string]FlagAuditPolicy{
		config.SessionMultiPRFlag: notificationActionsFlagAuditPolicy,
		config.ArtifactScanV2Flag: notificationActionsFlagAuditPolicy,
	}
}
