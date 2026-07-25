# Agent Protocol Architecture — Comparative Research

## Origin

Follow-on to `project_plans/backlog-cross-platform-audit/`. That audit found several design
smells in stapler-squad's session-control layer:

- A bespoke, terminal-scraping session driver (`session/session_driver.go`) that detects
  readiness via regex on tmux pane content, types keystrokes, and polls every 2s — fragile by
  construction (see the historical fix commits for indented spinners, CR-overwritten spinners,
  asterism prefixes, etc.).
- Two parallel, inconsistent mechanisms for driving a session: the old tmux+`AutonomousDriver`
  path and the newer direct headless-pool call path (ADR-022), with no single abstraction.
- Backlog capabilities split inconsistently across raw RPC, MCP tools
  (`server/mcp/tools_backlog.go`), and UI — some exist in one layer but not the others (e.g.
  `AttachSessionToItem` is RPC-only with no UI; GitHub sync is RPC-stubbed with no UI at all).

The user wants a robust architecture that scales, and specifically asked to evaluate adopting
a standard **agent protocol** rather than continuing to grow bespoke plumbing. Two concrete
protocols were named by the user via links:

- Google's **A2A (Agent2Agent)** — https://github.com/a2aproject/A2A
- Zed's **ACP (Agent Client Protocol)** — flagged via
  https://github.com/Xuanwo/acp-claude-code/issues/64 (a community ACP↔Claude Code bridge)

Anthropic's own **MCP (Model Context Protocol)** is already partially adopted in this repo and
is a third natural candidate to evaluate more deeply rather than assume.

## Ask

Comparative research, not a commitment to any one protocol yet:

1. For each of MCP, A2A, and ACP: what problem does it actually solve, who controls/maintains
   it, how mature/adopted is it, and — critically — **what part of stapler-squad's current
   architecture would it replace or wrap**, concretely (name the files/subsystems).
2. Where the three overlap or conflict (e.g. can more than one coexist — MCP for tool exposure
   *and* ACP for session control?).
3. A synthesis with a recommendation: adopt, adopt-partially, or defer, per protocol — with the
   concrete first integration step if "adopt."

## Constraints / context to carry into research

- stapler-squad's actual pain point is controlling AI coding sessions (mostly Claude Code, tmux
  + git worktrees) reliably across machines — not inter-agent delegation to third-party
  services, unless A2A research reveals that's actually the more relevant framing.
- Any recommended change must be incremental — this is a live, shipped product with existing
  users (even if the backlog feature itself is flagged off by default).
- Do not assume the user's phrasing ("agent context protocol") maps 1:1 to any one of these;
  research each on its own merits.

## Deliverables

- `research/mcp.md`
- `research/a2a.md`
- `research/acp.md`
- `recommendation.md` (top-level synthesis)
