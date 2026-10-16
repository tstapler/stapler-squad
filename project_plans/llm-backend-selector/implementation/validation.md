# Validation
| AC | Coverage |
|---|---|
| 1 | unit: each backend satisfies interface, Caps table test |
| 2 | unit: settings update changes Resolve without restart; grep-gate no os.Getenv |
| 3,4 | unit/integration: call sites use selector; ast-grep check for remaining `LookPath("claude")` |
| 5,7 | table test: capability/availability fallback with reason |
| 6 | table test: model maps |
| 8 | fake CostSink test per backend |
| 9 | RPC test + Playwright e2e for panel |
| 10 | test: capacity probe independent of selector |
| 11 | make ci |
## Pre-mortem
Top risks: silent quality loss (mitigate: opt-in, audit log); resume miscategorised (capability test); consolette model names unverified (verify on live router before shipping).
