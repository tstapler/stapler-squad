# Requirements: Rules Page UX Redesign

## Problem Statement

The approval rules page (`/rules`) exposes a flat form with raw regex text fields. The backend classifier (`pkg/classifier/classifier.go`) has a rich `CommandCriteria` struct that enables precise, structured matching — Programs, Subcommands, BlockedSubcommands, RequiredFlags, ForbiddenFlags, PythonModes, SafePythonImportsOnly — but **none of this is accessible through the UI**.

Users wanting to create non-trivial rules (e.g. "allow Python only when running a .py script", "block git reset --hard", "escalate any npm publish") must understand Go regex and the internal classifier data model. This blocks mixed-technical teams from creating safe, precise rules.

## Goals

1. **Expose `CommandCriteria` fields** through a structured visual builder — no regex required for common cases
2. **Template library** — pre-built starting points for the most common rule patterns (Python, git, Node, Docker, etc.)
3. **Live rule preview** — show example commands that would and wouldn't match before saving
4. **Readable rule display** — replace raw regex chips in the table with plain-language summaries
5. **Non-destructive** — all existing rules (seed, claude-settings, user) remain fully functional
6. **Analytics-to-rule workflow** — "Add rule" actions in analytics pre-fill the rule builder with the observed program/subcommand/tool, closing the loop between "what am I seeing?" and "how do I handle it automatically?"
7. **Richer analytics display** — surface uncovered patterns more prominently with decision context (was the manual review ultimately allowed or denied? how often?) to guide rule decisions

## Non-Goals

- Changing the backend classifier logic
- Replacing regex support (raw regex stays available as a power-user escape hatch)
- Redesigning the analytics panel
- Mobile/responsive redesign

---

## User Stories

### US-1: Structured Bash rule builder

**As a** mixed-technical user  
**I want to** build rules for Bash commands by selecting programs, subcommands, and flags from structured inputs  
**So that** I don't need to write or understand regex

**Acceptance criteria:**
- AC-1.1: Rule builder has a "Bash command" mode that shows structured fields: Programs (tag input), Subcommands (tag input), BlockedSubcommands (tag input), RequiredFlags (tag input), ForbiddenFlags (tag input)
- AC-1.2: All structured fields accept multiple values via tag-style input (type value + Enter or comma to add)
- AC-1.3: Saving a structured rule stores it in a way the existing backend can evaluate (via CommandCriteria fields or equivalent)
- AC-1.4: Users can switch to "Advanced / regex" mode within the same builder to see/edit the raw patterns

### US-2: Python-specific rule mode

**As a** developer who runs Python in CI/CD  
**I want to** create rules that match specific Python invocation modes (script, module, inline, version)  
**So that** I can allow `python script.py` and `python -m pytest` while escalating `python -c "..."` with external imports

**Acceptance criteria:**
- AC-2.1: When Programs includes a Python interpreter (python, python3, pypy, etc.), a "Python mode" section appears with checkboxes for: `script` (.py file), `module` (-m), `inline` (-c), `version` (-V)
- AC-2.2: A toggle "Require safe stdlib imports only" appears when `inline` mode is selected
- AC-2.3: The builder shows a plain-language summary: "Match python3 running a script or module — NOT inline -c"
- AC-2.4: Seed rule `seed-allow-bash-python-run` is rendered with this UI (not raw regex chips)

### US-3: Template library

**As a** user setting up rules for the first time  
**I want to** start from a curated template rather than a blank form  
**So that** common patterns are easy and I don't accidentally miss edge cases

**Acceptance criteria:**
- AC-3.1: A "Start from template" button opens a template picker showing at minimum: Python script/module, Python inline stdlib, Git read-only, Git write, npm/yarn, Docker read, Docker write, Escalate unknown program
- AC-3.2: Selecting a template pre-fills all form fields including structured criteria
- AC-3.3: User can edit any pre-filled field before saving
- AC-3.4: Templates have brief plain-language descriptions of what they match

### US-4: Live rule preview

**As a** user building a complex rule  
**I want to** see example commands that would and wouldn't match before saving  
**So that** I can confirm my rule is correct without deploying it

**Acceptance criteria:**
- AC-4.1: Rule builder shows a "Preview matches" section that updates as fields change
- AC-4.2: Preview lists 3–5 example commands that WOULD match the current criteria
- AC-4.3: Preview lists 2–3 example commands that would NOT match (boundary cases)
- AC-4.4: Preview updates within 200ms of a field change (debounced)
- AC-4.5: Preview is generated client-side for instant feedback (no round-trip required)

### US-5: Plain-language rule display in the table

**As a** user reviewing existing rules  
**I want to** understand what each rule matches without reading raw regex  
**So that** I can quickly audit the rule set

**Acceptance criteria:**
- AC-5.1: In the rules table, structured criteria rules show a human-readable "Match" description (e.g. "git push with any flag" instead of raw regex chips)
- AC-5.2: Seed rules that use `CommandCriteria` are displayed with their structured fields broken out, not as opaque patterns
- AC-5.3: Rules using raw regex still show the regex, clearly labeled as "Pattern: `<regex>`"
- AC-5.4: Decision badges (Auto-Allow, Auto-Deny, Escalate) and risk levels are shown prominently

### US-6: Edit existing user rules

**As a** user managing rules I created earlier  
**I want to** edit a user rule inline  
**So that** I don't have to delete and recreate it

**Acceptance criteria:**
- AC-6.1: User rules have an "Edit" button that opens the same structured builder pre-filled with existing values
- AC-6.2: Save updates the existing rule (upsert by ID)
- AC-6.3: Cancel restores the previous state

### US-7: Create rule from observed behavior (analytics → rule builder)

**As a** user reviewing analytics  
**I want to** click "Create rule" on any observed program, subcommand, tool, or coverage gap entry  
**So that** the rule builder opens pre-populated with the relevant fields and I can pick a decision without starting from scratch

**Acceptance criteria:**
- AC-7.1: "Add rule →" links in the Command Distribution table navigate to the rule builder with `programs` and `subcommands` pre-filled from the row's data
- AC-7.2: "Add rule →" links in the Coverage Gaps → Uncovered Tools section pre-fill `toolName` (or `toolCategory` for MCP) in the rule builder
- AC-7.3: "Add rule →" links in Coverage Gaps → Uncovered Bash Programs pre-fill `programs` field
- AC-7.4: The rule builder is opened in-page (scroll to or expand the builder, or open a modal) — not just a bare href="/rules"
- AC-7.5: Pre-filled fields are visually highlighted so the user understands what was auto-populated
- AC-7.6: The analytics panel and rule builder panels can communicate (e.g. shared state, URL query params, or a lifted state handler)

### US-8: Richer analytics — decision context on manual reviews

**As a** user deciding whether to create an auto-allow or auto-deny rule  
**I want to** see how my past manual reviews resolved (allowed vs denied) for each observed program/subcommand  
**So that** I know whether a pattern is "always safe" (→ auto-allow) or "frequently denied" (→ auto-deny)

**Acceptance criteria:**
- AC-8.1: Command Distribution table adds two columns: "Manual Allow" and "Manual Deny" counts (already in `SubcommandStatProto` if present, else fetched separately)
- AC-8.2: Coverage Gaps → Uncovered programs/tools show a "Usually allowed / Usually denied / Mixed" badge based on historical manual decisions
- AC-8.3: Summary cards add a "Manual review outcome" breakdown: X% of escalations were ultimately allowed, Y% denied
- AC-8.4: These stats update with the window selector (7/14/30/90 days)

---

## Constraints

- **No backend changes required** for US-1 through US-5: the existing `UpsertApprovalRule` RPC accepts `CommandPattern` (regex string) and `ToolName`/`ToolPattern`; structured criteria must be either serialized into `commandPattern` regex OR the proto must be extended (see Technical Notes)
- **Proto gap**: `CommandCriteria` fields (Programs, Subcommands, flags, PythonModes) are NOT currently in `ApprovalRuleProto`. The UI must work within the existing proto OR the proto must be extended. **Preferred: extend the proto** to add structured fields to avoid lossy regex serialization.
- **Mixed-technical users**: UI must not require regex knowledge for common cases, but must not hide the power either
- **Vanilla-extract CSS**: all new styles must use `.css.ts` files per ADR-009 (no new `.module.css`)

---

## Technical Notes

### Proto extension needed

`ApprovalRuleProto` in `types.proto` currently lacks `CommandCriteria` fields. To store structured rules without lossy regex serialization, add:

```proto
message ApprovalRuleProto {
  // ... existing fields ...
  
  // Structured criteria (replaces raw commandPattern for Criteria-based rules)
  repeated string programs = 20;
  repeated string subcommands = 21;
  repeated string blocked_subcommands = 22;
  repeated string required_flags = 23;
  repeated string forbidden_flags = 24;
  repeated string python_modes = 25;
  bool safe_python_imports_only = 26;
}
```

The backend `rules_store.go` must map these proto fields to `CommandCriteria` when building classifier `Rule` objects.

### Client-side preview

The preview (US-4) can be implemented entirely client-side using a TypeScript port of the key matching logic. Only the Programs/Subcommands/Flags matching needs porting — full regex and Python-mode analysis can be skipped for the preview.

### Rule rendering (US-5)

A `describeRule(rule: ApprovalRuleProto): string` helper should produce readable descriptions:
- `programs: ["git"], subcommands: ["push"]` → "git push (any flags)"  
- `programs: ["python3"], pythonModes: ["script", "module"]` → "python3 running a script or module"
- `commandPattern: "^git log"` → "Pattern: `^git log`"

---

## Success Metrics

- A non-technical user can create a "allow Python running a .py script" rule in under 2 minutes without reading documentation
- A user can create a "block git push --force" rule using only structured fields (no regex)
- All existing seed rules are legible in the table without understanding regex

---

## Scope Boundary

**In scope (this PR):**
- Structured rule builder (US-1, US-2)
- Template library (US-3)  
- Live preview (US-4)
- Plain-language table display (US-5)
- Edit existing user rules (US-6)
- Analytics → rule builder pre-fill workflow (US-7)
- Richer analytics decision context (US-8)
- Proto extension for structured criteria fields
- Backend mapping of structured fields to `CommandCriteria`

**Out of scope:**
- Analytics panel redesign
- Approval queue redesign
- Mobile layout
- Bulk rule import/export
