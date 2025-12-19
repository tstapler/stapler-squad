package workspace

import (
	"context"
	"testing"
	"time"

	"claude-squad/session"
)

func TestNewRegistry(t *testing.T) {
	config := DefaultRegistryConfig()
	registry := NewRegistry(config)

	if registry == nil {
		t.Fatal("expected registry to be created")
	}

	if registry.config.CacheTTL != 30*time.Second {
		t.Errorf("expected cache TTL of 30s, got %v", registry.config.CacheTTL)
	}

	if err := registry.Close(); err != nil {
		t.Errorf("close failed: %v", err)
	}
}

func TestRegistryRegisterAndGet(t *testing.T) {
	registry := NewRegistry(DefaultRegistryConfig())
	defer registry.Close()

	ctx := context.Background()

	// Register a workspace
	ws := &TrackedWorkspace{
		Path:           "/test/workspace",
		RepositoryRoot: "/test",
		SessionTitle:   "test-session",
		SessionStatus:  session.Running,
	}

	err := registry.Register(ctx, ws)
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}

	// Get it back
	retrieved, err := registry.Get(ctx, "/test/workspace")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}

	if retrieved == nil {
		t.Fatal("expected workspace to be retrieved")
	}

	if retrieved.Path != ws.Path {
		t.Errorf("expected path %s, got %s", ws.Path, retrieved.Path)
	}

	if retrieved.SessionTitle != ws.SessionTitle {
		t.Errorf("expected session %s, got %s", ws.SessionTitle, retrieved.SessionTitle)
	}
}

func TestRegistryList(t *testing.T) {
	registry := NewRegistry(DefaultRegistryConfig())
	defer registry.Close()

	ctx := context.Background()

	// Register multiple workspaces
	workspaces := []*TrackedWorkspace{
		{
			Path:           "/repo1/ws1",
			RepositoryRoot: "/repo1",
			SessionTitle:   "session-1",
			SessionStatus:  session.Running,
		},
		{
			Path:           "/repo1/ws2",
			RepositoryRoot: "/repo1",
			SessionTitle:   "session-2",
			SessionStatus:  session.Paused,
		},
		{
			Path:           "/repo2/ws3",
			RepositoryRoot: "/repo2",
			SessionTitle:   "session-3",
			SessionStatus:  session.Running,
			IsOrphaned:     true,
		},
	}

	for _, ws := range workspaces {
		if err := registry.Register(ctx, ws); err != nil {
			t.Fatalf("register failed: %v", err)
		}
	}

	// List all
	all, err := registry.List(ctx, nil)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}

	if len(all) != 3 {
		t.Errorf("expected 3 workspaces, got %d", len(all))
	}

	// List by repository
	repo1Workspaces, err := registry.ListByRepository(ctx, "/repo1")
	if err != nil {
		t.Fatalf("list by repository failed: %v", err)
	}

	if len(repo1Workspaces) != 2 {
		t.Errorf("expected 2 workspaces in repo1, got %d", len(repo1Workspaces))
	}
}

func TestRegistryFilter(t *testing.T) {
	registry := NewRegistry(DefaultRegistryConfig())
	defer registry.Close()

	ctx := context.Background()

	// Register workspaces
	workspaces := []*TrackedWorkspace{
		{
			Path:           "/ws1",
			RepositoryRoot: "/ws1",
			SessionStatus:  session.Running,
			IsOrphaned:     false,
		},
		{
			Path:           "/ws2",
			RepositoryRoot: "/ws2",
			SessionStatus:  session.Paused,
			IsOrphaned:     true,
		},
	}

	for _, ws := range workspaces {
		registry.Register(ctx, ws)
	}

	// Filter excluding orphaned
	filter := &WorkspaceFilter{
		IncludeOrphaned: false,
	}

	filtered, err := registry.List(ctx, filter)
	if err != nil {
		t.Fatalf("list with filter failed: %v", err)
	}

	if len(filtered) != 1 {
		t.Errorf("expected 1 non-orphaned workspace, got %d", len(filtered))
	}

	// Filter including orphaned
	filter.IncludeOrphaned = true
	all, err := registry.List(ctx, filter)
	if err != nil {
		t.Fatalf("list with filter failed: %v", err)
	}

	if len(all) != 2 {
		t.Errorf("expected 2 workspaces with orphaned, got %d", len(all))
	}
}

func TestRegistryUnregister(t *testing.T) {
	registry := NewRegistry(DefaultRegistryConfig())
	defer registry.Close()

	ctx := context.Background()

	ws := &TrackedWorkspace{
		Path:           "/test/workspace",
		RepositoryRoot: "/test",
		SessionTitle:   "test-session",
	}

	registry.Register(ctx, ws)

	// Unregister
	err := registry.Unregister(ctx, "/test/workspace")
	if err != nil {
		t.Fatalf("unregister failed: %v", err)
	}

	// Should not be found
	retrieved, err := registry.Get(ctx, "/test/workspace")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}

	if retrieved != nil {
		t.Error("expected workspace to be removed")
	}
}

func TestRegistryMarkOrphaned(t *testing.T) {
	registry := NewRegistry(DefaultRegistryConfig())
	defer registry.Close()

	ctx := context.Background()

	ws := &TrackedWorkspace{
		Path:           "/test/workspace",
		RepositoryRoot: "/test",
		SessionTitle:   "test-session",
		IsOrphaned:     false,
	}

	registry.Register(ctx, ws)

	// Mark as orphaned
	err := registry.MarkOrphaned(ctx, "/test/workspace")
	if err != nil {
		t.Fatalf("mark orphaned failed: %v", err)
	}

	// Check it's orphaned
	retrieved, _ := registry.Get(ctx, "/test/workspace")
	if !retrieved.IsOrphaned {
		t.Error("expected workspace to be marked as orphaned")
	}

	if retrieved.SessionTitle != "" {
		t.Error("expected session title to be cleared")
	}

	if !retrieved.NeedsAttention {
		t.Error("expected workspace to need attention")
	}
}
