# ADR-001: Claim Label as Best-Effort, Not Authoritative Dedup

## Status
Proposed

## Context
When a GitHub issue is imported as a backlog item, we want to prevent other Stapler Squad instances from picking it up. Two mechanisms are possible:
1. Write a GitHub label to the issue (visible to all instances, cross-host, also visible on GitHub.com)
2. Rely on the cross-host gossip protocol (`cross-host-claim-dedup` plan)

## Decision
Use **both**, but treat the GitHub label as best-effort and gossip as authoritative:
- The label is applied on import (best-effort, fire-and-forget, write permission not assumed)
- The gossip protocol is the authoritative cross-host dedup gate for blocking duplicate imports
- The label is the fast UI filter in `ListGitHubIssues` for other instances

## Consequences
- **Label add can fail silently**: token may not have `repo` write scope (only `read:org` / `public_repo`). This is acceptable because gossip still blocks.
- **Label is user-visible on GitHub**: users who inspect the issue on github.com see `stapler-squad` as a label. This is a feature, not a bug — it communicates "this is being automated."
- **Label removal on archive**: we should remove the label if we abandon the work, so the issue can be picked up again. This is AC-6 in requirements.
- **Default label name**: `"stapler-squad"` — configurable per ItemSource via `claim_label` in plugin config.
