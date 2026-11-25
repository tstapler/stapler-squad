package flagsvc

import (
	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
)

// The six live-settable rollout flags of attention-inbox-consolidation. All are
// default OFF and global-only; they are plain registry flags read from config at
// decision time, so a flip takes effect without a restart. No env vars.
const (
	AttentionProseQuestionDetectionFlagName   = "attention_prose_question_detection"
	SessionEventLogFlagName                   = "session_event_log"
	AttentionAckBypassFixFlagName             = "attention_ack_bypass_fix"
	AttentionIdleClockOutputFlagName          = "attention_idle_clock_output"
	AttentionProseQuestionLLMTiebreakFlagName = "attention_prose_question_llm_tiebreak"
	AttentionInboxConsolidationFlagName       = "attention_inbox_consolidation"
)

// AttentionFlags is one point-in-time read of the six flags. Callers that run a
// cycle (the review-queue poller) resolve it once per cycle and use that value
// throughout, so a mid-cycle flip cannot split one cycle across two behaviours.
type AttentionFlags struct {
	ProseQuestionDetection   bool
	SessionEventLog          bool
	AckBypassFix             bool
	IdleClockOutput          bool
	ProseQuestionLLMTiebreak bool
	InboxConsolidation       bool
}

// ResolveAttentionFlags reads the six flags from cfg.
// reg must be non-nil.
func ResolveAttentionFlags(cfg *config.Config, reg *featureflags.Registry) AttentionFlags {
	reg = FlagReader{Reg: reg}.registry()
	on := func(name string) bool { return reg.Enabled(cfg, name) }
	return AttentionFlags{
		ProseQuestionDetection:   on(AttentionProseQuestionDetectionFlagName),
		SessionEventLog:          on(SessionEventLogFlagName),
		AckBypassFix:             on(AttentionAckBypassFixFlagName),
		IdleClockOutput:          on(AttentionIdleClockOutputFlagName),
		ProseQuestionLLMTiebreak: on(AttentionProseQuestionLLMTiebreakFlagName),
		InboxConsolidation:       on(AttentionInboxConsolidationFlagName),
	}
}

// LoadAttentionFlags resolves the flags against the live config.
func LoadAttentionFlags(reg *featureflags.Registry) AttentionFlags {
	return ResolveAttentionFlags(config.LoadConfig(), reg)
}
