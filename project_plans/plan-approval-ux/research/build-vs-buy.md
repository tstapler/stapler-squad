# Build vs. Buy: Plan Approval UX

**Agent**: 6 (Build vs. Buy) | **Phase**: 2 — Research

Research question: for the three sub-features (markdown rendering, line/section-level
commenting, approve/reject-with-reason workflow), what should be built from scratch vs.
sourced from an existing solution?

## Summary verdict

| Piece | Verdict | Source |
|---|---|---|
| Markdown rendering | **Recommended — reuse existing dependency** | `react-markdown` + `remark-gfm`, already installed and already used in this exact section of the UI |
| Line/section-level commenting | **Recommended — build custom, minimal** | No existing library fits; hand-rolled anchor-based commenting is simpler than any general-purpose library |
| SaaS/managed commenting service | **Not recommended** | Solo local-only tool; no justification for hosted infra |
| Reject-with-reason workflow state | **Recommended — build custom** | Trivial state + RPC, mirrors existing `TriggerTriageRequest.feedback` pattern |

---

## 1. Existing OSS library or framework

### Markdown rendering — `react-markdown` + `remark-gfm`

**Already a dependency** in `web-app/package.json`:
- `react-markdown@^10.1.0` (MIT license)
- `remark-gfm@^4.0.1` (MIT license, adds GFM tables/strikethrough/task lists/autolinks)
- `dompurify@^3.4.3` (MPL-2.0/Apache-2.0, available if raw-HTML sanitization is ever needed)

**Already used in the exact code path this feature touches.** `DescriptionSection.tsx`
(`web-app/src/components/backlog/detail/DescriptionSection.tsx`), a sibling component to
`PlanArtifactsSection.tsx` in the same `backlog/detail/` directory, renders markdown today:

```tsx
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
...
<div className={markdownStyles.markdownBody} data-testid="backlog-description-rendered">
  <ReactMarkdown remarkPlugins={[remarkGfm]}>{item.description}</ReactMarkdown>
</div>
```

It even imports a shared stylesheet, `../markdownBody.css` (vanilla-extract, per
`.claude/rules/css-architecture.md`), that's ready to reuse as-is for plan rendering.
Two other files import `react-markdown` the same way: `web-app/src/app/help/page.tsx` and
`web-app/src/components/backlog/BacklogItemForm.tsx`.

**Pros:**
- Zero new dependency — no license review, no bundle-size delta beyond what's already shipped
- Proven maturity: `react-markdown` is the de facto standard React markdown renderer (weekly
  downloads in the millions), actively maintained by the unified/remark ecosystem (`remarkjs`,
  same maintainers as `rehype`/`unified`)
- Handles the hard edge cases correctly out of the box: tables, nested lists, fenced code
  blocks, GFM autolinks, task-list checkboxes — exactly the constructs SDD plan/requirements
  docs use
- Sanitizes by not rendering raw HTML by default (react-markdown does not use
  `dangerouslySetInnerHTML` unless `rehype-raw` is explicitly added), which matters here since
  plan content is LLM-generated and technically "untrusted" input rendered in-app
- Already wired to a shared CSS module (`markdownBody.css`) that gives visual consistency
  across description, plan artifacts, and any future rendered-markdown surface

**Cons:**
- None specific to this project — this is already the chosen tool, used adjacent to the exact
  file (`PlanArtifactsSection.tsx`) this feature will modify

**Verdict: Recommended.** Not just "don't build a parser" — literally copy the existing
`DescriptionSection.tsx` pattern into `PlanArtifactsSection.tsx`. This directly satisfies the
constraint in `requirements.md`: *"no new dependencies unless justified in research."* No
justification is needed because nothing new is required.

### Line/section-level commenting — no fitting existing library

Evaluated categories:

- **Rich-text/CRDT collaborative editors** (e.g. TipTap + Yjs, Slate, ProseMirror-based
  comment plugins) — explicitly **out of scope** per `requirements.md`: *"Building a full
  generic rich-text/CRDT collaborative editor."* These are built for concurrent multi-user
  editing; this is a solo-developer, read-mostly review flow. Massive overkill: bundle size
  (100s of KB), new state-management paradigm, steep integration cost with the existing
  `react-markdown` read-only rendering approach.
- **Dedicated markdown-commenting libraries** (e.g. `markdown-it` plugins for inline
  annotation, `react-comment-editor` style packages) — searched; nothing mature and
  maintained exists as a standalone "attach a comment to a line/section of rendered markdown"
  library. What exists in this space is almost always bundled into a full collaborative editor
  (Notion-style, Google Docs-style) rather than offered as a composable primitive.
  Requirements explicitly rule out building/adopting that class of tool.
  License risk is also moot since there's no viable candidate to adopt.
- **GitHub's own PR-review commenting UI** is not a library — it's server-side diff/anchor
  logic tightly coupled to GitHub's data model (commit SHA + file path + diff hunk position),
  not portable as an npm package.

**What's actually needed** (per `requirements.md` §5, "Line-level feedback capability"): a
mechanism to attach free-text feedback to *a specific part* of the plan — not full
character-offset diffing, not concurrent multi-cursor editing, not persistent anchors across
arbitrary re-edits. Given `react-markdown` already renders each block as identifiable React
nodes, the simplest correct primitive is **anchor comments at heading/paragraph granularity**:
each rendered block (heading, paragraph, list item, code block) gets a stable index or
slugified-heading key, and a comment is `{ anchorKey: string, text: string }` stored
server-side. This is well within "trivial custom code" territory — a `rehype`/`remark`
visitor to assign per-block keys (a few dozen lines) plus a small React state layer for the
comment UI (hover affordance + textarea), no new dependency required beyond what's already
installed.

**Pros of building custom, minimal:**
- Right-sized to the actual requirement (heading/paragraph-level, not char-level)
- Uses only the two already-installed unified-ecosystem tools (`react-markdown`,
  `remark-gfm`) which already expose the AST needed to assign stable anchor keys
- Full control over how anchors survive plan regeneration (the Pitfalls research dimension
  flags this: "what happens to line-referenced comments when the plan document is
  regenerated and line numbers shift" — heading/section-based anchors are far more robust to
  regeneration than raw line numbers, since headings are more likely to survive a
  content-preserving edit than exact line positions)
- No license, maintenance, or supply-chain surface added

**Cons:**
- Requires original design work (block-key assignment scheme, comment-to-anchor storage
  schema) rather than dropping in a library — but this is inherent complexity that a
  general-purpose library would not remove; it would only add framework overhead on top

**Verdict: Recommended — build custom, minimal.** This is the one place where "build" beats
"buy" outright: no existing library targets this narrow use case, and the narrow scope
(anchor-to-block, not character-diff, not concurrent-editing) makes hand-rolling
straightforward rather than risky.

## 2. SaaS/managed API

Evaluated: hosted commenting/review-as-a-service platforms (e.g. Hypothesis-style web
annotation APIs, CommentBox, Disqus-style embeds, or a hosted review tool like
Reviewable/CodeStream-as-a-service pattern).

**Not recommended**, for reasons intrinsic to this project's shape, confirmed by
`requirements.md`'s own framing:
- **Single-user, local-first tool.** `requirements.md` "Stakeholders" section: *"Primary:
  Tyler (solo developer), sole user/reviewer of backlog-driven plans."* Hosted
  commenting/review infra exists to solve multi-party, cross-device, notification-driven
  collaboration — none of which applies to a solo developer reviewing their own AI-generated
  plans in one browser session.
- **Adds a network dependency for a local tool.** `stapler-squad` runs entirely on
  `localhost:8543` against local git worktrees and a local ent/SQLite-backed store. A hosted
  API would introduce an external network call (latency, an outage dependency, and an
  internet-connectivity requirement) into a workflow that today works fully offline.
  It would also mean plan content — which may include repo-internal code excerpts, file
  paths, or architecture details — leaves the local machine for no functional gain.
  Additionally, hosted comment services are subscription-priced per-seat/per-project; here
  the entire "team" is one user and zero cost, zero latency, zero external dependency is
  strictly better.
- **No integration path exists.** The reject-with-reason feedback needs to flow back into
  `TriggerTriageRequest.feedback`/an eventual `RejectPlan` RPC and drive the *next*
  `sdd:2-research`/`sdd:3-plan` invocation — that's tightly coupled backend orchestration
  logic. A hosted comment API would only ever hold the raw comment text; the actual
  plumbing into the SDD pipeline still has to be built in this Go/ConnectRPC backend
  regardless, so buying a SaaS layer wouldn't remove any of the real engineering work — it
  would just add an extra system to keep in sync.

**Verdict: Not recommended.**

## 3. LLM-generated implementation vs. battle-tested library

- **Markdown parsing/rendering**: strongly prefer the battle-tested library — already the
  chosen path (see §1). Markdown grammar has many non-obvious edge cases (loose vs. tight
  lists, nested fenced code blocks with different fence lengths, reference-style links,
  GFM table alignment syntax, HTML-in-markdown escaping) that a hand-rolled or
  LLM-generated parser would almost certainly get wrong in some corner the SDD pipeline's
  own generated docs happen to hit (e.g. a `plan.md` with a nested code fence inside a
  numbered list, which is common in this project's own plan documents). `react-markdown`
  is the correct choice and is already in use one file away.
- **Reject-with-reason workflow state**: appropriate for custom code — this is a state
  field (`planApproved` → add e.g. a `planReviewStatus` enum or a `planRejectionReason`
  string) plus a new RPC (`RejectPlan`/`RequestPlanChanges`) that mirrors the existing,
  already-shipped `TriggerTriageRequest.feedback` field pattern
  (`proto/session/v1/backlog.proto`). There is no algorithmic complexity here — it's
  plumbing a string through an existing pattern the codebase already validated for the
  analogous triage-feedback case. No parsing, no edge cases, no library could meaningfully
  reduce risk here; a library would in fact add indirection for something this simple.

**Verdict**: markdown parsing → library (non-negotiable); reject-with-reason state →
custom code (correct default, nothing to buy).

## 4. Fork or adapt — existing patterns in this codebase to reuse

Confirmed by direct inspection (not assumption):

- **`DescriptionSection.tsx`** (`web-app/src/components/backlog/detail/DescriptionSection.tsx`)
  is the direct template to adapt for `PlanArtifactsSection.tsx`: same `ReactMarkdown` +
  `remarkGfm` + shared `markdownBody.css` pattern, same `CollapsibleSection` wrapper, same
  directory. `PlanArtifactsSection.tsx` currently renders only
  `<code>{item.planArtifactsPath}</code>` as inert text (confirmed by reading the file) — the
  fix is to fetch/render the plan file's markdown content the same way `DescriptionSection`
  renders `item.description`, not to invent a new rendering approach.
- **`markdownBody.css`** (`web-app/src/components/backlog/markdownBody.css.ts`) is a shared
  vanilla-extract stylesheet already used by two of the three `react-markdown` call sites —
  reuse it directly rather than writing new markdown-block CSS, keeping this feature visually
  consistent with the description rendering and complying with
  `.claude/rules/css-architecture.md`'s token-reuse requirement.
- **`TriggerTriageRequest.feedback`** (`proto/session/v1/backlog.proto`) is the pattern to
  fork for reject-with-reason: same shape (`item_id` + free-text `feedback`), same intended
  consumption point (feeds the next pipeline-phase invocation). `requirements.md` explicitly
  calls this out as the precedent to extend rather than reinvent.
- **No existing commenting/annotation UI** was found anywhere in `web-app/src` (searched for
  `annotat`, inline-comment, `CommentThread`, line-comment patterns — no matches outside
  generated protobuf/catalog files unrelated to this feature). Confirms §1's conclusion: the
  line-level commenting piece has no in-repo prior art to fork and must be designed fresh
  (as scoped-down custom code, not a new dependency).

**Verdict: Adapt, don't rebuild.** `DescriptionSection.tsx` + `markdownBody.css` +
`TriggerTriageRequest.feedback` together cover ~2 of the 3 sub-features almost entirely via
in-repo pattern reuse. Only the line-level comment anchor mechanism is genuinely new, and its
scope is intentionally small (see §1).
