// Package services_test (an external test package, not services) is required
// here because this test exercises server/mcp's ResolveItemLinkViaMCPAdapter
// directly — server/mcp already imports server/services (backlogHandlers.backlogSvc),
// so an internal (package services) test file in this directory cannot import
// server/mcp without creating a build cycle. Same reasoning as
// watch_sessions_native_streaming_integration_test.go's package doc comment.
package services_test

import (
	"context"
	"encoding/json"
	"testing"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/mcp"
	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/domain"
)

// newParityTestStorage returns a *session.Storage backed by a fresh in-memory
// EntRepository, mirroring session/item_link_test.go's
// newTestStorageForItemLink helper (which this test's fixtures are modeled
// on) so both ownership checks below run against the identical scenario.
func newParityTestStorage(t *testing.T) *session.Storage {
	t.Helper()
	repo := session.NewTestEntRepository(t)
	storage, err := session.NewStorageWithRepository(repo)
	require.NoError(t, err)
	return storage
}

// mcpErrorCodeAndRemediation decodes the (code, remediation) pair from an
// error-shaped *mcpgo.CallToolResult, the JSON shape errResult produces
// (server/mcp/tools_discovery.go).
func mcpErrorCodeAndRemediation(t *testing.T, res *mcpgo.CallToolResult) (code, remediation string) {
	t.Helper()
	require.NotNil(t, res)
	require.NotEmpty(t, res.Content)
	tc, ok := res.Content[0].(mcpgo.TextContent)
	require.True(t, ok, "expected TextContent, got %T", res.Content[0])

	var parsed struct {
		Success bool `json:"success"`
		Error   struct {
			Code        string `json:"code"`
			Remediation string `json:"remediation"`
		} `json:"error"`
	}
	require.NoError(t, json.Unmarshal([]byte(tc.Text), &parsed))
	require.False(t, parsed.Success, "expected an error result")
	return parsed.Error.Code, parsed.Error.Remediation
}

// ownershipParityCase is one (callerUUID, itemID) fixture and its expected
// (error code, remediation text) pair on both the MCP and RPC ownership-check
// paths.
type ownershipParityCase struct {
	name        string
	callerUUID  string
	itemID      string
	wantMCPCode string
	wantRPCCode connect.Code
}

// assertOwnershipParity exercises both the MCP adapter (resolveItemLink, via
// ResolveItemLinkViaMCPAdapter) and the RPC adapter
// (GuidanceRequestService.CreateGuidanceRequest's ownership check) against
// tc's fixture, and asserts they produce a matching (error code, remediation
// text) pair — split out of the table test below to keep that function under
// the house function-length gate.
func assertOwnershipParity(t *testing.T, ctx context.Context, storage *session.Storage, svc *services.GuidanceRequestService, tc ownershipParityCase) {
	t.Helper()

	_, mcpRes := mcp.ResolveItemLinkViaMCPAdapter(ctx, storage, tc.callerUUID, tc.itemID)
	mcpCode, mcpRemediation := mcpErrorCodeAndRemediation(t, mcpRes)
	assert.Equal(t, tc.wantMCPCode, mcpCode)
	assert.NotEmpty(t, mcpRemediation)

	_, rpcErr := svc.CreateGuidanceRequest(ctx, connect.NewRequest(&sessionv1.CreateGuidanceRequestRequest{
		Scope:             string(domain.RequestScopeBacklogItem),
		ItemId:            &tc.itemID,
		QuestionText:      "does this dedup?",
		QuestionType:      string(domain.QuestionTypeYesNo),
		CallerSessionUuid: tc.callerUUID,
	}))
	require.Error(t, rpcErr)
	var connectErr *connect.Error
	require.ErrorAs(t, rpcErr, &connectErr)
	assert.Equal(t, tc.wantRPCCode, connectErr.Code(),
		"MCP code %q must map to the same RPC connect.Code for this fixture", mcpCode)
	assert.Contains(t, connectErr.Message(), mcpRemediation,
		"RPC error message must carry the identical remediation text the MCP path returns")
}

// TestGuidanceRequestOwnershipParity_should_ProduceMatchingCodeAndRemediation_When_MCPAndRPCPathsCheckSameFixture
// is the regression test server/mcp/testsupport_guidance.go's
// ResolveItemLinkViaMCPAdapter doc comment and connectErrorForItemLink's doc
// comment (guidance_request_service.go) both reference: it verifies the MCP
// adapter's resolveItemLink and GuidanceRequestService's own
// checkCreateGuidanceRequestOwnership — both wrapping session.ResolveItemLink
// — produce an identical (error code, remediation text) pair for the same
// ownership fixture, guarding against the two adapters drifting apart.
func TestGuidanceRequestOwnershipParity_should_ProduceMatchingCodeAndRemediation_When_MCPAndRPCPathsCheckSameFixture(t *testing.T) {
	t.Parallel()
	storage := newParityTestStorage(t)
	ctx := context.Background()
	svc := services.NewGuidanceRequestService(storage, nil, nil)

	linkedItem, err := storage.CreateBacklogItem(ctx, session.BacklogItemData{Title: "parity linked item"})
	require.NoError(t, err)

	tests := []ownershipParityCase{
		{
			name:        "caller not linked to an existing item",
			callerUUID:  uuid.New().String(),
			itemID:      linkedItem.ID,
			wantMCPCode: mcp.ErrPermissionDenied,
			wantRPCCode: connect.CodePermissionDenied,
		},
		{
			name:        "item does not exist",
			callerUUID:  uuid.New().String(),
			itemID:      uuid.New().String(),
			wantMCPCode: mcp.ErrItemNotFound,
			wantRPCCode: connect.CodeNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assertOwnershipParity(t, ctx, storage, svc, tt)
		})
	}
}
