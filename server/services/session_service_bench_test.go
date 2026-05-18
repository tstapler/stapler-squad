package services

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"runtime"
	"testing"

	"connectrpc.com/connect"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/gen/proto/go/session/v1/sessionv1connect"
	"github.com/tstapler/stapler-squad/server/events"
	"github.com/tstapler/stapler-squad/session"
)

// benchEventBusBufferSize is the channel capacity for the EventBus used in benchmarks.
// Matches the capacity used in production to keep benchmark conditions realistic.
const benchEventBusBufferSize = 64

// benchServiceFixture holds the live server, client, and cleanup function for
// a single benchmark setup. The server is started once and reused across all
// iterations to measure pure RPC overhead rather than startup cost.
// repo is exposed so benchmarks that need to pre-seed data can do so without
// duplicating the full setup sequence.
type benchServiceFixture struct {
	server  *httptest.Server
	client  sessionv1connect.SessionServiceClient
	repo    *session.EntRepository
	cleanup func()
}

// benchmarkServiceSetup creates a fully wired SessionService backed by a
// real SQLite database, registers it as a ConnectRPC handler on an httptest
// server, and returns the fixture plus a cleanup function.
//
// The server uses HTTP/1.1 (Connect's default wire protocol) so no h2c or
// TLS configuration is needed.
func benchmarkServiceSetup(b *testing.B) *benchServiceFixture {
	b.Helper()

	// Temp directory for the SQLite database.
	tmpDir, err := os.MkdirTemp("", "bench-session-svc-*")
	if err != nil {
		b.Fatalf("failed to create temp dir: %v", err)
	}
	dbPath := fmt.Sprintf("%s/sessions.db", tmpDir)

	// Repository → Storage → EventBus → SessionService
	repo, err := session.NewEntRepository(session.WithDatabasePath(dbPath))
	if err != nil {
		_ = os.RemoveAll(tmpDir)
		b.Fatalf("failed to create repository: %v", err)
	}

	storage, err := session.NewStorageWithRepository(repo)
	if err != nil {
		_ = repo.Close()
		_ = os.RemoveAll(tmpDir)
		b.Fatalf("failed to create storage: %v", err)
	}

	bus := events.NewEventBus(benchEventBusBufferSize)
	svc := NewSessionService(storage, bus)

	// Register handler on an HTTP mux.
	mux := http.NewServeMux()
	path, handler := sessionv1connect.NewSessionServiceHandler(svc)
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)

	client := sessionv1connect.NewSessionServiceClient(
		srv.Client(),
		srv.URL,
	)

	cleanup := func() {
		srv.Close()
		bus.Close()
		_ = repo.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return &benchServiceFixture{
		server:  srv,
		client:  client,
		repo:    repo,
		cleanup: cleanup,
	}
}

// preloadSessions inserts n sessions into the repository before the benchmark
// loop begins. Each session gets a unique title and a Running status.
func preloadSessions(b *testing.B, repo *session.EntRepository, n int) {
	b.Helper()
	ctx := context.Background()
	for i := 0; i < n; i++ {
		data := session.InstanceData{
			Title:   fmt.Sprintf("bench-session-%04d", i),
			Path:    "/tmp/bench",
			Branch:  "main",
			Status:  session.Active,
			Program: "claude",
		}
		if err := repo.Create(ctx, data); err != nil {
			b.Fatalf("failed to pre-load session %d: %v", i, err)
		}
	}
}

// --------------------------------------------------------------------------
// Benchmarks
// --------------------------------------------------------------------------

// BenchmarkSessionService_ListSessions_Empty measures ListSessions with an
// empty storage back-end.
func BenchmarkSessionService_ListSessions_Empty(b *testing.B) {
	fix := benchmarkServiceSetup(b)
	b.Cleanup(fix.cleanup)

	ctx := context.Background()

	runtime.GC()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resp, err := fix.client.ListSessions(ctx, connect.NewRequest(&sessionv1.ListSessionsRequest{}))
		if err != nil {
			b.Fatalf("ListSessions failed: %v", err)
		}
		_ = resp.Msg.Sessions
	}

	b.StopTimer()
	b.Cleanup(func() {
		b.ReportMetric(float64(runtime.NumGoroutine()), "goroutines")
	})
}

// BenchmarkSessionService_ListSessions_50Sessions measures ListSessions with
// 50 sessions pre-loaded in the database.
func BenchmarkSessionService_ListSessions_50Sessions(b *testing.B) {
	fix := benchmarkServiceSetup(b)
	b.Cleanup(fix.cleanup)

	preloadSessions(b, fix.repo, 50)

	ctx := context.Background()

	runtime.GC()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resp, err := fix.client.ListSessions(ctx, connect.NewRequest(&sessionv1.ListSessionsRequest{}))
		if err != nil {
			b.Fatalf("ListSessions failed: %v", err)
		}
		_ = resp.Msg.Sessions
	}

	b.StopTimer()
	b.Cleanup(func() {
		b.ReportMetric(float64(runtime.NumGoroutine()), "goroutines")
	})
}

// BenchmarkSessionService_GetSession measures GetSession for a known session
// title. The session is looked up via the storage fallback path (no poller).
func BenchmarkSessionService_GetSession(b *testing.B) {
	fix := benchmarkServiceSetup(b)
	b.Cleanup(fix.cleanup)

	const targetTitle = "bench-get-target"
	ctx := context.Background()
	if err := fix.repo.Create(ctx, session.InstanceData{
		Title:   targetTitle,
		Path:    "/tmp/bench",
		Branch:  "main",
		Status:  session.Active,
		Program: "claude",
	}); err != nil {
		b.Fatalf("failed to create target session: %v", err)
	}

	runtime.GC()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		resp, err := fix.client.GetSession(ctx, connect.NewRequest(&sessionv1.GetSessionRequest{
			Id: targetTitle,
		}))
		if err != nil {
			b.Fatalf("GetSession failed: %v", err)
		}
		_ = resp.Msg.Session
	}

	b.StopTimer()
	b.Cleanup(func() {
		b.ReportMetric(float64(runtime.NumGoroutine()), "goroutines")
	})
}

// BenchmarkSessionService_StreamTerminal_NotFound measures the round-trip
// overhead of the StreamTerminal bidi-streaming RPC when the requested session
// does not exist. Since creating a real PTY requires a live tmux session
// (unavailable in benchmarks), this benchmark measures ConnectRPC streaming
// infrastructure overhead: connection setup, initial message framing, session
// lookup, and error response serialization.
func BenchmarkSessionService_StreamTerminal_NotFound(b *testing.B) {
	fix := benchmarkServiceSetup(b)
	b.Cleanup(fix.cleanup)

	ctx := context.Background()

	runtime.GC()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stream := fix.client.StreamTerminal(ctx)
		_ = stream.Send(&sessionv1.TerminalData{SessionId: "nonexistent-bench-session"})
		_ = stream.CloseRequest()
		for {
			_, err := stream.Receive()
			if err != nil {
				break
			}
		}
	}

	b.StopTimer()
	b.Cleanup(func() {
		b.ReportMetric(float64(runtime.NumGoroutine()), "goroutines")
	})
}

// BenchmarkSessionService_StreamTerminal_NotStarted measures StreamTerminal
// RPC overhead when a session exists in storage but has not been started
// (no PTY allocated). This exercises the session lookup and pre-condition
// check paths without requiring a real tmux session.
func BenchmarkSessionService_StreamTerminal_NotStarted(b *testing.B) {
	fix := benchmarkServiceSetup(b)
	b.Cleanup(fix.cleanup)

	const targetTitle = "bench-stream-target"
	ctx := context.Background()
	if err := fix.repo.Create(ctx, session.InstanceData{
		Title:   targetTitle,
		Path:    "/tmp/bench",
		Branch:  "main",
		Status:  session.Active,
		Program: "claude",
	}); err != nil {
		b.Fatalf("failed to create target session: %v", err)
	}

	runtime.GC()
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		stream := fix.client.StreamTerminal(ctx)
		_ = stream.Send(&sessionv1.TerminalData{SessionId: targetTitle})
		_ = stream.CloseRequest()
		for {
			_, err := stream.Receive()
			if err != nil {
				break
			}
		}
	}

	b.StopTimer()
	b.Cleanup(func() {
		b.ReportMetric(float64(runtime.NumGoroutine()), "goroutines")
	})
}
