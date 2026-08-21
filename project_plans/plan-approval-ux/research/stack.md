# Research: Stack — Plan Approval UX

**Dimension**: Stack | **Phase**: 2 — Research

## Question 1: Does `web-app` already have markdown rendering / diff / editor capability?

**Yes — all the primitives needed already exist as dependencies and are used elsewhere in the codebase.** No new npm dependency is needed for in-app plan rendering or a diff/line-anchored comment surface. All versions/paths below verified directly against `web-app/package.json` and the source tree (not assumed from the prior draft).

Confirmed in `web-app/package.json`:

| Package | Version | Already used at |
|---|---|---|
| `react-markdown` | `^10.1.0` | `src/components/backlog/detail/DescriptionSection.tsx`, `src/components/backlog/BacklogItemForm.tsx`, `src/app/help/page.tsx` |
| `remark-gfm` | `^4.0.1` | same call sites as `react-markdown` (GFM tables/strikethrough/task lists) |
| `shiki` | `^4.0.2` | `src/components/sessions/FileContentViewer.tsx` (syntax highlighting) |
| `codemirror` | `^6.0.2` | `src/components/sessions/SessionDetail.tsx`, `src/components/sessions/FileContentViewer.tsx` |
| `@codemirror/lang-markdown` | `^6.5.0` | same CodeMirror call sites |
| `@monaco-editor/react` (+ `monaco-editor` `^0.55.1`) | `^4.7.0` | `src/app/config/ConfigPageContent.tsx` (JSON config editor only) |

**Correction to the prior draft of this file**: the earlier version of this document claimed `monaco-editor` was *not* a dependency. That was wrong — `@monaco-editor/react` ^4.7.0 and `monaco-editor` ^0.55.1 are both present in `package.json` and used in `ConfigPageContent.tsx` for the JSON config editor. It is not currently used for markdown/plan content anywhere, but it is already in the frontend bundle, so reaching for it (e.g. for a precise line-gutter API) would not be a "new dependency" in the sense the requirements doc's constraint cares about — it would still add real bundle weight for a use case `react-markdown` already covers more cheaply, so it's not recommended as the default choice (see Q2b).

No `diff2html`, `react-diff-view`, or `unidiff`/`parse-diff` packages in `package.json` — the existing diff viewer (`DiffRenderer.tsx`, see Q3) is hand-rolled and dependency-free.

`react-markdown` v10.1.0 is confirmed current/latest as of this research (verified via web search, Aug 2026) — no version bump available or needed.

### Canonical existing pattern: `DescriptionSection.tsx`

This is the closest 1:1 precedent for rendering `PlanArtifactsSection`'s plan content — same layer of the same feature (`web-app/src/components/backlog/detail/`), same "collapsible section showing a markdown field on `BacklogItem`" shape:

```tsx
// web-app/src/components/backlog/detail/DescriptionSection.tsx
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import * as markdownStyles from "../markdownBody.css";
...
<div className={markdownStyles.markdownBody} data-testid="backlog-description-rendered">
  <ReactMarkdown remarkPlugins={[remarkGfm]}>{item.description}</ReactMarkdown>
</div>
```

`markdownBody.css.ts` (`web-app/src/components/backlog/markdownBody.css.ts`) is a shared vanilla-extract style already tuned for rendered markdown inside the backlog detail view — reuse it directly for plan content rather than inventing new markdown typography, per `.claude/rules/css-architecture.md`.

`PlanArtifactsSection.tsx` (`web-app/src/components/backlog/detail/PlanArtifactsSection.tsx`) currently renders only the path as `<code>{item.planArtifactsPath}</code>` — it does not fetch or render file content at all. Rendering the plan document's actual content requires **a new backend read path** (an RPC or extension to `BacklogItem`/`GetBacklogItem` that returns file contents for `plan.md`/`requirements.md`/`research/*.md`/`validation.md` under `planArtifactsPath`, since the frontend has no filesystem access). This is an Architecture-dimension finding, not a Stack one, but it gates whether markdown rendering has anything to render — flagged here so it isn't lost.

## Question 2: Community-recommended versions/patterns for (a) markdown rendering, (b) inline/line-anchored commenting

**(a) Markdown rendering in React (current state of the art, verified via web search):**
- `react-markdown` v10 (already pinned, confirmed latest) is still the standard choice — unstyled-by-default, renders to real React elements (not `dangerouslySetInnerHTML`, so no XSS-sanitization dependency needed), composes with the unified/remark/rehype ecosystem. `remark-gfm` v4 (already pinned) covers GitHub-flavored tables/task-lists/strikethrough, which SDD plan docs use (checklists in `plan.md`, tables in `research/*.md`). No version bump needed.
- If code blocks inside a plan need syntax highlighting (SDD plans sometimes include fenced code snippets), `shiki` is already a dependency (used in `FileContentViewer.tsx`) and can be wired into `react-markdown` via a custom `code` component renderer — no new package required.

**(b) Inline / line-anchored commenting UI (GitHub PR review style):**
- There is no single dominant off-the-shelf React library for "GitHub PR-style inline markdown/diff comments." The realistic options, in order of fit for this codebase:
  1. **Hand-rolled, reusing `DiffRenderer`'s line-row pattern** (see Q3) — parse the doc into lines, render each as a DOM node with a stable line number, attach a gutter click handler that opens a comment composer inline. Zero new dependencies, matches an established in-repo pattern, matches the "no new dependency unless justified" constraint and the "no full CRDT/rich-text editor" out-of-scope note in requirements.md.
  2. **`react-diff-view`** (otakustay/react-diff-view) — renders unified/split diffs with per-line gutter hooks; would need to be added as a new dependency. Only worth it if the feature evolves toward diffing plan revisions against each other (not currently in scope — scope is commenting on a single rendered plan, not reviewing a diff between plan versions).
  3. **CodeMirror's `gutter()` extension** (already a dependency via `codemirror`/`@codemirror/lang-markdown`, used in `FileContentViewer.tsx`) — gives a native per-line gutter click API without hand-rolling line-splitting, at the cost of moving off `react-markdown`'s rendered-prose look toward a code-editor-style monospace view. Worth prototyping if the Architecture/Pitfalls research finds raw-line splitting of rendered Markdown too fragile (see below).
- Given requirements.md explicitly deprioritizes a "full generic rich-text/CRDT collaborative editor" (Out of Scope) and this is a solo-developer, single-reviewer tool, the recommended shape for this scale is **option 1, the hand-rolled gutter-click pattern** extending `DiffRenderer`'s existing approach. Reserve `react-diff-view` or a CodeMirror-based viewer as fallbacks only if hand-rolled line-mapping proves insufficient in practice.

## Question 3: Existing text-rendering/annotation patterns in this codebase to reuse

`web-app/src/components/shared/DiffRenderer.tsx` + `DiffRenderer.css.ts` + `web-app/src/lib/utils/parseDiff.ts` (all confirmed present) is the codebase's own **hand-rolled, dependency-free diff/line renderer** — architecturally the best template for line-level plan feedback, because it already solves "parse text into an addressable line structure and render each line as a distinct DOM node with a stable line number":

```ts
// web-app/src/lib/utils/parseDiff.ts
export interface DiffLine {
  type: "add" | "delete" | "context";
  content: string;
  oldLineNumber?: number;
  newLineNumber?: number;
}
```

`DiffRenderer.tsx` renders each `DiffLine` as its own row with a `lineNumber` / `lineContent` CSS class pair (`DiffRenderer.css.ts`), plus a file-jump sidebar (`fileTree`) driven by `useRef` + `scrollIntoView`. For plan review, the equivalent shape: parse `plan.md` into an array of `{ lineNumber, content }` rows (no diff parsing needed — a plain line splitter, or per-Markdown-AST-node position if finer-than-line granularity is wanted), render each as a row, attach a comment affordance to the gutter — same DOM shape `DiffRenderer` already uses, just simpler (no add/delete/context distinction).

`FileContentViewer.tsx` (`web-app/src/components/sessions/FileContentViewer.tsx`) is the other close precedent: renders arbitrary file content with CodeMirror + shiki inside session detail. If the plan-render surface ultimately wants an editable/CodeMirror-based view instead of read-only `react-markdown`, that component is the pattern to extend — CodeMirror's `gutter()` extension (option 2b above) is a more natural fit for precise line-anchored comments than manually splitting rendered HTML.

No existing generic "comment"/"annotation" component exists anywhere in `web-app/src/components` or `src/lib` (grep for `comment|annotat` under those trees turns up only the **GitHub PR review comment** feature — `pr:get-comments` / `pr:post-comment` RPCs, registered in `web-app/src/lib/features/features/pr.ts` — which posts/reads comments on a *real GitHub PR* via the GitHub API, not local structured storage. That's a different problem (proxying an external system) and not directly reusable for commenting on a locally-rendered plan document, but it does confirm the "comment tied to a piece of content" concept has one precedent in this codebase's RPC surface, if the Architecture dimension wants a naming/shape reference).

The closest UI precedent for "a text feedback box tied to an action" (not line-anchored, just a single feedback string) is the triage-refinement flow in `BacklogItemDetail.tsx`:

```tsx
// web-app/src/components/backlog/BacklogItemDetail.tsx (~line 721)
async (feedback: string) => {
  ...
  await triggerTriage(item.id, feedback);
```

and a separate gate-reopen flow (~line 805–813) which appends feedback text into a timestamped note (`[Revision feedback <timestamp>]\n<feedback>`) rather than a structured field — worth flagging to the Architecture dimension as a possible existing convention (freeform note append) vs. a new structured `RejectPlan(reason)` RPC field, per Q4 below.

## Question 4: ConnectRPC/proto patterns for `ApprovePlan` and `TriggerTriageRequest.feedback` — modeling a `RejectPlan`/`RequestPlanChanges` RPC consistently

Verified directly against `proto/session/v1/backlog.proto` and `server/services/backlog_service_lifecycle.go`:

**Existing message shapes** (`proto/session/v1/backlog.proto`):
```protobuf
message TriggerTriageRequest {
  string item_id = 1;
  // feedback, if non-empty, requests a refinement of the item's most recent
  // completed triage result instead of a fresh triage run. Requires a prior
  // completed triage result to exist.
  string feedback = 2;
}

message ApprovePlanRequest {
  string item_id = 1;
}
message ApprovePlanResponse {
  BacklogItem item = 1;
}
```
RPC registration: `rpc TriggerTriage(TriggerTriageRequest) returns (TriggerTriageResponse) {}` and `rpc ApprovePlan(ApprovePlanRequest) returns (ApprovePlanResponse) {}`, both on the same service.

**Pattern to mirror for a new `RejectPlan`/`RequestPlanChanges` RPC**, consistent with both siblings:
```protobuf
message RejectPlanRequest {
  string item_id = 1;
  // reason is required free-text feedback explaining what should change,
  // mirroring TriggerTriageRequest.feedback's refinement-input pattern.
  string reason = 2;
}
message RejectPlanResponse {
  BacklogItem item = 1;
}
```
- Field numbering: `item_id = 1` is the universal first field across every per-item RPC request in this proto file (`ApprovePlanRequest`, `TriggerTriageRequest`, `AttachSessionToItemRequest`, etc.) — keep that convention.
- Response shape: `ApprovePlanResponse { BacklogItem item = 1; }` is the convention for mutation RPCs on a single item — return the full updated `BacklogItem`, not just a status/bool, so the frontend can re-render from one response without a follow-up `GetBacklogItem` call. `RejectPlanResponse` should follow the same shape.
- Comment style: doc-comment above the field explaining semantics (see `TriggerTriageRequest.feedback`'s comment) — apply the same to a new `reason`/`feedback` field.

**Backend handler pattern** (`server/services/backlog_service_lifecycle.go:617-657`, `ApprovePlan`):
```go
// +api: backlog:approve-plan
func (s *BacklogService) ApprovePlan(
	ctx context.Context,
	req *connect.Request[sessionv1.ApprovePlanRequest],
) (*connect.Response[sessionv1.ApprovePlanResponse], error) {
	if s.storage == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("storage not available"))
	}
	item, err := s.storage.GetBacklogItem(ctx, req.Msg.ItemId)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("backlog item %q not found", req.Msg.ItemId))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get backlog item: %w", err))
	}
	if item.PlanArtifactsPath == "" {
		return nil, connect.NewError(connect.CodeFailedPrecondition, fmt.Errorf("no plan artifacts found — run TriggerTriage first"))
	}
	// ... precondition: plan artifacts must exist on disk ...
	update := session.BacklogItemUpdate{PlanApproved: &approved, PlanApprovedAt: &now}
	updated, err := s.storage.UpdateBacklogItem(ctx, req.Msg.ItemId, update, nil)
	// ...
	return connect.NewResponse(&sessionv1.ApprovePlanResponse{Item: backlogItemToProto(updated, s.buildCostLookup())}), nil
}
```
A `RejectPlan` handler should follow the identical shape: same nil-storage guard, same `GetBacklogItem` + `ent.IsNotFound` error mapping, same `os.Stat` precondition on `PlanArtifactsPath`, same `session.BacklogItemUpdate` partial-update struct, same `backlogItemToProto` response construction, same `// +api: backlog:reject-plan` marker for the feature registry (`.claude/rules/feature-registry.md`).

`TriggerTriage`'s handler (`server/services/backlog_service_triage.go:1834`, `TestTriggerTriage_RefineWithFeedback` in `backlog_service_test.go:2784`) is the pattern to check for how a non-empty `feedback` string is threaded into a re-run — the Architecture dimension should trace that flow specifically to decide whether `RejectPlan.reason` reuses the same prompt-injection path or needs its own.

## Question 5: ent ORM schema conventions for adding new fields

Verified against `session/ent/schema/backlog_item.go` and `session/ent/generate.go`.

**Regeneration command — mandatory, non-negotiable** (`session/ent/generate.go`):
```go
//go:generate go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema
```
Run exactly this (or `go generate ./session/ent/`) after any schema edit. Omitting `--feature sql/upsert` silently breaks `UpsertRule` and similar generated upsert methods without a compile error — see `.claude/rules/ent-schema-generation.md`. Workflow: edit schema → run the correct generate command → `go build ./...` → commit all `session/ent/` changes together in the same commit as the schema edit.

**Field conventions observed in `BacklogItem.Fields()`** — a likely template for whatever this feature needs (e.g. a `plan_rejection_reason string`, `plan_rejected_at time.Time`, or a `plan_status` enum-as-string field, depending on what the Architecture phase decides):
- Boolean status flags pair with a nullable timestamp: `plan_approved bool` (`.Default(false)`) is paired with `plan_approved_at time.Time` (`.Optional().Nillable()`). If "changes requested" becomes a third state, the existing pair-pattern (`field.Bool` + `field.Time().Optional().Nillable()`) is the established shape to replicate, e.g. `plan_rejected_at`.
- Free-text fields default to `.Optional()` with no default value (see `notes`, `plan_artifacts_path`, `pr_url`) — a `plan_rejection_reason` field should follow this, not `.NotEmpty()`, since it's empty until first rejection.
- JSON-blob-in-a-string fields are documented with `.Comment("JSON []Foo")` (see `acceptance_criteria`, `user_modified_fields`, `shipped_file_stats`) — if line-level feedback needs structured storage (e.g. an array of `{line, comment}` objects) rather than a new relational entity, this string+JSON-comment convention is the lightweight option already used repeatedly in this schema, avoiding a new ent type + migration + edge for what might be a small, append-only blob.
- Enum-like state is modeled as a **plain string field with a `Default()`**, not a Go/ent enum type — see `status string` (`.Default("idea")`) and `pipeline_mode string` (`.Default("")`, comment pointing to a separate `session.IsValidBacklogCategory`-style Go-side validator). If plan-approval grows a true multi-value state (`none`/`pending`/`approved`/`changes_requested`), this string-plus-Go-side-validator pattern — not a new ent enum — is the established convention to follow, consistent with `status` and `category`.
- Cascading child records use `edge.To(..., Annotations(entsql.OnDelete(entsql.Cascade)))` — see `status_events`, `stuck_states`, `progress_notes` edges. If line-level feedback comments become their own entity (rather than a JSON blob field), model it as a new edge following this same cascade-delete pattern, mirroring `BacklogProgressNote` as the closest existing "note attached to a backlog item" entity type (worth the Architecture dimension reading `session/ent/schema/backlog_progress_note.go` as a template before inventing a new entity from scratch).
- Indexes are added deliberately per query pattern (`index.Fields("status", "priority")` etc.) — only add a new index if a new query pattern genuinely needs one; don't index every new field by default.

## Recommendation Summary

- **No new npm dependency required** for markdown rendering (`react-markdown` ^10.1.0 + `remark-gfm` ^4.0.1, already deployed at `DescriptionSection.tsx`, confirmed current) or for line-addressable content display (hand-rolled pattern already proven in `DiffRenderer.tsx` / `parseDiff.ts`). `@monaco-editor/react` and CodeMirror are also already-bundled fallbacks if line-gutter precision becomes a hard requirement, but are heavier and not the default recommendation.
- Reuse `markdownBody.css` (vanilla-extract, `web-app/src/components/backlog/markdownBody.css.ts`) for typography and the `DiffRenderer` line-row + gutter DOM/CSS pattern for line addressing — do not introduce `react-diff-view`/`diff2html`/a new Monaco integration unless the Architecture or Pitfalls research phases find the hand-rolled line-mapping approach insufficient (e.g. because Markdown AST nodes don't map 1:1 to raw source lines for wrapped paragraphs — flag this to Pitfalls).
- **The one real gap is backend, not frontend stack**: there is currently no RPC/field that returns the actual file *contents* of `planArtifactsPath`'s `plan.md`/`requirements.md`/`research/*.md`/`validation.md` to the browser — `PlanArtifactsSection.tsx` only ever received the path string. This blocks Success Criterion 4 regardless of which frontend rendering library is chosen — flagged to the Architecture research dimension.
- **`RejectPlan`/`RequestPlanChanges` RPC**: model it as `RejectPlanRequest { string item_id = 1; string reason = 2; }` / `RejectPlanResponse { BacklogItem item = 1; }`, handler in `server/services/backlog_service_lifecycle.go` following `ApprovePlan`'s exact structure (nil-storage guard → `GetBacklogItem` → precondition checks → `session.BacklogItemUpdate` partial update → `backlogItemToProto` response), with a `// +api: backlog:reject-plan` marker. Whether `reason` reuses `TriggerTriage`'s feedback-refinement plumbing or is stored/consumed separately is an Architecture-dimension decision — trace `TestTriggerTriage_RefineWithFeedback` (`server/services/backlog_service_test.go:2784`) first.
- **ent schema additions**: follow the bool-flag + nullable-timestamp pairing (`plan_approved`/`plan_approved_at`) for any new binary state, plain-string-with-`Default()` (not an ent enum) for any new multi-value state, and `.Comment("JSON []Foo")`-annotated string fields for small structured blobs (e.g. line-level feedback entries) rather than a new entity — unless volume/query needs justify a full `BacklogPlanComment`-style entity modeled after `BacklogProgressNote`. Always regenerate with `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./schema` (per `session/ent/generate.go`) and commit schema + generated code together.
