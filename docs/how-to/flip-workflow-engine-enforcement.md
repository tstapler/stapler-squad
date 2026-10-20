# Flip workflow engine enforcement (operator, then agent, then system)

The backlog workflow engine decides which status transitions are allowed from the stage graph and gates you edit in Settings > Backlog Stages. Until you enforce a caller class, that class keeps the built-in rules and your stage edits have no effect on it. This page takes you through enforcing one class at a time, on evidence, and rolling back.

**Before deploying the binary that contains this change, confirm the deployed unit passes `--tmux-keep-server`.** Otherwise the restart kills every live tmux session. See [tmux-keep-server-on-restart](../explanation/tmux-keep-server-on-restart.md).

```bash
# Linux
grep -o -- '--tmux-keep-server' ~/.config/systemd/user/stapler-squad.service
# macOS
grep -A1 -- '--tmux-keep-server' ~/Library/LaunchAgents/*.plist
```

The command must print the flag. If it does not, fix `scripts/install-service.sh` as that page describes, not the deployed unit.

## Review dates

Fill this in when the binary first deploys. The review is a dated task, not a hope: the engine reports the date itself, but only you act on it.

```text
first deploy: <YYYY-MM-DD>
review due:   <first deploy + 14 days>
```

Calendar reminder line to paste:

```text
Review workflow engine enforcement: run make workflow-engine-soak-status, then the Outcome review in docs/how-to/flip-workflow-engine-enforcement.md
```

The service computes the same date as 14 days after the earliest `binary_started_at` in the shadow ledger. `make workflow-engine-soak-status` prints `REVIEW DUE <date>`. After that date, while no class is enforced, it also prints `REVIEW OVERDUE: no class is enforced; run the Outcome review`. This line is advisory and never changes the exit code. The Features page shows the same date from `review_due_at` in the status RPC.

## Concepts

### Caller classes and flags

Every status write belongs to one of three caller classes, each with its own enforce flag. All flags are live-settable in Settings > Features (`/settings/features`) and take effect in about 1 second. They are global only.

| Class | Who | Enforce flag | Default |
|---|---|---|---|
| `operator` | A human through the UI or an RPC | `backlog:workflow-engine-enforce` | off |
| `agent` | An AI session through MCP tools (`request_review`, `report_blocked`, `resume_work`, `report_duplicate`, `report_pr_created`, and the generic transition tool) | `backlog:workflow-engine-enforce-agent` | off |
| `system` | The server itself: queue claim, triage, lifecycle sweeps, PR and Jules pollers, archive, dispatch | `backlog:workflow-engine-enforce-system` | off |

An agent's `trigger_triage` MCP call is a `TriggerTriage` RPC classed `system`, so `-enforce-system` governs it, not `-enforce-agent`.

The shadow flag `backlog:workflow-engine-shadow` is on by default. Each class resolves to one of three modes:

- **Off**: the built-in rules apply, nothing is compared. This is today's behavior.
- **Shadow**: the built-in rules still decide. The engine also computes its own answer and counts every difference. This is what accrues evidence.
- **Enforce**: the configured graph and gates decide for that class.

Enforce wins over Shadow. With all three enforce flags off and shadow on, every class is in Shadow. With all four flags off, the engine is off.

The Features rows have the ids `flag-row-backlog-workflow-engine-shadow`, `-enforce`, `-enforce-agent` and `-enforce-system`. Link to one with `/settings/features#flag-row-backlog-workflow-engine-enforce-agent`.

### Where to look

| What | Where |
|---|---|
| Soak verdict per class, config diff | `make workflow-engine-soak-status` (reads `http://localhost:8543/actuator/workflow-engine`) |
| Class modes, 24 hour differences, verdict, stranded count, review date | `GetWorkflowEngineStatus` RPC; shown on the Stages page banner and in the Enforce dialog |
| Raw counters, JSON | `curl -s http://localhost:8543/actuator/workflow-engine` |
| Redacted repro records for each divergence | The path printed on an alarm line (`repro records: <path>`) |

Set `SSQ_URL` to read another instance, for example `SSQ_URL=http://localhost:62871 make workflow-engine-soak-status`. `/actuator` sits behind the same authentication as `/actuator/metrics`. A 401 makes the script exit 2.

### Soak parameters

The service holds these values as constants (`session/workflow_engine_soak.go`), so a verdict cannot be tuned to pass. The owner approved them on 2026-10-17.

| Parameter | Value |
|---|---|
| Window W, `operator` and `agent` | 7 days of history |
| Window W, `system` | 14 days of history |
| N, samples per expected edge | 20 shadowed `path=transition` calls |
| Floor, per class | 100 `path=transition` calls |
| Review date | 14 days after the earliest binary start |

The verdict for a class is READY only when all of these hold:

1. The class has evidence. Zero recorded calls is NOT READY (check that shadow is on).
2. History spans at least W.
3. At least the floor of `path=transition` samples exist. Samples on stateful-gate edges do not count.
4. Every expected edge for the class has at least N `path=transition` samples, or you acknowledged it.
5. There are zero `graph`, `gate_missing`, `gate_extra` and `error_kind` differences under `config=seeded`. These alarms fire on either path (`transition` or the legacy path), but READY counts `path=transition` samples only.
6. Every divergent edge is acknowledged when the config is `edited`. A window that spans an edit, or any unlabeled sample, counts as edited.
7. Every edge listed `UNVERIFIED in shadow (not_compared{stateful_gate})` is acknowledged. These are edges with a `human_approval`, `automated_review` or `custom` gate, which shadow cannot fully compare.
8. Every `guard_new` difference is acknowledged per `(caller, from, to)` (see the ack file below).

Lifecycle edges are not exempt. Under the engine they are gated per class, and the `system` soak is its own gate.

### Low-volume classes

N=20 and the floor of 100 are printed as targets. They are not reachable for the `operator` and `system` classes at the live volume (about 21 live items, INFERRED from the plan's review, not measured). For those classes, READY also accepts the four-part low-volume acknowledgement, which waives only the floor and the per-edge N. It requires zero divergence under the seeded config plus your acknowledgement that you reviewed all four artifacts:

1. The `--config-diff` output.
2. The stranded-item preview.
3. The per-caller table: every registered caller driven through the choke point, in `server/services/backlog_gate_classes_test.go`.
4. The replay report from `make workflow-engine-replay` on a copy of the database.

The operator class expects every edge in the graph, so it needs the low-volume acknowledgement or per-edge acknowledgements to read READY at live volume.

## Prepare

### 1. Take the first-hour config check

In the first deployed hour, and again before each flip, print the live stage configuration compared with the built-in seed, plus the stranded-item preview:

```bash
make workflow-engine-soak-status ARGS=--config-diff
```

The preview lists items that would lose every exit, or sit in a disabled stage, under Enforce. If it lists any item, the command exits 1. Do not flip a class until you have decided what to do with each previewed item, and acknowledged it. Fix the stage configuration, or move the item, before the flip.

### 2. Run the replay on a copy of the database

Never point the replay at the live database file. It refuses a path inside a live state directory, and it refuses a copy with a non-empty `-wal` sidecar.

```bash
sqlite3 ~/.stapler-squad/<your database file> ".backup /tmp/workflow-replay.db"
make workflow-engine-replay DB=/tmp/workflow-replay.db
```

Replace `<your database file>` with the SQLite file in your config directory. Exit 0 means no fixed agent or system edge is denied with no gate. Exit 1 means one is, and the output names it. Exit 2 means the `DB` argument is missing. The output includes `go test` lines such as `=== RUN`.

The replay marks `ready->pr_pending` and `idea->pr_pending` as `n/a:no_stored_transition`. Those system-required edges have no stored stage transition, so no configuration edit can attach an operator gate to them. For those edges the guarantee holds at the graph level only.

### 3. Write the acknowledgement file

`make workflow-engine-soak-status` takes an owner acknowledgement file through `ARGS="--ack-file <path>"`. The file never changes server state. The script posts it with the request, and the verdict is computed for that request only. The server does not store acknowledgements, so the Features page and the Stages banner (which read the status RPC) never see them. They show NOT READY or NEEDS_OWNER_ACK for a class that needs one, even when the script reads READY. The Enforce dialog's own checkbox is your acknowledgement there.

The file is JSON with one object per class under `classes`. The server rejects an unknown field or class with HTTP 400, and the script then exits 2, so a typo cannot silently acknowledge nothing. The file is limited to 64 KB.

```json
{
  "classes": {
    "operator": {
      "edges": ["idea->ready"],
      "divergent_edges": [],
      "stateful_edges": ["review->done"],
      "guard_new": ["operator_submit_manual_review|review->done"],
      "low_volume": {
        "config_diff": true,
        "stranded_preview": true,
        "caller_table": true,
        "replay_report": true
      }
    }
  }
}
```

| Field | Meaning | Entry format |
|---|---|---|
| `edges` | An expected edge you accept with fewer than N samples | `from->to` |
| `divergent_edges` | An edge with a divergence under an edited config that matches a `--config-diff` entry you reviewed | `from->to` |
| `stateful_edges` | A stateful-gate edge you accept as unverified in shadow | `from->to` |
| `guard_new` | A newly governed `(caller, from, to)` where a Default guard that the legacy path never asked now applies | `caller\|from->to` |
| `low_volume` | The four-part acknowledgement above; all four must be `true` to count | booleans |

`guard_new` is not agent-only. Under the seeded config it appears for the agent caller `mcp_request_review` (`in_progress->done`, `pr_pending->done`) and for the operator caller `operator_submit_manual_review` (`review->done`). System callers never produce it. A `guard_new` difference is never waived by the low-volume acknowledgement. Each one needs its own entry.

Run the verdict with the file:

```bash
make workflow-engine-soak-status ARGS="--ack-file $HOME/workflow-engine-acks.json"
```

Without `--class`, the target checks `operator`, `agent` and `system` in turn and exits with the worst code. The output prints the reasons a class is not READY. The unacknowledged `guard_new` lines name the exact `caller|from->to` string to add.

Exit codes:

| Exit | Meaning |
|---|---|
| 0 | READY |
| 1 | NOT READY, or NEEDS_OWNER_ACK |
| 2 | Service unreachable, a 401, or a rejected ack file |

Check one class: `make workflow-engine-soak-status ARGS="--class agent --ack-file $HOME/workflow-engine-acks.json"`.

## Flip the operator class

Operator enforcement affects only what you do in the UI and over RPC. Do it first.

1. Run `make workflow-engine-soak-status ARGS="--class operator --ack-file $HOME/workflow-engine-acks.json"` and confirm `verdict=READY` and exit 0.
2. Complete the config diff, stranded preview and replay steps above. Acknowledge every previewed item.
3. In Settings > Features, find "Enforce for operators (this UI)" (Step 1 of 3). Turn it on.
4. The dialog loads the class's readiness evidence: differences in the last 24 hours, the soak verdict, and the stranded count. It re-checks at confirm time. Read it. If it says "Not clean", it requires you to tick the acknowledgement box before Enforce is enabled.
5. Confirm. The change applies within about 1 second.
6. Watch the Stages banner and the stuck-items list during the next day. A denied operator action returns an error to you immediately.

## Flip the agent class

Do this after the operator class has soaked. The Features row (Step 2 of 3) shows an advisory, not a block, if operators are not enforced yet.

1. Acknowledge the agent class per `(caller, from, to)` `guard_new` edge in the ack file. The agent flag must not be flipped until the agent verdict reads READY with those acknowledgements.
2. Run `make workflow-engine-soak-status ARGS="--class agent --ack-file $HOME/workflow-engine-acks.json"` and confirm exit 0.
3. Turn on "Enforce for agents (MCP tools)" in Settings > Features and read the evidence dialog as above.

Under Enforce, a denied MCP call returns a `TRANSITION_DENIED` JSON result to the agent with `blocked_by`, `hint` and `remediation`. The server also opens a `transition_denied` stuck item.

## Flip the system class

The system class governs automation: queue claim, triage, lifecycle sweeps, PR and Jules pollers, and archive. Do it last.

Before you flip it:

1. Confirm the deployed binary contains the Story 1.5.8 completeness work. The `backlog:workflow-engine-enforce-system` flag is registered in that story, after the spawn-point guards, the detector-local guards and the re-drive sweep, so it does not exist in earlier binaries. Run its completeness tests on the deployed commit and read the exit status: `go test ./session -run 'TestEveryStuckDetectorIsClassified|TestDeniedGuards_should_AllShareOnePredicate|TestDetectorLocalGuards_should_CallTheSharedHelper' -count=1` and `go test ./server/services -run 'TestEveryRemediationReasonIsClassified|TestSpawnPointGuards_should_ShareOnePredicateAndCoverEveryRemediationAction' -count=1`.
2. Confirm the system class reads READY in `make workflow-engine-soak-status` with acknowledgements. Its window is 14 days.
3. Confirm the deployed unit passes `--tmux-keep-server` (see the top of this page).
4. Review `--config-diff` and the stranded-item preview for the system edges. An operator gate on any of these parks that automation as a `transition_denied` stuck item:

   - `idea->ready`
   - `queued->in_progress`
   - `in_progress->review`
   - `review->pr_pending`
   - `pr_pending->done`
   - `done->archived`

   Remove the gate, or accept that the item waits for your approval.
5. Know that a flip does not hold in-flight items. A denial opens a row on the item's next attempt, and `ReconcileStuck` finishes it once the gate is satisfied.

Then turn on "Enforce for automation (lifecycle, sync, pollers)" (Step 3 of 3) in Settings > Features and read the dialog.

## Resolve a `transition_denied` item

A denied agent or system transition appears in the stuck-items section as "Automatic transition denied" (`transition_denied`). The row carries the caller, the `from` and `to` statuses and the reason, such as an unsatisfied operator gate or a disabled edge. There is one row per item, so a second caller on the same item overwrites the first caller's context.

Choose one of these:

- **Approve the gate.** Approve the pending gate on the item (the item's gate checklist, or the `RecordGateApproval` RPC). The next attempt finds the gate satisfied.
- **Re-enable the edge.** In Settings > Backlog Stages, enable the transition that was disabled. The editor refuses to disable an edge that an agent tool or system caller can request, including every `*->archived` edge, so this applies to edges you added or changed.
- **Roll the flag back to Shadow.** Turn the class's enforce flag off (see Roll back). The denial no longer holds, and the next guard or sweep resolves the row.

### What automation does while a denial is open

Automation parks quietly behind an open `transition_denied` row. It does not retry noisily, and it does not open duplicate rows. Spawn points and detectors skip the held item. The next `ReconcileStuck` tick (every 60 seconds) re-checks the denial. Once the gate is satisfied or the class is no longer enforced, the tick resolves the row. For the three pure-status callers (`autonomous_complete`, `maybe_trigger_triage`, `triage_complete`), it also performs the transition. Denials from the Default built-in guards (`default_guard`) never hold an item. Snoozing the row in the UI releases the item.

### Override as an operator

A non-empty `override_reason` has different effects by class. Enforce only.

| Caller | What `override_reason` does |
|---|---|
| Operator, `operator_transition_rpc` (the `TransitionBacklogItemStatus` RPC) | Bypasses Configured operator gates, and bypasses the Configured graph as long as the target is a known stage. This is the exit for an item stranded by a disabled edge or stage. The audit note and a manual-override notification record it. Default's five sentinels (`ErrACRequired`, `ErrPlanRequired`, `ErrPlanArtifactsRequired`, `ErrPlanArtifactsMissing`, `ErrUnresolvedBlockers`) still run afterwards. |
| Other operator callers | Forwarded to the Default guard unchanged. Does not bypass the Configured graph. |
| Agent | Accepted only by the Default guard for the exemptions it has today: `review|pr_pending->ready` and `->done`. Never bypasses a Configured gate, the graph, or the five sentinels. The generic MCP transition tool does not carry the field today. |
| System | No override exists. |

## Roll back

Rolling back needs no deploy.

1. In Settings > Features, open the class's row and select "Revert to Off". This sets that class's enforce flag off. The class returns to Shadow when the shadow flag is on, which is the default, and to Off otherwise.
2. Wait about 1 second for the mode to refresh.

To stop all comparison as well, also turn off `backlog:workflow-engine-shadow`. With all four flags off, behavior equals today's.

Open `transition_denied` rows resolve on the next guard or `ReconcileStuck` tick after the class leaves Enforce.

## Outcome review

On the review due date, run these steps and write down the result beside the verdicts. The review is about the factory outcome, not only the counters.

1. Run `make workflow-engine-soak-status ARGS="--ack-file $HOME/workflow-engine-acks.json"` and record each class's verdict and mode.
2. Record the stuck-item rate: the number of distinct items with an open stuck row (from `ListStuckBacklogItems`) divided by the `ListBacklogItems` count. The baseline is 6 of 21 on 2026-10-18.
3. Record the manual interventions of the week: approvals, overrides, snoozes and hand-edited statuses.
4. Compare both numbers with the baseline.
5. If no class is enforced at the review date, decide per class, in writing: flip it, or record why not and set the next review date.

## Related

- [tmux-keep-server-on-restart](../explanation/tmux-keep-server-on-restart.md)
- [manage-systemd-service](manage-systemd-service.md)
- [state-isolation](../reference/state-isolation.md) for where the config directory and database live on an isolated instance
- [flip-git-backend-cohort](flip-git-backend-cohort.md), the same shadow-then-flip pattern for the git backend
