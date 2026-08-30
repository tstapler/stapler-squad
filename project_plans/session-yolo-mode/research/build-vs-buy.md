# Build vs. Buy: `auto_approve` Session Toggle

**Phase**: 2 — Research | **Feature**: `project_plans/session-yolo-mode/requirements.md`

## Summary

Every piece of this feature is small, framework-native plumbing or a first-party lookup table
with no non-trivial algorithm anywhere in it. No third-party library, SaaS, or codegen tool is
relevant. The only real decision at each layer is "which existing in-repo pattern to copy,"
not "build vs. buy" in the traditional sense — so each verdict below is really "hand-write
following precedent X" vs. "extend existing component Y."

---

## 1. Per-agent flag lookup table

**Question**: is there an existing "agent CLI flag registry" (open source or in-repo) to adopt,
or should this be a small first-party `map[string]string`?

### What exists today

- No general per-agent *flag* table exists. Agent-binary detection today is done ad hoc via
  `strings.Contains` checks on the program string, not a keyed lookup:
  - `session/claude_adapter.go:28` — `strings.Contains(strings.ToLower(program), "claude")`
  - `session/instance_program.go:14` — `strings.Contains(program, "claude") || strings.Contains(program, "agy") || strings.Contains(program, "antigravity")`
  - `session/claude_command_builder.go:20` — doc comment confirms the program string can be
    `"claude"`, `"claude --model sonnet"`, or `"aider"` — i.e., a free-form shell command, not
    a structured agent enum.
- There **is** an adjacent-but-different existing mechanism with a confusingly similar name:
  **`auto_yes`** (`session/ent/schema/session.go:46-47`, `field.Bool("auto_yes").Default(false)`;
  proto `proto/session/v1/types.proto:47`, `proto/session/v1/session.proto:497` and others;
  Go field `config/types.go:161`, `config/config.go:246`). `auto_yes` is consumed by
  `daemon/daemon.go:42-43` (`instance.SetAutoYes(true)`) and `session/instance_actor_setters.go:319-332`
  (`setAutoYesLocked`) — it's a **daemon-driven auto-respond-to-detected-prompts** mechanism
  (poll scrollback, auto-send a response when a known prompt pattern appears), not a
  CLI-flag-injection mechanism. It solves a related but distinct problem: `auto_yes` answers
  prompts as they appear; the new `auto_approve` prevents the CLI from asking in the first
  place via `--dangerously-skip-permissions`/`--yes`. **Naming risk**: `auto_approve` is close
  enough to `auto_yes` to confuse users/future maintainers reading session state fields side by
  side — worth a doc comment on the new field distinguishing the two, but not worth reusing or
  merging the mechanisms (they trigger at different layers: CLI startup flag vs. runtime tmux
  polling).
- No open-source "agent CLI flag registry" project exists for this problem space (this is
  inherently specific to which handful of agent CLIs stapler-squad launches) — not something
  a general search of the Go/npm ecosystem would surface, and not worth searching for further
  given the requirements doc already scopes this to exactly two agents (Claude Code, Aider).

### Pros / Cons / Verdict

**Build (first-party `map[string]string` or small switch, e.g. in `session/` alongside
`claude_command_builder.go`)**
- Pros: ~10 lines; matches the existing ad hoc detection style already used for program
  strings; zero new dependencies; trivially extensible for future agents (the requirements doc
  explicitly scopes "beyond Claude Code and Aider" as out-of-scope, so no need to
  over-generalize now).
- Cons: yet another place doing `strings.Contains` on the program string, duplicating logic
  already spread across `claude_adapter.go` and `instance_program.go` rather than
  consolidating agent detection into one place.
- Verdict: **Recommended.** Add the flag lookup near the existing program-detection helpers
  (or as a small function called from the same place `claude_command_builder.go`'s `Build()`
  already special-cases Claude vs. non-Claude commands), so it reads as an extension of the
  existing ad hoc pattern rather than a new abstraction layer per
  `.claude/rules/interface-pollution-checklist.md` (no interface needed for a 2-entry lookup).

**Buy/adopt existing registry**
- Pros: none identified.
- Cons: no such library exists for this narrow, repo-specific concern.
- Verdict: **Not recommended** — no candidate exists to evaluate.

---

## 2. Boolean-field-through-schema-layers plumbing (proto/ent/service/UI)

**Question**: does the repo have a generator/scaffolding target that automates adding a new
field across proto+ent+service+UI, or should this hand-follow the `autonomous_mode`/`one_off`
precedent?

### What exists today

- `Makefile` has codegen targets that **regenerate bindings from schema you've already
  edited by hand** — none of them scaffold a new field across layers:
  - `proto-gen` (`Makefile:398`) — regenerates Go + TS from `.proto` files after you edit them.
  - `ent-gen` (`Makefile:415`) — regenerates ent ORM code from `session/ent/schema/*.go` after
    you edit the schema (must use `--feature sql/upsert` per
    `.claude/rules/ent-schema-generation.md`, not the plain `ent-gen` invocation blindly).
  - `registry-generate` (`Makefile:109`, composed of `registry-generate-backend`:77,
    `registry-generate-frontend`:92, `registry-aggregate`:104) — scans source for `// +api:`/
    `// +feature:` markers and writes `docs/registry/features/*.json`; documents features
    *after* you've added markers, doesn't add the field itself.
  - No target scaffolds a new proto field + ent column + service switch case + frontend prop in
    one step.
- Full precedent trace for `autonomous_mode` (the closer analog per the requirements doc, since
  `one_off` needed a new `SessionType` while `autonomous_mode` — like the proposed
  `auto_approve` — reuses an existing session type):
  1. **Proto**: `proto/session/v1/session.proto:557` (`bool autonomous_mode = 23;` on
     `CreateSessionRequest`) and `:612` (`optional bool autonomous_mode = 10;` on the update
     request, `optional` so it's a tri-state "leave unchanged" vs. "set true/false").
  2. **Ent schema**: `session/ent/schema/session.go:48-50` —
     `field.Bool("autonomous_mode").Default(false).Comment("Crew autonomy mode — when true, the Fixer injects correction prompts without user confirmation.")`.
  3. **Go domain struct**: `session/instance.go:163-166` (`AutonomousMode bool` on the runtime
     instance) and `:529-531` (`AutonomousMode bool` on `CreateOptions`).
  4. **Service — path-guard exception**: `server/services/session_service.go:1262-1264` — the
     required-path guard explicitly excludes `!req.Msg.AutonomousMode` sessions.
  5. **Service — create wiring**: `server/services/session_service.go:1503` —
     `AutonomousMode: req.Msg.AutonomousMode,` passed into `CreateOptions`.
  6. **Service — update wiring**: `server/services/session_service.go:1802-1809` — `optional`
     proto field checked via `req.Msg.AutonomousMode != nil && *req.Msg.AutonomousMode != instance.AutonomousMode`
     then `instance.SetAutonomousMode(*req.Msg.AutonomousMode, "")`, gated by a runtime
     precondition (`headlessPool == nil` → error) specific to autonomous mode's own semantics.
  7. **Frontend checkbox**: `web-app/src/components/sessions/OmnibarCreationPanel.tsx` — four
     separate `<label className={checkboxClass}><input type="checkbox" .../></label>` blocks
     (lines ~382, ~466, ~558, ~595, ~803) for the different existing boolean toggles in that
     panel — this is the literal pattern a new `auto_approve` checkbox should copy line-for-line.
- This confirms the requirements doc's own claim: `autonomous_mode` is a complete, working,
  in-repo template for exactly this "flag on existing session type" shape, spanning proto →
  ent → Go struct → service (both create and update paths) → frontend checkbox.

### Pros / Cons / Verdict

**Build (hand-follow `autonomous_mode`, using the existing `proto-gen`/`ent-gen`/
`registry-generate` Makefile targets to regenerate bindings after each manual schema edit)**
- Pros: identical shape already proven in production code at all 7 touchpoints cited above;
  zero new tooling; the Makefile's regeneration targets already do the only mechanical part
  (binding generation) that *is* automatable.
- Cons: still manual, multi-file work — but that's true of the precedent too, and introducing
  a meta-scaffolding tool for a pattern used maybe twice a year is not justified.
- Verdict: **Recommended.** No new tooling needed; run `proto-gen` after editing
  `session.proto`, `ent-gen` (with `--feature sql/upsert`) after editing
  `session/ent/schema/session.go`, and `registry-generate` after adding the frontend
  marker/backend `// +api:` marker.

**Buy/build a cross-layer field-scaffolding generator**
- Pros: would save repetitive edits if this pattern recurred often.
- Cons: no such target exists in this repo today (confirmed by reading the full `Makefile`
  codegen section, lines 398-432 plus the registry targets 77-115); building one is
  disproportionate tooling investment for a boolean field added a few times a year, and would
  itself need maintenance/testing.
- Verdict: **Not recommended** for this feature.

---

## 3. UI toggle component

**Question**: does `web-app/src/components/` have a reusable Toggle/Checkbox/Switch primitive
to reuse instead of a new one-off `<input type="checkbox">`?

### What exists today

- No reusable `Toggle`/`Switch` component exists under `web-app/src/components/ui/` or
  elsewhere. `OmnibarCreationPanel.tsx` itself establishes the repo's actual convention: every
  existing boolean toggle in that panel (Create-a-new-git-repository, and at least three others
  at lines ~466, ~558, ~595, ~803) is a raw `<label className={checkboxClass}><input
  type="checkbox" checked={...} onChange={...} /><span>Label text</span></label>`, sharing one
  `checkboxClass` import (`OmnibarCreationPanel.tsx:14`, `checkbox as checkboxClass` from the
  panel's `.css.ts`). This is the panel's established local convention, not a one-off pattern
  invented per-checkbox — every boolean in this exact panel already looks like this.

### Pros / Cons / Verdict

**Build (reuse the existing inline `checkboxClass` + raw `<input type="checkbox">` pattern)**
- Pros: matches all four existing checkboxes in the same file exactly; zero new component
  surface; a reviewer scanning the diff sees a fifth instance of an already-repeated pattern,
  not a new abstraction to evaluate.
- Cons: none specific to this feature — the "cons" of raw checkboxes (no keyboard/ARIA
  polish beyond native `<input>` semantics) apply equally to the four existing ones and are out
  of scope to fix here.
- Verdict: **Recommended.** Do not introduce a new `Toggle`/`Switch` component for this
  feature — that would be inconsistent with every other boolean toggle in the exact same file
  and adds a component with a single call site, the "speculative interface" smell (in UI-component
  form) flagged by `.claude/rules/interface-pollution-checklist.md` smell #1.

**Buy/introduce a new shared Toggle/Switch primitive**
- Pros: would be a nicer long-term UI primitive if the repo starts wanting toggle-switch
  visuals (vs. checkboxes) broadly.
- Cons: no existing demand signal for it (zero current usages of a switch-style toggle
  anywhere searched); would be introduced for exactly one call site, i.e. a new abstraction
  with no second consumer — the opposite of what this codebase's own review checklist asks
  reviewers to catch.
- Verdict: **Not recommended** for this feature; a separate design-system task if/when a
  second real toggle-switch use case appears.

---

## 4. Badge component

**Question**: does `web-app/src/components/` have a reusable Badge/Pill/Tag component that a
new "⚡ Auto" badge should extend, versus a new bespoke one?

### What exists today

- A generic, reusable `Badge` component **does** exist: `web-app/src/components/ui/Badge.tsx`
  — `export function Badge({ intent, size, ...props }) { return <span className={badge({ intent, size })} {...props} />; }`,
  backed by a vanilla-extract `recipe` in `web-app/src/components/ui/Badge.css.ts:4-33` with
  `intent` variants (`default`, `success`, `warning`, `error`, `primary`) and a `size` variant.
- However, **session-card badges do not use it, by established local convention.** The
  directly analogous existing badge — `autonomousBadge`, shown for `session.autonomousMode` —
  is a bespoke local style in `web-app/src/components/sessions/SessionCard.css.ts:804-815`
  (`export const autonomousBadge = style({ display: "inline-flex", ..., background:
  vars.color.accentBg, ... borderRadius: vars.radii.full, ... })`), applied directly via
  `className={autonomousBadge}` at four call sites in `SessionCard.tsx` (lines 573, 583, 595,
  606) for different sub-states (active, done, stuck). `SessionCard.tsx` has an entire family
  of sibling badges built the same bespoke way (`workflowBadge` immediately follows
  `autonomousBadge` at `SessionCard.css.ts:817`), plus dedicated badge *components* elsewhere
  in `sessions/` (`StatusBadge.tsx`, `SourceBadge.tsx`, `CIStatusBadge.tsx`, etc.) that also
  don't route through `ui/Badge.tsx`.
- Net effect: `ui/Badge.tsx` exists and is reusable in principle, but session-card-specific
  badges have their own established local idiom (a `.css.ts` `style()` per badge, referencing
  the same `vars.*` design tokens `ui/Badge.tsx` also uses) that every sibling badge in this
  exact component already follows.

### Pros / Cons / Verdict

**Build a local `.css.ts` style (following `autonomousBadge`'s exact shape) rather than using `ui/Badge.tsx`**
- Pros: matches every existing badge in `SessionCard.tsx` (`autonomousBadge`, `workflowBadge`,
  and others) rather than introducing the one badge in that file that's structured differently;
  uses the same design tokens (`vars.color.*`, `vars.radii.full`, `vars.space`) either way, so
  there's no token-consistency cost to not using `ui/Badge.tsx`; trivially supports the
  active/click-to-disable interaction pattern `autonomousBadge` already has (button vs. span
  variants at `SessionCard.tsx:571-590`), which the "⚡ Auto" badge likely wants too (toggle
  after creation is a "Should Have" in the requirements doc).
  can copy directly.
- Cons: continues a pattern where `ui/Badge.tsx` exists but isn't the single source of truth
  for badge styling — mild inconsistency at the codebase level, though pre-existing and not
  something this feature introduces.
- Verdict: **Recommended.** Add an `autoApproveBadge` style to `SessionCard.css.ts` next to
  `autonomousBadge`/`workflowBadge`, following their exact shape, and render it in
  `SessionCard.tsx` next to the existing `session.autonomousMode &&` block. This is the
  "reuse existing pattern" choice, even though the literal component being reused is a sibling
  style block rather than `ui/Badge.tsx`.

**Use the generic `ui/Badge.tsx` component instead**
- Pros: it is the more "correct" reusable primitive in the abstract, and does support an
  `intent`/`size` variant API that could express "⚡ Auto" as e.g. `intent="warning"`.
- Cons: would make this one session-card badge structurally inconsistent with every other
  badge in the same file (`autonomousBadge`, `workflowBadge`, `StatusBadge`, `SourceBadge`,
  `CIStatusBadge` all bypass it); doesn't obviously support the click-to-toggle button variant
  `autonomousBadge` already implements without extra wrapper work.
- Verdict: **Viable but not recommended** — defensible in isolation, but breaks local
  consistency with zero offsetting benefit for a single badge instance.

---

## 5. LLM-generated-code correctness risk

There is no non-trivial algorithm anywhere in this feature — it is a boolean field threaded
through existing schema layers (proto/ent/service/UI, all framework-generated or
hand-mirrored-from-precedent) plus a 2-entry string lookup table for CLI flag selection. There
is no parsing, no concurrency, no numerical/statistical logic, and no security-sensitive
computation beyond "pass this string through if this bool is true" (the actual
permission-bypass *semantics* live entirely inside the external agent CLI binaries, which this
feature does not touch — explicitly out of scope per the requirements doc). Consequently the
usual "LLM-generated code vs. a battle-tested library" tradeoff does not apply: there is no
correctness risk here that a third-party library or SaaS dependency would reduce, and building
by hand — directly following the `autonomous_mode` precedent at every layer — is unambiguously
the correct choice.

---

## Overall Recommendation

| Piece | Verdict | Follow |
|---|---|---|
| Per-agent flag lookup | Build | New ~10-line lookup near `claude_command_builder.go`; note naming distinction from `auto_yes` |
| Schema-layer plumbing | Build (hand-follow precedent) | `autonomous_mode`'s 7 touchpoints (proto/session.proto:557,612 → ent/schema/session.go:48-50 → instance.go:163-166,529-531 → session_service.go:1262-1264,1503,1802-1809 → OmnibarCreationPanel.tsx checkbox pattern) |
| UI toggle | Build (reuse existing inline pattern) | `checkboxClass` + raw `<input type="checkbox">`, matching 4 existing checkboxes in `OmnibarCreationPanel.tsx` |
| Badge | Build (reuse existing sibling-badge pattern) | New `.css.ts` style next to `autonomousBadge`/`workflowBadge` in `SessionCard.css.ts:804-830`, not `ui/Badge.tsx` |
| Algorithm/library risk | N/A | No non-trivial logic exists in this feature |

No build-vs-buy tradeoff in the traditional sense exists anywhere in this feature — every
decision reduces to "which existing in-repo pattern to copy," and each has a direct, cited
precedent.
