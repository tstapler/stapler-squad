# Stack Research: Workflow History and Archiving

## ent ORM Schema Extension

Two new optional fields were added to `session/ent/schema/session.go`:

- `workflow_id`: `field.String(...).Optional()` — stores the spawning workflow's UUID as a plain string. No FK edge to the Workflow entity.
- `archived_at`: `field.Time(...).Optional().Nillable()` — stores a nullable timestamp. `Nillable()` generates `*time.Time` in the entity struct, which enables three-state semantics: absent (never set), present+nil (cleared), present+non-nil (archived).

Both fields have dedicated indexes (`index.Fields("workflow_id")` and `index.Fields("archived_at")`) to support efficient filter queries. All ent codegen was run with the required `--feature sql/upsert` flag (per `session/ent/generate.go`).

## ConnectRPC Auto-Registration Pattern

`ArchiveSession` and `UnarchiveSession` are annotated with `// +api: session:archive` and `// +api: session:unarchive` markers. These follow the existing service handler pattern: implement the method on `*SessionService`, and the ConnectRPC framework routes it automatically. No changes to server registration boilerplate were needed — the handlers are picked up by the existing `connectrpc.com/connect` generated handler wiring.

## vanilla-extract CSS for New Styles

`WorkflowForm.css.ts` and `WorkflowsPanel.css.ts` were created as colocated `.css.ts` files alongside their components. Both import from `@vanilla-extract/css` (`style`) and consume tokens from `@/styles/theme.css` (`vars`). No hardcoded hex values or magic numbers appear; all colors and spacing reference `vars.color.*`, `vars.space[N]`, and `vars.fontSize.*`. The `WorkflowsPanel.tsx` form overlay uses `createPortal(..., document.body)` to escape ancestor CSS transforms, consistent with the CSS architecture rules.

## Three-Layer Persistence

The feature follows the existing Instance → InstanceData → ent layering:

1. `Instance` struct (`session/instance.go`): holds `WorkflowID string` and `ArchivedAt *time.Time` as live in-memory fields.
2. `InstanceData` struct (`session/storage.go`): mirrors the same fields with identical JSON tags (`workflow_id`, `archived_at`). `instance_serialization.go` copies them bidirectionally in `ToInstanceData()` and `FromInstanceData()`.
3. ent repository (`session/ent_repository.go`): `SetNillableWorkflowID` / `SetNillableArchivedAt` on save; `sess.WorkflowID` / `sess.ArchivedAt` on load.

## Lifecycle Listener Pattern

`LifecycleListener` is an interface (`OnLifecycleEvent(event, reason)`) defined in `session/instance.go`. Instances hold a slice of registered listeners protected by a `deadlock.Mutex`. `fireLifecycleEvent` iterates listeners after state transitions. `SessionService.wireAutoArchiveCallback` registers an `autoArchiveListener` on every instance that has a non-empty `WorkflowID`; this wiring call appears in three paths: `CreateSession`, `loadInstancesWithWiring`, and the fallback load path in `ListSessions`.
