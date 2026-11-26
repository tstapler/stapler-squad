# Feature Flag Flip: `attention_ack_bypass_fix` — Stack Research

## 1. Exact files and line numbers to change

### Primary change: `server/services/feature_flag_specs.go` lines 320–323

Current state (no `Default` field, so `Default: false` by zero value):
```go
{
    Name:        attentionAckBypassFixFlagName,
    Description: "Attention ack-bypass fix: an acknowledged item no longer reappears or hides a newer question because of unrelated activity. Applies at once; global only. Default: off.",
},
```

Required change — add `Default: true` and update the description string to say "Default: on":
```go
{
    Name:        attentionAckBypassFixFlagName,
    Description: "Attention ack-bypass fix: an acknowledged item no longer reappears or hides a newer question because of unrelated activity. Applies at once; global only. Default: on.",
    Default:     true,
},
```

Convention confirmed from other on-by-default flags (e.g. `programCLIFlagProbeFlagName` at line 15–19,
`ChangelogFlagName` at lines 25–28): they carry `Default: true` and say "Default: on." in the description.

### Required test update: `server/services/feature_flags_attention_test.go`

`TestGetFeatureFlag_Should_ReturnFalseWithRegistryEntry_When_FreshInstanceForEachOfSixFlags` (lines 21–38)
explicitly asserts that all six attention flags are off by default:

- **Line 29**: `require.False(t, f.Default)` — must become `require.True` (or be removed from the loop
  for `attentionAckBypassFixFlagName`)
- **Line 34**: `require.False(t, testFlagRegistry().Default(name))` — same
- **Line 37**: `require.Equal(t, AttentionFlags{}, LoadAttentionFlags(testFlagRegistry()))` — must become
  `require.Equal(t, AttentionFlags{AckBypassFix: true}, LoadAttentionFlags(testFlagRegistry()))`

The simplest fix is to split the loop: keep `attentionAckBypassFixFlagName` in a separate sub-test that
asserts `require.True`, and remove it from the existing loop that asserts `require.False`. Also rename
the test function to reflect that only five flags remain off by default.

## 2. How `Spec.Default` works

`featureflags.Spec` (`server/services/featureflags/registry.go:16–23`) has a `Default bool` field.

```go
type Spec struct {
    Name        string
    Description string
    Default bool   // what the flag resolves to when never explicitly persisted
    Scopes  []string
}
```

`Registry.Enabled(cfg, name)` calls `cfg.GetFeatureFlagWithDefault(name, r.Default(name))`, so
`Spec.Default = true` makes the flag resolve to `true` for every installation that has never
explicitly set it — no env vars, no config migration needed. A flip takes effect immediately
(no restart), since it is "read from config at decision time" per the comment in `feature_flags_attention.go`.

## 3. Test files that need updating

| File | Lines affected | What to change |
|---|---|---|
| `server/services/feature_flags_attention_test.go` | 21–38 | Split loop: `AckBypassFix` asserts `true`; remaining five assert `false`; fix line 37's `AttentionFlags{}` baseline |
| `server/services/feature_flag_specs.go` | 320–323 | Add `Default: true`, update description string |

No other test files assert the default value directly:
- `config_surface_proof_test.go:206` only records `attention_ack_bypass_fix` as reachable via
  `UpdateFeatureFlag` — it does not test the default.
- `config_surface_proof_flags_test.go:150` only verifies the flag is readable via `AttentionFlags.AckBypassFix` —
  does not test the default.

## 4. Constant and wiring references (read-only context, no changes needed)

- Constant: `attentionAckBypassFixFlagName = "attention_ack_bypass_fix"` at
  `server/services/feature_flags_attention.go:14`
- Resolver wire: `AckBypassFix: on(attentionAckBypassFixFlagName)` at
  `server/services/feature_flags_attention.go:40`
- Suppression gate using this flag: `session/review_queue_suppression.go:131` (`ackSnooze` rule)
