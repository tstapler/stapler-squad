# Research: Stack & Patterns — session-pr-creation

Agent 1 (Stack). Scope: concrete libraries/frameworks/versions/patterns for
building the pre-fill/edit PR modal + mechanical (non-agentic) PR-creation RPC.

## Go backend

### Versions (`go.mod`)
- `connectrpc.com/connect v1.19.0` — RPC framework (not the older
  `github.com/bufbuild/connect-go v1.10.0`, which is present only as an
  indirect/legacy dep; new handlers use `connectrpc.com/connect`).
- `entgo.io/ent v0.14.5` — ORM. Not directly relevant here: session/PR state
  is stored on the in-memory `Instance` struct + `storage.SaveInstances()`
  (see below), not via an ent mutation — this feature does not touch the ent
  schema at all.
- `google.golang.org/protobuf v1.36.11`.

### RPC handler pattern to copy
`RunOneShot` (`server/services/session_service.go:3616`) is the direct
sibling to model the new RPC on:
1. Validate required fields → `connect.NewError(connect.CodeInvalidArgument, ...)`.
2. Resolve the session: `inst := s.findInstance(req.Msg.SessionId)` → `connect.CodeNotFound` if nil.
3. Resolve worktree dir: `inst.GetEffectiveRootDir()` fallback `inst.Path`.
4. Do the mechanical work — for the new RPC this is `GitWorktree.CreatePR`
   (`session/git/worktree_git.go:329`), not a `headlessPool.CallBlocking`
   agent turn.
5. Persist PR URL/number back onto the session and save:
   ```go
   inst.SetGitHubPR(prURL, prNumber)
   s.storage.SaveInstances(s.allInstances())
   s.eventBus.Publish(events.NewSessionUpdatedEvent(inst, []string{"github_pr_url", "github_pr_number"}))
   ```
   This is the exact mutation the new RPC must reuse (`server/services/session_service.go:3691-3696`)
   so the session card badge updates identically to the existing path.
6. If the session is backlog-linked, also call
   `s.backlogLifecycleListener.RecordPRCreatedOutOfBand(ctx, inst.UUID, prURL, prNumber)`
   (`server/services/session_service.go:3706-3708`) — same reasoning applies to
   the new mechanical RPC: without it a backlog item's PR created via this new
   button is invisible to `ReconcilePRPending`.

### The mechanical PR pieces already built (reuse, don't reimplement)
- `headless.DraftPRDescription(ctx, pool, itemTitle, itemDescription, diff, branchName) (string, error)`
  (`session/headless/features.go:280`) — one LLM call, body only. For a
  non-backlog session, `itemDescription` has no backlog item to draw from;
  pass session title/empty description, following the same call shape
  `pushAndCreatePR` uses (`session/backlog_lifecycle.go:3651`).
- `GetGitDiff(ctx, worktreePath, baseCommitSHA)` — feeds the diff into
  `DraftPRDescription`; already used at `session/backlog_lifecycle.go:3648`.
- `(*GitWorktree).CreatePR(title, body string) (prURL string, prNumber int, err error)`
  (`session/git/worktree_git.go:329`) — literal `gh pr create`, with
  existing-PR reuse baked in via `findExistingPR` (checked before AND after
  the create call, to handle races).
- `(*GitWorktree).HasCommitsAheadOfMain(mainBranch string) (bool, error)`
  (`session/git/worktree_git.go:428`) — pre-flight check to avoid gh's "No
  commits between X and Y" error; `pushAndCreatePR` calls this before
  `CreatePR` (`session/backlog_lifecycle.go:3638`) and treats an inconclusive
  check as "proceed anyway." Requirement #1 needs an equivalent check before
  the modal even opens ("at least one commit ahead of its base branch").
- `findExistingPR()` is unexported (package-private to `session/git`) — the
  new RPC handler in `server/services` cannot call it directly; it must go
  through the exported `CreatePR`, which already does the existing-PR check
  internally and returns the existing URL/number without erroring
  (`session/git/worktree_git.go:338-341`). For requirement #4 ("modal
  reflects existing PR instead of attempting a duplicate"), the handler
  should check `inst.GitHubPrUrl`/`GitHubPrNumber` (already persisted on the
  session from any prior `RunOneShot`/mechanical call) before calling
  `CreatePR`, OR just call `CreatePR` and rely on its built-in
  short-circuit — the latter is simpler and matches the existing
  `pushAndCreatePR` pattern of checking `item.PrNumber > 0 && item.PrURL != ""`
  first (`session/backlog_lifecycle.go:3622`) as a fast path, falling through
  to `CreatePR`'s own reuse check otherwise.

### Proto pattern to copy
`RunOneShotRequest`/`RunOneShotResponse` (`proto/session/v1/session.proto:1946-1959`)
is the message-shape template — flat fields, no nested messages, matching
this repo's proto style. A new RPC (e.g. `CreatePRFromDiff` or similar) needs
its own request/response pair with fields for `session_id`, `title`, `body`,
`base_branch`, and a response with `pr_url`, `pr_number`, `error`. Also add a
`DraftPRDescription`-triggering RPC (or a `preview: bool` mode on the same
RPC) to populate the modal's initial title/body before the user commits —
check whether the acceptance criteria implies two round trips (fetch
pre-fill, then confirm) or one RPC that both drafts and can be dry-run;
lean toward two RPCs: one read-only "draft" call and one "create" call, so
`GET`-shaped idempotent preview never has PR-creation side effects.

### `make proto-gen`
Runs `buf generate proto` (Makefile:398-413), incremental via a stamp file
(`$(PROTO_STAMP)`) compared against `proto/**/*.proto` mtimes and the
`protoc-gen-es` binary. Regenerates both:
- `gen/proto/go/session/v1/*.pb.go` (Go)
- `web-app/src/gen/session/v1/*_pb.ts` (TypeScript)

`buf.yaml` / `buf.gen.yaml` / `buf.gen.go-only.yaml` live at repo root — no
version pin investigation needed beyond what's already configured; this repo
already has working `buf` tooling wired through `make proto-gen`'s
`ensure-tools` prerequisite. Per project MEMORY.md instinct
(`instinct_alias_session.md`): `web-app/src/gen` is tracked in git despite
`.gitignore`, and `buf-setup-action` can rate-limit in CI — regenerate
locally and commit the generated files rather than relying on CI to
regenerate.

## Frontend (web-app)

### Versions (`web-app/package.json`)
- `@bufbuild/protobuf ^2.11.0`
- `@connectrpc/connect ^2.1.1`, `@connectrpc/connect-web ^2.1.1`
- `@bufbuild/protoc-gen-es ^2.11.0` (dev, codegen)

### Modal pattern to copy
No standalone reusable `<Modal>` component exists — each modal is inlined
per-feature inside its owning component, following this exact shape
(`web-app/src/components/sessions/SessionActionsOverflow.tsx`):
1. `useState` flags for open/closed + field state (e.g. `isCheckpointOpen`,
   `checkpointLabel`, `checkpointError`).
2. A `useRef<HTMLDivElement>` per dialog + `useFocusTrap(ref, isOpen, triggerRef)`
   (`@/lib/hooks/useFocusTrap`) for a11y focus trapping.
3. Rendered via `createPortal(<div className={confirmDialog}>...</div>, document.body)`
   — required per `.claude/rules/css-architecture.md`'s "no `position: fixed`
   modal without `createPortal`" rule (ancestor `transform`/`filter` silently
   breaks fixed positioning otherwise).
4. Outer overlay div closes on click (`onClick` sets open state false),
   inner dialog div stops propagation (`onClick={(e) => e.stopPropagation()}`).
5. `role="dialog" aria-modal="true" aria-labelledby="<id>Title"`.
6. Styles come from `SessionActionsOverflow.css.ts`, which is actually a
   re-export of tokens/classes defined in the sibling `SessionCard.css.ts`
   (`export { confirmDialog, renameDialog, dialogContent, dialogActions,
   submitButton, cancelButton, ... } from "./SessionCard.css"`) — per
   `.claude/rules/css-architecture.md`, new component styles should be
   vanilla-extract (`.css.ts`), and this repo already has the exact
   `dialogContent`/`dialogActions`/`submitButton`/`cancelButton`/`renameInput`/
   `errorMessage` tokens ready to reuse for a new "Create PR" modal rather
   than inventing new class names.
7. The **checkpoint modal is the closest existing analog** to the new PR
   modal: single-field editable input pre-populated to nothing, submit
   button disabled while `isCreatingCheckpoint`/on empty input, inline
   `checkpointError` display on failure. The new PR modal is a superset
   (title input + multi-line body textarea + base-branch selector), same
   loading/error/disabled-button conventions apply.

### RPC-call hook pattern (`useSessionService.ts`)
`runOneShot` (`web-app/src/lib/hooks/useSessionService.ts:563-582`) is the
template: a `useCallback` wrapping `clientRef.current.<method>({...})`,
`dispatch(setError(...))` on catch, returns the typed response or `null`.
New hook functions for the mechanical RPC (and its draft/preview call, if
split into two RPCs) should follow this exact shape — same error-dispatch
pattern, same `clientRef.current` null-guard, same return-typed-or-null
contract — so the modal component's caller code matches every other
RPC-backed action in this file.

Wiring chain to replicate (per requirement #7, replacing not duplicating):
`SessionActionsOverflow.tsx` (`onRunOneShot` prop) → `SessionCard` →
`SessionList` → `web-app/src/app/page.tsx:294` (`handleRunOneShot`) and
`web-app/src/app/review-queue/page.tsx:343`. The new mechanical-PR button
replaces this `onRunOneShot`-driven "Create PR" affordance at the same
touchpoints (component prop → card → list → both page-level handlers) —
do not leave both wired.

### Diff-viewer alternate entry point
`GetSessionDiff` RPC exists (`proto/session/v1/session.proto:36`,
`GetSessionDiffRequest`/`Response` at lines 652/657) — acceptance criterion
1 mentions the action can also live "on a session card / diff viewer";
whichever component renders the diff view is the second place the button
needs equivalent wiring if the modal is surfaced there too (not just the
overflow menu).

## Testing stack

### Go
- `make build && make test` — build first (proto codegen + ent codegen are
  build-time dependencies of some packages), then `go test ./...`.
- Targeted: `go test ./server/services` (requires `make build` first for
  generated proto/ent code), `go test ./session/...`.
- Existing sibling tests to model the new RPC's Go test on:
  `session/backlog_lifecycle_test.go` has ~13 `TestPushAndCreatePR_*`
  functions (e.g. `TestPushAndCreatePR_ReusesExistingPR_WhenAlreadySet:2614`,
  `TestPushAndCreatePR_CreatePRFails_LeavesItemInReview_AndNotifies:2409`,
  `TestPushAndCreatePR_ZeroDiffBranch_FallsBackToDone:2676`) — same
  scenarios (existing-PR reuse, create failure, zero-commits pre-flight)
  apply to the new RPC's unit tests, just asserting RPC response/error
  instead of backlog state transitions. Naming convention:
  `TestFoo_should_<Effect>_When_<Condition>`.
- `make quick-check` — build + test + lint, fast validation gate.
- `make lint` is required — `make build` fails if lint fails.

### Frontend (Jest/RTL)
- `cd web-app && npx jest --no-coverage`
- Targeted: `npx jest --testPathPatterns="<pattern>" --no-coverage`
- No `SessionActionsOverflow.test.tsx` currently exists in
  `web-app/src/components/sessions/` — the new modal's test file would be a
  net-new `*.test.tsx` colocated with wherever the modal component lives
  (either inline in `SessionActionsOverflow.tsx` tests, or a new extracted
  `CreatePRModal.tsx` + `CreatePRModal.test.tsx` if the modal is factored
  out as its own component — factoring it out is cleaner given the
  field-count jump from checkpoint's single input to title+body+branch).

### E2E (Playwright)
Per `.claude/rules/e2e-test-conventions.md` (CI-enforced):
1. `// @feature session:create, ...` header comment naming the relevant
   feature IDs (this feature will need new IDs registered per
   `.claude/rules/feature-registry.md` — e.g. `session:create-pr-mechanical`
   backend, `create-pr-modal` frontend).
2. No `waitForTimeout` — use `expect(locator).toHaveValue(...)` /
   `waitForSelector`.
3. Locators: `data-testid` or ARIA roles only, no CSS class selectors.
4. New reusable page-interaction helpers go in `tests/e2e/pages/`.
- Run: `cd tests/e2e && npx playwright test <new-spec>.spec.ts`. Server
  auto-managed by `global-setup.ts` — never start one manually.

### Feature registry
Per `.claude/rules/feature-registry.md`: new backend RPC → create
`docs/registry/features/backend/<feature>.json` with `markerFound: true` if
a `// +api: scope:action` marker is added to the handler (`RunOneShot`
already has one: `// +api: session:run-one-shot` at
`server/services/session_service.go:3613`, follow the same convention). New
frontend modal → `docs/registry/features/frontend/<feature>.json`. Run
`make registry-generate` after, verify `docs/registry/coverage-gaps.json`
doesn't grow.

## Open questions for the plan phase
- Whether pre-fill (`DraftPRDescription`) and create (`CreatePR`) should be
  one RPC or two — leaning two (draft is read-only/idempotent, create has
  side effects) but this is a plan-phase architecture call, not a stack
  question.
- Whether the new mechanical RPC needs its own Go test file or extends
  `session_service_test.go` — no existing `RunOneShot`-specific Go test was
  found in `server/services/session_service_test.go` (only
  `TestSessionService_should_ReturnUpdatedPort_When_...` and
  `..._ReturnEmptyString_When_...` unrelated MCP-URL tests exist there), so
  there's no existing RunOneShot-RPC test pattern to strictly copy at the
  `server/services` layer — the closest RPC-level precedent is at the
  `session/backlog_lifecycle_test.go` `pushAndCreatePR` layer instead.
