package backlogsvc

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/session/backlogstore"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
)

const integrationFixLabel = "integration-fix"

// IntegrationOutcome reports what OnIntegrationChildFailed did.
type IntegrationOutcome struct {
	FixChildID        string  // set when a fix child was created
	AttributedTo      string  // sibling the failure was attributed to ("" = unattributed)
	Confidence        float64 // overlap score of AttributedTo
	GuidanceRequestID string  // set when the failure escalated to the epic's owner
}

// OnEpicPlanningComplete creates the epic's integration child once all siblingIDs exist. The
// caller is the Epic items planning hook (the parent-child edge is not in this repo yet), so
// siblings arrive as an argument. No-op when the flag is off, the epic defines no e2e command
// (warns), or an integration child already exists (idempotent).
func (s *BacklogService) OnEpicPlanningComplete(ctx context.Context, epicID string, siblingIDs []string) (*backlogdto.BacklogItemData, error) {
	if s.flags == nil || !s.flags.Enabled(s.cfg, featureflags.EpicIntegrationChildFlagName) {
		return nil, nil
	}
	epic, err := s.storage.GetBacklogItem(ctx, epicID)
	if err != nil {
		return nil, fmt.Errorf("load epic %s: %w", epicID, err)
	}
	if epic.IntegrationChildID != "" {
		return s.storage.GetBacklogItem(ctx, epic.IntegrationChildID)
	}
	cmd := backlogstore.ExtractE2ECommand(string(epic.AcceptanceCriteria), s.doneCommandTemplate(ctx, epic.PipelineMode))
	if cmd == "" {
		log.Warn("[EpicIntegration] epic has no e2e: acceptance criterion or done_command_template; integration child not created", "epic", epic.ID)
		return nil, nil
	}
	// Created at idea (ADR-007: only workflow-engine paths may move status); the normal
	// idea -> ready flow promotes it, and its sibling dependencies hold the dequeue until they resolve.
	child, _, err := s.storage.Repo.CreateIntegrationChild(ctx, epic.ID, siblingIDs, cmd)
	return child, err
}

func (s *BacklogService) doneCommandTemplate(ctx context.Context, slug string) string {
	if slug == "" || s.pipelineModeRepo == nil {
		return ""
	}
	pm, err := s.pipelineModeRepo.GetBySlug(ctx, slug)
	if err != nil {
		return ""
	}
	return pm.DoneCommandTemplate
}

// effectiveIntegrationFixCap mirrors effectiveReworkCap: nil override = global default,
// 0 = unlimited, >0 = the item's own cap.
func (s *BacklogService) effectiveIntegrationFixCap(epic *backlogdto.BacklogItemData) int {
	if epic.ReworkCapOverride != nil {
		return *epic.ReworkCapOverride
	}
	return s.cfg.MaxAutoReworkIterationsOrDefault()
}

// OnIntegrationChildFailed handles an integration child whose end-to-end check failed.
// failingFiles are the files implicated by the failure (the integration session's diff or the
// failing check's output). It attributes the failure to a sibling by file overlap and creates a
// fix child, or escalates to the epic's owner when confidence is below the configured threshold,
// the fix-child cap is reached, or no epic owns the child.
func (s *BacklogService) OnIntegrationChildFailed(ctx context.Context, integrationChildID string, failingFiles []string) (*IntegrationOutcome, error) {
	child, err := s.storage.GetBacklogItem(ctx, integrationChildID)
	if err != nil {
		return nil, fmt.Errorf("load integration child: %w", err)
	}
	if !child.IsIntegrationChild {
		return nil, fmt.Errorf("item %s is not an integration child", integrationChildID)
	}
	epic, err := s.storage.Repo.GetEpicForIntegrationChild(ctx, child.ID)
	if err != nil {
		return nil, err
	}
	siblingIDs, err := s.integrationSiblingIDs(ctx, child.ID)
	if err != nil {
		return nil, err
	}
	files, err := s.storage.Repo.SiblingShippedFiles(ctx, siblingIDs)
	if err != nil {
		return nil, err
	}
	sibling, confidence := backlogstore.AttributeByFileOverlap(failingFiles, files)
	out := &IntegrationOutcome{AttributedTo: sibling, Confidence: confidence}

	fixCap := s.effectiveIntegrationFixCap(epic)
	capped := fixCap > 0 && epic.IntegrationFixCount >= fixCap
	if sibling != "" && confidence >= s.cfg.IntegrationAttributionThresholdOrDefault() && !capped {
		fix, err := s.createIntegrationFixChild(ctx, epic, child, sibling, failingFiles)
		if err != nil {
			return nil, err
		}
		out.FixChildID = fix.ID
		return out, nil
	}
	reason := "the failure could not be attributed to one sibling with enough confidence"
	if capped {
		reason = fmt.Sprintf("the fix-child cap (%d) is reached", fixCap)
	}
	gid, err := s.escalateIntegrationFailure(ctx, epic, child, failingFiles, reason)
	out.GuidanceRequestID = gid
	return out, err
}

// integrationSiblingIDs returns the integration child's blockers that are original siblings
// (not fix children it later gained).
func (s *BacklogService) integrationSiblingIDs(ctx context.Context, childID string) ([]string, error) {
	blockedBy, _, err := s.storage.ListBacklogItemDependencies(ctx, childID)
	if err != nil {
		return nil, fmt.Errorf("list integration child dependencies: %w", err)
	}
	var ids []string
	for _, ref := range blockedBy {
		item, err := s.storage.GetBacklogItem(ctx, ref.ItemID)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(item.Labels, integrationFixLabel) {
			ids = append(ids, ref.ItemID)
		}
	}
	return ids, nil
}

func (s *BacklogService) createIntegrationFixChild(ctx context.Context, epic, child *backlogdto.BacklogItemData, siblingID string, failingFiles []string) (*backlogdto.BacklogItemData, error) {
	sibling, err := s.storage.GetBacklogItem(ctx, siblingID)
	if err != nil {
		return nil, err
	}
	fix, err := s.storage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{
		Title: "Fix integration failure from: " + sibling.Title,
		Description: fmt.Sprintf("The end-to-end check of epic %q failed and was attributed to %q (files: %s). "+
			"Fix the failure so the integration check can pass.", epic.Title, sibling.Title, strings.Join(failingFiles, ", ")),
		AcceptanceCriteria: `[{"index":0,"text":"The epic's end-to-end check passes after this fix","status":"pending"}]`,
		Priority:           epic.Priority,
		RepoPath:           epic.RepoPath,
		BaseBranch:         epic.BaseBranch,
		PipelineMode:       epic.PipelineMode,
		SkipPlanning:       true,
		AutoSpawnSession:   true,
		Labels:             []string{integrationFixLabel},
	})
	if err != nil {
		return nil, fmt.Errorf("create integration fix child: %w", err)
	}
	for _, edge := range []backlogdto.BacklogItemDependencyEdge{
		{BlockerID: siblingID, BlockedID: fix.ID},
		{BlockerID: fix.ID, BlockedID: child.ID},
	} {
		if err := s.storage.AddBacklogItemDependency(ctx, edge); err != nil {
			return nil, fmt.Errorf("link integration fix child: %w", err)
		}
	}
	if err := s.storage.Repo.IncrementIntegrationFixCount(ctx, epic.ID); err != nil {
		return nil, err
	}
	return fix, nil
}

func (s *BacklogService) escalateIntegrationFailure(ctx context.Context, epic, child *backlogdto.BacklogItemData, failingFiles []string, reason string) (string, error) {
	id, err := uuid.Parse(epic.ID)
	if err != nil {
		return "", fmt.Errorf("parse epic id: %w", err)
	}
	row, err := s.storage.CreateGuidanceRequest(ctx, domain.CreateGuidanceRequestInput{
		Scope:  domain.RequestScopeBacklogItem,
		ItemID: &id,
		QuestionText: fmt.Sprintf("Integration check %q failed (%s). Implicated files: %s. How should the epic proceed?",
			child.Title, reason, strings.Join(failingFiles, ", ")),
		QuestionType: domain.QuestionTypeShortAnswer,
	})
	if err != nil {
		return "", fmt.Errorf("escalate integration failure: %w", err)
	}
	return row.ID, nil
}
