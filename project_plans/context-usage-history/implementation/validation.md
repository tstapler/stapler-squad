# Validation
AC1 -> repo persistence test + restart test. AC2 -> service test. AC3 -> detector table tests (grew/no-compact, grew/compacted, short, unpriced). AC4 -> parser fixture + repo test. AC5 -> double-parse idempotency test. AC6 -> existing findings/capacity tests green; retention test.
Pre-mortem: compaction format drift (mitigate: fixture + abstain), proto enum drift, write amplification.
