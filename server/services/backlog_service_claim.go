package services

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// crossHostClaimDedupFlagName gates every cross-host claim check. Off by
// default: with it off, creation and dequeue behave exactly as they did before
// the claim substrate existed. Claim recording (Storage.CreateBacklogItem) is
// always on.
const crossHostClaimDedupFlagName = "cross_host_claim_dedup"

// minClaimOverrideReasonLength mirrors the web app's MIN_OVERRIDE_REASON_LENGTH
// (GateVerdictBox) so the server enforces what the UI form already requires.
const minClaimOverrideReasonLength = 5

// SetClaimChecker wires the cross-host claim checker and dispute resolver.
// Either may be nil.
func (s *BacklogService) SetClaimChecker(checker CrossHostClaimChecker, resolver ClaimDisputeResolver) {
	s.claimChecker = checker
	s.claimDisputeResolver = resolver
}

func (s *BacklogService) crossHostClaimDedupEnabled() bool {
	if s.claimDedupFlag != nil {
		return s.claimDedupFlag()
	}
	return config.LoadConfig().GetFeatureFlag(crossHostClaimDedupFlagName)
}

func (s *BacklogService) checker() CrossHostClaimChecker {
	if s.claimChecker == nil {
		return unimplementedClaimChecker{}
	}
	return s.claimChecker
}

func validateClaimOverrideReason(reason string) error {
	if len(strings.TrimSpace(reason)) < minClaimOverrideReasonLength {
		return connect.NewError(connect.CodeInvalidArgument,
			fmt.Errorf("override reason must be at least %d characters", minClaimOverrideReasonLength))
	}
	return nil
}

func alreadyClaimedElsewhereProto(externalURL string, record session.ClaimRecord) *sessionv1.AlreadyClaimedElsewhere {
	return &sessionv1.AlreadyClaimedElsewhere{
		ExternalUrl:    externalURL,
		ClaimingHostId: record.ClaimingHostID.String(),
		ItemDeepLink:   record.ItemDeepLink,
		Disputed:       record.Disputed,
	}
}

// preCreateClaimCheck describes one creation attempt for gateOnClaim.
type preCreateClaimCheck struct {
	ExternalURL string
	Override    bool
	Reason      string
	// LogPrefix names the call site in audit logs, e.g. "import_github_issue".
	LogPrefix string
}

// claimGateResult is gateOnClaim's outcome. Claimed is non-nil when creation
// must not proceed.
type claimGateResult struct {
	Claimed *sessionv1.AlreadyClaimedElsewhere
}

// gateOnClaim runs the live cross-host claim check before an interactive
// creation. Its result's Claimed is set when creation must not proceed. An indeterminate check or a checker error never blocks: creation
// proceeds optimistically and the gap is logged. A no-op when the feature flag
// is off or there is no external URL.
func (s *BacklogService) gateOnClaim(ctx context.Context, c preCreateClaimCheck) (claimGateResult, error) {
	if c.ExternalURL == "" || !s.crossHostClaimDedupEnabled() {
		return claimGateResult{}, nil
	}
	if c.Override {
		if err := validateClaimOverrideReason(c.Reason); err != nil {
			return claimGateResult{}, err
		}
	}

	verdict, err := s.checker().CheckClaim(ctx, c.ExternalURL)
	if err != nil {
		log.Warn(c.LogPrefix+".claim_check_failed", "external_url", c.ExternalURL, "err", err)
		return claimGateResult{}, nil
	}
	switch verdict.Kind {
	case ClaimHeldByOther:
		if !c.Override {
			return claimGateResult{Claimed: alreadyClaimedElsewhereProto(c.ExternalURL, verdict.Record)}, nil
		}
		log.Info(c.LogPrefix+".claim_override",
			"external_url", c.ExternalURL,
			"claiming_host_id", verdict.Record.ClaimingHostID.String(),
			"reason", strings.TrimSpace(c.Reason))
	case ClaimCheckIndeterminate:
		log.Warn(c.LogPrefix+".claim_check_indeterminate", "external_url", c.ExternalURL)
	}
	return claimGateResult{}, nil
}

// ErrCrossHostClaimDedupDisabled is returned by CheckCrossHostClaim while the
// cross_host_claim_dedup feature flag is off.
var ErrCrossHostClaimDedupDisabled = errors.New("cross_host_claim_dedup is disabled")

// CheckCrossHostClaim runs the same live claim check the import and create paths
// use, for the check_cross_host_claim MCP tool.
func (s *BacklogService) CheckCrossHostClaim(ctx context.Context, externalURL string) (ClaimVerdict, error) {
	if !s.crossHostClaimDedupEnabled() {
		return NewUnclaimedVerdict(), ErrCrossHostClaimDedupDisabled
	}
	return s.checker().CheckClaim(ctx, externalURL)
}

// skipForForeignClaim reports whether DequeueNextQueuedItems must skip item
// because another host holds its claim, and makes the skip visible as a
// StuckReasonBlockedByClaim row. Local-only: it runs under dequeueMu, so it must
// never touch the network. Callers check the feature flag once per sweep.
func (s *BacklogService) skipForForeignClaim(ctx context.Context, item *session.BacklogItemData) bool {
	if item.ExternalURL == "" {
		return false
	}
	verdict, err := s.checker().CheckClaimLocalOnly(ctx, item.ExternalURL)
	if err != nil {
		log.Warn("dequeue.claim_check_failed", "item", item.ID, "err", err)
		return false
	}
	if verdict.Kind != ClaimHeldByOther {
		return false
	}
	log.Info("dequeue.blocked_by_claim",
		"item", item.ID,
		"external_url", item.ExternalURL,
		"claiming_host_id", verdict.Record.ClaimingHostID.String())
	s.notifyBlockedByClaim(ctx, item, verdict.Record)
	return true
}

// notifyBlockedByClaim writes the durable StuckReasonBlockedByClaim row,
// mirroring notifyBlockedByDependency's call shape.
func (s *BacklogService) notifyBlockedByClaim(ctx context.Context, item *session.BacklogItemData, claim session.ClaimRecord) {
	message := fmt.Sprintf("claimed by host %s (%s); override to work on it from this host",
		claim.ClaimingHostID.String(), claim.ItemDeepLink)
	applied, err := s.storage.MarkStuck(ctx, item.ID, domain.StuckReasonBlockedByClaim, session.BacklogStatus(item.Status), message)
	if err != nil {
		log.Warn("[notifyBlockedByClaim] MarkStuck failed", "item", item.ID, "error", err)
		return
	}
	if applied {
		if _, notifyErr := s.storage.MarkStuckNotified(ctx, item.ID, domain.StuckReasonBlockedByClaim); notifyErr != nil {
			log.Warn("[notifyBlockedByClaim] MarkStuckNotified failed", "item", item.ID, "error", notifyErr)
		}
	}
}

func (s *BacklogService) resolveClaimBlockedLogged(ctx context.Context, itemID string) {
	if _, err := s.storage.ResolveStuck(ctx, itemID, domain.StuckReasonBlockedByClaim); err != nil {
		log.Warn("[resolveClaimBlockedLogged] ResolveStuck failed", "item", itemID, "error", err)
	}
}

// OverrideClaimBlock dequeues and spawns an item DequeueNextQueuedItems skipped
// for StuckReasonBlockedByClaim, bypassing only the claim check. It still honors
// the WIP cap and every transition guard.
// +api: OverrideClaimBlock
func (s *BacklogService) OverrideClaimBlock(ctx context.Context, req *connect.Request[sessionv1.OverrideClaimBlockRequest]) (*connect.Response[sessionv1.OverrideClaimBlockResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	itemID := strings.TrimSpace(req.Msg.ItemId)
	if itemID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("item_id is required"))
	}
	if err := validateClaimOverrideReason(req.Msg.Reason); err != nil {
		return nil, err
	}

	s.dequeueMu.Lock()
	defer s.dequeueMu.Unlock()

	item, err := s.storage.GetBacklogItem(ctx, itemID)
	if err != nil {
		if errors.Is(err, session.ErrNotFound) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", itemID))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to load backlog item: %w", err))
	}
	if status := session.BacklogStatus(item.Status); status != session.BacklogStatusQueued && status != session.BacklogStatusReady {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("item is %q; only queued or ready items can have a claim block overridden", item.Status))
	}

	liveCount, err := s.countLiveBacklogWorkSessions(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("count live work sessions: %w", err))
	}
	if s.maxConcurrentBacklogWorkItems()-liveCount <= 0 {
		return nil, connect.NewError(connect.CodeResourceExhausted,
			fmt.Errorf("no free work slot right now; the item stays queued, retry the override when a slot frees"))
	}

	claimingHost := ""
	if verdict, checkErr := s.checker().CheckClaimLocalOnly(ctx, item.ExternalURL); checkErr == nil && verdict.Kind == ClaimHeldByOther {
		claimingHost = verdict.Record.ClaimingHostID.String()
	}
	log.Info("dequeue.claim_override",
		"item_id", item.ID,
		"external_url", item.ExternalURL,
		"claiming_host_id", claimingHost,
		"reason", strings.TrimSpace(req.Msg.Reason))

	blockers, err := s.storage.UnresolvedBlockerItemIDs(ctx, []string{item.ID})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("check unresolved blockers: %w", err))
	}
	if !s.claimAndSpawnCandidate(ctx, *item, blockers[item.ID]) {
		return nil, connect.NewError(connect.CodeFailedPrecondition,
			fmt.Errorf("the item could not be claimed and spawned; see its stuck reason and the server log"))
	}
	s.resolveClaimBlockedLogged(ctx, item.ID)

	updated, err := s.storage.GetBacklogItem(ctx, item.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to reload backlog item: %w", err))
	}
	return connect.NewResponse(&sessionv1.OverrideClaimBlockResponse{
		Item: backlogItemToProto(updated, s.engine, s.buildCostLookup()),
	}), nil
}

// ResolveClaimDispute clears the local Disputed flag for an external URL once a
// human has looked at it. It never picks a winner.
// +api: ResolveClaimDispute
func (s *BacklogService) ResolveClaimDispute(ctx context.Context, req *connect.Request[sessionv1.ResolveClaimDisputeRequest]) (*connect.Response[sessionv1.ResolveClaimDisputeResponse], error) {
	externalURL := strings.TrimSpace(req.Msg.ExternalUrl)
	if externalURL == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("external_url is required"))
	}
	if s.claimDisputeResolver == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("cross-host claims are not enabled on this host"))
	}
	if err := s.claimDisputeResolver.ResolveDispute(ctx, externalURL); err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to resolve claim dispute: %w", err))
	}
	log.Info("claim_dispute.resolved", "external_url", externalURL, "reason", strings.TrimSpace(req.Msg.Reason))
	return connect.NewResponse(&sessionv1.ResolveClaimDisputeResponse{}), nil
}

// flagGatedForeignClaimLookup makes a session.ForeignClaimLookup honor the
// cross_host_claim_dedup flag, so SyncOne is unchanged while the flag is off.
type flagGatedForeignClaimLookup struct {
	inner session.ForeignClaimLookup
}

// NewFlagGatedForeignClaimLookup wraps inner so it reports no claims while the
// cross_host_claim_dedup feature flag is off.
func NewFlagGatedForeignClaimLookup(inner session.ForeignClaimLookup) session.ForeignClaimLookup {
	return flagGatedForeignClaimLookup{inner: inner}
}

func (f flagGatedForeignClaimLookup) ForeignClaim(externalURL string) (session.ClaimRecord, bool) {
	if !config.LoadConfig().GetFeatureFlag(crossHostClaimDedupFlagName) {
		return session.ClaimRecord{}, false
	}
	return f.inner.ForeignClaim(externalURL)
}
