# Plan
1. Proto: `ProbeProgram` RPC + `FlagInfo` message; proto-gen; registry-generate.
2. `config/clihelp` package: `Parse(helpText) []Flag` + fixtures + tests.
3. `Probe(ctx, executor, command)`: LookPath, run `--help` w/ timeout, size cap, process-group kill, cache.
4. DefaultsService handler `ProbeProgram` + tests with fake executor.
5. Frontend hook `useProgramProbe(command)` (debounced, on blur).
6. ProgramsManager: found/not-found badge, flag autocomplete (datalist), unknown-flag warning, tooltips (tap-friendly).
7. Session creation UI: same warning/validation via hook.
8. Tests: jest for hook/component; Go race tests; e2e spec w/ `// @feature` header.
9. Docs: how-to note in docs/reference.
Adversarial notes: probe of TUI tools that hang -> timeout/kill; flags in help differing from real accepted flags -> warning only, never blocking; jscpd threshold margin small (0.10% vs 0.12%) -> share test mocks.
