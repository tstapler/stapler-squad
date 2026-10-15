package services

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/pkg/classifier"
	"github.com/tstapler/stapler-squad/server/deliverygate"
)

type countingAutoApprovalLogger struct {
	allow, deny atomic.Int64
}

func (c *countingAutoApprovalLogger) AppendAutoApproved(_, _, _, _, _, _, _, decision string) error {
	switch decision {
	case "allow":
		c.allow.Add(1)
	case "deny":
		c.deny.Add(1)
	}
	return nil
}

func postEnvWriteDeny(t *testing.T, h *ApprovalHandler, sessionID string) {
	t.Helper()
	body, _ := json.Marshal(map[string]interface{}{
		"tool_name":  "Write",
		"tool_input": map[string]interface{}{"content": "SECRET=x", "file_path": "/tmp/project/.env"},
		"cwd":        "/tmp/project",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/hooks/permission-request", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-CS-Session-ID", sessionID)
	h.HandlePermissionRequest(httptest.NewRecorder(), req)
}

func gateOn(flagOn bool, hiddenTitle, visibleTitle string) *deliverygate.Gate {
	g := deliverygate.NewGate(deliverygate.WithFlagLoader(func() (deliverygate.FlagSettings, error) {
		return deliverygate.FlagSettings{Global: flagOn}, nil
	}))
	g.Flags().Reload()
	g.Index().Replace([]deliverygate.Entry{
		{Title: hiddenTitle, Hidden: true, Kind: deliverygate.KindReview},
		{Title: visibleTitle},
	})
	return g
}

// T-PS-02: a hidden session's auto-allow row is dropped, its deny row and the
// analytics path are untouched; visible sessions and gate-off keep every row.
func TestAutoApproved_ShouldDropHiddenAllowKeepDenyAndAnalytics_WhenGateOn(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		flagOn    bool
		session   string
		wantAllow int64
		wantDeny  int64
	}{
		{"hidden, gate on", true, "hidden-s", 0, 1},
		{"visible, gate on", true, "visible-s", 1, 1},
		{"hidden, gate off", false, "hidden-s", 1, 1},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			h, _ := newTestHandler(5 * time.Second)
			h.SetClassifier(classifier.NewRuleBasedClassifier())
			logger := &countingAutoApprovalLogger{}
			h.SetAutoApprovalLogger(logger)
			h.SetAutoApprovedGate(gateOn(c.flagOn, "hidden-s", "visible-s"))

			postPermissionRequestWithCommand(t, h, c.session, "Bash", "ls -la && pwd")
			postEnvWriteDeny(t, h, c.session)

			if got := logger.allow.Load(); got != c.wantAllow {
				t.Errorf("allow rows = %d, want %d", got, c.wantAllow)
			}
			if got := logger.deny.Load(); got != c.wantDeny {
				t.Errorf("deny rows = %d, want %d (deny is the audit trail)", got, c.wantDeny)
			}
		})
	}
}

// With no gate wired the handler behaves exactly as before.
func TestAutoApproved_ShouldAppendAllRows_WhenNoGateWired(t *testing.T) {
	t.Parallel()
	h, _ := newTestHandler(5 * time.Second)
	h.SetClassifier(classifier.NewRuleBasedClassifier())
	logger := &countingAutoApprovalLogger{}
	h.SetAutoApprovalLogger(logger)
	postPermissionRequestWithCommand(t, h, "any", "Bash", "ls -la && pwd")
	if logger.allow.Load() != 1 {
		t.Fatalf("allow rows = %d, want 1", logger.allow.Load())
	}
}
