# UX Research: session-classifier-pipeline (automatic session tagging)

Scope: UX for sync (regex/dependency) + async (LLM-fallback) auto-tagging, surfaced in the existing
web UI (`web-app/`), building on the manual tag system in `docs/reference/tag-organization.md` and
the existing rules UI in `web-app/src/components/sessions/ApprovalRulesPanel.tsx` /
`SuggestedRuleCard.tsx`.

## 1. Comparable UX patterns

| Product | Mechanism | How auto vs. manual is distinguished |
|---|---|---|
| **GitHub label bots** (label-bot, Issue-Label Bot, auto-labeler) | Bot applies a label via API/Action | **Not visually distinguished at all** — GitHub's label chip has no built-in provenance affordance. Bots compensate with a timeline comment ("I have applied labels matching..."), i.e. provenance lives in the *activity log*, not the label itself. [Auto label action](https://github.com/marketplace/actions/auto-label), [Issue-Label Bot](https://github.com/machine-learning-apps/Issue-Label-Bot) |
| **Gmail filter-applied labels** | User-authored filter rule applies a label silently on mail arrival | No visual marker on the label chip either; provenance is discoverable only by opening Settings → Filters and finding which rule references that label. This is a known usability gap (users can't tell "why is this labeled X" from the inbox view) — worth avoiding, not emulating. |
| **Linear Triage Intelligence** (2025 "auto-apply triage suggestions") | AI-suggested triage properties (label, team, assignee, priority), auto-applied per opt-in rule | **Best precedent found.** Auto-applied properties are "clearly marked in the suggestions header, and you can hover over them to review the reasoning or make changes." Users opt in per-property/per-value (e.g. "always auto-apply the `bug` label, but not others"), and every applied label already supports a hover tooltip showing its description. [Linear changelog](https://linear.app/changelog/2025-09-19-auto-apply-triage-suggestions), [How we built Triage Intelligence](https://linear.app/now/how-we-built-triage-intelligence) |
| **Zendesk auto-tagging** | ML-based ticket tagging | Auto-tags render identically to manual tags but are listed as their own category in the tag-management admin view, and macros can filter specifically on "system-generated" tags — provenance is a backend attribute exposed in admin/reporting surfaces, not the ticket view. |

**Convergent finding:** none of these products give auto-applied tags a *permanently different visual
treatment* in the primary list view (no different color/border/icon baked into the chip itself) — that
would create visual noise once auto-tags are common, and it fights the "tags are just tags" mental
model this repo already established (docs/reference/tag-organization.md's blue pills are undifferentiated
by design). The convention that repeats is **provenance on demand**: same-looking chip, but a
hover/click affordance (tooltip, popover, or expandable detail) reveals *why* it's there — mirroring
this repo's own precedent in `ApprovalRulesPanel.tsx` (`title="Built-in rules cannot be disabled"` on a
`builtInBadge`, and `title={...}` tooltips on priority/hit-count columns).

**Recommendation:** Don't invent a new "robot" icon or distinct pill color for auto-tags. Instead:
- Keep the existing `tag` pill styling (`SessionCard.tsx`'s `role="list"`/`role="listitem"` tags).
- Add a small, non-intrusive provenance indicator *only* on hover/focus (a `title` attribute for a
  cheap first pass, or a proper popover if richer content — rule name, matched pattern, LLM
  confidence — is wanted later). This matches the "no permanent visual tax, information on demand"
  pattern Linear and this repo's own rules panel both converged on independently.
- Reserve a genuinely distinct visual state for the **`Unclassified` fallback tag** specifically (see
  §4) — that one *should* look different, because it signals "this needs your attention," unlike a
  successfully-applied rule/LLM tag which needs no attention.

## 2. User mental model (extending the existing manual-tagging UX)

Per `docs/reference/tag-organization.md`, users' current mental model is simple: tags are things *I*
typed into the Tag Editor Modal (Enter to add, × to remove), and grouping/filtering is multi-membership
(a session with 3 tags shows up in 3 groups). Once auto-tagging ships, this model needs to survive two
changes without becoming confusing:

- **A tag appearing that the user didn't type.** Expectation: users will assume something is broken or
  ask "did I add this?" unless there's a lightweight, discoverable "why" affordance (§1's hover
  provenance). This is the single biggest UX risk in the project — silent auto-tags erode trust in the
  tag system generally (the exact failure mode Gmail filters have).
- **Editing/removing an auto-applied tag.** Users will expect the same `×` removal affordance they
  already have (consistency), but removal semantics differ from manual tags: a sync rule that still
  matches will just re-apply the tag on the next mutation/fixpoint pass, silently undoing the user's
  removal. This needs either (a) explicit UI feedback ("this tag may reappear — it's rule-driven") at
  the moment of removal, or (b) a **pin/suppress** mechanism (per-session tag exclusion list, checked
  before re-applying) so a user's explicit removal sticks. Given the project's `Out of Scope` already
  excludes a full rules-page redesign, the minimal version is: on removing a tag that has known rule
  provenance, show a short inline confirmation ("This tag is applied by rule '{name}' and may
  reappear — remove anyway?" / "Don't re-apply this tag to this session") rather than silently allowing
  a removal that gets undone on the next mutation with no explanation. Silently reverting a user's
  explicit action is worse than not offering removal at all.
- **Grouping-mode interaction.** The existing "Group by Tag" mode (multi-membership) needs no change —
  auto-tags are still just `Tags []string` entries — but once auto-tagging exists, "Group by Tag" views
  will suddenly gain many more populated groups than before. No new mode is needed; this is a volume
  change, not a structural one.

## 3. Accessibility (ARIA/keyboard) — provenance affordance and rules CRUD

Per `.claude/skills/e2e-test-conventions/SKILL.md` (this repo's hard, CI-enforced convention): all e2e
locators must use `data-testid` or ARIA roles — never CSS classes — so any new UI here must be built
with accessible roles/labels from the start, not retrofitted.

**"Why was this tag applied" affordance:**
- A first-pass `title` attribute (native browser tooltip) is keyboard-accessible for free (focus + the
  browser's native tooltip-on-focus behavior) and needs no ARIA work — this is what
  `ApprovalRulesPanel.tsx` already does for `builtInBadge` and priority/hit-count columns. Cheapest
  correct option given the project's explicit non-goal of a full rules-page redesign.
- If a richer popover is built instead (rule name + matched field + pattern, or LLM confidence/model),
  it must follow WAI-ARIA disclosure-pattern conventions: the trigger element needs
  `aria-expanded`/`aria-controls`, the popover content needs `role="dialog"` or `role="tooltip"`
  depending on whether it's purely informational (tooltip semantics, dismiss on blur/Escape) or
  interactive (dialog semantics, focus-trapped), and the trigger must be reachable via Tab and
  activatable via Enter/Space — a hover-only affordance with no focus/keyboard path fails WCAG 2.1.1
  and is untestable by this repo's ARIA-only e2e locator rule (Playwright's `hover()` works, but a
  keyboard-only e2e path needs a focusable trigger regardless).
- Each auto-applied tag pill should carry a distinguishing `aria-label` even in the simple version —
  e.g. `aria-label="Tag: Frontend (auto-applied by rule)"` vs. the existing `aria-label="Session tags"`
  list wrapper — so screen reader users get the provenance distinction that sighted users get from the
  hover affordance, rather than losing it entirely.

**Rules-editing CRUD UI (tagging-rule management):**
The project's `In Scope` explicitly says to integrate with, not duplicate, the existing rules surface
(`ApprovalRulesPanel.tsx`/`RuleBuilderForm.tsx`). That surface already has the accessibility scaffolding
to extend:
- Source filter tabs (`sourceFilter`, e.g. `all`/`user`/`seed`) — reusable pattern for a `tagging` source
  tab if tagging rules share the same list view, or a parallel tab set if they get a separate panel.
- Existing per-row `aria-label`s (`` `Edit rule ${rule.name}` ``, `` `Delete rule ${rule.name}` ``,
  `` `${rule.enabled ? "Disable" : "Enable"} rule` ``) — the same naming convention should extend
  directly to tagging rules (`` `Edit tagging rule ${rule.name}` ``, etc.) so existing e2e page helpers
  and conventions transfer without inventing new ARIA-label phrasing.
- The rule builder's form is already keyboard-navigable (native `<input>`/`<select>`/`<textarea>` per
  `fieldInput`/`fieldSelect`/`fieldTextarea` in `RuleBuilderForm.tsx`) — a new "tag name" field or
  "match against: name/branch/path/program" selector should follow the same field-row pattern rather
  than a custom control.

## 4. Error / edge-case UX

**LLM fallback failure → `Unclassified` tag.** This must be visually distinct from a successful
classification, for the same reason Linear marks auto-applied *suggestions* distinctly from confirmed
ones: an `Unclassified` tag is not information about the session, it's a **system signal that
classification didn't happen** — treating it as an ordinary tag would let it silently pollute "Group by
Tag" views (a large, meaningless "Unclassified" bucket) and make users think a rule fired when none did.
Recommendation:
- Render `Unclassified` with a visually distinct style (e.g. a muted/dashed-border pill, or a small
  warning glyph) rather than the standard blue pill — this is the one case in this project that
  *should* break from "auto-tags look like manual tags," because its entire purpose is to flag "no
  answer available," not to classify.
- Tooltip/title text should say why: e.g. `title="LLM classification failed or timed out — will retry
  next poll cycle"` — actionable, not just a dead-end label, and tells the user this is transient
  rather than a permanent verdict.
- `Unclassified` should not itself be user-removable in the same "will just reappear" way as a
  rule-driven tag (§2) — it should simply disappear once a later poll cycle succeeds, so no removal UI
  is needed for it at all; removing it manually would be immediately undone on the next successful
  classification anyway (harmless, but pointless — recommend suppressing the remove `×` on this
  specific tag value to avoid a confusing no-op interaction).

**Rules-editing UI: invalid regex.** The rule builder form should validate the regex client-side before
save (e.g. `new RegExp(pattern)` in a try/catch) and show inline field-level error text next to the
pattern input — consistent with the existing `saveError` state pattern already present in
`SuggestedRuleCard.tsx` (`const [saveError, setSaveError] = useState<string | null>(null)`) — rather
than only surfacing a server-side rejection after submit. This also needs an accessible error
association: `aria-invalid="true"` on the pattern field plus `aria-describedby` pointing at the error
message element, so screen reader users get the same validation feedback sighted users see inline.

**Tag-dependency rule that never fires (dependency tag never appears).** This is a silent-dead-rule
problem, not a crash — nothing will ever error, the rule just never contributes a tag. Two UX
options, not mutually exclusive:
1. **Passive surfacing in the rules list**: an analytics-style column already exists for approval rules
   (`title="Number of times this rule fired in the last 7 days"` per `ApprovalRulesPanel.tsx`) — the
   same "fire count" column extended to tagging rules would let a user notice a rule with a permanent
   zero without any new UI concept.
2. **Active surfacing at authoring time**: when creating/editing a tag-dependency rule, if the
   dependency tag name doesn't match any other rule's *output* tag (a simple string-set check against
   currently configured rules), show a non-blocking warning inline ("No rule currently produces the tag
   '{X}' — this rule may never fire") — a linting hint, not a save-blocking validation, since the
   dependency tag could legitimately come from manual tagging instead of another rule.
   Given the project's `Out of Scope` excludes a full rules-page redesign, option 1 (reuse the existing
   fire-count column) is the pragmatic default; option 2 is a reasonable stretch/fast-follow if time
   allows in Phase 3 planning.

## 5. Jobs-to-be-done

stapler-squad's purpose (per `README.md`) is running multiple concurrent AI coding agents
(Claude Code, Codex, Gemini, Aider) with a real-time dashboard, and its organization features exist
specifically so a user managing many simultaneous sessions can group/filter/scan them quickly. Applying
JTBD lens to auto-tagging:

- **Functional job**: "Let me tell at a glance what kind of work each of my N concurrent sessions is
  doing, without reading each one." Auto-tagging is a scaling mechanism — manual tagging works at 3
  sessions, breaks down at 15+ (the exact problem stated in `requirements.md`'s Problem Statement).
  The job is classification-as-triage: which sessions are bug fixes vs. refactors vs. test work, so the
  user can prioritize review order.
- **Emotional job**: "Reduce the anxiety of losing track of what's running." A user juggling many
  headless agent sessions is prone to the same fear as an overflowing inbox — auto-tagging (especially
  combined with grouping) converts an undifferentiated list into a scannable, categorized surface,
  reducing the cognitive load of "did I forget about that session." This is the same emotional job Gmail
  filters and Linear triage rules serve for their respective inboxes/backlogs — the auto-tag itself
  isn't the goal, the *feeling of an organized, non-overwhelming queue* is.
- **Social job**: less central here since stapler-squad is a personal/solo tool (per `README.md`'s
  "mission control" framing, single-user dashboard), but there's a secondary job for any
  multi-workspace/team-adjacent use: consistent, rule-driven tags make a session's category legible to
  *anyone else glancing at the dashboard* (e.g. reviewing a teammate's screen-share) without relying on
  the original author's personal tagging discipline — auto-tags are less prone to inconsistent naming
  than free-text manual tags typed by different people at different times.

## Sources

- [GitHub Marketplace: Auto label action](https://github.com/marketplace/actions/auto-label)
- [Issue-Label Bot (machine-learning-apps)](https://github.com/machine-learning-apps/Issue-Label-Bot)
- [Linear changelog: Auto-apply triage suggestions (2025-09-19)](https://linear.app/changelog/2025-09-19-auto-apply-triage-suggestions)
- [Linear: How we built Triage Intelligence](https://linear.app/now/how-we-built-triage-intelligence)
- [Linear Docs: Issue labels](https://linear.app/docs/labels)
- `docs/reference/tag-organization.md` (this repo)
- `web-app/src/components/sessions/ApprovalRulesPanel.tsx`, `SuggestedRuleCard.tsx`,
  `RuleBuilderForm.tsx`, `SessionCard.tsx` (this repo, read for existing provenance/badge/ARIA
  conventions)
- `.claude/skills/e2e-test-conventions/SKILL.md` (this repo)
- `README.md` (this repo, product framing)
