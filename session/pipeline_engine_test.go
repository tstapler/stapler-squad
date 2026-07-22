package session

import (
	"bytes"
	"context"
	"errors"
	stdlog "log"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session/ent"
)

// ─── fakePipelineModeRepository ─────────────────────────────────────────────

// fakePipelineModeRepository is a test double for PipelineModeRepository.
// Only ListEnabled is exercised by pipeline_engine.go — the remaining methods
// return an error if called, since no test in this file should reach them.
type fakePipelineModeRepository struct {
	listEnabledFn func(ctx context.Context) ([]*ent.PipelineMode, error)
}

var _ PipelineModeRepository = (*fakePipelineModeRepository)(nil)

func (f *fakePipelineModeRepository) Create(context.Context, PipelineModeCreateInput) (*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.Create: not implemented")
}

func (f *fakePipelineModeRepository) Update(context.Context, uuid.UUID, PipelineModeUpdateInput) (*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.Update: not implemented")
}

func (f *fakePipelineModeRepository) Delete(context.Context, uuid.UUID) error {
	return errors.New("fakePipelineModeRepository.Delete: not implemented")
}

func (f *fakePipelineModeRepository) GetByID(context.Context, uuid.UUID) (*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.GetByID: not implemented")
}

func (f *fakePipelineModeRepository) GetBySlug(context.Context, string) (*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.GetBySlug: not implemented")
}

func (f *fakePipelineModeRepository) ListAll(context.Context) ([]*ent.PipelineMode, error) {
	return nil, errors.New("fakePipelineModeRepository.ListAll: not implemented")
}

func (f *fakePipelineModeRepository) ListEnabled(ctx context.Context) ([]*ent.PipelineMode, error) {
	return f.listEnabledFn(ctx)
}

// swapWarningLog redirects log.WarningLog to a buffer for the duration of the
// calling test, restoring the original on cleanup. Mirrors the established
// pattern in session/review_gate_test.go.
func swapWarningLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := log.WarningLog
	log.WarningLog = stdlog.New(&buf, "WARNING: ", 0)
	t.Cleanup(func() { log.WarningLog = orig })
	return &buf
}

func assertWarnLogContainsUnresolved(t *testing.T, logOutput, itemID, mode string) {
	t.Helper()
	if !strings.Contains(logOutput, "[PipelineEngine]") {
		t.Errorf("warn log missing [PipelineEngine] prefix: %q", logOutput)
	}
	if !strings.Contains(logOutput, "item="+itemID) {
		t.Errorf("warn log missing item=%s: %q", itemID, logOutput)
	}
	if !strings.Contains(logOutput, mode) {
		t.Errorf("warn log missing mode %q: %q", mode, logOutput)
	}
}

// ─── Story 1.3.1: interface shape ───────────────────────────────────────────

func TestPipelineEngine_should_CompileWithSingleConcreteImplementation_When_CachingPipelineEngineSatisfiesInterface(t *testing.T) {
	var _ PipelineEngine = (*CachingPipelineEngine)(nil)
}

// ─── Story 1.3.2: pipelineModeCache concurrency ─────────────────────────────

func TestPipelineModeCache_Get_should_ReturnStableImmutableSnapshot_When_ConcurrentInvalidateRunsUnderRace(t *testing.T) {
	repo := &fakePipelineModeRepository{
		listEnabledFn: func(context.Context) ([]*ent.PipelineMode, error) {
			return []*ent.PipelineMode{{
				Slug:                  "quick",
				Name:                  "Quick Fix",
				StatusCommandTemplate: "status template",
			}}, nil
		},
	}
	cache := &pipelineModeCache{}
	if err := cache.Load(context.Background(), repo); err != nil {
		t.Fatalf("Load: %v", err)
	}

	// Hold a Get() result across a concurrent Invalidate() and assert its
	// fields are unchanged afterward — the actual invariant this design
	// protects: an in-flight LLM call reading resolvedPipelineMode fields
	// hundreds of ms after Get returned must see a stable, immutable
	// snapshot even if invalidations happen concurrently.
	held, ok := cache.Get("quick")
	if !ok {
		t.Fatalf("expected quick to be present before concurrent invalidate")
	}
	wantName := held.Name
	wantStatus := held.StatusCommandTemplate

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
				}
				rm, ok := cache.Get("quick")
				if ok && (rm.Slug != "quick" || rm.StatusCommandTemplate == "") {
					t.Errorf("torn/partial read observed: %+v", rm)
				}
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			if err := cache.Invalidate(context.Background(), repo); err != nil {
				t.Errorf("Invalidate: %v", err)
			}
		}
	}()

	time.Sleep(50 * time.Millisecond)
	close(stop)
	wg.Wait()

	if held.Name != wantName || held.StatusCommandTemplate != wantStatus {
		t.Errorf("held Get() result mutated after concurrent Invalidate: got %+v", held)
	}
}

func TestPipelineModeCache_Get_should_ReturnFalse_When_SlugNotPresent(t *testing.T) {
	cache := &pipelineModeCache{}
	repo := &fakePipelineModeRepository{
		listEnabledFn: func(context.Context) ([]*ent.PipelineMode, error) {
			return []*ent.PipelineMode{{Slug: "quick"}}, nil
		},
	}
	if err := cache.Load(context.Background(), repo); err != nil {
		t.Fatalf("Load: %v", err)
	}

	rm, ok := cache.Get("missing")
	if ok {
		t.Fatalf("expected ok=false for missing slug, got rm=%+v", rm)
	}
	if rm != (resolvedPipelineMode{}) {
		t.Fatalf("expected zero-value resolvedPipelineMode, got %+v", rm)
	}
}

func TestPipelineModeCache_Invalidate_should_ReflectLastStartedCallResult_When_ConcurrentInvalidateCallsRaceWithAsymmetricLatency(t *testing.T) {
	cache := &pipelineModeCache{}

	repoA := &fakePipelineModeRepository{
		listEnabledFn: func(context.Context) ([]*ent.PipelineMode, error) {
			// A starts first and is slow — a plain atomic.Pointer (no writer
			// mutex) would let A's Store land after B's, reverting the cache
			// to stale v1 data. The writer mutex must prevent that.
			time.Sleep(50 * time.Millisecond)
			return []*ent.PipelineMode{{Slug: "marker-mode", Name: "v1"}}, nil
		},
	}
	repoB := &fakePipelineModeRepository{
		listEnabledFn: func(context.Context) ([]*ent.PipelineMode, error) {
			return []*ent.PipelineMode{{Slug: "marker-mode", Name: "v2"}}, nil
		},
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := cache.Invalidate(context.Background(), repoA); err != nil {
			t.Errorf("A Invalidate: %v", err)
		}
	}()
	time.Sleep(5 * time.Millisecond)
	go func() {
		defer wg.Done()
		if err := cache.Invalidate(context.Background(), repoB); err != nil {
			t.Errorf("B Invalidate: %v", err)
		}
	}()
	wg.Wait()

	rm, ok := cache.Get("marker-mode")
	if !ok {
		t.Fatalf("expected marker-mode present after both invalidates")
	}
	if rm.Name != "v2" {
		t.Fatalf("expected final cache to reflect v2 (the call whose DB read began last), got %q — lost-update regression", rm.Name)
	}
}

func TestPipelineModeCache_Load_should_ComputeContentHash_When_BuildingResolvedPipelineMode(t *testing.T) {
	mode := &ent.PipelineMode{
		Slug:                  "quick",
		Name:                  "Quick Fix",
		StatusCommandTemplate: "status {{item_id}}",
		DoneCommandTemplate:   "done",
		FailCommandTemplate:   "fail",
		ReviewCommandTemplate: "review",
		ShipCommandTemplate:   "ship",
		HelpCommandTemplate:   "help",
		TriagePromptTemplate:  "triage",
		ReviewPromptTemplate:  "review-prompt",
		InitialPromptTemplate: "initial",
	}
	repo := &fakePipelineModeRepository{
		listEnabledFn: func(context.Context) ([]*ent.PipelineMode, error) {
			return []*ent.PipelineMode{mode}, nil
		},
	}
	cache := &pipelineModeCache{}
	if err := cache.Load(context.Background(), repo); err != nil {
		t.Fatalf("Load: %v", err)
	}

	rm, ok := cache.Get("quick")
	if !ok {
		t.Fatalf("expected quick to be present")
	}

	wantHash := ComputeContentHash(
		mode.StatusCommandTemplate, mode.DoneCommandTemplate, mode.FailCommandTemplate,
		mode.ReviewCommandTemplate, mode.ShipCommandTemplate, mode.HelpCommandTemplate,
		mode.TriagePromptTemplate, mode.ReviewPromptTemplate, mode.InitialPromptTemplate,
	)
	if rm.ContentHash != wantHash {
		t.Errorf("ContentHash = %q, want %q", rm.ContentHash, wantHash)
	}
	if len(rm.ContentHash) != 16 {
		t.Errorf("ContentHash length = %d, want 16", len(rm.ContentHash))
	}

	// Two modes with identical content produce identical hashes.
	mode2 := *mode
	mode2.Slug = "quick2"
	repo2 := &fakePipelineModeRepository{
		listEnabledFn: func(context.Context) ([]*ent.PipelineMode, error) {
			return []*ent.PipelineMode{&mode2}, nil
		},
	}
	cache2 := &pipelineModeCache{}
	if err := cache2.Load(context.Background(), repo2); err != nil {
		t.Fatalf("Load (cache2): %v", err)
	}
	rm2, ok := cache2.Get("quick2")
	if !ok {
		t.Fatalf("expected quick2 to be present")
	}
	if rm2.ContentHash != rm.ContentHash {
		t.Errorf("expected identical content to produce identical hashes: %q != %q", rm2.ContentHash, rm.ContentHash)
	}
}

// ─── Story 1.3.3: CachingPipelineEngine fail-closed resolution ─────────────

func TestCachingPipelineEngine_SlashCommandSet_should_ShortCircuitCacheAndDB_When_ModeIsDefault(t *testing.T) {
	// A zero-value CachingPipelineEngine has a nil cache and nil repo. If
	// SlashCommandSet ever touched e.cache.Get for a default-mode item, the
	// nil *pipelineModeCache receiver would panic and fail this test — that
	// is the "spy that fails the test if Get is invoked" mechanism.
	engine := &CachingPipelineEngine{}

	item := &BacklogItemData{ID: "abc-123", Title: "Fix bug", AcceptanceCriteria: ""}

	got, err := engine.SlashCommandSet(item)
	if err != nil {
		t.Fatalf("SlashCommandSet: %v", err)
	}
	want, err := buildDefaultSlashCommandSet(item)
	if err != nil {
		t.Fatalf("buildDefaultSlashCommandSet: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SlashCommandSet(default) = %v, want %v", got, want)
	}
}

func TestCachingPipelineEngine_SlashCommandSet_should_FallBackToDefaultAndEmitWarnLog_When_ModeSlugUnresolvable(t *testing.T) {
	repo := &fakePipelineModeRepository{
		listEnabledFn: func(context.Context) ([]*ent.PipelineMode, error) {
			return nil, nil // no modes: "deleted-mode" can never resolve
		},
	}
	cache := &pipelineModeCache{}
	if err := cache.Load(context.Background(), repo); err != nil {
		t.Fatalf("Load: %v", err)
	}
	engine := &CachingPipelineEngine{repo: repo, cache: cache}

	item := &BacklogItemData{
		ID:                 "abc-123",
		Title:              "Fix bug",
		AcceptanceCriteria: "",
		PipelineMode:       "deleted-mode",
	}

	t.Run("SlashCommandSet", func(t *testing.T) {
		buf := swapWarningLog(t)

		got, err := engine.SlashCommandSet(item)
		if err != nil {
			t.Fatalf("SlashCommandSet: %v", err)
		}
		want, err := buildDefaultSlashCommandSet(item)
		if err != nil {
			t.Fatalf("buildDefaultSlashCommandSet: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("got %v, want %v", got, want)
		}
		assertWarnLogContainsUnresolved(t, buf.String(), item.ID, "deleted-mode")
	})

	t.Run("TriagePromptFor", func(t *testing.T) {
		buf := swapWarningLog(t)

		got := engine.TriagePromptFor(item, "/tmp/plan.md")
		want := BuildHeadlessTriagePrompt(item, "/tmp/plan.md")
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		assertWarnLogContainsUnresolved(t, buf.String(), item.ID, "deleted-mode")
	})

	t.Run("ReviewPromptFor", func(t *testing.T) {
		buf := swapWarningLog(t)

		got := engine.ReviewPromptFor(item, nil, "diff content", false, "notes", ReviewContextExtras{})
		want := BuildHeadlessReviewPrompt(item, nil, "diff content", false, "notes", ReviewContextExtras{})
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		assertWarnLogContainsUnresolved(t, buf.String(), item.ID, "deleted-mode")
	})

	t.Run("InteractiveReviewPromptFor", func(t *testing.T) {
		buf := swapWarningLog(t)

		got := engine.InteractiveReviewPromptFor(item, nil, "diff content", false, "review-session-id", "notes")
		want := BuildReviewPrompt(item, nil, "diff content", false, "review-session-id", "notes")
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		assertWarnLogContainsUnresolved(t, buf.String(), item.ID, "deleted-mode")
	})

	t.Run("InitialPromptFor", func(t *testing.T) {
		buf := swapWarningLog(t)

		got := engine.InitialPromptFor(item, nil)
		want := BuildTokenBudgetedPrompt(item, nil)
		if got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
		assertWarnLogContainsUnresolved(t, buf.String(), item.ID, "deleted-mode")
	})
}

func TestCachingPipelineEngine_ContentHashFor_should_ReturnEmptyAndFalse_When_ModeIsDefaultOrUnresolved(t *testing.T) {
	engine := &CachingPipelineEngine{cache: &pipelineModeCache{}}

	hash, ok := engine.ContentHashFor(PipelineModeDefault)
	if hash != "" || ok {
		t.Fatalf("default mode: got (%q, %v), want (\"\", false)", hash, ok)
	}

	buf := swapWarningLog(t)

	hash, ok = engine.ContentHashFor("missing")
	if hash != "" || ok {
		t.Fatalf("missing mode: got (%q, %v), want (\"\", false)", hash, ok)
	}
	if buf.Len() != 0 {
		t.Fatalf("ContentHashFor must not emit a Warn log for an unresolved slug (documented exemption), got: %q", buf.String())
	}
}

func TestNewPipelineEngine_should_ReturnUsableEngineWithEmptyCacheAndWarnLog_When_InitialCacheLoadFails(t *testing.T) {
	buf := swapWarningLog(t)

	repo := &fakePipelineModeRepository{
		listEnabledFn: func(context.Context) ([]*ent.PipelineMode, error) {
			return nil, errors.New("db unavailable")
		},
	}

	engine, err := NewPipelineEngine(repo)
	if err != nil {
		t.Fatalf("NewPipelineEngine returned a non-nil error: %v", err)
	}
	if engine == nil {
		t.Fatalf("NewPipelineEngine returned a nil engine")
	}
	if !strings.Contains(buf.String(), "[PipelineEngine]") || !strings.Contains(buf.String(), "cache.Load failed at startup") {
		t.Fatalf("expected a startup Warn log naming the cache.Load failure, got: %q", buf.String())
	}

	defaultItem := &BacklogItemData{ID: "default-item"}
	if _, err := engine.SlashCommandSet(defaultItem); err != nil {
		t.Fatalf("default item SlashCommandSet: %v", err)
	}

	buf.Reset()
	nonDefaultItem := &BacklogItemData{ID: "nd-item", PipelineMode: "quick"}
	got, err := engine.SlashCommandSet(nonDefaultItem)
	if err != nil {
		t.Fatalf("SlashCommandSet: %v", err)
	}
	want, err := buildDefaultSlashCommandSet(nonDefaultItem)
	if err != nil {
		t.Fatalf("buildDefaultSlashCommandSet: %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected fallback to default output")
	}
	if !strings.Contains(buf.String(), "unresolved pipeline_mode") {
		t.Fatalf("expected the normal unresolved-slug fallback Warn log, got: %q", buf.String())
	}
}

func TestNewPipelineEngine_should_PopulateCacheFromRepository_When_ListEnabledSucceeds(t *testing.T) {
	calls := 0
	repo := &fakePipelineModeRepository{
		listEnabledFn: func(context.Context) ([]*ent.PipelineMode, error) {
			calls++
			return []*ent.PipelineMode{{Slug: "quick", Name: "Quick Fix"}}, nil
		},
	}

	engine, err := NewPipelineEngine(repo)
	if err != nil {
		t.Fatalf("NewPipelineEngine: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly 1 ListEnabled call at construction, got %d", calls)
	}

	rm, ok := engine.cache.Get("quick")
	if !ok {
		t.Fatalf("expected quick to be resolvable after construction with no additional DB call")
	}
	if rm.Name != "Quick Fix" {
		t.Fatalf("got Name=%q, want %q", rm.Name, "Quick Fix")
	}
	if calls != 1 {
		t.Fatalf("Get must not trigger an additional DB call, calls=%d", calls)
	}
}

// ─── Story 1.3.3d/1.3.3e: renderTemplate + resolved-mode rendering ─────────

func TestCachingPipelineEngine_SlashCommandSet_should_RenderModeTemplates_When_ModeResolves(t *testing.T) {
	repo := &fakePipelineModeRepository{
		listEnabledFn: func(context.Context) ([]*ent.PipelineMode, error) {
			return []*ent.PipelineMode{{
				Slug:                  "quick",
				Name:                  "Quick Fix",
				StatusCommandTemplate: "status for {{item_id}}: {{item_title}}",
				DoneCommandTemplate:   "done {{criteria_index}} of {{criteria_count}}: {{criteria_text}}",
				FailCommandTemplate:   "fail {{criteria_index}} of {{criteria_count}}: {{criteria_text}}",
				ReviewCommandTemplate: "review {{item_id}} at {{repo_path}}",
				ShipCommandTemplate:   "ship {{item_id}}",
				HelpCommandTemplate:   "help for {{item_id}}, {{criteria_count}} criteria, unknown={{made_up_placeholder}}",
			}}, nil
		},
	}
	cache := &pipelineModeCache{}
	if err := cache.Load(context.Background(), repo); err != nil {
		t.Fatalf("Load: %v", err)
	}
	engine := &CachingPipelineEngine{repo: repo, cache: cache}

	item := &BacklogItemData{
		ID:                 "abc-123",
		Title:              "Fix bug",
		RepoPath:           "/repo/path",
		PipelineMode:       "quick",
		AcceptanceCriteria: `[{"index":0,"text":"first"},{"index":1,"text":"second"}]`,
	}

	got, err := engine.SlashCommandSet(item)
	if err != nil {
		t.Fatalf("SlashCommandSet: %v", err)
	}

	if got["status.md"] != "status for abc-123: Fix bug" {
		t.Errorf("status.md = %q", got["status.md"])
	}
	if got["done-0.md"] != "done 0 of 2: first" {
		t.Errorf("done-0.md = %q", got["done-0.md"])
	}
	if got["fail-1.md"] != "fail 1 of 2: second" {
		t.Errorf("fail-1.md = %q", got["fail-1.md"])
	}
	if got["review.md"] != "review abc-123 at /repo/path" {
		t.Errorf("review.md = %q", got["review.md"])
	}
	if got["ship.md"] != "ship abc-123" {
		t.Errorf("ship.md = %q", got["ship.md"])
	}
	// Unrecognized {{made_up_placeholder}} token is left un-substituted
	// (Phase-1-only passthrough, Task 1.3.3d).
	wantHelp := "help for abc-123, 2 criteria, unknown={{made_up_placeholder}}"
	if got["help.md"] != wantHelp {
		t.Errorf("help.md = %q, want %q", got["help.md"], wantHelp)
	}
}

// TestCachingPipelineEngine_InteractiveReviewPromptFor_should_RenderCustomTemplate_When_ModeResolves
// is the regression guard for the "custom PipelineMode's ReviewPromptTemplate
// silently does nothing for the automatic review gate" gap: it proves a
// resolved custom mode's ReviewPromptTemplate is rendered (with the same
// placeholders + criteria_count ReviewPromptFor uses), not the hardcoded
// BuildReviewPrompt content.
func TestCachingPipelineEngine_InteractiveReviewPromptFor_should_RenderCustomTemplate_When_ModeResolves(t *testing.T) {
	repo := &fakePipelineModeRepository{
		listEnabledFn: func(context.Context) ([]*ent.PipelineMode, error) {
			return []*ent.PipelineMode{{
				Slug:                 "quick",
				Name:                 "Quick Fix",
				ReviewPromptTemplate: "custom review for {{item_id}}: {{item_title}} ({{criteria_count}} criteria)",
			}}, nil
		},
	}
	cache := &pipelineModeCache{}
	if err := cache.Load(context.Background(), repo); err != nil {
		t.Fatalf("Load: %v", err)
	}
	engine := &CachingPipelineEngine{repo: repo, cache: cache}

	item := &BacklogItemData{
		ID:           "abc-123",
		Title:        "Fix bug",
		PipelineMode: "quick",
	}
	acSnapshot := []AcCriterion{{Index: 0, Text: "first"}, {Index: 1, Text: "second"}}

	got := engine.InteractiveReviewPromptFor(item, acSnapshot, "diff content", false, "review-session-id", "notes")
	want := "custom review for abc-123: Fix bug (2 criteria)"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if strings.Contains(got, "submit_review_verdict") {
		t.Fatalf("resolved custom mode must render ReviewPromptTemplate verbatim, not fall back to BuildReviewPrompt's tool-call instructions: %q", got)
	}
}

// ─── Story 1.7.3: isolated commit gate — real SQL-level injection ─────────

// TestPipelineEngine_should_FallBackToDefaultNotCrash_When_UnresolvableSlugInjectedDirectlyViaSQL
// is the real-repository counterpart to
// TestCachingPipelineEngine_SlashCommandSet_should_FallBackToDefaultAndEmitWarnLog_When_ModeSlugUnresolvable
// (which uses an in-memory fakePipelineModeRepository test double). This test
// instead persists a BacklogItem row with a nonexistent pipeline_mode slug
// through a real ent-backed Storage (createTestStorage — the same SQLite-
// backed repository production code uses) and resolves it through a
// CachingPipelineEngine backed by a real EntPipelineModeRepository over the
// SAME ent client, with zero PipelineMode rows ever created — i.e. the slug is
// unresolvable via an actual DB round-trip, not a hand-built map. This is the
// automatable form of Story 1.7.3's acceptance criterion: "manually setting an
// item's pipeline_mode column to a nonexistent slug via direct SQL and
// triggering triage produces the Warn-log-and-default-fallback behavior, not a
// crash."
func TestPipelineEngine_should_FallBackToDefaultNotCrash_When_UnresolvableSlugInjectedDirectlyViaSQL(t *testing.T) {
	storage, cleanup := createTestStorage(t)
	defer cleanup()

	entClient := storage.GetEntClient()
	if entClient == nil {
		t.Fatalf("expected a non-nil ent client from createTestStorage's real EntRepository")
	}

	// Persist the item through the real repository — PipelineMode round-trips
	// through the full ent Create path (session/ent_repository_backlog.go's
	// SetPipelineMode call), exactly like a direct SQL write bypassing any
	// RPC/UI would.
	created, err := storage.CreateBacklogItem(context.Background(), BacklogItemData{
		Title:        "SQL-injected unresolvable mode item",
		PipelineMode: "nonexistent-slug-via-sql",
	})
	if err != nil {
		t.Fatalf("CreateBacklogItem: %v", err)
	}
	if created.PipelineMode != "nonexistent-slug-via-sql" {
		t.Fatalf("expected PipelineMode to round-trip, got %q", created.PipelineMode)
	}

	// No PipelineMode rows exist in this DB at all — ListEnabled returns an
	// empty set, so "nonexistent-slug-via-sql" is genuinely unresolvable via a
	// real DB read, not just absent from a hand-built fake.
	pipelineModeRepo := NewEntPipelineModeRepository(entClient)
	engine, err := NewPipelineEngine(pipelineModeRepo)
	if err != nil {
		t.Fatalf("NewPipelineEngine: %v", err)
	}

	buf := swapWarningLog(t)

	// Must not panic/crash and must fall back to default-mode output.
	gotFiles, err := engine.SlashCommandSet(created)
	if err != nil {
		t.Fatalf("SlashCommandSet: %v", err)
	}
	wantFiles, err := buildDefaultSlashCommandSet(created)
	if err != nil {
		t.Fatalf("buildDefaultSlashCommandSet: %v", err)
	}
	if !reflect.DeepEqual(gotFiles, wantFiles) {
		t.Fatalf("expected default-mode fallback output, got %v, want %v", gotFiles, wantFiles)
	}
	assertWarnLogContainsUnresolved(t, buf.String(), created.ID, "nonexistent-slug-via-sql")

	buf.Reset()
	gotPrompt := engine.TriagePromptFor(created, "/tmp/artifacts")
	wantPrompt := BuildHeadlessTriagePrompt(created, "/tmp/artifacts")
	if gotPrompt != wantPrompt {
		t.Fatalf("expected default-mode triage prompt fallback")
	}
	assertWarnLogContainsUnresolved(t, buf.String(), created.ID, "nonexistent-slug-via-sql")
}
