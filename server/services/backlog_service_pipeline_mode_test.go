package services

// backlog_service_pipeline_mode_test.go — tests for the 5 PipelineMode CRUD
// RPCs (Epic 2.2 of project_plans/backlog-configurable-pipeline). Test names
// and scenarios are taken verbatim from
// project_plans/backlog-configurable-pipeline/implementation/validation.md's
// "Story 2.2.x" rows.

import (
	"bytes"
	"context"
	"errors"
	stdlog "log"
	"sync"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	tslog "github.com/tstapler/stapler-squad/log"
	"github.com/tstapler/stapler-squad/session"
	"github.com/tstapler/stapler-squad/session/ent"
)

// swapWarningLog redirects tslog.WarningLog to a buffer for the duration of
// the calling test, restoring the original on cleanup. Mirrors the
// established pattern in session/pipeline_engine_test.go.
func swapWarningLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	orig := tslog.WarningLog
	tslog.WarningLog = stdlog.New(&buf, "WARNING: ", 0)
	t.Cleanup(func() { tslog.WarningLog = orig })
	return &buf
}

// failAfterNListEnabledRepo wraps a real session.PipelineModeRepository,
// forcing ListEnabled to fail once its call count exceeds failAfter. Every
// other method (Create/Update/Delete/GetByID/GetBySlug/ListAll) — and any
// ListEnabled call at or before failAfter — delegates straight through to
// the embedded real repository. Used to force
// CachingPipelineEngine.InvalidateCache's internal ListEnabled re-fetch to
// fail on a specific call (e.g. the one triggered by an Update) without
// disturbing the earlier calls (engine construction's initial Load, an
// earlier Create's invalidation) that must keep succeeding against the real
// backing store.
type failAfterNListEnabledRepo struct {
	session.PipelineModeRepository
	mu        sync.Mutex
	callCount int
	failAfter int
}

func (r *failAfterNListEnabledRepo) ListEnabled(ctx context.Context) ([]*ent.PipelineMode, error) {
	r.mu.Lock()
	r.callCount++
	n := r.callCount
	r.mu.Unlock()
	if n > r.failAfter {
		return nil, errors.New("forced ListEnabled failure")
	}
	return r.PipelineModeRepository.ListEnabled(ctx)
}

// newPipelineModeTestService builds a *BacklogService wired with a real
// ent-backed PipelineModeRepository and a real CachingPipelineEngine
// constructed over that same repository — mirroring how
// server/dependencies.go wires production (both share one instance).
func newPipelineModeTestService(t *testing.T) (*BacklogService, session.PipelineModeRepository, *session.CachingPipelineEngine) {
	t.Helper()
	storage := createTestStorage(t)
	repo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	engine, err := session.NewPipelineEngine(repo)
	require.NoError(t, err)
	svc := NewBacklogService(storage, nil, nil, nil, engine, repo)
	return svc, repo, engine
}

// ─── TestGetPipelineMode ────────────────────────────────────────────────────

// TestGetPipelineMode_should_ReturnDerivedContentHash_When_ModeHasNonEmptyTemplates
// proves content_hash is derived on read from the row's live 9
// content-template fields (proto field 17), not left as "".
func TestGetPipelineMode_should_ReturnDerivedContentHash_When_ModeHasNonEmptyTemplates(t *testing.T) {
	svc, _, _ := newPipelineModeTestService(t)
	ctx := t.Context()

	createResp, err := svc.CreatePipelineMode(ctx, connect.NewRequest(&sessionv1.CreatePipelineModeRequest{
		Slug:                  "quick",
		Name:                  "Quick Fix",
		Enabled:               true,
		StatusCommandTemplate: "status content",
		TriagePromptTemplate:  "Fix {{item_id}} fast.",
	}))
	require.NoError(t, err)
	require.NotEmpty(t, createResp.Msg.Item.ContentHash)

	getResp, err := svc.GetPipelineMode(ctx, connect.NewRequest(&sessionv1.GetPipelineModeRequest{
		Slug: "quick",
	}))
	require.NoError(t, err)

	wantHash := session.ComputeContentHash(
		"status content", "", "", "", "", "", "Fix {{item_id}} fast.", "", "",
	)
	assert.Equal(t, wantHash, getResp.Msg.Item.ContentHash)
	assert.NotEmpty(t, getResp.Msg.Item.ContentHash)
	assert.Equal(t, "quick", getResp.Msg.Item.Slug)
}

// ─── TestCreatePipelineMode ─────────────────────────────────────────────────

// TestCreatePipelineMode_should_PersistAndInvalidateCacheSynchronously_When_ValidInput
// (Story 2.2.1) proves CreatePipelineMode calls InvalidateCache synchronously
// before returning: immediately after Create returns, resolving the new
// mode's TriagePromptFor (no restart, no explicit invalidate call from the
// test) reflects the new mode's content.
func TestCreatePipelineMode_should_PersistAndInvalidateCacheSynchronously_When_ValidInput(t *testing.T) {
	svc, _, engine := newPipelineModeTestService(t)
	ctx := t.Context()

	createResp, err := svc.CreatePipelineMode(ctx, connect.NewRequest(&sessionv1.CreatePipelineModeRequest{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "Fix {{item_id}} fast.",
	}))
	require.NoError(t, err)
	assert.Equal(t, "quick", createResp.Msg.Item.Slug)

	item := &session.BacklogItemData{ID: "item-abc", Title: "test item", PipelineMode: "quick"}
	gotPrompt := engine.TriagePromptFor(item, "")
	assert.Equal(t, "Fix item-abc fast.", gotPrompt,
		"expected the new mode's rendered prompt with no stale-cache window")
}

// ─── TestUpdatePipelineMode ─────────────────────────────────────────────────

// TestUpdatePipelineMode_should_ReturnSuccessWithWarnLog_When_CacheInvalidationFailsAfterSuccessfulDBWrite
// (Story 2.2.1's explicit focus area) proves a cache-invalidation failure
// after a successful DB write does NOT fail the RPC: the DB write (Update)
// succeeds against the real repository, but the InvalidateCache-triggered
// ListEnabled re-fetch is forced to fail — the handler must still return a
// success response containing the updated row's data, and log a
// [PipelineEngine] Warn line naming the failure.
func TestUpdatePipelineMode_should_ReturnSuccessWithWarnLog_When_CacheInvalidationFailsAfterSuccessfulDBWrite(t *testing.T) {
	storage := createTestStorage(t)
	realRepo := session.NewEntPipelineModeRepository(storage.GetEntClient())
	// failAfter=2: call 1 is the engine's construction-time Load (must
	// succeed), call 2 is CreatePipelineMode's invalidation (must succeed so
	// the mode actually exists to update). Call 3 — UpdatePipelineMode's
	// invalidation — is the one under test and must fail.
	wrapped := &failAfterNListEnabledRepo{PipelineModeRepository: realRepo, failAfter: 2}
	engine, err := session.NewPipelineEngine(wrapped)
	require.NoError(t, err)
	svc := NewBacklogService(storage, nil, nil, nil, engine, wrapped)
	ctx := t.Context()

	createResp, err := svc.CreatePipelineMode(ctx, connect.NewRequest(&sessionv1.CreatePipelineModeRequest{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "original prompt",
	}))
	require.NoError(t, err)
	id := createResp.Msg.Item.Id

	buf := swapWarningLog(t)

	updateResp, err := svc.UpdatePipelineMode(ctx, connect.NewRequest(&sessionv1.UpdatePipelineModeRequest{
		Id:                   id,
		TriagePromptTemplate: strPtr("updated prompt"),
	}))
	require.NoError(t, err, "cache-invalidation failure after a successful DB write must not fail the RPC")
	require.NotNil(t, updateResp)
	assert.Equal(t, "updated prompt", updateResp.Msg.Item.TriagePromptTemplate)
	assert.Equal(t, "quick", updateResp.Msg.Item.Slug)

	assert.Contains(t, buf.String(), "[PipelineEngine]")
	assert.Contains(t, buf.String(), "cache invalidation failed after successful write")
	assert.Contains(t, buf.String(), id)
}

// ─── TestListPipelineModes ──────────────────────────────────────────────────

// TestListPipelineModes_should_IncludeDisabledModes_When_CalledForManagementUI
// (Story 2.2.2) proves ListPipelineModes is backed by ListAll, not
// ListEnabled — the management UI must see disabled modes too (e.g. to
// re-enable them), unlike PipelineEngine's cache.
func TestListPipelineModes_should_IncludeDisabledModes_When_CalledForManagementUI(t *testing.T) {
	svc, _, _ := newPipelineModeTestService(t)
	ctx := t.Context()

	_, err := svc.CreatePipelineMode(ctx, connect.NewRequest(&sessionv1.CreatePipelineModeRequest{
		Slug: "quick", Name: "Quick Fix", Enabled: true,
	}))
	require.NoError(t, err)
	_, err = svc.CreatePipelineMode(ctx, connect.NewRequest(&sessionv1.CreatePipelineModeRequest{
		Slug: "legacy", Name: "Legacy Mode", Enabled: false,
	}))
	require.NoError(t, err)

	listResp, err := svc.ListPipelineModes(ctx, connect.NewRequest(&sessionv1.ListPipelineModesRequest{}))
	require.NoError(t, err)
	require.Len(t, listResp.Msg.Items, 2)

	bySlug := map[string]*sessionv1.PipelineMode{}
	for _, item := range listResp.Msg.Items {
		bySlug[item.Slug] = item
	}
	require.Contains(t, bySlug, "quick")
	require.Contains(t, bySlug, "legacy")
	assert.True(t, bySlug["quick"].Enabled)
	assert.False(t, bySlug["legacy"].Enabled, "disabled modes must still be returned for the management UI")
}

// ─── TestDeletePipelineMode ─────────────────────────────────────────────────

// TestDeletePipelineMode_should_SucceedAndInvalidateCache_When_ModeStillReferencedByBacklogItem
// (Story 2.2.2's explicit focus area) proves Delete does not hard-block on
// existing BacklogItemData.PipelineMode references — it relies on
// PipelineEngine's fail-closed resolution (Story 1.3.3) instead of
// referential-integrity enforcement.
func TestDeletePipelineMode_should_SucceedAndInvalidateCache_When_ModeStillReferencedByBacklogItem(t *testing.T) {
	svc, repo, engine := newPipelineModeTestService(t)
	ctx := t.Context()

	createResp, err := svc.CreatePipelineMode(ctx, connect.NewRequest(&sessionv1.CreatePipelineModeRequest{
		Slug: "quick", Name: "Quick Fix", Enabled: true, TriagePromptTemplate: "quick prompt",
	}))
	require.NoError(t, err)
	id := createResp.Msg.Item.Id

	_, err = svc.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:        "item referencing quick",
		Status:       string(session.BacklogStatusIdea),
		PipelineMode: "quick",
	})
	require.NoError(t, err)

	_, ok := engine.ContentHashFor(session.PipelineMode("quick"))
	require.True(t, ok, "setup: 'quick' must resolve before delete")

	_, err = svc.DeletePipelineMode(ctx, connect.NewRequest(&sessionv1.DeletePipelineModeRequest{Id: id}))
	require.NoError(t, err, "delete must succeed even though an existing item still references this mode")

	all, err := repo.ListAll(ctx)
	require.NoError(t, err)
	assert.Empty(t, all)

	// Cache invalidated synchronously: the mode no longer resolves.
	_, ok = engine.ContentHashFor(session.PipelineMode("quick"))
	assert.False(t, ok, "expected the cache to no longer resolve 'quick' after delete+invalidate")
}

// ─── TestTriggerTriage fallback on deleted mode ─────────────────────────────

// TestTriggerTriage_should_FallBackToDefaultWithWarnLog_When_ReferencedModeDeletedBeforeTriage
// (Story 2.2.2's explicit focus area) is the end-to-end version of Story
// 1.3.3's fail-closed behavior, now exercised via a real Delete + real cache
// invalidation instead of a test fixture: create mode -> select on item ->
// delete mode -> trigger triage -> assert default-mode output + Warn log.
func TestTriggerTriage_should_FallBackToDefaultWithWarnLog_When_ReferencedModeDeletedBeforeTriage(t *testing.T) {
	svc, _, _ := newPipelineModeTestService(t)
	ctx := t.Context()
	pool := &fakeHeadlessPool{response: validTriageJSON()}
	svc.SetHeadlessPool(pool)

	createResp, err := svc.CreatePipelineMode(ctx, connect.NewRequest(&sessionv1.CreatePipelineModeRequest{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "QUICK MODE TRIAGE: {{item_title}}",
	}))
	require.NoError(t, err)
	id := createResp.Msg.Item.Id

	item, err := svc.storage.CreateBacklogItem(ctx, session.BacklogItemData{
		Title:        "quick-mode item about to lose its mode",
		Status:       string(session.BacklogStatusIdea),
		Priority:     3,
		RepoPath:     t.TempDir(),
		PipelineMode: "quick",
	})
	require.NoError(t, err)

	_, err = svc.DeletePipelineMode(ctx, connect.NewRequest(&sessionv1.DeletePipelineModeRequest{Id: id}))
	require.NoError(t, err)

	buf := swapWarningLog(t)

	_, trigErr := svc.TriggerTriage(ctx, connect.NewRequest(&sessionv1.TriggerTriageRequest{ItemId: item.ID}))
	require.NoError(t, trigErr)

	require.Eventually(t, func() bool {
		return pool.callCount() == 1
	}, 5*time.Second, 50*time.Millisecond, "expected exactly one headless triage call")

	gotPrompt := pool.firstCall().userPrompt
	assert.NotContains(t, gotPrompt, "QUICK MODE TRIAGE:",
		"the deleted mode's template must not be used")
	assert.Contains(t, gotPrompt, "Perform pre-implementation triage",
		"expected the default BuildHeadlessTriagePrompt boilerplate after fallback")

	assert.Contains(t, buf.String(), "[PipelineEngine]")
	assert.Contains(t, buf.String(), "unresolved pipeline_mode")
	assert.Contains(t, buf.String(), "quick")
}

// ─── TestPipelineModeCRUD round trip ────────────────────────────────────────

// TestPipelineModeCRUD_should_RoundTripCreateGetUpdateDelete_When_CalledSequentially
// (Story 2.2.3) exercises the full Create->Get->Update->Delete sequence
// against a real (ent-backed) test DB, asserting each step's response
// matches expected state, including that content_hash changes when a
// content-template field is updated.
func TestPipelineModeCRUD_should_RoundTripCreateGetUpdateDelete_When_CalledSequentially(t *testing.T) {
	svc, _, _ := newPipelineModeTestService(t)
	ctx := t.Context()

	createResp, err := svc.CreatePipelineMode(ctx, connect.NewRequest(&sessionv1.CreatePipelineModeRequest{
		Slug:                  "quick",
		Name:                  "Quick Fix",
		Description:           "A fast pipeline mode",
		Enabled:               true,
		TriagePromptTemplate:  "original triage prompt",
		ReviewPromptTemplate:  "original review prompt",
		InitialPromptTemplate: "original initial prompt",
	}))
	require.NoError(t, err)
	created := createResp.Msg.Item
	require.NotEmpty(t, created.Id)
	assert.Equal(t, "quick", created.Slug)
	assert.Equal(t, "Quick Fix", created.Name)
	originalHash := created.ContentHash
	require.NotEmpty(t, originalHash)

	getResp, err := svc.GetPipelineMode(ctx, connect.NewRequest(&sessionv1.GetPipelineModeRequest{Slug: "quick"}))
	require.NoError(t, err)
	assert.Equal(t, created.Id, getResp.Msg.Item.Id)
	assert.Equal(t, originalHash, getResp.Msg.Item.ContentHash)

	updateResp, err := svc.UpdatePipelineMode(ctx, connect.NewRequest(&sessionv1.UpdatePipelineModeRequest{
		Id:                   created.Id,
		Name:                 strPtr("Quick Fix v2"),
		TriagePromptTemplate: strPtr("updated triage prompt"),
	}))
	require.NoError(t, err)
	updated := updateResp.Msg.Item
	assert.Equal(t, "quick", updated.Slug, "slug is immutable and must be unchanged by Update")
	assert.Equal(t, "Quick Fix v2", updated.Name)
	assert.Equal(t, "updated triage prompt", updated.TriagePromptTemplate)
	assert.Equal(t, "original review prompt", updated.ReviewPromptTemplate, "unset fields must not be clobbered by partial update")
	assert.NotEqual(t, originalHash, updated.ContentHash, "content_hash must change when a content-template field changes")

	getAfterUpdate, err := svc.GetPipelineMode(ctx, connect.NewRequest(&sessionv1.GetPipelineModeRequest{Slug: "quick"}))
	require.NoError(t, err)
	assert.Equal(t, updated.ContentHash, getAfterUpdate.Msg.Item.ContentHash)

	_, err = svc.DeletePipelineMode(ctx, connect.NewRequest(&sessionv1.DeletePipelineModeRequest{Id: created.Id}))
	require.NoError(t, err)

	_, err = svc.GetPipelineMode(ctx, connect.NewRequest(&sessionv1.GetPipelineModeRequest{Slug: "quick"}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

// ─── Story 2.3.1: structural validation at the RPC boundary ───────────────
//
// The 3 tests below match plan.md's Story 2.3.1 Given-When-Then acceptance
// criteria scenarios exactly (project_plans/backlog-configurable-pipeline/
// implementation/plan.md, Epic 2.3).

// TestCreatePipelineMode_should_ReturnCodeInvalidArgumentNamingInvalidField_When_SlugInvalid
// covers: Given CreatePipelineModeRequest{slug: "Quick Fix!", ...} (invalid
// slug — uppercase + space + punctuation), When CreatePipelineMode is
// called, Then it returns connect.CodeInvalidArgument with a message naming
// the invalid field, and no row is written.
func TestCreatePipelineMode_should_ReturnCodeInvalidArgumentNamingInvalidField_When_SlugInvalid(t *testing.T) {
	svc, repo, _ := newPipelineModeTestService(t)
	ctx := t.Context()

	_, err := svc.CreatePipelineMode(ctx, connect.NewRequest(&sessionv1.CreatePipelineModeRequest{
		Slug:                 "Quick Fix!",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "Fix {{item_id}}.",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "slug")

	all, listErr := repo.ListAll(ctx)
	require.NoError(t, listErr)
	assert.Empty(t, all, "no row should be written when slug validation fails")
}

// TestCreatePipelineMode_should_ReturnCodeInvalidArgumentNamingFieldAndToken_When_UnrecognizedPlaceholderUsed
// covers: Given CreatePipelineModeRequest{slug: "quick", triage_prompt_template:
// "Fix {{item_id}} using {{made_up_placeholder}}.", ...} (unrecognized
// placeholder), When CreatePipelineMode is called, Then it returns
// connect.CodeInvalidArgument naming triage_prompt_template and the
// unrecognized token made_up_placeholder, and no row is written.
func TestCreatePipelineMode_should_ReturnCodeInvalidArgumentNamingFieldAndToken_When_UnrecognizedPlaceholderUsed(t *testing.T) {
	svc, repo, _ := newPipelineModeTestService(t)
	ctx := t.Context()

	_, err := svc.CreatePipelineMode(ctx, connect.NewRequest(&sessionv1.CreatePipelineModeRequest{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "Fix {{item_id}} using {{made_up_placeholder}}.",
	}))
	require.Error(t, err)
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	assert.Contains(t, err.Error(), "triage_prompt_template")
	assert.Contains(t, err.Error(), "made_up_placeholder")

	all, listErr := repo.ListAll(ctx)
	require.NoError(t, listErr)
	assert.Empty(t, all, "no row should be written when placeholder validation fails")
}

// TestCreatePipelineMode_should_Succeed_When_AllPlaceholdersAreRecognized
// covers: Given CreatePipelineModeRequest{slug: "quick", triage_prompt_template:
// "Fix {{item_id}}: {{item_title}}.", ...} (all recognized placeholders),
// When CreatePipelineMode is called, Then it succeeds.
func TestCreatePipelineMode_should_Succeed_When_AllPlaceholdersAreRecognized(t *testing.T) {
	svc, repo, _ := newPipelineModeTestService(t)
	ctx := t.Context()

	resp, err := svc.CreatePipelineMode(ctx, connect.NewRequest(&sessionv1.CreatePipelineModeRequest{
		Slug:                 "quick",
		Name:                 "Quick Fix",
		Enabled:              true,
		TriagePromptTemplate: "Fix {{item_id}}: {{item_title}}.",
	}))
	require.NoError(t, err)
	assert.Equal(t, "quick", resp.Msg.Item.Slug)

	all, listErr := repo.ListAll(ctx)
	require.NoError(t, listErr)
	assert.Len(t, all, 1, "the row should be written when validation passes")
}
