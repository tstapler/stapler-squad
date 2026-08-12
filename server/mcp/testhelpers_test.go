package mcp

import (
	"encoding/json"
	"testing"
	"time"

	mcpgo "github.com/mark3labs/mcp-go/mcp"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/scrollback"
)

// stubStore implements session.InstanceStore for tests.
type stubStore struct {
	instances []*session.Instance
	saveErr   error
	loadErr   error
}

func (s *stubStore) LoadInstances() ([]*session.Instance, error) {
	return s.instances, s.loadErr
}

func (s *stubStore) SaveInstances(insts []*session.Instance) error {
	s.instances = insts
	return s.saveErr
}

func (s *stubStore) AddInstance(inst *session.Instance) error {
	s.instances = append(s.instances, inst)
	return s.saveErr
}

func (s *stubStore) DeleteInstance(title string) error {
	for i, inst := range s.instances {
		if inst.Title == title {
			s.instances = append(s.instances[:i], s.instances[i+1:]...)
			return nil
		}
	}
	return nil
}

func (s *stubStore) UpdateInstanceLastUserResponse(title string, t time.Time) error {
	return nil
}

func (s *stubStore) UpdateInstanceMetadata(currentTitle string, newTitle, category, note, workingDir *string) error {
	return nil
}

func (s *stubStore) ListInstanceData() ([]session.InstanceData, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	data := make([]session.InstanceData, 0, len(s.instances))
	for _, inst := range s.instances {
		data = append(data, inst.ToInstanceData())
	}
	return data, nil
}

// makeScrollbackMgr creates a scrollback manager backed by a temp directory.
func makeScrollbackMgr(t *testing.T) *scrollback.ScrollbackManager {
	t.Helper()
	cfg := scrollback.DefaultScrollbackConfig()
	cfg.StoragePath = t.TempDir()
	return scrollback.NewScrollbackManager(cfg)
}

// makeToolReq constructs a CallToolRequest with the given args.
func makeToolReq(args map[string]interface{}) mcpgo.CallToolRequest {
	return mcpgo.CallToolRequest{
		Params: mcpgo.CallToolParams{
			Arguments: args,
		},
	}
}

// parseResult extracts and unmarshals the JSON text from a CallToolResult into
// a generic map. Use this when the precise result type is not known statically.
func parseResult(t *testing.T, res *mcpgo.CallToolResult) map[string]interface{} {
	t.Helper()
	if res == nil {
		t.Fatal("parseResult: result is nil")
	}
	if len(res.Content) == 0 {
		t.Fatal("parseResult: result has no content")
	}
	tc, ok := res.Content[0].(mcpgo.TextContent)
	if !ok {
		t.Fatalf("parseResult: content[0] is not TextContent, got %T", res.Content[0])
	}
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Text), &m); err != nil {
		t.Fatalf("parseResult: unmarshal JSON: %v\nJSON: %s", err, tc.Text)
	}
	return m
}
