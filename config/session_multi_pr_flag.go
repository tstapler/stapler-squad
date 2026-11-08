package config

// SessionMultiPRFlag is the rollout flag of the session-multi-pr-tracking project
// (ADR-001..004). Its scopes are the four readers plus the refresher kill switch.
const SessionMultiPRFlag = "session_multi_pr"

// ArtifactScanV2Flag gates the artifact blob v2 rescan (default off).
const ArtifactScanV2Flag = "artifact_scan_v2"

// Reader scopes, in the order they must be enabled.
const (
	SessionMultiPRScopePoller  = "reader:poller"
	SessionMultiPRScopeBacklog = "reader:backlog"
	SessionMultiPRScopeActions = "reader:actions"
	SessionMultiPRScopeUI      = "reader:ui"

	// SessionMultiPRScopeRefresherOff is the kill switch of the state refresher.
	// It is inverse-named because a scope resolves as override, else global, else
	// the flag's single default (false): a default-on scope cannot be expressed.
	SessionMultiPRScopeRefresherOff = "refresher_off"
)

// SessionMultiPRScopes lists every scope the flag accepts.
func SessionMultiPRScopes() []string {
	return []string{
		SessionMultiPRScopePoller, SessionMultiPRScopeBacklog, SessionMultiPRScopeActions,
		SessionMultiPRScopeUI, SessionMultiPRScopeRefresherOff,
	}
}

// sessionMultiPRReaderOrder lists the reader scopes in dependency order: a
// reader is effective only when every earlier one is, so it never runs ahead of
// the data it depends on.
func sessionMultiPRReaderOrder() []string {
	return []string{SessionMultiPRScopePoller, SessionMultiPRScopeBacklog, SessionMultiPRScopeActions, SessionMultiPRScopeUI}
}

// SessionMultiPR reports whether the reader scope is effective: its own value
// (scoped override, else global, else off) AND every earlier reader's, in the
// order poller, backlog, actions, ui. The order is applied here, at read time;
// the write path stores whatever value it is given. An unknown scope is off.
func (c *Config) SessionMultiPR(scope string) bool {
	for _, s := range sessionMultiPRReaderOrder() {
		if !c.GetFeatureFlagScoped(SessionMultiPRFlag, s, false) {
			return false
		}
		if s == scope {
			return true
		}
	}
	return false
}

// SessionMultiPRRefresher reports whether the flag-independent state refresher
// runs: on unless refresher_off is explicitly set. Neither the global flag nor a
// reader scope changes it.
func (c *Config) SessionMultiPRRefresher() bool {
	off, _ := c.GetFeatureFlagScopedOverride(SessionMultiPRFlag, SessionMultiPRScopeRefresherOff)
	return !off
}

// SessionMultiPR is Config.SessionMultiPR over the live config.
func SessionMultiPR(scope string) bool { return LoadConfig().SessionMultiPR(scope) }

// SessionMultiPRRefresher is Config.SessionMultiPRRefresher over the live config.
func SessionMultiPRRefresher() bool { return LoadConfig().SessionMultiPRRefresher() }

// ArtifactScanV2Enabled reports whether the artifact blob v2 rescan may run.
func (c *Config) ArtifactScanV2Enabled() bool {
	return c.GetFeatureFlag(ArtifactScanV2Flag)
}
