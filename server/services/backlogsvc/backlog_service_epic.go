package backlogsvc

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/config"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/server/services/featureflags"
	"github.com/tstapler/stapler-squad/session/backlogstore"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/git"
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
	if !s.epicIntegrationEnabled() {
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

// epicIntegrationEnabled reads the flag from a fresh config: flag writes persist to disk and s.cfg
// is a startup snapshot, so reading it would never see a runtime flip (same as crossHostClaimDedupEnabled).
func (s *BacklogService) epicIntegrationEnabled() bool {
	if s.epicIntegrationFlag != nil {
		return s.epicIntegrationFlag()
	}
	return config.LoadConfig().GetFeatureFlag(featureflags.EpicIntegrationChildFlagName)
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

// handleIntegrationCheckFailure is the production caller of OnIntegrationChildFailed, reached from
// AutoReopenAfterFailedReview when the integration child's review fails. It returns true when the
// failure was handed off (fix child, open fix child, or escalation) so the caller must not also
// rework the child. A hand-off error is logged and still counts as handled: the usual cause is a
// pending guidance request already covering the epic, and reworking the check would only hide it.
func (s *BacklogService) handleIntegrationCheckFailure(ctx context.Context, child *backlogdto.BacklogItemData) bool {
	out, err := s.OnIntegrationChildFailed(ctx, child.ID, s.integrationFailingFiles(ctx, child))
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return false // not owned by an epic: rework it like any other item
		}
		log.Warn("[EpicIntegration] failure hand-off failed; leaving the integration child parked", "item", child.ID, "error", err)
		return true
	}
	return out.FixChildID != "" || out.GuidanceRequestID != ""
}

// integrationFailingFiles lists the files the integration child's latest work session changed,
// falling back to its recorded shipped files.
func (s *BacklogService) integrationFailingFiles(ctx context.Context, child *backlogdto.BacklogItemData) []string {
	repoPath, baseSHA, headSHA, _, err := s.storage.Repo.GetRepoPathAndLatestCompletedWorkSessionCommits(ctx, child.ID)
	if err == nil && repoPath != "" && baseSHA != "" && headSHA != "" {
		if stats, gitErr := git.FileStatsBetween(repoPath, baseSHA, headSHA); gitErr == nil && len(stats) > 0 {
			paths := make([]string, len(stats))
			for i, st := range stats {
				paths[i] = st.Path
			}
			return paths
		}
	}
	return backlogstore.ShippedFilePaths(child.ShippedFileStats)
}

// effectiveIntegrationFixCap mirrors effectiveReworkCap: nil override = global default,
// 0 = unlimited, >0 = the item's own cap.
func (s *BacklogService) effectiveIntegrationFixCap(epic *backlogdto.BacklogItemData) int {
	// A negative override is invalid (the RPC takes any int32): fall back to the global default
	// rather than reading it as "unlimited", which only 0 means.
	if epic.ReworkCapOverride != nil && *epic.ReworkCapOverride >= 0 {
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
	siblingIDs, openFixID, err := s.integrationSiblingIDs(ctx, child.ID)
	if err != nil {
		return nil, err
	}
	if openFixID != "" {
		// A fix child for an earlier failure is still open; a retried failure must not spawn another.
		return &IntegrationOutcome{FixChildID: openFixID}, nil
	}
	files, err := s.storage.Repo.SiblingShippedFiles(ctx, siblingIDs)
	if err != nil {
		return nil, err
	}
	failingFiles = sanitizeFailingFiles(failingFiles)
	sibling, confidence := backlogstore.AttributeByFileOverlap(failingFiles, files)
	out := &IntegrationOutcome{AttributedTo: sibling, Confidence: confidence}

	s.cfgMu.RLock()
	fixCap := s.effectiveIntegrationFixCap(epic)
	threshold := s.cfg.IntegrationAttributionThresholdOrDefault()
	s.cfgMu.RUnlock()
	reason := "the failure could not be attributed to one sibling with enough confidence"
	if sibling != "" && confidence >= threshold {
		// The slot is claimed atomically before the child exists so concurrent failures cannot exceed the cap.
		reserved, err := s.storage.Repo.ReserveIntegrationFix(ctx, epic.ID, fixCap)
		if err != nil {
			return nil, err
		}
		if reserved {
			fix, err := s.createIntegrationFixChild(ctx, epic, child, sibling, failingFiles)
			if err != nil {
				if relErr := s.storage.Repo.ReleaseIntegrationFix(ctx, epic.ID); relErr != nil {
					log.Warn("[EpicIntegration] could not release fix slot", "epic", epic.ID, "error", relErr)
				}
				return nil, err
			}
			out.FixChildID = fix.ID
			return out, nil
		}
		reason = fmt.Sprintf("the fix-child cap (%d) is reached", fixCap)
	}
	gid, err := s.escalateIntegrationFailure(ctx, epic, child, failingFiles, reason)
	out.GuidanceRequestID = gid
	return out, err
}

// integrationSiblingIDs returns the integration child's blockers that are original siblings
// (not fix children it later gained) and the ID of a still-open fix child, if any.
func (s *BacklogService) integrationSiblingIDs(ctx context.Context, childID string) (ids []string, openFixID string, err error) {
	blockedBy, _, err := s.storage.ListBacklogItemDependencies(ctx, childID)
	if err != nil {
		return nil, "", fmt.Errorf("list integration child dependencies: %w", err)
	}
	for _, ref := range blockedBy {
		item, err := s.storage.GetBacklogItem(ctx, ref.ItemID)
		if err != nil {
			return nil, "", err
		}
		if !slices.Contains(item.Labels, integrationFixLabel) {
			ids = append(ids, ref.ItemID)
		} else if !slices.Contains(domain.TerminalStatusStrings(), item.Status) {
			openFixID = ref.ItemID
		}
	}
	return ids, openFixID, nil
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
			// Remove the half-linked fix child so it cannot auto-spawn unattached.
			if delErr := s.storage.DeleteBacklogItem(ctx, fix.ID); delErr != nil {
				log.Warn("[EpicIntegration] could not remove half-linked fix child", "fix", fix.ID, "error", delErr)
			}
			return nil, fmt.Errorf("link integration fix child: %w", err)
		}
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

const (
	maxFailingFiles   = 50
	maxFailingPathLen = 300
)

// sanitizeFailingFiles keeps at most maxFailingFiles printable single-line paths: they reach an
// agent prompt and a guidance question.
func sanitizeFailingFiles(paths []string) []string {
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		if p == "" || len(p) > maxFailingPathLen || strings.IndexFunc(p, unicode.IsControl) >= 0 {
			continue
		}
		out = append(out, p)
		if len(out) == maxFailingFiles {
			break
		}
	}
	return out
}
