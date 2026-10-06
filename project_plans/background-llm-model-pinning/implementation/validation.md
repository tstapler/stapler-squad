# Validation
| Requirement | Test |
|---|---|
| Default pipeline pins Sonnet for work/review | unit on spawn path asserts programOverride has --model sonnet ID |
| Headless features default Haiku | fake runner captures args per feature |
| Opus only explicit | resolver table test |
| Live change | config update test then next call uses new value; no restart |
| Non-claude safe | resolver test program=aider -> "" |
| Fallback on bad value | resolver test + warn log |
Pre-mortem: stale hash false drift; pool sessions reused across models (key by model); panel string-setting plumbing larger than estimated.
