package mcp

import (
	"context"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
	"github.com/tstapler/stapler-squad/session/hostclaim"
	"github.com/tstapler/stapler-squad/session/instance"
)

// recordingBacklogRPC is the BacklogRPC test fake: it records each call's arguments and
// returns canned results, so handler tests pin what the tools hand to the service.
type recordingBacklogRPC struct {
	hosts       []string
	blocked     []session.BlockedItem
	triageLive  bool
	triageLiveQ []string

	addReqs        []*sessionv1.AddBacklogItemDependencyRequest
	removeReqs     []*sessionv1.RemoveBacklogItemDependencyRequest
	triggerReqs    []*sessionv1.TriggerTriageRequest
	epicChildReqs  []*sessionv1.CreateEpicIntegrationChildRequest
	transitionReqs []*sessionv1.TransitionBacklogItemStatusRequest
	transitionActs []string
	transitionResp *sessionv1.TransitionBacklogItemStatusResponse
	claimURLs      []string
	claimErr       error
}

var _ BacklogRPC = (*recordingBacklogRPC)(nil)

func (f *recordingBacklogRPC) LookupCrossHostClaim(_ context.Context, u string) (hostclaim.ClaimVerdict, error) {
	f.claimURLs = append(f.claimURLs, u)
	return hostclaim.NewUnclaimedVerdict(), f.claimErr
}
func (f *recordingBacklogRPC) MaybeTriggerTriage(context.Context, string, bool, string) bool {
	return false
}
func (f *recordingBacklogRPC) IsTriageLive(id string) bool {
	f.triageLiveQ = append(f.triageLiveQ, id)
	return f.triageLive
}
func (f *recordingBacklogRPC) EnterpriseHosts() []string              { return f.hosts }
func (f *recordingBacklogRPC) BlockedCleanups() []session.BlockedItem { return f.blocked }
func (f *recordingBacklogRPC) ListBacklogItems(context.Context, *connect.Request[sessionv1.ListBacklogItemsRequest]) (*connect.Response[sessionv1.ListBacklogItemsResponse], error) {
	return connect.NewResponse(&sessionv1.ListBacklogItemsResponse{}), nil
}
func (f *recordingBacklogRPC) AttachSessionToItem(context.Context, *connect.Request[sessionv1.AttachSessionToItemRequest]) (*connect.Response[sessionv1.AttachSessionToItemResponse], error) {
	return connect.NewResponse(&sessionv1.AttachSessionToItemResponse{}), nil
}
func (f *recordingBacklogRPC) AddBacklogItemDependency(_ context.Context, r *connect.Request[sessionv1.AddBacklogItemDependencyRequest]) (*connect.Response[sessionv1.AddBacklogItemDependencyResponse], error) {
	f.addReqs = append(f.addReqs, r.Msg)
	return connect.NewResponse(&sessionv1.AddBacklogItemDependencyResponse{}), nil
}
func (f *recordingBacklogRPC) RemoveBacklogItemDependency(_ context.Context, r *connect.Request[sessionv1.RemoveBacklogItemDependencyRequest]) (*connect.Response[sessionv1.RemoveBacklogItemDependencyResponse], error) {
	f.removeReqs = append(f.removeReqs, r.Msg)
	return connect.NewResponse(&sessionv1.RemoveBacklogItemDependencyResponse{}), nil
}
func (f *recordingBacklogRPC) CreateEpicIntegrationChild(_ context.Context, r *connect.Request[sessionv1.CreateEpicIntegrationChildRequest]) (*connect.Response[sessionv1.CreateEpicIntegrationChildResponse], error) {
	f.epicChildReqs = append(f.epicChildReqs, r.Msg)
	return connect.NewResponse(&sessionv1.CreateEpicIntegrationChildResponse{Item: &sessionv1.BacklogItem{Id: "child-1"}}), nil
}
func (f *recordingBacklogRPC) TriggerTriage(_ context.Context, r *connect.Request[sessionv1.TriggerTriageRequest]) (*connect.Response[sessionv1.TriggerTriageResponse], error) {
	f.triggerReqs = append(f.triggerReqs, r.Msg)
	return connect.NewResponse(&sessionv1.TriggerTriageResponse{}), nil
}
func (f *recordingBacklogRPC) TransitionBacklogItemStatus(ctx context.Context, r *connect.Request[sessionv1.TransitionBacklogItemStatusRequest]) (*connect.Response[sessionv1.TransitionBacklogItemStatusResponse], error) {
	f.transitionReqs = append(f.transitionReqs, r.Msg)
	f.transitionActs = append(f.transitionActs, domain.TransitionActorFromContext(ctx))
	if f.transitionResp != nil {
		return connect.NewResponse(f.transitionResp), nil
	}
	return connect.NewResponse(&sessionv1.TransitionBacklogItemStatusResponse{}), nil
}

func callTool(t *testing.T, fn func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error), args map[string]any) *mcpgo.CallToolResult {
	t.Helper()
	req := mcpgo.CallToolRequest{}
	req.Params.Arguments = args
	res, err := fn(context.Background(), req)
	require.NoError(t, err)
	return res
}

func TestBacklogHandlers_should_UseBacklogRPCPort_When_FakeInjected(t *testing.T) {
	t.Parallel()
	const blocker, blocked = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222"

	t.Run("enterpriseHosts returns the service's hosts, nil when unwired", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, (&backlogHandlers{}).enterpriseHosts())
		fake := &recordingBacklogRPC{hosts: []string{"ghe.example.com"}}
		assert.Equal(t, []string{"ghe.example.com"}, (&backlogHandlers{backlogSvc: fake}).enterpriseHosts())
	})

	t.Run("headlessLive asks the service about triage, and only triage", func(t *testing.T) {
		t.Parallel()
		fake := &recordingBacklogRPC{triageLive: false}
		h := &backlogHandlers{backlogSvc: fake}
		assert.False(t, h.headlessLive(domain.HeadlessRoleTriage, "item-1"))
		assert.Equal(t, []string{"item-1"}, fake.triageLiveQ)
		assert.True(t, (&backlogHandlers{}).headlessLive(domain.HeadlessRoleTriage, "item-1"), "unwired fails closed to live")
		fake.triageLive = true
		assert.True(t, h.headlessLive(domain.HeadlessRoleTriage, "item-1"))
	})

	t.Run("writeBlockedCleanup lists only the matching item", func(t *testing.T) {
		t.Parallel()
		fake := &recordingBacklogRPC{blocked: []session.BlockedItem{
			{ItemID: "other", Path: "/p/other", Reason: "other-reason", Since: time.Now()},
			{ItemID: "item-1", Path: "/p/one", Reason: "held by owner", Since: time.Now().Add(-time.Hour)},
		}}
		var sb strings.Builder
		(&backlogHandlers{backlogSvc: fake}).writeBlockedCleanup(&sb, "item-1")
		assert.Contains(t, sb.String(), "/p/one")
		assert.Contains(t, sb.String(), "held by owner")
		assert.NotContains(t, sb.String(), "other-reason")
	})

	t.Run("dependency tools pass blocker and blocked through", func(t *testing.T) {
		t.Parallel()
		fake := &recordingBacklogRPC{}
		h := &backlogHandlers{backlogSvc: fake}
		args := map[string]any{"blocker_item_id": blocker, "blocked_item_id": blocked}
		callTool(t, h.addBacklogItemDependency, args)
		callTool(t, h.removeBacklogItemDependency, args)
		require.Len(t, fake.addReqs, 1)
		assert.Equal(t, blocker, fake.addReqs[0].BlockerItemId)
		assert.Equal(t, blocked, fake.addReqs[0].BlockedItemId)
		require.Len(t, fake.removeReqs, 1)
		assert.Equal(t, blocker, fake.removeReqs[0].BlockerItemId)
		assert.Equal(t, blocked, fake.removeReqs[0].BlockedItemId)
	})

	t.Run("trigger_triage passes item and feedback", func(t *testing.T) {
		t.Parallel()
		fake := &recordingBacklogRPC{}
		callTool(t, (&backlogHandlers{backlogSvc: fake}).triggerTriage, map[string]any{"item_id": blocker, "feedback": "look again"})
		require.Len(t, fake.triggerReqs, 1)
		assert.Equal(t, blocker, fake.triggerReqs[0].ItemId)
		assert.Equal(t, "look again", fake.triggerReqs[0].Feedback)
	})

	t.Run("transition tool stamps the agent actor and omits override", func(t *testing.T) {
		t.Parallel()
		fake := &recordingBacklogRPC{transitionResp: &sessionv1.TransitionBacklogItemStatusResponse{}}
		res := callTool(t, (&backlogHandlers{backlogSvc: fake}).transitionBacklogItemStatus,
			map[string]any{"item_id": blocker, "target_status": "review", "expected_status": "in_progress"})
		assert.False(t, res.IsError)
		require.Len(t, fake.transitionReqs, 1)
		assert.Equal(t, "review", fake.transitionReqs[0].TargetStatus)
		assert.Equal(t, "in_progress", fake.transitionReqs[0].ExpectedStatus)
		assert.Empty(t, fake.transitionReqs[0].OverrideReason)
		assert.Equal(t, instance.TriggeredByAgent, fake.transitionActs[0])
	})

	t.Run("unwired service answers unavailable, not a panic", func(t *testing.T) {
		t.Parallel()
		h := &backlogHandlers{}
		for name, fn := range map[string]func(context.Context, mcpgo.CallToolRequest) (*mcpgo.CallToolResult, error){
			"trigger": h.triggerTriage, "transition": h.transitionBacklogItemStatus, "add": h.addBacklogItemDependency,
		} {
			res := callTool(t, fn, map[string]any{"item_id": blocker, "blocker_item_id": blocker, "blocked_item_id": blocked, "target_status": "review", "expected_status": "in_progress"})
			assert.Contains(t, toolText(t, res), errBacklogSvcNotWired, name)
		}
	})
}

func TestNewClaimModule_should_RegisterOnlyWhenBacklogRPCWired(t *testing.T) {
	t.Parallel()
	assert.Nil(t, newClaimModule(nil, nil), "a nil interface leaves the claim tool unregistered")
	assert.NotNil(t, newClaimModule(&recordingBacklogRPC{}, nil))
}

func TestCreateEpicIntegrationChildTool_should_ForwardEpicAndSiblings(t *testing.T) {
	t.Parallel()
	const epic, a, b = "11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"
	fake := &recordingBacklogRPC{}
	h := &backlogHandlers{backlogSvc: fake}

	res := callTool(t, h.createEpicIntegrationChild, map[string]any{"epic_item_id": epic, "sibling_item_ids": []any{a, b}})
	require.Contains(t, toolText(t, res), "Integration child child-1 is in place")
	require.Len(t, fake.epicChildReqs, 1)
	assert.Equal(t, epic, fake.epicChildReqs[0].EpicItemId)
	assert.Equal(t, []string{a, b}, fake.epicChildReqs[0].SiblingItemIds)

	bad := callTool(t, h.createEpicIntegrationChild, map[string]any{"epic_item_id": epic, "sibling_item_ids": []any{"not-a-uuid"}})
	assert.Contains(t, toolText(t, bad), ErrInvalidArgument)
	empty := callTool(t, h.createEpicIntegrationChild, map[string]any{"epic_item_id": epic})
	assert.Contains(t, toolText(t, empty), ErrInvalidArgument)
	assert.Len(t, fake.epicChildReqs, 1, "invalid calls never reach the service")
}

func TestIntegrationChildResultText_should_SayPassed_When_ChildAlreadyPassed(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"done", "archived"} {
		assert.Contains(t, integrationChildResultText(&sessionv1.BacklogItem{Id: "c", Status: status}, "e", 2), "already passed")
	}
	assert.Contains(t, integrationChildResultText(&sessionv1.BacklogItem{Id: "c", Status: "idea"}, "e", 2), "cannot be marked done")
}
