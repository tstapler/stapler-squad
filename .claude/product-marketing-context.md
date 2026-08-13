# Product Marketing Context

```
type: open-source
name: Seneschal
tagline: Your agentic estate, managed.
positioning: tmux for the Agentic Age
```

---

## 1. Project Overview

**Name**: Seneschal

**One-liner**: The seneschal of your AI coding agents — session management, approval gates, and real-time oversight from a single browser tab.

**What it does**: Seneschal is a locally-run web dashboard that lets developers run multiple AI coding agents simultaneously — Claude Code, Aider, Gemini CLI, Codex — in isolated tmux sessions with git worktrees. Every agent gets a live terminal stream and diff view. A structured review queue and 42+ auto-approval rules handle routine decisions; the rest surface as petitions for you to assent or deny. You are the lord. Seneschal manages your estate.

**Origin story**: Forked from [claude-squad](https://github.com/smtg-ai/claude-squad), a TUI for managing multiple Claude Code sessions. A browser is a better interface for this job — it opens instantly in multiple windows, works over SSH without port-forwarding a terminal, and lets you monitor agents from any device on the network.

**Category**: AI agent management / developer productivity tooling
**Project type**: CLI that starts a web server — `brew install seneschal` → `localhost:8543`
**Stack**: Go + ConnectRPC backend, React 19 + Next.js frontend, xterm.js terminals, SQLite

---

## 2. The Estate Metaphor

The medieval seneschal managed the lord's estate: overseeing workers, controlling what left the grounds, adjudicating disputes, and handling routine affairs without troubling the lord for every small decision.

In the agentic age, your AI agents are your holdings. Seneschal manages them on your behalf.

| Estate concept | Product concept |
|----------------|-----------------|
| Holdings | Agent sessions |
| Plots under cultivation | Git worktrees (isolated per agent) |
| Petitions to the court | Pending approvals — agent actions awaiting human assent |
| Standing orders | Auto-approval rules (42 built-in; what Seneschal handles without asking) |
| The court | Review queue — where petitions are heard and decided |
| Estate view | Dashboard — all holdings visible at a glance |
| Fallowing a plot | Session hibernation — checkpoint and suspend |
| Outstanding obligations | Unfinished work — uncommitted changes, commits ahead of main |
| The lord | You |

Use this vocabulary selectively in marketing copy and onboarding. Don't force it into technical documentation.

---

## 3. Audience

**Primary users**: Software engineers who run AI coding assistants daily and want to parallelize their work — running 3–5 agents across separate branches simultaneously without losing track of any of them.

**Secondary users**: Technical leads who want visibility into agent activity across a project — what's running, what's waiting for a decision, what's diverged.

**Contributors**: Go/React/TypeScript developers interested in AI tooling, terminal emulation, real-time streaming, and developer productivity infrastructure.

**Not for**: Developers who only run one agent at a time; teams with strict policies blocking localhost web servers; non-technical users expecting a hosted SaaS.

---

## 4. Problem & Differentiation

**The frustration**: Running AI agents in parallel means juggling multiple terminal windows, missing approval prompts, not knowing which agent needs attention, and git branches stepping on each other.

**What people use instead**: claude-squad (TUI), raw terminal windows, ad-hoc tmux configs. They fall short because TUIs don't work well across SSH or multiple monitors, and raw terminals have no organization, approval tooling, or cross-session visibility.

**Core design bet**: A browser is a better interface for agent oversight than a terminal. It opens instantly in multiple windows, works over SSH with just port forwarding, and is accessible from any device on the network.

**Word-of-mouth pitch**: "It's tmux for the agentic age — but in a browser, with an approval queue and diff viewer built in."

**The differentiator that matters most**: The approval gate. Seneschal is the only tool that gives you a structured, auditable, rule-driven review layer between your agents and your codebase. The court where petitions are heard.

---

## 5. Brand Voice & Tone

**Personality**: Pragmatic, technically precise, quietly intellectual, unpretentious. The kind of tool that doesn't explain itself more than once.

**Technical depth**: Expert-first. Assumes you know what tmux, git worktrees, and AI coding agents are. No hand-holding.

**Writing style**: Terse and declarative. Short sentences. Direct value statements. The estate metaphor enriches without replacing precision.

**Use**: "sessions", "holdings", "petitions", "approval rules", "worktrees", "review queue", "standing orders", "assent", "real-time", "isolated"

**Avoid**: "AI-powered", "revolutionize", "effortlessly", "seamlessly", "unlock", "supercharge", "cutting-edge", "empower"

**Voice examples**:
- *"Your agentic estate, managed."*
- *"Five agents running. Three petitions pending. One browser tab."*
- *"The seneschal handles the routine. You handle the rest."*
- *"Standing orders cover the safe operations. Everything else comes to you."*

---

## 6. Visual Direction

**Color mood**: Dark + technical. Deep slate backgrounds, indigo primary, VS Code-style terminal surfaces. Status indicators in green/amber/red. The palette of a tool built for focus, not decoration.

**Color palette**:
- Background: `#0f1117`
- Primary: `#6366f1` (hover: `#818cf8`, active: `#4f46e5`)
- Text: `#e2e8f0` / `#94a3b8` (muted)
- Success: `#22c55e` | Warning: `#f59e0b` | Error: `#ef4444`
- Terminal surface: `#1e1e1e` / `#252526`

**Typography**: Monospace-forward for terminal and code content; geometric sans for UI chrome. Dense, information-rich layouts — every pixel earns its place.

**Aesthetic references**: Linear's precision meets VS Code's dark theme meets a medieval estate ledger — structured, authoritative, nothing wasted.

**Logo direction**: Combination mark. The word "Seneschal" in a geometric sans with a small icon — possibilities include a stylized key (the seneschal held the keys to the estate), a wax seal motif, or a simple grid of nodes representing parallel sessions under one authority. Avoid anything that reads as a robot, brain, or sparkle.

---

## 7. Adoption Goals

**Primary metric**: GitHub stars → installs → active daily users

**Discovery path**: Word of mouth in AI developer communities (Twitter/X, Hacker News, r/LocalLLaMA, Claude Discord, Aider community), GitHub search for "claude-code manager", blog posts from power users running parallel agent workflows.

**Trust signals**: Active commit history, forked lineage from claude-squad, MIT license, real demo GIF in README, single-binary install, 42 built-in approval rules that work on day one.

**Adoption barrier**: "I already have terminals open. Do I really need a web UI?" — overcome by showing the approval queue and parallel visibility as genuinely new capability, not just a prettier terminal.

**Aha moment**: The first time an agent petitions for approval and you catch it immediately from the dashboard — without switching windows, without losing context on the four other agents running in parallel.

---

## 8. Key Messages

**Headline**: Your agentic estate, managed.

**Supporting points**:
1. Run five agents in parallel without losing track of any — every holding visible in one court.
2. Standing orders handle the routine. The review queue surfaces only the petitions that need you.
3. Isolated worktrees mean each agent works its own plot — no branch conflicts, no dirty state bleed.

**CTA**: `brew install seneschal` → open `localhost:8543` → start your first parallel session.

---

## 9. GitHub Presence

**README purpose**: Quick start (install → run → first session) with demo GIF up top. Convince in 60 seconds, then provide depth. Not a reference manual.

**Social proof**: Stars, install count, forked from claude-squad (credibility through lineage).

**Contribution posture**: Welcoming — PRs and bug reports encouraged. Good first issues labeled.

**Suggested GitHub topics**: `ai-agents`, `claude-code`, `aider`, `developer-tools`, `tmux`, `agent-management`, `git-worktrees`, `approval-workflows`

---

## 10. Name Vetting Record

Vetted via `pm-name-vetting` skill on 2026-05-21.

| Name | Status | Notes |
|------|--------|-------|
| **Seneschal** | ✅ Clear | Four minor repos, none significant, none in our space |
| **Tribune** | ✅ Clear | Tribune Publishing has repos; no dev tool conflict |
| **Ganglion** | ✅ Clear | Only OpenBCI hardware; different domain |
| Epoch | ⚠️ Ambiguous | One small AI interface project |
| Arbiter | ⚠️ Ambiguous | Multi-agent simulation framework, different niche |
| Thalamus | ⚠️ Ambiguous | Neuroscience + AI serving, different domains |
| Limen | ⚠️ Ambiguous | VPN/proxy platform, different space |
| Vigil | ❌ Conflicted | Multiple active AI agent projects |
| Meridian | ❌ Conflicted | Two Claude Code-adjacent projects |
| Synapse | ❌ Conflicted | Matrix homeserver (tens of thousands of stars) |
| Membrane | ❌ Conflicted | Elixir multimedia framework by Software Mansion |
| Relay | ❌ Conflicted | Facebook's GraphQL framework |
| Seam | ❌ Conflicted | Active smart building API company |
| Plexus | ❌ Conflicted | AI API gateway + Maven ecosystem |
| Quorum | ❌ Conflicted | ConsenSys blockchain project |
| Mentat | ❌ Conflicted | AbanteAI's AI coding assistant |
| Conduit | ❌ Conflicted | **Literally competing product** — GitHub's workspace manager for coding agents |
| Giskard | ❌ Conflicted | Active AI testing platform |
| Weave | ❌ Conflicted | W&B toolkit + multi-agent orchestration |
| Argus | ❌ Conflicted | Multiple monitoring tools; one named "Claude Code Agent Monitoring" |
| Panoptes | ❌ Conflicted | Yahoo network telemetry + workflow monitoring tool |

---

## 11. Skills That Read This File

| Skill | What it uses |
|-------|-------------|
| `pm-brand-guidelines` | Voice, visual direction, color palette |
| `ui-frontend-design` | Aesthetic references, color mood, typography |
| `ui-logo-designer` | Logo direction, personality, visual references |
| `pm-name-vetting` | `name_candidates_vetted` section |
