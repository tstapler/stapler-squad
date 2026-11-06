# Build vs buy
- Adopt upstream: not viable; fix is on v6 main only, v5.19.x lacks it and stapler-squad pins the v5 fork.
- Stock git CLI fallback only: rejected; defeats the in-process go-git path (perf) and the fork's purpose.
- Verdict: patch the existing fork (3-line fix + tests), tag ssq.2. Recommended.
