package config

// WakeTriggersFlag is the rollout flag of Epic 4 (ADR-008). Its scopes are
// "trigger:<type>" and "tool:<name>".
const WakeTriggersFlag = "agent_wake_triggers"

// WakeTriggerScope and WakeToolScope build the flag scopes.
func WakeTriggerScope(triggerType string) string { return "trigger:" + triggerType }
func WakeToolScope(tool string) string           { return "tool:" + tool }

// WakeDispatcherRunning applies the flag matrix row "dispatcher running": the
// global flag, or an explicit scoped true for any trigger type. triggerTypes
// are the type names to look at.
func (c *Config) WakeDispatcherRunning(triggerTypes []string) bool {
	if c.GetFeatureFlag(WakeTriggersFlag) {
		return true
	}
	for _, t := range triggerTypes {
		if v, ok := c.GetFeatureFlagScopedOverride(WakeTriggersFlag, WakeTriggerScope(t)); ok && v {
			return true
		}
	}
	return false
}

// WakeTriggerEnabled is "create a trigger of a type": Scoped(trigger:<type>)
// AND the dispatcher running.
func (c *Config) WakeTriggerEnabled(triggerType string, triggerTypes []string) bool {
	return c.GetFeatureFlagScoped(WakeTriggersFlag, WakeTriggerScope(triggerType), false) && c.WakeDispatcherRunning(triggerTypes)
}

// WakeToolEnabled is "an MCP tool": Scoped(tool:<name>) AND the dispatcher running.
func (c *Config) WakeToolEnabled(tool string, triggerTypes []string) bool {
	return c.GetFeatureFlagScoped(WakeTriggersFlag, WakeToolScope(tool), false) && c.WakeDispatcherRunning(triggerTypes)
}
