# Session lifecycle

Generated from `session/state_machine.go`'s `transitionDefs`. Regenerate with:

```sh
go test ./session -run '^TestSessionLifecycleDiagram$' -update-session-lifecycle-diagram
```

```mermaid
%%{init: {'theme': 'base', 'themeVariables': {'fontFamily': 'ui-sans-serif, system-ui'}}}%%
stateDiagram-v2
    Active --> Crashed
    Active --> Hibernated
    Active --> Paused
    Active --> Stopped
    Crashed --> Active
    Crashed --> Stopped
    Creating --> Active
    Creating --> Failed
    Creating --> Stopped
    Failed --> Creating
    Hibernated --> Active
    Hibernated --> Stopped
    Paused --> Active
    Paused --> Stopped
    Stopped --> Active
```

This graph shows table-validated lifecycle transitions, not every status write. `Restoring` and `PermanentlyFailed` are statuses outside this transition table. Detected agent states (processing, idle, approval/input required), retry budgets, and `ArchivedAt` are separate dimensions; they are not nodes in this lifecycle graph. Edges describe legality, not permission to auto-revive an archived session.
