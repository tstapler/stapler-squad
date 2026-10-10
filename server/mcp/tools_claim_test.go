package mcp

import (
	"context"
	"testing"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/services"
	"github.com/tstapler/stapler-squad/session"
)

type fakeClaimCheckService struct {
	verdict services.ClaimVerdict
	err     error
}

func (f fakeClaimCheckService) LookupCrossHostClaim(context.Context, string) (services.ClaimVerdict, error) {
	return f.verdict, f.err
}

func callCheckClaim(t *testing.T, svc claimCheckService, args map[string]interface{}) map[string]interface{} {
	t.Helper()
	h := &claimHandlers{svc: svc}
	res, err := h.checkCrossHostClaim(context.Background(), makeToolReq(args))
	require.NoError(t, err)
	return parseResult(t, res)
}

func TestCheckCrossHostClaimTool_should_ReturnClaimedTrueWithHostAndDeepLink_When_VerdictHeldByOther(t *testing.T) {
	hostA, err := session.LoadOrCreateHostIdentity(t.TempDir())
	require.NoError(t, err)
	record := session.NewSignedClaimRecord(hostA, "https://github.com/o/r/issues/42", "ssq://hostA/backlog/v1/bl_1", time.Now())

	m := callCheckClaim(t, fakeClaimCheckService{verdict: services.NewHeldByOtherVerdict(record)},
		map[string]interface{}{"external_url": "https://github.com/o/r/issues/42"})

	assert.Equal(t, true, m["success"])
	assert.Equal(t, true, m["claimed"])
	assert.Equal(t, true, m["checked"])
	assert.Equal(t, hostA.ID.String(), m["claiming_host_id"])
	assert.Equal(t, "ssq://hostA/backlog/v1/bl_1", m["item_deep_link"])
}

func TestCheckCrossHostClaimTool_should_ReturnCheckedFalseNeverClaimedFalseAlone_When_VerdictIndeterminate(t *testing.T) {
	m := callCheckClaim(t, fakeClaimCheckService{verdict: services.NewIndeterminateVerdict()},
		map[string]interface{}{"external_url": "https://github.com/o/r/issues/42"})

	assert.Equal(t, false, m["claimed"])
	assert.Equal(t, false, m["checked"], "a caller must be able to tell 'could not confirm' from 'confirmed clear'")
	assert.NotEmpty(t, m["note"])
}

func TestCheckCrossHostClaimTool_should_ReturnCheckedTrue_When_VerdictUnclaimed(t *testing.T) {
	m := callCheckClaim(t, fakeClaimCheckService{verdict: services.NewUnclaimedVerdict()},
		map[string]interface{}{"external_url": "https://github.com/o/r/issues/42"})

	assert.Equal(t, false, m["claimed"])
	assert.Equal(t, true, m["checked"])
	assert.NotContains(t, m, "claiming_host_id")
}

func TestCheckCrossHostClaimTool_should_ReportDisabledNotClear_When_FeatureFlagOff(t *testing.T) {
	m := callCheckClaim(t, fakeClaimCheckService{err: services.ErrCrossHostClaimDedupDisabled},
		map[string]interface{}{"external_url": "https://github.com/o/r/issues/42"})

	assert.Equal(t, false, m["claimed"])
	assert.Equal(t, false, m["checked"])
	assert.Contains(t, m["note"], "disabled")
}

func TestCheckCrossHostClaimTool_should_RejectMissingExternalURL(t *testing.T) {
	m := callCheckClaim(t, fakeClaimCheckService{}, map[string]interface{}{})
	assert.Equal(t, false, m["success"])
}

func TestCheckCrossHostClaimTool_should_ReturnUnclaimedResult_When_RegisteredAndCalledAgainstRealBacklogServiceWithNoClaims(t *testing.T) {
	svc := services.NewBacklogService(nil, nil, nil, nil, nil, nil)
	s := mcpserver.NewMCPServer("test", "0")
	registerClaimTools(s, &claimHandlers{svc: svc})
	require.NotNil(t, s.GetTool("check_cross_host_claim"), "tool is registered")

	m := callCheckClaim(t, svc, map[string]interface{}{"external_url": "https://github.com/o/r/issues/42"})

	assert.Equal(t, true, m["success"])
	assert.Equal(t, false, m["claimed"])
}
