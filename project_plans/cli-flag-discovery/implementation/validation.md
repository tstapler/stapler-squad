# Validation
| AC | Coverage |
|---|---|
| 1,2 | Go handler test, fake CommandExecutor (found/missing) |
| 3 | Go tests: timeout, output cap, cache hit/invalidate on mtime |
| 4 | Parser table tests with fixture help texts |
| 5,6,9 | Jest ProgramsManager tests; tap tooltip |
| 7 | Jest for session-creation component |
| 8 | Go test with sleeping script binary |
| 10 | make ci, jest |
Pre-mortem: hanging binaries, parser false positives (warn-only), proto-gen not committed (gitignored gen/), jscpd ratchet.
