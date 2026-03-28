package demo

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/tstapler/stapler-squad/session"
)

// DemoServer manages a stapler-squad server instance for demo recording.
type DemoServer struct {
	cmd     *exec.Cmd
	port    int
	testDir string
	url     string
}

// StartDemoServer seeds a fresh SQLite database with mock sessions and starts
// the server binary in test mode. It does NOT wait for the server to be
// healthy – call WaitForHealth after calling this.
func StartDemoServer(t *testing.T) *DemoServer {
	t.Helper()

	testDir, err := os.MkdirTemp("", "demo-server-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	port, err := findFreePort()
	if err != nil {
		t.Fatalf("failed to find free port: %v", err)
	}

	s := &DemoServer{
		port:    port,
		testDir: testDir,
		url:     fmt.Sprintf("http://localhost:%d", port),
	}

	// Seed the database before starting the server so it loads the sessions on boot.
	if err := s.SeedMockSessions(6); err != nil {
		t.Fatalf("failed to seed mock sessions: %v", err)
	}

	binaryPath := filepath.Join(projectRoot(), "stapler-squad")
	if err := ensureBinary(binaryPath); err != nil {
		t.Fatalf("failed to ensure binary: %v", err)
	}

	cmd := exec.Command(binaryPath,
		"--test-mode",
		"--test-dir", testDir,
	)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("PORT=%d", port),
		fmt.Sprintf("STAPLER_SQUAD_TEST_DIR=%s", testDir),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start demo server: %v", err)
	}
	s.cmd = cmd

	return s
}

// WaitForHealth polls /health until the server is ready or the timeout expires.
func (s *DemoServer) WaitForHealth(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	healthURL := s.url + "/health"
	client := &http.Client{Timeout: 2 * time.Second}

	for time.Now().Before(deadline) {
		resp, err := client.Get(healthURL)
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("server at %s did not become healthy within %s", s.url, timeout)
}

// SeedMockSessions writes realistic demo sessions directly into the SQLite
// database before the server starts, so no tmux processes are required.
func (s *DemoServer) SeedMockSessions(count int) error {
	dbPath := filepath.Join(s.testDir, "sessions.db")
	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	if err != nil {
		return fmt.Errorf("failed to open repository: %w", err)
	}
	defer repo.Close()

	sessions := mockSessions()
	if count > len(sessions) {
		count = len(sessions)
	}
	ctx := context.Background()
	for _, data := range sessions[:count] {
		if err := repo.Create(ctx, data); err != nil {
			return fmt.Errorf("failed to create session %q: %w", data.Title, err)
		}
	}
	return nil
}

// URL returns the base URL of the demo server.
func (s *DemoServer) URL() string {
	return s.url
}

// Stop terminates the server process and removes the temp data directory.
func (s *DemoServer) Stop() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Signal(os.Interrupt)
		done := make(chan error, 1)
		go func() { done <- s.cmd.Wait() }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			_ = s.cmd.Process.Kill()
		}
		s.cmd = nil
	}
	_ = os.RemoveAll(s.testDir)
}

// mockSessions returns 6 realistic InstanceData records spanning Backend,
// Frontend, and Infrastructure categories. No tmux processes required —
// statuses are Paused, Ready, and NeedsApproval only (no Running) so the
// review-queue poller does not reclassify them when no tmux process exists.
//
// Two sessions include "payment" in their title/tags so the search demo
// filters from 6 → 2 results, which is visually more compelling than 3 → 1.
func mockSessions() []session.InstanceData {
	now := time.Now()
	return []session.InstanceData{
		// Paused mid-task: payment integration with large diff in progress.
		{
			Title:                "payment-stripe-integration",
			Path:                 "/Users/dev/services/payment-service",
			Branch:               "feature/stripe-webhooks",
			Status:               session.Paused,
			Program:              "claude",
			Category:             "Backend",
			Tags:                 []string{"Payments", "API", "Priority"},
			CreatedAt:            now.Add(-45 * time.Minute),
			UpdatedAt:            now.Add(-20 * time.Second),
			LastTerminalUpdate:   now.Add(-20 * time.Second),
			LastMeaningfulOutput: now.Add(-20 * time.Second),
			DiffStats:            session.DiffStatsData{Added: 287, Removed: 43},
		},
		// Needs attention: agent hit a permission gate.
		{
			Title:                "fix-api-timeout",
			Path:                 "/Users/dev/services/api-gateway",
			Branch:               "fix/connection-pool-exhaustion",
			Status:               session.NeedsApproval,
			Program:              "claude",
			Category:             "Backend",
			Tags:                 []string{"Bug", "API", "Urgent"},
			CreatedAt:            now.Add(-30 * time.Minute),
			UpdatedAt:            now.Add(-1 * time.Minute),
			LastTerminalUpdate:   now.Add(-1 * time.Minute),
			LastMeaningfulOutput: now.Add(-1 * time.Minute),
			DiffStats:            session.DiffStatsData{Added: 56, Removed: 18},
		},
		// Paused: OAuth2 refactor waiting for review.
		{
			Title:                "auth-refactor",
			Path:                 "/Users/dev/services/auth-service",
			Branch:               "feature/oauth2-pkce",
			Status:               session.Paused,
			Program:              "claude",
			Category:             "Backend",
			Tags:                 []string{"Auth", "Security"},
			CreatedAt:            now.Add(-3 * time.Hour),
			UpdatedAt:            now.Add(-25 * time.Minute),
			LastTerminalUpdate:   now.Add(-25 * time.Minute),
			LastMeaningfulOutput: now.Add(-25 * time.Minute),
			DiffStats:            session.DiffStatsData{Added: 342, Removed: 89},
		},
		// Paused: analytics dashboard frontend — large diff ready for review.
		{
			Title:                "dashboard-redesign",
			Path:                 "/Users/dev/apps/web-client",
			Branch:               "feature/analytics-dashboard",
			Status:               session.Paused,
			Program:              "claude",
			Category:             "Frontend",
			Tags:                 []string{"React", "UX", "Priority"},
			CreatedAt:            now.Add(-1 * time.Hour),
			UpdatedAt:            now.Add(-35 * time.Second),
			LastTerminalUpdate:   now.Add(-35 * time.Second),
			LastMeaningfulOutput: now.Add(-35 * time.Second),
			DiffStats:            session.DiffStatsData{Added: 415, Removed: 127},
		},
		// Ready: Kubernetes HPA config ready for review.
		{
			Title:                "k8s-autoscaling",
			Path:                 "/Users/dev/infra/terraform-modules",
			Branch:               "feature/hpa-config",
			Status:               session.Ready,
			Program:              "claude",
			Category:             "Infrastructure",
			Tags:                 []string{"DevOps", "Kubernetes"},
			CreatedAt:            now.Add(-2 * time.Hour),
			UpdatedAt:            now.Add(-15 * time.Minute),
			LastTerminalUpdate:   now.Add(-15 * time.Minute),
			LastMeaningfulOutput: now.Add(-15 * time.Minute),
			DiffStats:            session.DiffStatsData{Added: 73, Removed: 11},
		},
		// Paused: payment receipts via aider (shows multi-agent support).
		{
			Title:                "payment-email-notifications",
			Path:                 "/Users/dev/services/notification-service",
			Branch:               "feature/payment-receipts",
			Status:               session.Paused,
			Program:              "aider",
			Category:             "Backend",
			Tags:                 []string{"Payments", "Notifications"},
			CreatedAt:            now.Add(-20 * time.Minute),
			UpdatedAt:            now.Add(-45 * time.Second),
			LastTerminalUpdate:   now.Add(-45 * time.Second),
			LastMeaningfulOutput: now.Add(-45 * time.Second),
			DiffStats:            session.DiffStatsData{Added: 98, Removed: 22},
		},
	}
}

// findFreePort returns an available TCP port on localhost.
func findFreePort() (int, error) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, fmt.Errorf("failed to find free port: %w", err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// projectRoot returns the absolute path to the repository root by walking up
// from the location of this source file.
func projectRoot() string {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	// filename = <root>/tests/demo/helpers.go  →  go up two levels
	return filepath.Clean(filepath.Join(filepath.Dir(filename), "../.."))
}

// ensureBinary builds the stapler-squad binary if it is missing or stale
// (older than one hour).
func ensureBinary(binaryPath string) error {
	info, err := os.Stat(binaryPath)
	if err == nil && time.Since(info.ModTime()) < time.Hour {
		return nil // Binary is recent enough.
	}

	fmt.Println("Building stapler-squad binary...")
	root := filepath.Dir(binaryPath)
	cmd := exec.Command("go", "build", "-o", binaryPath, ".")
	cmd.Dir = root
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go build failed: %w", err)
	}
	fmt.Println("Binary built.")
	return nil
}
