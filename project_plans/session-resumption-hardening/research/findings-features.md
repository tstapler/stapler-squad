# Findings: Features — Checkpoint UX Survey

## Summary

The dominant pattern across developer tools for named point-in-time bookmarks is: **modal/inline label input with an auto-generated default, followed by a chronological list with label + timestamp + metadata**. git stash naming is universally disliked for friction (users forget the `-m` flag); JetBrains Local History's "Put Label" is the gold standard — one click, one field, instant confirmation. The recommendation for this app is a popover/tooltip UI triggered by a bookmark icon button: label input pre-filled with a timestamp default, "Create" button, and a collapsible list below the session card showing label + time + git SHA prefix + conversation UUID prefix.

## Options Surveyed

1. **git stash** — `git stash push -m "before refactor"` — named stash as named-point-in-time metaphor
2. **JetBrains Local History** — "Put Label" right-click action; labels appear inline in the history tree
3. **tmux-resurrect** — saves/restores entire tmux session layout; no per-session checkpoint/bookmark concept
4. **VSCode workspace restore** — restores open files/tabs on reopen; no user-visible named restore points
5. **Browser DevTools breakpoints** — named breakpoints with condition/log annotations in a list panel
6. **Linear saved views / bookmarks** — named filter sets; label + icon + list

## Trade-off Matrix

| Pattern | Naming friction | List readability | Discoverability | Reversibility |
|---|---|---|---|---|
| Modal with empty text input (git stash style) | High — user must always type | Good if labels are meaningful | Low — hidden in a menu | N/A — bookmarks are non-destructive |
| Popover with pre-filled timestamp default | Low — can accept default or rename | Good | Medium — needs visible button | N/A |
| Auto-label only (no user input) | Zero | Poor — all entries look alike | N/A | N/A |
| JetBrains "Put Label" (inline tree entry) | Low — single field, enter to confirm | Excellent — inline in timeline | High — visible in history panel | N/A |
| Keyboard shortcut + quick label (VS Code command palette style) | Medium — requires knowing shortcut | Depends on UI | Low initially | N/A |

## Risk and Failure Modes

**Too many unnamed/timestamp-only checkpoints**: If the default label is never changed, the list becomes a wall of `"2026-04-15 14:32:01"` entries. Mitigation: show label prominently, use relative time ("3 minutes ago") as secondary, truncate long lists to latest 10 with "show all" expand.

**Label collision**: Two checkpoints with identical labels are confusing. Mitigation: de-duplicate is not required (IDs are UUIDs), but sort by timestamp so most recent is first.

**Checkpoint list grows unbounded**: With no max, a session running for weeks could accumulate hundreds. Mitigation: cap display at 10, offer "delete" per entry, consider a soft cap with warning at 50.

**Git SHA absent for directory-only sessions**: Sessions without a git worktree have no commit SHA. Mitigation: show "—" for SHA field; don't hide the checkpoint or error.

## Migration and Adoption Cost

New feature — no migration. The data model (`session.Checkpoint` struct with `Label`, `Timestamp`, `GitCommitSHA`, `ClaudeConvUUID`) is already defined. The API exists. Only the web UI component is missing.

## Operational Concerns

**Unbounded list**: Cap display at 10 most recent; "Load more" or delete old ones. Server-side, there is no persistence cap currently — `instance.Checkpoints` is a slice with no max.

**Real-time updates**: If a user creates a checkpoint in one browser tab, it should appear in another. ConnectRPC streaming (existing pattern) can push the updated session with checkpoints included. Alternatively, the checkpoint list can be fetched on demand via `ListCheckpoints`.

**Deletion UX**: Accidental deletion is permanent (no undo in current model). Show a confirmation tooltip or use a 2-second delay with cancel.

## Prior Art and Lessons Learned

**JetBrains Local History "Put Label"**: Best-in-class. Right-click → "Put Label" → single text field → enter. Labels appear as yellow marker lines in the history tree. Learnings: (1) pre-filled default reduces friction dramatically, (2) inline display in a timeline is more readable than a modal list, (3) label is optional — the entry exists even without one.

**git stash naming frustrations**: The `-m` / `--message` flag is not discoverable. Most users either never name stashes or learn it after losing a stash in a list of 20. Lesson: **make naming the default path, not an opt-in flag**.

**tmux-resurrect**: No checkpoint concept. Restores the entire session layout from a saved file. No per-session metadata. Lesson: coarse-grained saves are simple but insufficient for AI coding workflows where the conversation context is the valuable artifact.

**DevTools breakpoints list**: Persistent checkbox list with label, condition, and location. Very readable. Lesson: show metadata (git SHA, UUID) as secondary text below the label, not as columns — columns waste horizontal space on narrow cards.

## Open Questions

- [ ] Should there be a maximum checkpoint count per session? If yes, what should it be? — blocks: server-side enforcement in `CreateCheckpoint`
- [ ] Should the checkpoint list live in a drawer/panel below the session card, or in a separate modal? — blocks: component placement decision

## Recommendation

**Recommended UX pattern**: Popover triggered by a bookmark icon (🔖) button on the session card action bar.

- **On click**: small popover with a single text input pre-filled with `"Checkpoint YYYY-MM-DD HH:MM"`, and a "Create" button
- **On submit**: optimistic UI — adds entry to list immediately, calls `CreateCheckpoint` RPC in background
- **Checkpoint list**: collapsible section below session card or in the session detail panel; show label prominently, `HH:MM:SS` timestamp secondary, `git:abc1234` and `conv:def5678` as small pills
- **Delete**: ✕ button per entry; single click with 2s undo toast
- **Empty state**: "No checkpoints yet — click 🔖 to save your place"

This follows the JetBrains pattern (low friction, pre-filled default, immediate confirmation) while fitting into the existing card-based UI.

## Pending Web Searches

1. `tmux resurrect save restore named sessions bookmark 2024 2025` — verify whether tmux-resurrect has any checkpoint concept [TRAINING_ONLY - verify]
2. `JetBrains local history "put label" UX 2024` — verify the Put Label interaction details [TRAINING_ONLY - verify]
3. `checkpoint UI design developer tools bookmark session list best practices` — any newer patterns since 2024
4. `git stash named stash UX frustrations developer complaints` — verify the friction complaints are widespread
