package prwatch

import "time"

// HumanFeedbackMode decides who may cause a comment or review steer (D7, D11).
// CI steers never depend on it.
type HumanFeedbackMode string

const (
	ModeAll       HumanFeedbackMode = "all"
	ModeAllowlist HumanFeedbackMode = "allowlist"
	ModeOff       HumanFeedbackMode = "off"
)

// Settings is the engine's view of the inbox_pr_watch config. The server
// adapter maps config onto it (defaults, clamps and corrections are applied
// there), so prwatch never imports config.
type Settings struct {
	ConsumerID string // "squad:<hostname>"
	URL        string
	TokenPath  string
	StatusURL  string // appended to every notification (D14, D15)
	GuidePath  string // operator doc location for notifications (D15)
	LockPath   string // leader lock path, shown in leader_not_held

	PollInterval            time.Duration
	ReconcileInterval       time.Duration
	CoverageRecheck         time.Duration
	MinSteerInterval        time.Duration // 0 disables coalescing
	HeldNoticeAfter         time.Duration
	HeldAgeCeiling          time.Duration
	StateNotifyGrace        time.Duration
	LeaderRetry             time.Duration
	FollowupObserve         time.Duration
	IgnoreRepos             []string // lower-case owner/name
	IgnoreBotActors         bool
	BotActorAllowlist       []string
	HumanFeedbackMode       HumanFeedbackMode
	ActorAllowlist          []string
	StrangerNoticeThreshold int
}
