package scenario

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

// Client calls a Connect JSON unary RPC.
type Client interface {
	Call(ctx context.Context, service, method string, req, resp any) error
}

// HTTPClient is a Client over the server's /api Connect JSON endpoint.
type HTTPClient struct {
	BaseURL string
	HTTP    *http.Client
}

// Call posts req as JSON to /api/session.v1.<service>/<method>.
func (h HTTPClient) Call(ctx context.Context, service, method string, req, resp any) error {
	body, err := json.Marshal(req)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/session.v1.%s/%s", strings.TrimRight(h.BaseURL, "/"), service, method)
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	hr.Header.Set("Content-Type", "application/json")
	hc := h.HTTP
	if hc == nil {
		hc = &http.Client{Timeout: 30 * time.Second}
	}
	res, err := hc.Do(hr)
	if err != nil {
		return fmt.Errorf("%s/%s: %w", service, method, err)
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(res.Body)
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s/%s: status %d: %s", service, method, res.StatusCode, data)
	}
	if resp == nil {
		return nil
	}
	return json.Unmarshal(data, resp)
}

// SeedRPC runs the post-boot stage: flags, approval rules, backlog items and
// canned-terminal sessions. Route fixtures are the recorder's job.
func (c *Composite) SeedRPC(ctx context.Context, cl Client, ref time.Time) error {
	b, err := c.Bundle(ref)
	if err != nil {
		return err
	}
	for _, f := range b.Flags {
		req := map[string]any{"name": f.Name, "enabled": f.Enabled}
		if err := cl.Call(ctx, "SessionService", "UpdateFeatureFlag", req, nil); err != nil {
			return err
		}
	}
	for _, r := range b.ApprovalRules {
		req := map[string]any{"rule": map[string]any{
			"id": r.ID, "name": r.Name, "toolName": r.ToolName, "commandPattern": r.CommandPattern,
			"decision": r.Decision, "riskLevel": r.RiskLevel, "priority": r.Priority, "enabled": true, "source": "user",
		}}
		if err := cl.Call(ctx, "SessionService", "UpsertApprovalRule", req, nil); err != nil {
			return err
		}
	}
	if err := seedBacklog(ctx, cl, b.Backlog); err != nil {
		return err
	}
	if err := seedAutomation(ctx, cl, b); err != nil {
		return err
	}
	return c.seedTerminals(ctx, cl, b.Terminals)
}

func seedBacklog(ctx context.Context, cl Client, items []BacklogItem) error {
	repo := "/demo/shop/checkout-service"
	for _, it := range items {
		req := map[string]any{"title": it.Title, "description": it.Description, "priority": it.Priority, "repoPath": repo}
		if it.Status != "idea" && it.Status != "" {
			// The server creates an item with criteria as "ready" and rejects leaving idea without them.
			req["skipPlanning"] = true // in_progress needs an approved plan or skip_planning
			req["acceptanceCriteria"] = []map[string]any{{"index": 0, "text": "Behaviour matches the description", "status": "pending"}}
		}
		var out struct {
			Item struct {
				ID string `json:"id"`
			} `json:"item"`
		}
		if err := cl.Call(ctx, "BacklogService", "CreateBacklogItem", req, &out); err != nil {
			return err
		}
		if it.Status == "idea" || it.Status == "" || it.Status == "ready" {
			continue
		}
		for _, step := range backlogPath(it.Status) {
			tr := map[string]any{"itemId": out.Item.ID, "targetStatus": step, "overrideReason": "demo seed"}
			if err := cl.Call(ctx, "BacklogService", "TransitionBacklogItemStatus", tr, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func seedAutomation(ctx context.Context, cl Client, b Bundle) error {
	for _, r := range b.TaggingRules {
		req := map[string]any{"rule": map[string]any{
			"id": r.ID, "name": r.Name, "pathPattern": r.PathPattern, "outputTag": r.OutputTag,
			"priority": r.Priority, "enabled": true, "source": "user",
		}}
		if err := cl.Call(ctx, "SessionService", "UpsertTaggingRule", req, nil); err != nil {
			return err
		}
	}
	for _, w := range b.Workflows {
		req := map[string]any{"slug": w.Slug, "name": w.Name, "description": w.Description, "command": w.Command,
			"targetDirectory": "/demo/shop/checkout-service", "cronExpression": w.Cron, "cronEnabled": w.Cron != ""}
		if err := cl.Call(ctx, "SessionService", "CreateWorkflow", req, nil); err != nil {
			return err
		}
	}
	for _, n := range b.Notifications {
		req := map[string]any{"sessionId": "demo", "notificationType": n.Type, "title": n.Title, "message": n.Message}
		if err := cl.Call(ctx, "SessionService", "SendNotification", req, nil); err != nil {
			return err
		}
	}
	return nil
}

// backlogPath lists the statuses to walk from "ready" to target; the workflow graph has no shortcuts.
func backlogPath(target string) []string {
	chain := []string{"in_progress", "review", "done"}
	for i, s := range chain {
		if s == target {
			return chain[:i+1]
		}
	}
	return []string{target}
}

func (c *Composite) seedTerminals(ctx context.Context, cl Client, ts []Terminal) error {
	if len(ts) == 0 {
		return nil
	}
	if c.deps.WorkDir == "" {
		return fmt.Errorf("terminal scenarios need Deps.WorkDir")
	}
	repoDir := filepath.Join(c.deps.WorkDir, "demo", "shop", "checkout-service")
	if err := ensureFixtureRepo(repoDir); err != nil {
		return err
	}
	for _, t := range ts {
		script, err := writeTranscript(filepath.Join(c.deps.WorkDir, "terminals"), t)
		if err != nil {
			return err
		}
		req := map[string]any{"title": t.Title, "path": repoDir, "sessionType": "SESSION_TYPE_DIRECTORY", "program": script}
		if err := cl.Call(ctx, "SessionService", "CreateSession", req, nil); err != nil {
			return err
		}
	}
	return nil
}
