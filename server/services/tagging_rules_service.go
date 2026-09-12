package services

import (
	"context"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/pkg/classifier"
)

// taggingRuleFireWindow is the lookback window backing the "Fires(7d)" column
// ux.md's Surface 6 mandates for the tagging-rules tab.
const taggingRuleFireWindow = 7 * 24 * time.Hour

// TaggingRuleWithFireCount pairs a TaggingRuleSpec with its recent fire count, so
// ListTaggingRules's callers get both without a second round-trip.
type TaggingRuleWithFireCount struct {
	TaggingRuleSpec
	// FireCount7d is the number of times this rule matched in the last 7 days. Always 0
	// when the service's analyticsStore is nil or has no recorded fires for this rule —
	// never an error.
	FireCount7d int `json:"fire_count_7d"`
}

// TaggingRulesService rebuilds the live TaggingEngine after every CRUD mutation, mirroring
// RulesService's CRUD subset. AI-suggestion authoring is out of scope for tagging rules
// (see requirements.md's Out of Scope) — no promptBuilder/aiClient fields.
type TaggingRulesService struct {
	rulesStore     *TaggingRulesStore
	engine         *classifier.TaggingEngine
	analyticsStore *AnalyticsStore // nil = fire counts unavailable; ListTaggingRules defaults to 0
}

// NewTaggingRulesService creates a TaggingRulesService. analyticsStore may be nil — see
// TaggingRuleWithFireCount.FireCount7d's doc comment.
func NewTaggingRulesService(rulesStore *TaggingRulesStore, engine *classifier.TaggingEngine, analyticsStore *AnalyticsStore) *TaggingRulesService {
	return &TaggingRulesService{
		rulesStore:     rulesStore,
		engine:         engine,
		analyticsStore: analyticsStore,
	}
}

// ListTaggingRules returns every stored tagging rule, each merged with its 7-day fire
// count by RuleID.
func (ts *TaggingRulesService) ListTaggingRules(ctx context.Context) ([]TaggingRuleWithFireCount, error) {
	specs := ts.rulesStore.All()

	fireCounts := map[string]int{}
	if ts.analyticsStore != nil {
		counts, err := ts.analyticsStore.GetTaggingRuleFireCounts(ctx, time.Now().Add(-taggingRuleFireWindow))
		if err != nil {
			log.Warn("[TaggingRulesService] failed to load tagging rule fire counts", "err", err)
		} else {
			fireCounts = counts
		}
	}

	result := make([]TaggingRuleWithFireCount, len(specs))
	for i, spec := range specs {
		result[i] = TaggingRuleWithFireCount{
			TaggingRuleSpec: spec,
			FireCount7d:     fireCounts[spec.ID],
		}
	}
	return result, nil
}

// UpsertTaggingRule creates or updates a tagging rule and, on success, rebuilds the live
// TaggingEngine so the change takes effect on the very next ApplyToFixpoint call — no
// restart or explicit reload needed. A rejected (e.g. bad-regex) upsert never touches the
// engine, leaving it unchanged.
func (ts *TaggingRulesService) UpsertTaggingRule(ctx context.Context, spec TaggingRuleSpec) (TaggingRuleSpec, error) {
	saved, err := ts.rulesStore.Upsert(ctx, spec)
	if err != nil {
		return TaggingRuleSpec{}, err
	}
	ts.rebuildEngine()
	log.Info("[TaggingRulesService] upserted tagging rule", "id", saved.ID)
	return saved, nil
}

// DeleteTaggingRule removes a tagging rule by ID and rebuilds the live TaggingEngine.
func (ts *TaggingRulesService) DeleteTaggingRule(ctx context.Context, id string) error {
	if err := ts.rulesStore.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete tagging rule: %w", err)
	}
	ts.rebuildEngine()
	log.Info("[TaggingRulesService] deleted tagging rule", "id", id)
	return nil
}

// rebuildEngine replaces the live TaggingEngine's rules with the current store contents.
// Serialization belongs here (not in the engine — ReplaceRules is already atomic per-call,
// but this service is the one place that knows "read store, then replace engine" must run
// as one unit relative to itself), mirroring RulesService.rebuildClassifier's rationale.
// TaggingRulesStore.Upsert/Delete already hold their own mutex around the store mutation
// (see that type), so serializing here only needs to guard the store-read + engine-replace
// pair — a single TaggingRulesStore method, ToRules, already returns a full atomic snapshot,
// so no additional lock is needed beyond the store's own.
func (ts *TaggingRulesService) rebuildEngine() {
	ts.engine.ReplaceRules(ts.rulesStore.ToRules())
}

// taggingRuleSpecToProto converts a TaggingRuleSpec plus its 7-day fire count to the
// wire representation, mirroring rules_service.go's specToProto.
func taggingRuleSpecToProto(spec TaggingRuleSpec, fireCount7d int) *sessionv1.TaggingRuleProto {
	p := &sessionv1.TaggingRuleProto{
		Id:             spec.ID,
		Name:           spec.Name,
		NamePattern:    spec.NamePattern,
		BranchPattern:  spec.BranchPattern,
		PathPattern:    spec.PathPattern,
		ProgramPattern: spec.ProgramPattern,
		RequiredTags:   spec.RequiredTags,
		OutputTag:      spec.OutputTag,
		Priority:       int32(spec.Priority), // #nosec G115 -- bounded the same way as ApprovalRuleProto.Priority (see specToProto)
		Enabled:        spec.Enabled,
		Source:         spec.Source,
		FireCount_7D:   int32(fireCount7d), // #nosec G115 -- fire counts are small, bounded by the 7-day analytics window
	}
	if !spec.CreatedAt.IsZero() {
		p.CreatedAt = timestamppb.New(spec.CreatedAt)
	}
	return p
}

// protoToTaggingRuleSpec converts the wire representation back to a TaggingRuleSpec for
// persistence. FireCount7d is server-computed and never round-tripped back in.
func protoToTaggingRuleSpec(p *sessionv1.TaggingRuleProto) TaggingRuleSpec {
	spec := TaggingRuleSpec{
		ID:             p.GetId(),
		Name:           p.GetName(),
		NamePattern:    p.GetNamePattern(),
		BranchPattern:  p.GetBranchPattern(),
		PathPattern:    p.GetPathPattern(),
		ProgramPattern: p.GetProgramPattern(),
		RequiredTags:   p.GetRequiredTags(),
		OutputTag:      p.GetOutputTag(),
		Priority:       int(p.GetPriority()),
		Enabled:        p.GetEnabled(),
		Source:         p.GetSource(),
	}
	if p.GetCreatedAt() != nil {
		spec.CreatedAt = p.GetCreatedAt().AsTime()
	}
	return spec
}

// ListTaggingRulesRPC is the ConnectRPC-facing counterpart of ListTaggingRules, returning
// the wire proto shape instead of TaggingRuleWithFireCount.
func (ts *TaggingRulesService) ListTaggingRulesRPC(
	ctx context.Context,
	req *connect.Request[sessionv1.ListTaggingRulesRequest],
) (*connect.Response[sessionv1.ListTaggingRulesResponse], error) {
	rules, err := ts.ListTaggingRules(ctx)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	out := make([]*sessionv1.TaggingRuleProto, 0, len(rules))
	for _, r := range rules {
		out = append(out, taggingRuleSpecToProto(r.TaggingRuleSpec, r.FireCount7d))
	}
	return connect.NewResponse(&sessionv1.ListTaggingRulesResponse{Rules: out}), nil
}

// UpsertTaggingRuleRPC is the ConnectRPC-facing counterpart of UpsertTaggingRule. Unlike
// UpsertApprovalRule's RPC (which requires a caller-supplied id — see tools_rules.go's
// documented discrepancy), this generates an id via uuid.New() when the caller omits one,
// matching Story 5.1.1's acceptance criterion of upserting a rule with no id.
func (ts *TaggingRulesService) UpsertTaggingRuleRPC(
	ctx context.Context,
	req *connect.Request[sessionv1.UpsertTaggingRuleRequest],
) (*connect.Response[sessionv1.UpsertTaggingRuleResponse], error) {
	if req.Msg.GetRule() == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("rule is required"))
	}
	r := req.Msg.GetRule()
	if r.GetOutputTag() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("rule.output_tag is required"))
	}

	spec := protoToTaggingRuleSpec(r)
	isCreate := spec.ID == ""
	if isCreate {
		spec.ID = uuid.New().String()
	}
	if spec.Source == "" {
		spec.Source = "user"
	}

	saved, err := ts.UpsertTaggingRule(ctx, spec)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&sessionv1.UpsertTaggingRuleResponse{
		Rule:    taggingRuleSpecToProto(saved, 0),
		Created: isCreate,
	}), nil
}

// DeleteTaggingRuleRPC is the ConnectRPC-facing counterpart of DeleteTaggingRule.
func (ts *TaggingRulesService) DeleteTaggingRuleRPC(
	ctx context.Context,
	req *connect.Request[sessionv1.DeleteTaggingRuleRequest],
) (*connect.Response[sessionv1.DeleteTaggingRuleResponse], error) {
	if req.Msg.GetId() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("id is required"))
	}
	if err := ts.DeleteTaggingRule(ctx, req.Msg.GetId()); err != nil {
		return nil, connect.NewError(connect.CodeNotFound, err)
	}
	return connect.NewResponse(&sessionv1.DeleteTaggingRuleResponse{
		Success: true,
		Message: fmt.Sprintf("Rule %s deleted", req.Msg.GetId()),
	}), nil
}
