# Research 03 — Architecture: Proto Extension + Backend Mapping

## 1. Current Proto Field Numbers in ApprovalRuleProto

From `proto/session/v1/types.proto` (lines 806–822), the current fields use numbers **1–14**:

```
1: id            7: decision
2: name          8: risk_level
3: tool_name     9: reason
4: tool_pattern  10: alternative
5: command_pat.  11: priority
6: file_pattern  12: enabled
             13: source
             14: created_at
```

**Field numbers 15–19 are currently unused and safe to occupy.** The requirements doc proposes starting at 20 to leave a buffer — that is also fine. Proto field numbers are wire-stable once published; choosing numbers conservatively avoids future conflicts.

### Recommended Field Assignments

```proto
// Structured CommandCriteria fields (proto field numbers 20–28)
repeated string programs               = 20;
repeated string subcommands            = 21;
repeated string blocked_subcommands    = 22;
repeated string required_flags         = 23;
repeated string forbidden_flags        = 24;
repeated string python_modes           = 25;
bool safe_python_imports_only          = 26;
repeated string required_flag_prefixes = 27;  // needed for sed -i rule
string tool_category                   = 28;  // already in RuleSpec, missing from proto
```

`required_flag_prefixes` (field 27) is worth including because the existing seed rule for `sed -i` uses it, and a user may want to create similar rules.

`tool_category` (field 28) is already persisted in `ApprovalRuleData` and `RuleSpec` but is absent from `ApprovalRuleProto`. Adding it closes an existing gap even though it is not required for the new UI.

## 2. How rules_service.go Converts Proto → classifier.Rule (Current Gaps)

### Current Path (proto → wire → store → classifier)

```
UpsertApprovalRule RPC
  → req.Msg.Rule (*ApprovalRuleProto)
  → RuleSpec {ID, Name, ToolName, ToolPattern, CommandPattern, FilePattern, Decision, ...}
  → RulesStore.Upsert(spec)       // persists to SQLite via ent
  → rebuildClassifier()
     → specsToRules(specs)        // compiles regex, maps decision/risk level
     → classifier.Rule{...}       // NO Criteria field — always nil
```

**The `Criteria *CommandCriteria` field on `classifier.Rule` is never set for user rules.** `specsToRules` in `rules_store.go` constructs `classifier.Rule` without `r.Criteria`. Seed rules set `Criteria` directly in Go code (`SeedRules()`) — no proto round-trip.

### What Is Missing for CommandCriteria

In `specsToRules()` (rules_store.go lines 232–274):

```go
// MISSING: populate r.Criteria from spec fields
if len(spec.Programs) > 0 || len(spec.Subcommands) > 0 || ... {
    r.Criteria = &classifier.CommandCriteria{
        Programs:              spec.Programs,
        Subcommands:           spec.Subcommands,
        BlockedSubcommands:    spec.BlockedSubcommands,
        RequiredFlags:         spec.RequiredFlags,
        ForbiddenFlags:        spec.ForbiddenFlags,
        PythonModes:           spec.PythonModes,
        SafePythonImportsOnly: spec.SafePythonImportsOnly,
        RequiredFlagPrefixes:  spec.RequiredFlagPrefixes,
    }
}
```

Similarly, `ruleToSpec()` (rules_service.go lines 234–257) converts `classifier.Rule` → `RuleSpec` for listing seed rules, but it has no path to extract `Criteria` fields back into spec fields. This means seed rules with `Criteria` currently show as empty `CommandPattern` in the UI — a display gap that the plain-language rendering (US-5) must handle using the proto's new structured fields.

Note: seed rules are not stored in the DB; they are re-created from `SeedRules()` on each request via `allRuleSpecs()`. To expose their structured criteria in the UI, `ruleToSpec()` must also be extended to populate the new criteria fields from `rule.Criteria`.

## 3. How rules_store.go Persists Rules — Schema Analysis

### Storage Backend
Rules are stored in **SQLite via the ent ORM**. The ent schema is defined in `session/ent/schema/approvalrule.go`. Persistence is row-per-rule (not a JSON blob). The current columns are:

```
rule_id (unique string)   tool_name       command_pattern   decision (int)
name                      tool_pattern    file_pattern      risk_level (int)
                          tool_category   reason            priority
                          alternative     enabled           source
                          created_at      updated_at
```

### Schema Changes Needed

To persist the new structured fields, new columns must be added to the ent schema:

```go
// In session/ent/schema/approvalrule.go Fields():
field.JSON("programs", []string{}).Optional().Default([]string{}),
field.JSON("subcommands", []string{}).Optional().Default([]string{}),
field.JSON("blocked_subcommands", []string{}).Optional().Default([]string{}),
field.JSON("required_flags", []string{}).Optional().Default([]string{}),
field.JSON("forbidden_flags", []string{}).Optional().Default([]string{}),
field.JSON("required_flag_prefixes", []string{}).Optional().Default([]string{}),
field.JSON("python_modes", []string{}).Optional().Default([]string{}),
field.Bool("safe_python_imports_only").Optional().Default(false),
```

The ent `field.JSON` type stores arrays as a JSON blob column in SQLite — no new join tables are needed. This is the simplest migration path.

Alternatively, store all criteria as a single JSON column `criteria_json TEXT` — simpler but less queryable. Given that there's no use case for querying individual criteria fields at the SQL level, either approach works. The JSON-per-field approach matches the existing column-per-field style of the schema and is preferable for clarity.

### Migration Path

ent auto-generates migrations via `entgo.io/ent/schema`. After adding the new fields:
1. Run `go generate ./session/ent/...` to regenerate ent code.
2. The new columns will be added with `Optional()` and `Default()` — existing rows get empty arrays / false without a manual data migration.
3. The ent `UpsertRule` call in `ent_repository.go` must be extended to `Set*()` the new fields.
4. `AllRules()` must map the new columns back to `ApprovalRuleData`.

### ApprovalRuleData Extension

`session/repository.go` `ApprovalRuleData` struct needs the new fields:

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

## 4. Safest Approach to Adding Structured Fields Without Breaking Existing Rules

### Key Safety Properties

1. **All new proto fields are optional with zero defaults**: `repeated string` fields default to empty slice; `bool` defaults to false. Existing rules round-trip through the new proto without change.

2. **CommandPattern remains operative**: When `programs` etc. are empty, the classifier falls through to `CommandPattern` regex matching as before. The new `Criteria` field on `classifier.Rule` is only populated when at least one structured field is non-empty — no behavior change for existing rules.

3. **AND semantics between old and new**: `classifier.matchesRule()` already evaluates `CommandPattern` AND `Criteria` independently (both must match if set). A rule with both set is stricter — users must understand this. The UI should prevent simultaneous use of structured criteria and raw `commandPattern` to avoid confusion (show a warning or mutually exclusive mode selection).

4. **Source filter still works**: The `Source: "user"` constraint on `RulesStore.Upsert` is unchanged — seed rules cannot be modified via the API.

5. **Export to JSON file** (`exportRulesLocked`) must also serialize the new fields — `RuleSpec` must include them for the standalone hook to consume.

### Migration for Existing User Rules

Existing user rules have `programs: []`, etc. They continue to work via `commandPattern` regex. Users can optionally edit them to add structured criteria. No forced migration.

## 5. Ent Schema Migration: What Is Needed

**Yes, a new ent schema migration is required** to add columns for structured criteria.

### Columns Needed

| Column | SQL Type | ent type | Default |
|--------|----------|----------|---------|
| `programs` | TEXT (JSON array) | `field.JSON("programs", []string{})` | `[]` |
| `subcommands` | TEXT (JSON array) | `field.JSON("subcommands", []string{})` | `[]` |
| `blocked_subcommands` | TEXT (JSON array) | `field.JSON(...)` | `[]` |
| `required_flags` | TEXT (JSON array) | `field.JSON(...)` | `[]` |
| `forbidden_flags` | TEXT (JSON array) | `field.JSON(...)` | `[]` |
| `required_flag_prefixes` | TEXT (JSON array) | `field.JSON(...)` | `[]` |
| `python_modes` | TEXT (JSON array) | `field.JSON(...)` | `[]` |
| `safe_python_imports_only` | BOOLEAN | `field.Bool(...)` | `false` |

All columns use `.Optional()` and `.Default(...)` so existing rows are not affected.

### Files to Modify

| File | Change |
|------|--------|
| `session/ent/schema/approvalrule.go` | Add 8 new field definitions |
| `session/ent/` (generated) | Re-run `go generate` |
| `session/repository.go` | Add 8 new fields to `ApprovalRuleData` |
| `session/ent_repository.go` | Extend `AllRules()` mapping + `UpsertRule()` `Set*()` calls |
| `server/services/rules_store.go` | Add new fields to `RuleSpec`; extend `specsToRules()` to populate `Criteria` |
| `server/services/rules_service.go` | Extend `specToProto()` and `ruleToSpec()` to round-trip new fields |
| `proto/session/v1/types.proto` | Add fields 20–28 to `ApprovalRuleProto` |
| Generated proto Go/TS code | Re-run `make generate-proto` |
| `web-app/src/lib/hooks/useApprovalRules.ts` | Add new fields to `create(ApprovalRuleProtoSchema, {...})` call |

### Dependency Order

1. Proto extension (types.proto) → regenerate proto code
2. Ent schema extension → regenerate ent code
3. Domain model extension (ApprovalRuleData, RuleSpec)
4. Storage layer (ent_repository.go)
5. Service layer (rules_store.go, rules_service.go)
6. Frontend hook + form
