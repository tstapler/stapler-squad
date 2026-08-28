package session

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/tmux"
)

// newNotFoundDetector returns a HistoryFileDetector that will resolve any
// candidate (PID<=0, arbitrary Path) to CorrelationNotFound -- no fixture
// JSONL files exist under its temp $HOME, and the candidate carries no PID
// for the fd-based fast path. This keeps commit tests focused on the
// persist/start/suspend orchestration rather than history-file correlation,
// which is already covered by import_correlate_test.go.
func newNotFoundDetector(t *testing.T) *HistoryFileDetector {
	t.Helper()
	return NewHistoryFileDetectorWithHomeDir(&mockProcessInspector{}, t.TempDir())
}

func TestCommitImportExternalSession_ReturnsErrCorrelationDrifted_When_FreshKindDiffersFromExpected(t *testing.T) {
	t.Parallel()
	store := &fakeInstanceStore{}
	detector := newNotFoundDetector(t)

	_, err := CommitImportExternalSession(context.Background(), CommitImportParams{
		Detector: detector,
		Storage:  store,
		Candidate: ExternalSessionCandidate{
			Path: t.TempDir(),
		},
		ExpectedCorrelation: CorrelationResult{Kind: CorrelationResolved, UUID: "some-uuid"},
	})

	if !errors.Is(err, ErrCorrelationDrifted) {
		t.Fatalf("expected ErrCorrelationDrifted, got %v", err)
	}
	if len(store.added) != 0 {
		t.Fatal("expected no instance to be persisted when correlation drifted")
	}
}

func TestCommitImportExternalSession_ReturnsErrPathAlreadyManaged_When_PathCollides(t *testing.T) {
	t.Parallel()
	path := t.TempDir()
	store := &fakeInstanceStore{
		instances: []InstanceData{{Title: "existing", Path: path}},
	}
	detector := newNotFoundDetector(t)

	_, err := CommitImportExternalSession(context.Background(), CommitImportParams{
		Detector:  detector,
		Storage:   store,
		Candidate: ExternalSessionCandidate{Path: path},
	})

	if !errors.Is(err, ErrPathAlreadyManaged) {
		t.Fatalf("expected ErrPathAlreadyManaged, got %v", err)
	}
	if len(store.added) != 0 {
		t.Fatal("expected no instance to be persisted when path already managed")
	}
}

func TestCommitImportExternalSession_ReturnsErrPathNotExist_When_CandidatePathMissing(t *testing.T) {
	t.Parallel()
	store := &fakeInstanceStore{}
	detector := newNotFoundDetector(t)

	_, err := CommitImportExternalSession(context.Background(), CommitImportParams{
		Detector: detector,
		Storage:  store,
		Candidate: ExternalSessionCandidate{
			Path: "/nonexistent/import-commit-test/path",
		},
	})

	if !errors.Is(err, ErrPathNotExist) {
		t.Fatalf("expected ErrPathNotExist, got %v", err)
	}
	if len(store.added) != 0 {
		t.Fatal("expected no instance to be persisted when candidate path does not exist")
	}
}

func TestCommitImportExternalSession_PersistsAndLinksAndSuspends_When_StartAndSuspendSucceed(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	suspended, err := NewSuspendedProcessStore()
	require.NoError(t, err)

	linker := NewHistoryLinker(newNotFoundDetector(t), nil)

	store := &fakeInstanceStore{}
	detector := newNotFoundDetector(t)

	cmd := spawnSleeper(t)
	pid := int32(cmd.Process.Pid)

	path := t.TempDir()
	candidate := ExternalSessionCandidate{
		Path:        path,
		Program:     "true",
		TmuxSession: "",
		PID:         0, // 0 so CorrelateCandidate falls straight to path-based lookup (-> NotFound)
	}

	result, err := CommitImportExternalSession(context.Background(), CommitImportParams{
		Detector:         detector,
		Storage:          store,
		Linker:           linker,
		Suspended:        suspended,
		AliveChecker:     &fakeAliveChecker{alive: true},
		Candidate:        candidate,
		OriginalPID:      pid,
		TmuxServerSocket: coldRestoreSocket(t),
	})
	require.NoError(t, err)
	require.NotNil(t, result.Instance)
	t.Cleanup(func() { _ = result.Instance.Kill() })

	if !store.hasInstance(result.Instance.Title) {
		t.Fatal("expected instance to be persisted in storage")
	}

	linked := linker.Instances()
	if len(linked) != 1 || linked[0] != result.Instance {
		t.Fatalf("expected HistoryLinker.AddInstance to have registered exactly the committed instance, got %v", linked)
	}

	record, found, err := suspended.Get(result.Instance.Title)
	require.NoError(t, err)
	if !found {
		t.Fatal("expected a suspended-process record to be persisted")
	}
	if record.PID != pid {
		t.Fatalf("expected suspended record PID %d, got %d", pid, record.PID)
	}

	// Clean up: resume the original process so t.Cleanup's Kill/Wait on the
	// sleeper doesn't hang on a stopped process.
	_ = ResumeOriginalProcess(pid)
}

// TestCommitImportExternalSession_HonorsSessionNameOverrideMap verifies that
// CommitImportExternalSession wires session.ResolveSessionBackend
// (tymux-bundled-integration Epic 4.4.3): with no per-request override
// concept for this restore-from-state path, a TymuxSessionOverrides entry
// keyed by the sanitized tmux session name still forces the resulting
// instance's backend even though the process-wide default is registered as
// tymux.
func TestCommitImportExternalSession_HonorsSessionNameOverrideMap(t *testing.T) {
	RegisterBackendProvider(BackendTymux)
	t.Cleanup(func() { RegisterBackendProvider(BackendTmux) })

	testDir := t.TempDir()
	t.Setenv("STAPLER_SQUAD_TEST_DIR", testDir)

	suspended, err := NewSuspendedProcessStore()
	require.NoError(t, err)

	linker := NewHistoryLinker(newNotFoundDetector(t), nil)

	store := &fakeInstanceStore{}
	detector := newNotFoundDetector(t)

	cmd := spawnSleeper(t)
	pid := int32(cmd.Process.Pid)

	path := t.TempDir()
	candidate := ExternalSessionCandidate{
		Path:        path,
		Program:     "true",
		TmuxSession: "",
		PID:         0, // see importInstanceTitle: with TmuxSession=="" and PID==0, title is deterministic.
	}
	title := importInstanceTitle(candidate)
	sessionKey := tmux.NewSessionName(title, tmux.TmuxPrefix).String()
	require.NoError(t, os.WriteFile(filepath.Join(testDir, "config.json"),
		[]byte(`{"tymux_session_overrides": {"`+sessionKey+`": false}}`), 0o644))

	result, err := CommitImportExternalSession(context.Background(), CommitImportParams{
		Detector:         detector,
		Storage:          store,
		Linker:           linker,
		Suspended:        suspended,
		AliveChecker:     &fakeAliveChecker{alive: true},
		Candidate:        candidate,
		OriginalPID:      pid,
		TmuxServerSocket: coldRestoreSocket(t),
	})
	require.NoError(t, err)
	require.NotNil(t, result.Instance)
	t.Cleanup(func() { _ = result.Instance.Kill() })

	assert.Equal(t, BackendTmux, result.Instance.Backend,
		"a TymuxSessionOverrides entry keyed by the sanitized tmux session name must force the backend even though the process-wide default is tymux")

	_ = ResumeOriginalProcess(pid)
}

func TestCommitImportExternalSession_CompensatingDeletesInstance_When_SuspendOriginalProcessFails(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	suspended, err := NewSuspendedProcessStore()
	require.NoError(t, err)

	linker := NewHistoryLinker(newNotFoundDetector(t), nil)

	store := &fakeInstanceStore{}
	detector := newNotFoundDetector(t)

	// A PID that cannot possibly exist -- SuspendOriginalProcess must fail
	// with ESRCH after instance.Start() has already succeeded and the
	// instance has already been persisted, exercising the compensating
	// delete (Story 1.2.3).
	const nonexistentPID = int32(1<<31 - 1)

	path := t.TempDir()
	candidate := ExternalSessionCandidate{
		Path:    path,
		Program: "true",
		PID:     0,
	}

	result, err := CommitImportExternalSession(context.Background(), CommitImportParams{
		Detector:         detector,
		Storage:          store,
		Linker:           linker,
		Suspended:        suspended,
		AliveChecker:     &fakeAliveChecker{alive: true},
		Candidate:        candidate,
		OriginalPID:      nonexistentPID,
		TmuxServerSocket: coldRestoreSocket(t),
	})
	if store.deletedInstance != nil {
		t.Cleanup(func() { _ = store.deletedInstance.Kill() })
	}

	if err == nil {
		t.Fatal("expected an error when SuspendOriginalProcess fails, got nil")
	}
	if result.Instance != nil {
		t.Fatal("expected CommitImportResult.Instance to be nil on failure")
	}

	if store.deletedTitle == "" {
		t.Fatal("expected DeleteInstance to have been called as part of compensating delete")
	}
	if store.hasInstance(store.deletedTitle) {
		t.Fatal("expected the persisted instance to be compensating-deleted after suspend failure")
	}

	if linked := linker.Instances(); len(linked) != 0 {
		t.Fatalf("expected HistoryLinker.AddInstance to never be called on failure, got %v", linked)
	}

	if _, found, gerr := suspended.Get(store.deletedTitle); gerr != nil {
		t.Fatalf("failed to read suspended process store: %v", gerr)
	} else if found {
		t.Fatal("expected suspended-process record to be removed after suspend failure")
	}
}

// TestCommitImportExternalSession_ReturnsError_When_AliveCheckerRejectsOriginalPID
// guards against committing (and suspending) the wrong process on PID reuse:
// if AliveChecker.IsAlive reports that OriginalPID/OriginalCreateTimeMs no
// longer identify the same process, CommitImportExternalSession must error
// out and must never reach SuspendOriginalProcess/Suspended.Add.
func TestCommitImportExternalSession_ReturnsError_When_AliveCheckerRejectsOriginalPID(t *testing.T) {
	t.Setenv("STAPLER_SQUAD_TEST_DIR", t.TempDir())

	suspended, err := NewSuspendedProcessStore()
	require.NoError(t, err)

	linker := NewHistoryLinker(newNotFoundDetector(t), nil)

	store := &fakeInstanceStore{}
	detector := newNotFoundDetector(t)

	cmd := spawnSleeper(t)
	pid := int32(cmd.Process.Pid)

	path := t.TempDir()
	candidate := ExternalSessionCandidate{
		Path:    path,
		Program: "true",
		PID:     0,
	}

	result, err := CommitImportExternalSession(context.Background(), CommitImportParams{
		Detector:             detector,
		Storage:              store,
		Linker:               linker,
		Suspended:            suspended,
		AliveChecker:         &fakeAliveChecker{alive: false},
		Candidate:            candidate,
		OriginalPID:          pid,
		OriginalCreateTimeMs: 12345,
		TmuxServerSocket:     coldRestoreSocket(t),
	})
	if store.deletedInstance != nil {
		t.Cleanup(func() { _ = store.deletedInstance.Kill() })
	}

	if err == nil {
		t.Fatal("expected an error when AliveChecker rejects the original PID, got nil")
	}
	if result.Instance != nil {
		t.Fatal("expected CommitImportResult.Instance to be nil on failure")
	}
	if store.deletedTitle == "" {
		t.Fatal("expected DeleteInstance to have been called as part of compensating delete")
	}

	if list, lerr := suspended.List(); lerr != nil {
		t.Fatalf("failed to list suspended process store: %v", lerr)
	} else if len(list) != 0 {
		t.Fatalf("expected SuspendOriginalProcess/Suspended.Add to never be reached, got %d suspended record(s)", len(list))
	}
}

// TestCommitImportExternalSession_ReturnsErrTmuxSessionNotFound_When_CandidateTmuxSessionMissing
// guards against persisting (and later using to run "tmux kill-session")
// a candidate's client-supplied TmuxSession name without confirming it
// actually corresponds to a live tmux session at commit time.
func TestCommitImportExternalSession_ReturnsErrTmuxSessionNotFound_When_CandidateTmuxSessionMissing(t *testing.T) {
	t.Parallel()
	store := &fakeInstanceStore{}
	detector := newNotFoundDetector(t)

	_, err := CommitImportExternalSession(context.Background(), CommitImportParams{
		Detector:    detector,
		Storage:     store,
		TmuxQuerier: newFakeTmuxSocketQuerier(),
		Candidate: ExternalSessionCandidate{
			Path:        t.TempDir(),
			TmuxSession: "gone-session",
		},
	})

	if !errors.Is(err, ErrTmuxSessionNotFound) {
		t.Fatalf("expected ErrTmuxSessionNotFound, got %v", err)
	}
	if len(store.added) != 0 {
		t.Fatal("expected no instance to be persisted when candidate's tmux session no longer exists")
	}
}
