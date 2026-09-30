---
name: github-issue-to-backlog
description: Use when a request should become both a tracked GitHub issue and a stapler-squad backlog item — e.g. "file an issue for X and add it to the backlog." For feature requests, verifies no duplicate exists (backlog search + codebase check), styles the ticket body via the project-coordinator agent (AIC framework) informed by product-manager rigor (Kano/RICE, success criteria, out-of-scope, open questions), creates it with gh, then imports it into the backlog via the ImportGitHubIssue RPC.
---

# GitHub Issue → Backlog Import

Four-step workflow for turning a request into a tracked GitHub issue and a stapler-squad backlog item: verify it isn't already covered, style the ticket, create it, import it.

## 1. Verify it doesn't already exist — before writing anything

Two checks, both required, before drafting a word of the ticket:

**a. Codebase check.** Grep/search for anything already touching the feature area. Half-built infra changes the ask from "build X" to "extend X" and makes the ticket evidence-based (file paths, existing types/functions) instead of speculative.

**b. Backlog check.** List existing backlog items and keyword-match against the request — a duplicate idea sitting in the backlog is easy to miss otherwise:

```bash
curl -s -X POST "http://localhost:8543/api/session.v1.BacklogService/ListBacklogItems" \
  -H "Content-Type: application/json" -H "Connect-Protocol-Version: 1" -d '{}' \
  -o /tmp/backlog_items.json
```

Then grep/scan `.items[].title` and `.items[].description` for relevant keywords (synonyms too — "quota" / "capacity" / "rate limit" / "throttle" all matter for the same underlying concept). If a near-duplicate exists, update it (`UpdateBacklogItem`) instead of creating a new one — don't fork the same idea into two items.

## 2. Style the ticket — project-coordinator + product-manager, for feature requests

For **feature requests** (not straightforward bug reports), don't draft the ticket body yourself. Launch the `project-coordinator` agent (it applies the AIC — ATOMIC/INVEST/CONTEXT — ticket framework) and explicitly ask it to apply product-manager rigor while writing the body:

- **Problem framed as an outcome/opportunity**, not a pre-decided solution (Cagan/Torres: problems, not solutions).
- **Kano classification** — is this a basic expectation, performance satisfier, or delighter?
- **RICE-style priority signal** — reach/impact/confidence/effort, qualitative is fine, no fabricated precision.
- **Explicit success criteria** — how you'd know it's working.
- **Explicit out-of-scope** — what this ticket deliberately does not cover.
- **Open questions preserved, not resolved** — architecture/design decisions belong to planning (`/sdd:*`), not the ticket.
- **A one-line suggested entry point** if the repo uses SDD (`/sdd:full` for open architectural questions, `/sdd:quick` for small/well-understood, `/sdd:fix-bug` for bugs).

Feed the agent the grounding from step 1 (file paths, what exists vs. what's missing, confirmation no duplicate exists) so it isn't re-deriving that research. Ask it to return **only** the finished ticket markdown (title + body) — no meta-commentary — since that text goes straight into `gh issue create --body-file`.

**Watch for scope creep**: `project-coordinator` may, on its own initiative, start writing SDD-style planning artifacts (e.g. `project_plans/<name>/requirements.md`) beyond the requested ticket text. That wasn't asked for by this workflow — flag it to the user rather than silently keeping or discarding it (they may want it as a head start on `/sdd:1-ideate`, or may not want an uncommitted file left behind).

For straightforward bug reports, skip the agent — ground the report in a repro/root-cause and file it directly (step 3).

## 3. Create the issue

```bash
gh issue create --repo <owner>/<repo> \
  --title "<title>" \
  --label <label> \
  --body-file <scratchpad>/issue_body.md
```

Write the body to a scratchpad file first (`--body-file`) rather than inlining — multi-paragraph `--body` strings are error-prone to quote correctly. Check `gh label list --repo <owner>/<repo>` for labels to apply.

Confirm which remote/repo before filing if the project has multiple (check `git remote -v` and any project memory/CLAUDE.md about remote roles — e.g. stapler-squad has both `origin` (personal) and `upstream-fanatics` (work), both public).

## 4. Import into the backlog

Prefer the MCP tool:

```
mcp__stapler-squad__import_github_issue(
  issue_url: "https://github.com/<owner>/<repo>/issues/<N>",
  repo_path: "/absolute/local/path/to/repo"   // must be absolute — not owner/repo shorthand
)
```

`repo_path` triggers auto-triage on creation by default (`skip_triage: true` to leave it in `idea` status untouched).

**If it fails with `PERMISSION_DENIED: STAPLER_SESSION_UUID not set`** — that MCP tool is gated to sessions spawned by stapler-squad itself; a plain Claude Code session (or main-repo Claude session) doesn't have it. Fall back to calling the same backend RPC directly against the running local service:

```bash
curl -s -X POST "http://localhost:8543/api/session.v1.BacklogService/ImportGitHubIssue" \
  -H "Content-Type: application/json" \
  -H "Connect-Protocol-Version: 1" \
  -d '{"issueUrl": "https://github.com/<owner>/<repo>/issues/<N>", "repoPath": "/absolute/local/path/to/repo"}' \
  -o /tmp/import_response.json
```

Read the response back (per the repo's evidence-over-claims convention — a write isn't confirmed until you've read the resulting state) rather than assuming success from a 200/empty curl exit code. Check the returned `item.id` and `status`.

To edit an already-imported item in place (e.g. after restyling the body per step 2, or fixing a near-duplicate found in step 1) rather than creating a new one, use `BacklogService.UpdateBacklogItem` the same way — same host/headers, `itemId` + the fields to change. Pass `expectedUpdatedAt` (the item's current `updatedAt` from a prior read) for optimistic-concurrency safety if the item may have been touched concurrently (e.g. an auto-triage session already picked it up).

This only works against an already-running local instance (`localhost:8543` is the live systemd-managed service in stapler-squad — see `docs/how-to/manage-systemd-service.md`). Do not start or restart the service just to make this call.

## Reference

- RPCs: `BacklogService.ImportGitHubIssue`, `ListBacklogItems`, `UpdateBacklogItem` — `server/services/backlog_service_sync.go`, proto in `proto/session/v1/backlog.proto`
- MCP wrapper + session-scoping guard: `server/mcp/tools_backlog.go`
- `project-coordinator` agent: AIC (ATOMIC-INVEST-CONTEXT) ticket framework
- Product-manager framing (Kano/RICE/Cagan/Torres/Singer): `~/.claude/skills/product-manager.md` or the `pm-product-management` skill
