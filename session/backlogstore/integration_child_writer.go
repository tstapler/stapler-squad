package backlogstore

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/ent"
	"github.com/tstapler/stapler-squad/session/ent/backlogitem"
)

// IntegrationCheckACPrefix marks an epic acceptance criterion whose remainder is the end-to-end check command.
const IntegrationCheckACPrefix = "e2e:"

// ExtractE2ECommand returns the epic's end-to-end check command: the text after the first
// "e2e:" acceptance criterion, else fallback (the pipeline mode's done_command_template).
// "" means the epic defines none.
func ExtractE2ECommand(acceptanceCriteriaJSON, fallback string) string {
	var acs []domain.AcCriterion
	if acceptanceCriteriaJSON != "" && json.Unmarshal([]byte(acceptanceCriteriaJSON), &acs) == nil {
		for _, ac := range acs {
			text := strings.TrimSpace(ac.Text)
			if len(text) >= len(IntegrationCheckACPrefix) && strings.EqualFold(text[:len(IntegrationCheckACPrefix)], IntegrationCheckACPrefix) {
				if cmd := strings.TrimSpace(text[len(IntegrationCheckACPrefix):]); cmd != "" {
					return sanitizedE2ECommand(cmd)
				}
			}
		}
	}
	return sanitizedE2ECommand(strings.TrimSpace(fallback))
}

// maxE2ECommandLen bounds the command pasted into the integration child's agent prompt.
const maxE2ECommandLen = 500

// sanitizedE2ECommand returns cmd, or "" when it is too long or holds a newline or other control
// character: the text becomes agent-prompt content, so a multi-line value could smuggle instructions.
func sanitizedE2ECommand(cmd string) string {
	if len(cmd) > maxE2ECommandLen {
		return ""
	}
	for _, r := range cmd {
		if unicode.IsControl(r) {
			return ""
		}
	}
	return cmd
}

// integrationGate blocks an epic's done transition while its integration child is unresolved.
func (b *Backlog) integrationGate(ctx context.Context, epic *ent.BacklogItem) error {
	if epic.IntegrationChildID == nil {
		return nil
	}
	child, err := b.client.BacklogItem.Get(ctx, *epic.IntegrationChildID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil // a hard-deleted child can never finish; treat as resolved, like a deleted blocker
		}
		return fmt.Errorf("check integration child of %s: %w", epic.ID, err)
	}
	// Only done is a passing e2e check: an archived child never ran (or abandoned) the check.
	if child.Status != string(domain.BacklogStatusDone) {
		return fmt.Errorf("%w: integration child %s is %s", backlogdto.ErrIntegrationCheckPending, child.ID, child.Status)
	}
	return nil
}

// CreateIntegrationChild creates the epic's integration child, blocked on every sibling, and
// records it as the epic's IntegrationChildID. Idempotent: when the epic already has one it is
// returned with created=false. e2eCommand must be non-empty.
func (b *Backlog) CreateIntegrationChild(ctx context.Context, epicID string, siblingIDs []string, e2eCommand string) (child *backlogdto.BacklogItemData, created bool, err error) {
	if sanitizedE2ECommand(strings.TrimSpace(e2eCommand)) == "" {
		return nil, false, fmt.Errorf("integration child needs a non-empty single-line e2e command of at most %d characters", maxE2ECommandLen)
	}
	parsedEpic, err := b.ResolveBacklogItemLookup(ctx, epicID)
	if err != nil {
		return nil, false, fmt.Errorf("%w: invalid epic id %q: %v", domain.ErrNotFound, epicID, err)
	}
	// Resolve before opening the transaction: a public-ID lookup uses a second connection.
	siblings := make([]uuid.UUID, 0, len(siblingIDs))
	seen := map[uuid.UUID]bool{parsedEpic: true}
	for _, sid := range siblingIDs {
		parsed, perr := b.ResolveBacklogItemLookup(ctx, sid)
		if perr != nil {
			return nil, false, fmt.Errorf("%w: invalid sibling id %q: %v", domain.ErrNotFound, sid, perr)
		}
		if !seen[parsed] {
			seen[parsed] = true
			siblings = append(siblings, parsed)
		}
	}
	tx, err := b.client.Tx(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("create integration child: begin transaction: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	epic, err := tx.BacklogItem.Get(ctx, parsedEpic)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, false, fmt.Errorf("%w: epic %s", domain.ErrNotFound, epicID)
		}
		return nil, false, err
	}
	if epic.IntegrationChildID != nil {
		existing, getErr := tx.BacklogItem.Get(ctx, *epic.IntegrationChildID)
		if getErr != nil {
			return nil, false, fmt.Errorf("load existing integration child: %w", getErr)
		}
		data := BacklogItemToData(existing)
		return &data, false, nil
	}

	publicID, err := backlogdto.NewBacklogItemID()
	if err != nil {
		return nil, false, fmt.Errorf("generate integration child public id: %w", err)
	}
	acs, err := json.Marshal([]domain.AcCriterion{{Index: 0, Text: "End-to-end check passes: " + e2eCommand, Status: domain.AcStatusPending}})
	if err != nil {
		return nil, false, fmt.Errorf("encode integration child criteria: %w", err)
	}
	item, err := tx.BacklogItem.Create().
		SetTitle("Integration check: " + epic.Title).
		SetPublicID(publicID.String()).
		SetDescription(integrationChildPrompt(epic.Title, e2eCommand)).
		SetAcceptanceCriteria(string(acs)).
		SetPriority(epic.Priority).
		SetRepoPath(epic.RepoPath).
		SetBaseBranch(epic.BaseBranch).
		SetPipelineMode(epic.PipelineMode).
		SetSkipPlanning(true).
		SetAutoSpawnSession(true).
		SetIsIntegrationChild(true).
		SetLabels([]string{"integration-check"}).
		Save(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("create integration child: %w", err)
	}
	for _, sib := range siblings {
		if err := tx.BacklogItemDependency.Create().SetBlockerID(sib).SetBlockedID(item.ID).Exec(ctx); err != nil {
			return nil, false, fmt.Errorf("add integration dependency %s -> %s: %w", sib, item.ID, err)
		}
	}
	// CAS: a concurrent creator that committed first leaves integration_child_id set.
	affected, err := tx.BacklogItem.Update().
		Where(backlogitem.ID(parsedEpic), backlogitem.IntegrationChildIDIsNil()).
		SetIntegrationChildID(item.ID).
		Save(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("record integration child on epic: %w", err)
	}
	if affected == 0 {
		// A concurrent creator won the CAS: drop our rows and return theirs, like the already-exists path.
		_ = tx.Rollback()
		winner, getErr := b.client.BacklogItem.Get(ctx, parsedEpic)
		if getErr != nil || winner.IntegrationChildID == nil {
			return nil, false, fmt.Errorf("integration child for epic %s was created concurrently", epicID)
		}
		existing, getErr := b.client.BacklogItem.Get(ctx, *winner.IntegrationChildID)
		if getErr != nil {
			return nil, false, fmt.Errorf("load concurrent integration child: %w", getErr)
		}
		data := BacklogItemToData(existing)
		return &data, false, nil
	}
	if err := tx.Commit(); err != nil {
		return nil, false, fmt.Errorf("create integration child: commit: %w", err)
	}
	data := BacklogItemToData(item)
	return &data, true, nil
}

func integrationChildPrompt(epicTitle, cmd string) string {
	return fmt.Sprintf("All sibling items of the epic %q are done, but each passed on its own checks. "+
		"Verify the whole feature works end to end.\n\nRun this end-to-end check and make it pass:\n\n    %s\n\n"+
		"Do not mark the criterion done unless the command exits 0. If it fails, report the failing files/areas; "+
		"do not paper over the failure.", epicTitle, cmd)
}

// GetEpicForIntegrationChild returns the epic whose IntegrationChildID is childID, or ErrNotFound.
func (b *Backlog) GetEpicForIntegrationChild(ctx context.Context, childID string) (*backlogdto.BacklogItemData, error) {
	parsed, err := b.ResolveBacklogItemLookup(ctx, childID)
	if err != nil {
		return nil, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, childID, err)
	}
	epic, err := b.client.BacklogItem.Query().Where(backlogitem.IntegrationChildID(parsed)).Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, fmt.Errorf("%w: no epic owns integration child %s", domain.ErrNotFound, childID)
		}
		return nil, err
	}
	data := BacklogItemToData(epic)
	return &data, nil
}

// ReserveIntegrationFix atomically claims one fix-child slot on the epic: it bumps
// integration_fix_count only while it is below fixCap (fixCap <= 0 means unlimited) and reports
// whether the slot was granted. Call it before creating the fix child and ReleaseIntegrationFix if
// creation fails.
func (b *Backlog) ReserveIntegrationFix(ctx context.Context, epicID string, fixCap int) (bool, error) {
	parsed, err := b.ResolveBacklogItemLookup(ctx, epicID)
	if err != nil {
		return false, fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, epicID, err)
	}
	q := b.client.BacklogItem.Update().Where(backlogitem.ID(parsed))
	if fixCap > 0 {
		q = q.Where(backlogitem.IntegrationFixCountLT(fixCap))
	}
	affected, err := q.AddIntegrationFixCount(1).Save(ctx)
	if err != nil {
		return false, fmt.Errorf("reserve integration fix slot: %w", err)
	}
	return affected == 1, nil
}

// ReleaseIntegrationFix returns a slot taken by ReserveIntegrationFix.
func (b *Backlog) ReleaseIntegrationFix(ctx context.Context, epicID string) error {
	parsed, err := b.ResolveBacklogItemLookup(ctx, epicID)
	if err != nil {
		return fmt.Errorf("%w: invalid id %q: %v", domain.ErrNotFound, epicID, err)
	}
	return b.client.BacklogItem.Update().
		Where(backlogitem.ID(parsed), backlogitem.IntegrationFixCountGT(0)).
		AddIntegrationFixCount(-1).Exec(ctx)
}
