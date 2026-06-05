# Implementation Plan: Rules UX Redesign

## Overview

This plan implements all 8 user stories (US-1 through US-8) across 5 epics. The dependency order is strict: Epic 1 (proto + backend) must land first; Epic 2 (URL prefill) depends on Epic 3 (builder form); Epic 4 and 5 are independent of each other but both depend on Epic 3 existing.

**Total stories: 18**
**Estimated files touched: ~30**

---

## Epic 1: Proto + Backend Extension

Satisfies: US-1 (AC-1.3), US-2 (AC-2.1–2.2), the persistence side of all structured-field stories.

All stories in this epic must be completed before any frontend work that sends structured fields to the server.

### Story 1.1 — Add structured fields to ApprovalRuleProto

**File:** `proto/session/v1/types.proto`

Add the following fields to the `ApprovalRuleProto` message immediately after the existing field 14 (`created_at`). Field numbers 15–19 are left as a buffer; structured criteria start at 20:

```proto
// Structured CommandCriteria fields (replaces raw commandPattern for criteria-based rules)
repeated string programs               = 20;
repeated string subcommands            = 21;
repeated string blocked_subcommands    = 22;
repeated string required_flags         = 23;
repeated string forbidden_flags        = 24;
repeated string python_modes           = 25;
bool            safe_python_imports_only = 26;
repeated string required_flag_prefixes = 27;
string          tool_category          = 28;
```

Rationale for each field:
- 20–24: Core `CommandCriteria` struct fields exposed in the structured builder (US-1)
- 25–26: Python-mode controls for the Python section (US-2)
- 27: `RequiredFlagPrefixes` — needed to correctly render and round-trip the existing `sed -i` seed rule (US-5)
- 28: `ToolCategory` — already in `RuleSpec`/`ApprovalRuleData`/ent schema but absent from the proto, causing a display gap

**Safety notes:**
- All `repeated string` fields default to empty slice (`[]`) on the wire; `bool` defaults to `false`. No existing deserialization breaks.
- Field numbers 15–19 remain reserved (do not use them) to avoid conflicts if the proto was shared with any external consumers.

**Verification:** `make generate-proto` must complete without error.

---

### Story 1.2 — Add structured criteria columns to ent schema

**File:** `session/ent/schema/approvalrule.go`

Append the following fields to the `Fields()` slice:

```go
field.JSON("programs", []string{}).
    Optional().
    Default([]string{}),
field.JSON("subcommands", []string{}).
    Optional().
    Default([]string{}),
field.JSON("blocked_subcommands", []string{}).
    Optional().
    Default([]string{}),
field.JSON("required_flags", []string{}).
    Optional().
    Default([]string{}),
field.JSON("forbidden_flags", []string{}).
    Optional().
    Default([]string{}),
field.JSON("required_flag_prefixes", []string{}).
    Optional().
    Default([]string{}),
field.JSON("python_modes", []string{}).
    Optional().
    Default([]string{}),
field.Bool("safe_python_imports_only").
    Optional().
    Default(false),
```

After editing, run:
```bash
go generate ./session/ent/...
```

This regenerates the ent client. The new columns are added as `TEXT` (JSON-encoded) and `INTEGER` (bool) with `NULL`-safe defaults; existing rows receive empty arrays and `false` without a manual migration.

**Verification:** `go build ./session/...` passes; existing rule rows still load correctly.

---

### Story 1.3 — Extend domain model and storage layer

**Files:**
- `session/repository.go` — `ApprovalRuleData` struct
- `session/ent_repository.go` — `AllRules()` and `UpsertRule()`
- `server/services/rules_store.go` — `RuleSpec` struct, `reload()`, `Upsert()`, `specsToRules()`

#### 1.3a — `ApprovalRuleData` extension (`session/repository.go`)

Add 8 new fields to the struct (after `FilePattern`):

```go
Programs              []string
Subcommands           []string
BlockedSubcommands    []string
RequiredFlags         []string
ForbiddenFlags        []string
RequiredFlagPrefixes  []string
PythonModes           []string
SafePythonImportsOnly bool
```

#### 1.3b — `ent_repository.go` — `AllRules()`

Extend the mapping loop to populate the new fields from the ent entity:

```go
result[i] = ApprovalRuleData{
    // ... existing fields ...
    Programs:              rule.Programs,
    Subcommands:           rule.Subcommands,
    BlockedSubcommands:    rule.BlockedSubcommands,
    RequiredFlags:         rule.RequiredFlags,
    ForbiddenFlags:        rule.ForbiddenFlags,
    RequiredFlagPrefixes:  rule.RequiredFlagPrefixes,
    PythonModes:           rule.PythonModes,
    SafePythonImportsOnly: rule.SafePythonImportsOnly,
}
```

#### 1.3c — `ent_repository.go` — `UpsertRule()`

Add `Set*()` calls for each new field before `.OnConflictColumns(...)`:

```go
SetPrograms(data.Programs).
SetSubcommands(data.Subcommands).
SetBlockedSubcommands(data.BlockedSubcommands).
SetRequiredFlags(data.RequiredFlags).
SetForbiddenFlags(data.ForbiddenFlags).
SetRequiredFlagPrefixes(data.RequiredFlagPrefixes).
SetPythonModes(data.PythonModes).
SetSafePythonImportsOnly(data.SafePythonImportsOnly).
```

#### 1.3d — `RuleSpec` extension (`rules_store.go`)

Add 8 new fields to `RuleSpec` (after `FilePattern`, before `Decision`):

```go
Programs              []string `json:"programs,omitempty"`
Subcommands           []string `json:"subcommands,omitempty"`
BlockedSubcommands    []string `json:"blocked_subcommands,omitempty"`
RequiredFlags         []string `json:"required_flags,omitempty"`
ForbiddenFlags        []string `json:"forbidden_flags,omitempty"`
RequiredFlagPrefixes  []string `json:"required_flag_prefixes,omitempty"`
PythonModes           []string `json:"python_modes,omitempty"`
SafePythonImportsOnly bool     `json:"safe_python_imports_only,omitempty"`
```

Using `omitempty` preserves the existing JSON export file format for rules without structured criteria.

#### 1.3e — `reload()` in `rules_store.go`

Extend the `specs[i] = RuleSpec{...}` mapping to include the new fields from `ApprovalRuleData`.

#### 1.3f — `specsToRules()` in `rules_store.go`

After the existing pattern compilation block, add:

```go
// Populate Criteria from structured fields when at least one is set.
if len(spec.Programs) > 0 ||
    len(spec.Subcommands) > 0 ||
    len(spec.BlockedSubcommands) > 0 ||
    len(spec.RequiredFlags) > 0 ||
    len(spec.ForbiddenFlags) > 0 ||
    len(spec.RequiredFlagPrefixes) > 0 ||
    len(spec.PythonModes) > 0 ||
    spec.SafePythonImportsOnly {
    r.Criteria = &classifier.CommandCriteria{
        Programs:              spec.Programs,
        Subcommands:           spec.Subcommands,
        BlockedSubcommands:    spec.BlockedSubcommands,
        RequiredFlags:         spec.RequiredFlags,
        ForbiddenFlags:        spec.ForbiddenFlags,
        RequiredFlagPrefixes:  spec.RequiredFlagPrefixes,
        PythonModes:           spec.PythonModes,
        SafePythonImportsOnly: spec.SafePythonImportsOnly,
    }
}
```

This populates the `Criteria` field that was always `nil` for user rules, activating the classifier's structured matching path.

**Verification:** Existing tests pass; a new integration test creates a rule with `Programs: ["git"], Subcommands: ["push"]` and verifies the classifier matches `git push` and rejects `git log`.

---

### Story 1.4 — Extend rules_service.go mapping

**File:** `server/services/rules_service.go`

#### 1.4a — `UpsertApprovalRule` handler

In the `spec := RuleSpec{...}` block, add mapping from proto fields to spec:

```go
Programs:              r.Programs,
Subcommands:           r.Subcommands,
BlockedSubcommands:    r.BlockedSubcommands,
RequiredFlags:         r.RequiredFlags,
ForbiddenFlags:        r.ForbiddenFlags,
RequiredFlagPrefixes:  r.RequiredFlagPrefixes,
PythonModes:           r.PythonModes,
SafePythonImportsOnly: r.SafePythonImportsOnly,
ToolCategory:          r.ToolCategory,
```

#### 1.4b — `specToProto()`

Extend the proto construction to include the new fields:

```go
Programs:              spec.Programs,
Subcommands:           spec.Subcommands,
BlockedSubcommands:    spec.BlockedSubcommands,
RequiredFlags:         spec.RequiredFlags,
ForbiddenFlags:        spec.ForbiddenFlags,
RequiredFlagPrefixes:  spec.RequiredFlagPrefixes,
PythonModes:           spec.PythonModes,
SafePythonImportsOnly: spec.SafePythonImportsOnly,
ToolCategory:          spec.ToolCategory,
```

#### 1.4c — `ruleToSpec()` — expose seed rule criteria

Extend the function to populate structured criteria when `r.Criteria != nil`:

```go
if r.Criteria != nil {
    spec.Programs              = r.Criteria.Programs
    spec.Subcommands           = r.Criteria.Subcommands
    spec.BlockedSubcommands    = r.Criteria.BlockedSubcommands
    spec.RequiredFlags         = r.Criteria.RequiredFlags
    spec.ForbiddenFlags        = r.Criteria.ForbiddenFlags
    spec.RequiredFlagPrefixes  = r.Criteria.RequiredFlagPrefixes
    spec.PythonModes           = r.Criteria.PythonModes
    spec.SafePythonImportsOnly = r.Criteria.SafePythonImportsOnly
}
if r.ToolCategory != "" {
    spec.ToolCategory = r.ToolCategory
}
```

This closes the gap where seed rules with `Criteria` showed as empty `CommandPattern` in the UI.

**Validation notes:**
- No regex validation is needed for Programs/Subcommands/Flags — they are plain string arrays.
- The existing `regexp.Compile` validation loop in `Upsert()` covers `ToolPattern`, `CommandPattern`, and `FilePattern` unchanged.
- The backend must reject (via `connect.CodeInvalidArgument`) any rule that sets both `commandPattern` and structured criteria fields simultaneously. Add a guard in `Upsert()`:

```go
hasCriteria := len(spec.Programs) > 0 || len(spec.Subcommands) > 0 ||
    len(spec.BlockedSubcommands) > 0 || len(spec.RequiredFlags) > 0 ||
    len(spec.ForbiddenFlags) > 0 || len(spec.RequiredFlagPrefixes) > 0 ||
    len(spec.PythonModes) > 0 || spec.SafePythonImportsOnly
if hasCriteria && spec.CommandPattern != "" {
    return RuleSpec{}, fmt.Errorf(
        "rule cannot set both commandPattern and structured criteria; use one mode")
}
```

#### 1.4d — `useApprovalRules.ts` hook update

**File:** `web-app/src/lib/hooks/useApprovalRules.ts`

Extend the `create(ApprovalRuleProtoSchema, {...})` call to include the new structured fields:

```typescript
programs:              ruleData.programs              ?? [],
subcommands:           ruleData.subcommands           ?? [],
blockedSubcommands:    ruleData.blockedSubcommands    ?? [],
requiredFlags:         ruleData.requiredFlags         ?? [],
forbiddenFlags:        ruleData.forbiddenFlags        ?? [],
requiredFlagPrefixes:  ruleData.requiredFlagPrefixes  ?? [],
pythonModes:           ruleData.pythonModes           ?? [],
safePythonImportsOnly: ruleData.safePythonImportsOnly ?? false,
toolCategory:          ruleData.toolCategory          ?? "",
```

---

### Story 1.5 — Build verification

Run in order:

```bash
make generate-proto        # regenerate proto Go + TypeScript stubs
go generate ./session/ent/...   # regenerate ent code
make build                 # full Go build
make test                  # all Go tests
```

All must pass before frontend work begins.

---

## Epic 2: Frontend — Shared Rule Builder State (Analytics ↔ Rule Builder)

Satisfies: US-7 (all ACs).

### Story 2.1 — URL query param prefill mechanism

**File to create:** `web-app/src/lib/ruleBuilderPrefill.ts`

Define the prefill payload type and encode/decode helpers:

```typescript
export interface RuleBuilderPrefill {
  programs?: string[];        // from Command Distribution "program" column
  subcommands?: string[];     // from Command Distribution "subcommand" column
  toolName?: string;          // from Uncovered Tools table
  toolCategory?: string;      // for MCP/generic tool categories
  suggestedDecision?: number; // AutoDecision enum value (optional hint)
}

export function encodePrefill(payload: RuleBuilderPrefill): string {
  return btoa(JSON.stringify(payload));
}

export function decodePrefill(encoded: string): RuleBuilderPrefill | null {
  try {
    return JSON.parse(atob(encoded)) as RuleBuilderPrefill;
  } catch {
    return null;
  }
}

export function buildPrefillHref(payload: RuleBuilderPrefill): string {
  return `/rules?prefill=${encodePrefill(payload)}`;
}
```

**File to modify:** The `/rules` page component (likely `web-app/src/app/rules/page.tsx` or wherever `ApprovalRulesPanel` is hosted — verify the route before implementing).

In the page component, read the `prefill` search param using `useSearchParams()` from Next.js:

```typescript
import { useSearchParams } from 'next/navigation';
import { decodePrefill } from '@/lib/ruleBuilderPrefill';

const searchParams = useSearchParams();
const rawPrefill = searchParams.get('prefill');
const prefill = rawPrefill ? decodePrefill(rawPrefill) : null;
```

Pass `prefill` as a prop to `ApprovalRulesPanel` or to the new `RuleBuilderForm` directly.

**Implementation notes for Next.js App Router:**
- `useSearchParams()` must be called in a Client Component (`"use client"`). The `/rules` page must be a Client Component or the param-reading must be in a child Client Component.
- The page should auto-scroll to the rule builder and auto-open the form when `prefill` is present: call `document.getElementById('rule-builder')?.scrollIntoView({ behavior: 'smooth' })` in a `useEffect` that fires when `prefill !== null`.
- After the user saves or cancels, clear the `prefill` param from the URL with `router.replace('/rules')` to prevent re-opening on refresh.

---

### Story 2.2 — Update "Add rule →" links in ApprovalAnalyticsPanel

**File:** `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`

Replace bare `href="/rules"` anchors with programmatic navigation using `buildPrefillHref`:

**Command Distribution table row** (currently `href="/rules"` with no prefill):

```typescript
import { buildPrefillHref } from '@/lib/ruleBuilderPrefill';

// In CommandDistributionTable, replace the <a> element:
<a
  href={buildPrefillHref({ programs: [s.programName], subcommands: [s.subcommand] })}
  className={addRuleLink}
  title={`Add a rule for ${s.programName} ${s.subcommand}`}
>
  Add rule →
</a>
```

**Uncovered Tools table row:**

```typescript
<a
  href={buildPrefillHref({ toolName: t.toolName })}
  className={addRuleLink}
  title={`Add a rule to cover ${t.toolName}`}
>
  Add rule →
</a>
```

**Uncovered Bash Programs table row:**

```typescript
<a
  href={buildPrefillHref({ programs: [p.programName] })}
  className={addRuleLink}
  title={`Add a rule for ${p.programName}`}
>
  Add rule →
</a>
```

**Visual highlight of prefilled fields (AC-7.5):** When `RuleBuilderForm` mounts with a prefill payload, apply a `.prefilled` CSS class (defined via vanilla-extract) to the affected tag inputs for 2 seconds, then fade it out. The class adds a `ring` highlight (e.g., `outline: 2px solid ${vars.color.primary}`).

---

## Epic 3: Frontend — Structured Rule Builder

Satisfies: US-1 (all ACs), US-2 (all ACs), US-3 (all ACs), US-4 (all ACs), US-6 (all ACs).

All components use vanilla-extract `.css.ts` files colocated with the component. No new CSS modules. All design tokens via `vars.*` from `@/styles/theme.css`.

### Story 3.1 — TagInput component

**Files to create:**
- `web-app/src/components/rules/TagInput.tsx`
- `web-app/src/components/rules/TagInput.css.ts`

**Component API:**

```typescript
interface TagInputProps {
  value: string[];
  onChange: (tags: string[]) => void;
  placeholder?: string;
  disabled?: boolean;
  label?: string;
  helperText?: string;
  isPrefilled?: boolean; // triggers highlight animation when true
}

export function TagInput(props: TagInputProps): JSX.Element
```

**Behavior contract:**
- Enter key or comma character commits current input text as a new tag (trim whitespace first; skip empty strings and exact duplicates)
- Backspace on empty input deletes the last tag in the array
- Click on chip `×` button removes that tag
- Paste event splits pasted text on comma and/or space, adds all non-empty unique parts as tags
- The outer container has the focused input border style when the inner `<input>` has focus
- `isPrefilled=true` applies a highlight ring that fades after 2 seconds

**Styling with `@vanilla-extract/recipes`:**

```typescript
// TagInput.css.ts
import { recipe } from '@vanilla-extract/recipes';
import { style, keyframes } from '@vanilla-extract/css';
import { vars } from '@/styles/theme.css';

export const tagChip = recipe({
  base: {
    display: 'inline-flex',
    alignItems: 'center',
    gap: vars.space[1],
    padding: `${vars.space[0]} ${vars.space[2]}`,
    borderRadius: vars.radii.sm,
    fontSize: vars.fontSize.sm,
    background: vars.color.actionPrimary,   // or a softer tint
    color: vars.color.textInverse,
  },
  variants: {
    disabled: {
      true: { opacity: 0.5 },
    },
  },
});
```

---

### Story 3.2 — RuleBuilderForm component

**Files to create:**
- `web-app/src/components/rules/RuleBuilderForm.tsx`
- `web-app/src/components/rules/RuleBuilderForm.css.ts`

**Internal state (managed with `react-hook-form` + `zod`):**

```typescript
import { z } from 'zod/v4';

const ruleFormSchema = z.object({
  name:                  z.string().min(1, 'Name is required'),
  mode:                  z.enum(['structured', 'regex']),
  // Tool target (only one of the three should be set)
  toolName:              z.string(),
  toolPattern:           z.string(),
  toolCategory:          z.string(),
  // Structured criteria (Structured mode only)
  programs:              z.array(z.string()),
  subcommands:           z.array(z.string()),
  blockedSubcommands:    z.array(z.string()),
  requiredFlags:         z.array(z.string()),
  forbiddenFlags:        z.array(z.string()),
  requiredFlagPrefixes:  z.array(z.string()),
  pythonModes:           z.array(z.string()),
  safePythonImportsOnly: z.boolean(),
  // Regex mode only
  commandPattern:        z.string(),
  filePattern:           z.string(),
  // Decision + risk
  decision:              z.number(),
  riskLevel:             z.string(),
  // Metadata
  reason:                z.string(),
  alternative:           z.string(),
  priority:              z.number().int().min(1).max(9999),
  enabled:               z.boolean(),
});
```

**Component API:**

```typescript
interface RuleBuilderFormProps {
  editRule?: ApprovalRuleProto;    // pre-fills form for edit mode (US-6)
  prefill?: RuleBuilderPrefill;    // pre-fills from analytics deep-link (US-7)
  onSave: (rule: Partial<ApprovalRuleProto> & { id: string }) => Promise<void>;
  onCancel: () => void;
}

export function RuleBuilderForm(props: RuleBuilderFormProps): JSX.Element
```

**Layout structure:**
1. **Mode toggle** at the top: segmented "Structured | Regex" control. Switching modes with filled fields shows a confirmation: "Switch to Regex mode? Structured criteria will be cleared." Uses browser `confirm()` or a small inline confirmation div (no external modal needed).
2. **Tool Target section** (both modes): Radio group — "Exact tool name" (text input), "Tool category" (dropdown: builtin-agent, mcp-read, mcp-write, any), "Tool pattern (regex)" (text input). Only one can be active at a time.
3. **Structured mode — Command Criteria section** (hidden in Regex mode):
   - Programs `<TagInput>` — placeholder: "e.g. git, python3"
   - Subcommands `<TagInput>` — placeholder: "e.g. push, pull"
   - Blocked Subcommands `<TagInput>` — placeholder: "e.g. reset, rebase"
   - Required Flags `<TagInput>` — placeholder: "e.g. --hard, -f"
   - Forbidden Flags `<TagInput>` — placeholder: "e.g. --force, --no-verify"
   - Required Flag Prefixes `<TagInput>` (under "Advanced ▸" accordion) — placeholder: "e.g. -i"
4. **Python Mode section** (visible only when `programs` contains any of: `python`, `python3`, `pypy`, `pypy3`, `python2`, or any string starting with `python`):
   - Checkboxes: Script (.py file), Module (-m), Inline (-c), Version (-V)
   - "Safe stdlib imports only" toggle — enabled only when Inline checkbox is checked. Shows tooltip: "Allows inline Python only when all imports are in the stdlib safelist."
5. **Regex mode section** (hidden in Structured mode):
   - Command Pattern textarea
   - File Pattern input
6. **Decision + Risk section** (both modes):
   - Decision: large segmented radio buttons — "Auto-Allow", "Auto-Deny", "Escalate"
   - Risk level: dropdown (Low, Medium, High, Critical)
   - Priority: number input with tier guide as helper text: "1000+ = deny first · 500+ = escalate before allow · 100–499 = allow tier · 1–99 = fires after built-in rules"
   - Priority auto-defaults on decision change: DENY → 950, ESCALATE → 450, ALLOW → 100
7. **Metadata section**:
   - Name (required)
   - Reason (optional)
   - Alternative (optional)
   - Enabled toggle
8. **Actions row**: "Save Rule" / "Cancel" buttons; if in edit mode, show "Updating: {rule.name}"

**Priority conflict warning:** After the Priority input, if an existing seed rule or user rule with higher or equal priority would also match the current criteria, show a yellow inline warning: "Seed rule '{name}' (priority {N}, {decision}) may fire first."

---

### Story 3.3 — RulePreview component

**Files to create:**
- `web-app/src/components/rules/RulePreview.tsx`
- `web-app/src/components/rules/RulePreview.css.ts`

**TypeScript port of CommandCriteria.Matches():**

```typescript
// web-app/src/lib/rulePreview.ts

export interface ParsedCommand {
  program: string;
  args: string[];
  subcommand: string; // extracted
}

export interface PreviewResult {
  matches: string[];   // 3–5 example commands that WOULD match
  nonMatches: string[]; // 2–3 boundary examples that would NOT match
}

// matchesCriteria ports the Go CommandCriteria.Matches() logic.
// Skips: SafePythonImportsOnly (requires AST), RedirectionPattern.
export function matchesCriteria(criteria: RuleCriteria, cmd: ParsedCommand): boolean
```

**Implementation approach:**

Port the following functions from the Go implementation:
- `matchesProgram(programs: string[], program: string): boolean` — exact match OR `program.startsWith(p + '.')` prefix check
- `extractSubcommand(program: string, args: string[]): string` — detect deep-subcommand programs, skip prefix-flag args; return first subcommand-like token (or first two for deep programs)
- The programs allow-list, subcommands allow-list, blockedSubcommands deny-list, requiredFlags exact-token matching, forbiddenFlags exact-token matching

**Example bank:** A static `EXAMPLE_COMMANDS: Record<string, ParsedCommand[]>` map keyed by program name covers: git, python3, npm, docker, gh, aws, sed, curl, pip, cargo, go, kubectl, terraform, node, make, jq, cat, rm.

**`useRulePreview` hook:**

```typescript
import { useDeferredValue } from 'react';

export function useRulePreview(criteria: RuleCriteria): PreviewResult {
  const deferred = useDeferredValue(criteria);
  return useMemo(() => computePreview(deferred), [deferred]);
}
```

`useDeferredValue` provides built-in 200 ms lag tolerance without an explicit `setTimeout` debounce.

**Component API:**

```typescript
interface RulePreviewProps {
  criteria: RuleCriteria; // current form state
}

export function RulePreview({ criteria }: RulePreviewProps): JSX.Element
```

**Layout:**
- Two-column flex layout: "Would match ✓" (green) | "Would not match ✗" (red)
- Each row: `<code>` monospace command string
- Empty state: "Add criteria above to see matching examples."
- If criteria is populated but no examples found: "No examples available for this program. Check that the program name is spelled correctly."

**Scope limitation (important):** The preview does NOT implement `SafePythonImportsOnly` or `RedirectionPattern` — too complex for client-side preview. When `safePythonImportsOnly: true` is set, show a notice: "Note: 'Safe imports only' filtering is not shown in preview — save the rule to test it."

---

### Story 3.4 — TemplateLibrary component

**Files to create:**
- `web-app/src/components/rules/TemplateLibrary.tsx`
- `web-app/src/components/rules/TemplateLibrary.css.ts`
- `web-app/src/lib/ruleTemplates.ts` (static data)

**Template data structure:**

```typescript
// web-app/src/lib/ruleTemplates.ts
export interface RuleTemplate {
  id: string;
  title: string;
  description: string;
  icon: string;       // emoji
  decision: AutoDecision;
  riskLevel: string;
  priority: number;
  programs?: string[];
  subcommands?: string[];
  blockedSubcommands?: string[];
  requiredFlags?: string[];
  forbiddenFlags?: string[];
  pythonModes?: string[];
  safePythonImportsOnly?: boolean;
  toolName?: string;
  toolCategory?: string;
  reason?: string;
  alternative?: string;
}
```

**Template list (13 templates):**

| id | title | decision | programs/tool |
|----|-------|----------|---------------|
| `python-script-module` | Python script/module | Allow | programs: [python3], pythonModes: [script, module] |
| `python-inline-stdlib` | Python inline — stdlib only | Allow | programs: [python3], pythonModes: [inline], safePythonImportsOnly: true |
| `python-inline-any` | Python inline — any imports | Escalate | programs: [python3], pythonModes: [inline] |
| `git-read` | Git read-only | Allow | programs: [git], subcommands: [log, status, diff, show, ls-files, blame, branch, tag] |
| `git-push` | Git push | Escalate | programs: [git], subcommands: [push] |
| `git-reset-hard` | Git reset --hard | Deny | programs: [git], subcommands: [reset], requiredFlags: [--hard] |
| `npm-install` | npm / yarn install | Allow | programs: [npm, yarn, pnpm], subcommands: [install, ci, i] |
| `npm-publish` | npm publish | Escalate | programs: [npm], subcommands: [publish] |
| `docker-read` | Docker read | Allow | programs: [docker], subcommands: [ps, images, logs, inspect, stats] |
| `docker-write` | Docker write | Escalate | programs: [docker], subcommands: [run, exec, build, push, rm, rmi, pull] |
| `mcp-read-tools` | MCP read tools | Allow | toolCategory: mcp-read |
| `file-editing-tools` | File editing tools | Allow | toolName: Edit (or toolCategory: builtin-file-write?) |
| `escalate-unknown` | Escalate unknown program | Escalate | (no criteria — catch-all for unknown programs; uses commandPattern: `.*`) |

**Component API:**

```typescript
interface TemplateLibraryProps {
  open: boolean;
  onClose: () => void;
  onSelect: (template: RuleTemplate) => void;
}

export function TemplateLibrary(props: TemplateLibraryProps): JSX.Element
```

**Implementation:**
- Uses `@radix-ui/react-dialog` (`Dialog.Root`, `Dialog.Portal`, `Dialog.Overlay`, `Dialog.Content`) for accessibility (focus trap, Escape key, ARIA)
- 3-column CSS grid of template cards (responsive: 2-column below 800px, 1-column below 480px)
- Each card: icon (emoji, large), title, description, decision badge (same design as rules table)
- "Start from scratch" link at the bottom dismisses without selecting
- No search box needed for 13 templates (add if template count grows beyond 15)

---

### Story 3.5 — Integrate into ApprovalRulesPanel

**File:** `web-app/src/components/sessions/ApprovalRulesPanel.tsx`

**Changes:**
1. Remove the existing inline form section (the `showForm` boolean + `RuleFormState` + `emptyForm` + all the `<label>/<input>/<select>` JSX in the form section).
2. Add a `RuleBuilderForm` import and replace the form section.
3. Add a `TemplateLibrary` dialog triggered by a "Start from template" button next to the "Add Custom Rule" button.
4. Add a `RulePreview` component below the `RuleBuilderForm` criteria section (pass current form state down via a callback or by lifting `RuleBuilderForm` state to a shared hook).
5. Add `editingRule: ApprovalRuleProto | null` state — set when an "Edit" button is clicked on a user rule row.
6. Pass `prefill` prop from the page's URL params (via prop drilling from the page or a context).

**Edit mode wiring (US-6):**

When `editingRule !== null`, pass it to `<RuleBuilderForm editRule={editingRule} .../>`. The form detects `editRule` is set and:
- Pre-fills all fields from the existing rule
- Uses the existing `rule.id` instead of generating a new `user-${Date.now()}` ID
- Shows "Updating: {rule.name}" in the save button and form title

**Cancel behavior (AC-6.3):** Sets `editingRule = null`, resets form, scrolls back to table.

**Builder anchor:** Add `id="rule-builder"` to the form section wrapper for scroll targeting from analytics deep links.

---

## Epic 4: Frontend — Enhanced Rule Table Display

Satisfies: US-5 (all ACs), US-6 (AC-6.1 edit button).

### Story 4.1 — `describeRule` helper

**File to create:** `web-app/src/lib/describeRule.ts`

```typescript
export interface RuleDescription {
  primary: string;      // main match description (one line)
  secondary?: string;   // optional clarification
  isStructured: boolean;
  isRegex: boolean;
}

export function describeRule(rule: ApprovalRuleProto): RuleDescription
```

**Logic:**

```
if programs.length > 0:
  if subcommands.length > 0:
    → "{programs.join('/')} {subcommands.join(', ')} (any flags)"
  elif pythonModes.length > 0:
    → "{programs.join('/')} running {modeDescription(pythonModes)}"
  else:
    → "{programs.join('/')} (any subcommand)"
  secondary: add "Blocked: {blocked.join(', ')}" if blockedSubcommands present
elif toolName != "":
  → "Tool: {toolName}"
elif toolCategory != "":
  → "Any {toolCategoryLabel(toolCategory)} tool"
elif toolPattern != "":
  → "Tools matching: {toolPattern}"
elif commandPattern != "":
  → "Pattern: `{commandPattern}`"
else:
  → "(no match criteria)"
```

Helper for Python modes:
```
modeDescription(["script"])              → "a .py script"
modeDescription(["module"])             → "-m module"
modeDescription(["inline"])             → "inline -c code"
modeDescription(["script", "module"])   → "a .py script or -m module"
modeDescription(["script", "module", "inline"]) → "a script, module, or inline code"
```

---

### Story 4.2 — MatchDescription component + table update

**File to create:** `web-app/src/components/rules/MatchDescription.tsx`

(No separate CSS file needed — share chip styles from existing `ApprovalRulesPanel.css.ts` or use inline styles for the new structured-description format.)

```typescript
export function MatchDescription({ rule }: { rule: ApprovalRuleProto }): JSX.Element
```

Renders the `RuleDescription` from `describeRule(rule)`:
- `primary` as the main visible text (not a code chip)
- `secondary` as smaller muted text below
- Regex rules: show `<code className={matchChip}>Pattern: {commandPattern}</code>` (existing style)
- Structured rules: plain prose with optional `<code>` elements for program/flag names

**File to modify:** `web-app/src/components/sessions/ApprovalRulesPanel.tsx`

Replace the Match column cell:
```tsx
// Before:
<div className={matchInfo}>
  {rule.toolName && <code className={matchChip}>{rule.toolName}</code>}
  {rule.commandPattern && <code className={matchChip}>{rule.commandPattern}</code>}
  ...
</div>

// After:
<MatchDescription rule={rule} />
```

---

### Story 4.3 — Edit button on user rule rows

**File:** `web-app/src/components/sessions/ApprovalRulesPanel.tsx`

In the table row action cell (currently only has Delete button for user rules), add an Edit button:

```tsx
{rule.source === "user" && (
  <>
    <button
      className={editButton}
      onClick={() => {
        setEditingRule(rule);
        document.getElementById('rule-builder')?.scrollIntoView({ behavior: 'smooth' });
      }}
      aria-label={`Edit rule ${rule.name}`}
    >
      Edit
    </button>
    <button
      className={deleteButton}
      onClick={() => deleteRule(rule.id)}
      aria-label={`Delete rule ${rule.name}`}
    >
      ✕
    </button>
  </>
)}
```

Add `editButton` style to `ApprovalRulesPanel.css.ts` — a small text-style button (no background, primary color, underline on hover).

---

## Epic 5: Frontend — Richer Analytics (US-8)

Satisfies: US-8 (AC-8.1, AC-8.2, AC-8.3, AC-8.4).

**Important pre-check:** `SubcommandStatProto` does NOT currently have `manual_allow` / `manual_deny` fields (only `count`). The `AnalyticsSummaryProto` has `decision_counts` map which includes `"manual_allow"` and `"manual_deny"` total counts — but these are not broken down per-program. To implement AC-8.1 and AC-8.2 at the per-program level, a proto extension is needed.

### Story 5.1 — Manual outcome summary card

**File:** `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`

Using the existing `summary.decisionCounts["manual_allow"]` and `summary.decisionCounts["manual_deny"]` fields (already in `AnalyticsSummaryProto.decision_counts` map, already populated by `ComputeSummary()` in `analytics_store.go`):

Add a new summary card to the `<div className={cards}>` section:

```tsx
{escalateCount > 0 && (
  <div className={`${card} ${cardManual}`}>
    <span className={cardValue}>
      {pct(summary.decisionCounts["manual_allow"] ?? 0, escalateCount)}%
    </span>
    <span className={cardLabel}>Manual → Allowed</span>
    <span className={cardSub}>
      {summary.decisionCounts["manual_deny"] ?? 0} denied
    </span>
  </div>
)}
```

This requires no proto changes — `decisionCounts` map already carries these keys.

---

### Story 5.2 — Command Distribution manual columns (proto extension required)

**Assessment:** `SubcommandStatProto` currently has only `program_name`, `subcommand`, `category`, `count`. There are no `manual_allow` / `manual_deny` fields. To add per-row manual outcome data requires:

1. **Proto extension** — add to `SubcommandStatProto`:
   ```proto
   int32 manual_allow = 5;
   int32 manual_deny = 6;
   ```
2. **Analytics store extension** — `SubcommandStat` struct needs `ManualAllow int` and `ManualDeny int` fields.
3. **`ComputeSummary()`** — the aggregation loop must track manual decisions per (program, subcommand) key.
4. **`summaryToProto()`** — map the new fields into `SubcommandStatProto`.
5. **Frontend** — add two columns to `CommandDistributionTable`.

**Implementation:**

The `SubcommandStat` accumulation in `analytics_store.go` (`ComputeSummary()`) iterates over `AnalyticsEntry` records. Each entry has a `Decision` string. Add to the map loop:

```go
if entry.Decision == "manual_allow" {
    stat.ManualAllow++
}
if entry.Decision == "manual_deny" {
    stat.ManualDeny++
}
subcommandStats[key] = stat
```

**Frontend `CommandDistributionTable` changes:**

```tsx
<th className={`${th} ${thRight}`}>Manual ✓</th>
<th className={`${th} ${thRight}`}>Manual ✗</th>
// ...
<td className={`${td} ${tdRight}`}>
  <span className={allowCount}>{s.manualAllow}</span>
</td>
<td className={`${td} ${tdRight}`}>
  <span className={denyCount}>{s.manualDeny}</span>
</td>
```

---

### Story 5.3 — Coverage gap outcome badges

**File:** `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx`

For uncovered programs/tools, add a "Usually allowed / Usually denied / Mixed" badge. This requires per-program manual outcome data — the same `manual_allow`/`manual_deny` fields added in Story 5.2 must also be added to `ProgramStatProto` (for uncovered programs) and `ToolStatProto` (for uncovered tools).

**Proto extension:**

```proto
message ProgramStatProto {
  string program_name = 1;
  string category = 2;
  int32  count = 3;
  int32  manual_allow = 4;  // ADD
  int32  manual_deny = 5;   // ADD
}

message ToolStatProto {
  string tool_name = 1;
  int32  count = 2;
  int32  manual_allow = 3;  // ADD
  int32  manual_deny = 4;   // ADD
}
```

**Badge logic:**

```typescript
function outcomeBadge(manualAllow: number, manualDeny: number): string | null {
  const total = manualAllow + manualDeny;
  if (total === 0) return null;
  const allowPct = manualAllow / total;
  if (allowPct >= 0.8) return "Usually allowed";
  if (allowPct <= 0.2) return "Usually denied";
  return "Mixed";
}
```

Add badge to the Uncovered Tools and Uncovered Bash Programs table rows.

---

## Dependency Order Summary

```
1.1 (proto) → 1.5 (build check)
    ↓
1.2 (ent schema) → 1.3 (domain model + storage) → 1.4 (service layer + hook)
    ↓
3.1 (TagInput) → 3.2 (RuleBuilderForm) → 3.3 (RulePreview)
                       ↓
              3.4 (TemplateLibrary) → 3.5 (integrate into panel)
                       ↓
    2.1 (prefill lib) → 2.2 (analytics links update)
                       ↓
         4.1 (describeRule) → 4.2 (MatchDescription) → 4.3 (Edit button)
                       ↓
    5.1 (summary card) → 5.2 (command distribution) → 5.3 (gap badges)
```

Epics 4 and 5 can be parallelized with Epics 2–3 once Epic 1 is done.

---

## File Inventory

### New files

| Path | Purpose |
|------|---------|
| `web-app/src/components/rules/TagInput.tsx` | Tag-input component (Story 3.1) |
| `web-app/src/components/rules/TagInput.css.ts` | Styles (Story 3.1) |
| `web-app/src/components/rules/RuleBuilderForm.tsx` | Structured form (Story 3.2) |
| `web-app/src/components/rules/RuleBuilderForm.css.ts` | Styles (Story 3.2) |
| `web-app/src/components/rules/RulePreview.tsx` | Live preview panel (Story 3.3) |
| `web-app/src/components/rules/RulePreview.css.ts` | Styles (Story 3.3) |
| `web-app/src/components/rules/TemplateLibrary.tsx` | Template picker modal (Story 3.4) |
| `web-app/src/components/rules/TemplateLibrary.css.ts` | Styles (Story 3.4) |
| `web-app/src/components/rules/MatchDescription.tsx` | Plain-language match cell (Story 4.2) |
| `web-app/src/lib/ruleBuilderPrefill.ts` | Prefill encode/decode (Story 2.1) |
| `web-app/src/lib/ruleTemplates.ts` | Static template data (Story 3.4) |
| `web-app/src/lib/rulePreview.ts` | Client-side criteria matching (Story 3.3) |
| `web-app/src/lib/describeRule.ts` | Rule description helper (Story 4.1) |

### Modified files

| Path | Epic/Story |
|------|-----------|
| `proto/session/v1/types.proto` | 1.1, 5.2, 5.3 |
| `session/ent/schema/approvalrule.go` | 1.2 |
| `session/repository.go` | 1.3 |
| `session/ent_repository.go` | 1.3 |
| `server/services/rules_store.go` | 1.3 |
| `server/services/rules_service.go` | 1.4 |
| `server/services/analytics_store.go` | 5.2, 5.3 |
| `web-app/src/lib/hooks/useApprovalRules.ts` | 1.4 |
| `web-app/src/components/sessions/ApprovalRulesPanel.tsx` | 3.5, 4.2, 4.3 |
| `web-app/src/components/sessions/ApprovalAnalyticsPanel.tsx` | 2.2, 5.1, 5.2, 5.3 |
| `/rules` page component (verify location) | 2.1 |

---

## Testing Notes

- **Unit tests for `rulePreview.ts`:** Test `matchesCriteria()` against the known Go behavior for each criteria field type. Cover: Programs prefix matching, subcommand extraction for deep vs shallow programs, blocked subcommand, required flags, forbidden flags.
- **Unit tests for `describeRule.ts`:** Cover all description branches.
- **Integration test for backend:** Create a structured rule via the API, retrieve it, verify the proto round-trips all new fields, and verify the classifier matches expected commands.
- **E2E:** One Playwright test covering the full workflow: analytics gap → "Add rule →" → builder opens pre-filled → save → rule appears in table with readable description.
