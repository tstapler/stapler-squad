# Requirements: Omni Bar Path Completion

Status: Draft | Phase: 1 - Ideation complete
Created: 2026-04-07

## Problem Statement

When creating a new session in the Omnibar (`Cmd+K`), users must type local paths manually (e.g., `/Users/username/projects/my-repo`). There is currently no feedback loop — users discover that a path doesn't exist only after submitting the form and receiving a server error. There is also no directory completion, so users must remember exact paths rather than being able to navigate/filter the filesystem interactively.

The impact is friction and failed session creation, particularly for:
- New paths not previously used (no history to fall back on)
- Deep nested directories (long paths to memorize)
- Typos in paths

## Success Criteria

1. Zero silent failures for invalid local paths — user is informed inline before submission
2. Users can reach a valid directory in fewer keystrokes than today via real-time directory filtering
3. The path input feels native — keyboard-navigable, non-intrusive, consistent with the existing Omnibar UX pattern

## Scope

### Must Have (MoSCoW)

- **Path existence indicator** — while typing a `LocalPath` or `PathWithBranch` type input, show inline visual feedback indicating whether the current path exists on the server filesystem (green check / red X / neutral spinner)
- **Real-time directory completions** — as user types a partial path, show a filterable dropdown of matching subdirectories at the current path prefix (fzf-style substring matching, not just prefix)
- **Keyboard-only navigation** — arrow keys + Enter/Tab to select a completion, Escape to dismiss dropdown, Tab to insert the selected completion into the input
- **Debounced API calls** — completion requests debounced to avoid excessive server calls (≤300ms)
- **Applies to main Omnibar input** — the primary text field in `Omnibar.tsx` where the path/URL is entered

### Nice to Have

- Tab key auto-completes to longest common prefix (shell-style tab completion)
- Show directory item count or `git` indicator next to completions
- Expand `~` to actual home directory in display

### Out of Scope (this iteration)

- Session search from the Omnibar (separate future feature)
- Remote path support (SSH, cloud storage, network mounts)
- Recently-used path history
- Path completion in the SessionWizard form (only the Omnibar for now)
- Working Directory and Existing Worktree path fields (follow-up)

## Constraints

**Tech stack:**
- Web UI: React (Next.js), TypeScript, ConnectRPC for API calls
- Backend: Go, ConnectRPC/Protobuf
- Existing component: `AutocompleteInput.tsx` (basic substring filter, client-side only; suggestions passed as prop)
- Existing hook: `useRepositorySuggestions` (fetches paths from existing sessions only — no filesystem traversal)
- Detector: `detector.ts` — already distinguishes `LocalPath` / `PathWithBranch` from GitHub URLs. Path validation/completion only applies to these two types.

**Architecture constraint:** Path validation and directory listing must be server-side (Go backend). The browser cannot access the local filesystem directly.

**Protobuf constraint:** New RPC methods require updating `proto/session/v1/session.proto` and running `make generate-proto`.

**No existing path validation API** — must be added.

## Context

### Existing Work

The Omnibar (`web-app/src/components/sessions/Omnibar.tsx`) already:
- Uses `detector.ts` to classify input as `LocalPath`, `PathWithBranch`, `GitHubPR`, etc.
- Shows a type badge inline (e.g., "📁 Local Path")
- Has `AutocompleteInput.tsx` used in `SessionWizard.tsx` for repository path (client-side suggestions only)
- `useRepositorySuggestions` hook fetches paths from existing sessions — not a filesystem browser

The `AutocompleteInput.tsx` component already supports:
- Dropdown with keyboard navigation (↑↓ arrows, Enter, Escape, Tab)
- `isLoading` state
- `error` prop for red border styling

What's missing:
1. A backend API to list directory contents at a given path
2. A backend API (or logic) to validate whether a path exists
3. A frontend hook to call these APIs with debouncing
4. Wiring of completions + validation into the Omnibar's main input

### Stakeholders

- Solo practitioner (Tyler) — primary user and developer
- Workflow: Frequent session creation across multiple repositories

## Research Dimensions Needed

- [x] Stack — existing components already well-understood from code archaeology; skip dedicated research
- [ ] Features — survey comparable tools for path completion UX patterns (fzf, fish shell, VS Code file picker)
- [ ] Architecture — design patterns for client/server path completion (debounce strategy, caching, streaming vs batch)
- [ ] Pitfalls — known failure modes (permissions, symlinks, tilde expansion, slow NFS/network paths, race conditions in debounced inputs)
