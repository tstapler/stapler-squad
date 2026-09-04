# ADR-002: New `prompt_library.proto` Service, Not an Extension of `session.proto` or `session/prompts.PromptStore`

**Status**: Accepted
**Date**: 2026-08-06
**Project**: prompt-library

## Context

Two existing structures could plausibly have absorbed this feature instead of a new proto file and Go package:

1. **`proto/session/v1/session.proto`** — the existing ~2500-line proto file already defines `CreateSessionRequest`/`Session` and most session-lifecycle RPCs. Adding template list/get/save RPCs here would avoid a new file.
2. **`session/prompts/store.go`'s `PromptStore`** — an already-shipped, already-tested component backing a single global JSON file (`~/.stapler-squad/prompts.json`) of auto-recorded prompt-entry history (`PromptEntry{ID, Text, LastUsedAt, UseCount}` — no `Name`/`Description`/`Tags`/`Scope`), surfaced in `SessionWizard.tsx` as "recent prompts."

`research/architecture.md` and `research/stack.md` both independently concluded a new, small, standalone proto file mirroring `github_user.proto` (169 lines) is the right shape; this ADR records why, since "add a new proto file" is a decision worth being able to point back to later.

## Decision

Add `proto/session/v1/prompt_library.proto` as a new, small, standalone file — same package (`session.v1`), compiling into the same `gen/proto/go/session/v1/sessionv1connect` Go package and TS module as every other proto file in this directory, but textually separate from `session.proto`. Define:

```protobuf
service PromptLibraryService {
  rpc ListPromptTemplates(ListPromptTemplatesRequest) returns (ListPromptTemplatesResponse) {}
  rpc GetPromptTemplate(GetPromptTemplateRequest) returns (GetPromptTemplateResponse) {}
  rpc SavePromptTemplate(SavePromptTemplateRequest) returns (SavePromptTemplateResponse) {}
}
```

`ListPromptTemplatesRequest` carries a `string path = 1` field (same semantics as `CreateSessionRequest.path`) so the backend can resolve the workspace-local directory per call. No new fields are added to `CreateSessionRequest` or `SessionType` — see requirements.md AC9 and `.claude/rules/session-creation-registry.md`; this feature does not touch that registry's 7 touchpoints.

The Go backend is a **new package**, `promptlibrary/` (repo root, parallel to `session/`, `config/`, `github/`), not an extension of `session/prompts/`. `session/prompts.PromptStore` is left completely unmodified.

## Alternatives Considered

- **Extend `session.proto` in place.** Rejected: `research/stack.md` notes the existing precedent is small standalone files for feature-scoped services sharing the `session.v1` package (`github_user.proto`, `insights.proto`, `headless.proto`) specifically to avoid growing an already-2500-line file further and to keep this feature's wire contract independently reviewable/diffable. Nothing about `PromptLibraryService`'s three RPCs has any coupling to `CreateSessionRequest` or session lifecycle that would justify colocating them.
- **Extend `session/prompts.PromptStore` with `Name`/`Description`/`Tags`/`Scope` fields and repurpose it as the template library.** Rejected on two independent grounds. First, correctness: `PromptStore` is a single flat JSON file (`~/.stapler-squad/prompts.json`) with no scope concept — retrofitting workspace-vs-global storage into it would mean either splitting one file into two conflicting schemas or inventing an ad hoc in-file scope tag, neither of which satisfies requirements.md §1's explicit "markdown + YAML frontmatter, git-shareable per-workspace" storage format (a JSON blob cannot be committed per-workspace and merged the way a directory of `.md` files can). Second, product semantics: `research/features.md` explicitly flags that "recent prompts" (auto-recorded history, no curation) and "prompt templates" (curated, named, tagged) are conceptually distinct features already surfaced separately in the UI (`SessionWizard.tsx`'s recent-prompts list vs. this feature's new `TemplatePicker.tsx`) — collapsing them into one backing store risks a subsequent maintainer conflating the two concepts in code even if the UI kept them visually separate.
- **No RPC at all — serve templates as static files the frontend fetches directly.** Rejected per requirements.md §6: this app's architecture is Go backend + React SPA over ConnectRPC with no client-side filesystem access; templates also need server-side parsing (YAML frontmatter → JSON) and validation (slug/path-traversal, size cap) before ever reaching the browser, which a raw static-file passthrough would not provide.

## Consequences

- `promptlibrary/` is a new top-level Go package with no dependency on `session/` internals beyond what's needed for workspace-root resolution (`go-git`, per `.claude/rules/prefer-go-git-over-subshells.md`) — it does not import `session/prompts`, and `session/prompts` does not import it. The two remain fully independent; a future consolidation (if ever warranted) is a separate, explicitly-scoped change, not a byproduct of this one.
- `server/server.go` registers `PromptLibraryServiceHandler` unconditionally (no feature-flag interceptor), matching `GitHubUserService`'s registration pattern rather than `BacklogService`'s beta-gated one — this feature ships without a flag from day one.
- Naming: the service, package, and UI section names (`PromptLibraryService`, `promptlibrary/`, `TemplatePicker`) all deliberately avoid the bare word "Prompts" in isolation, per `research/features.md`'s collision warning — every plan.md task and code comment introducing a new identifier here must include "template" or "library" in the name, not just "prompt."
