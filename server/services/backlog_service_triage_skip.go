package services

// backlog_service_triage_skip.go — cost-reduction policy for automatic triage: which model a
// triage call runs on, and when an automatic trigger is skipped instead of spending an
// agentic call. Manual TriggerTriage (incl. feedback refine) never consults the skip rules.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
)

const triageGateNoteAuthor = "Triage gate"

// resolveTriageModel returns the concrete model for a triage call. A model pinned by the
// pipeline mode always wins; otherwise the configured triage default applies, but only for
// the claude program (a non-claude executor would receive a Claude model ID). A configured
// value that fails to resolve falls back to the built-in default, never the account default.
func resolveTriageModel(cfg *config.Config, families map[string]string, program, pinnedModel string) string {
	if pinnedModel != "" {
		resolved, err := session.ResolveModel(families, pinnedModel)
		if err != nil {
			log.Warn("[PipelineEngine] failed to resolve triage model family alias, using empty model", "model", pinnedModel, "err", err)
			return ""
		}
		return resolved
	}
	if program != "" && program != "claude" {
		return ""
	}
	configured := cfg.HeadlessTriageModelOrDefault()
	if configured == "" {
		return ""
	}
	resolved, err := session.ResolveModel(families, configured)
	if err != nil {
		log.Warn("[TriggerTriage] configured headless_triage_model unresolvable, using built-in default", "model", configured, "err", err)
		resolved, err = session.ResolveModel(families, config.DefaultHeadlessTriageModel)
		if err != nil {
			return ""
		}
	}
	return resolved
}

// triageInputHash fingerprints the content triage reasons about: title, description and AC
// text only. AC index/status/notes are excluded — they mutate during work and would make
// the hash unstable.
func triageInputHash(title, description string, criteria []session.AcCriterion) string {
	h := sha256.New()
	h.Write([]byte(title))
	h.Write([]byte{0})
	h.Write([]byte(description))
	for _, c := range criteria {
		h.Write([]byte{0})
		h.Write([]byte(c.Text))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func itemTriageInputHash(item *session.BacklogItemData) string {
	criteria, _ := item.AcceptanceCriteria.Parse()
	return triageInputHash(item.Title, item.Description, criteria)
}

// newItemTriageSkipReason returns why a freshly created item needs no triage call ("" = run):
// it already carries acceptance criteria or an approved plan.
func newItemTriageSkipReason(item *session.BacklogItemData) string {
	if criteria, _ := item.AcceptanceCriteria.Parse(); len(criteria) > 0 {
		return "item already has acceptance criteria"
	}
	if item.PlanApproved {
		return "item already has an approved plan"
	}
	return ""
}

// unchangedRetriageSkipReason returns why re-triaging is pointless ("" = run): the last
// completed triage result was produced from input identical to the item's current content.
func unchangedRetriageSkipReason(item *session.BacklogItemData, sessions []session.ItemSessionSummary) string {
	prior, ok := findPriorTriageResult(sessions)
	if !ok || prior.InputHash == "" || prior.InputHash != itemTriageInputHash(item) {
		return ""
	}
	return "title, description and criteria are unchanged since the last completed triage"
}

// recordTriageSkip makes a skip visible on the item (activity note) and in the log.
func (s *BacklogService) recordTriageSkip(ctx context.Context, itemID, reason string) {
	log.Info("[TriageGate] triage skipped", "item", itemID, "reason", reason)
	msg := "Triage skipped: " + reason + ". Trigger triage manually to override."
	if err := s.storage.AppendActivityNote(ctx, itemID, "", triageGateNoteAuthor, msg); err != nil {
		log.Warn("[TriageGate] failed to record skip note", "item", itemID, "error", err)
	}
}

// triageResultInputHash hashes the item as it will look once result's criteria are applied,
// so a later automatic retriage of the unmodified item matches.
func triageResultInputHash(item *session.BacklogItemData, result *session.HeadlessTriageResult) string {
	criteria := result.AcceptanceCriteria
	if len(criteria) == 0 {
		criteria, _ = item.AcceptanceCriteria.Parse()
	}
	return triageInputHash(item.Title, item.Description, criteria)
}

