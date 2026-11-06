# Build vs buy
Build in-repo: small policy function + config accessors. No library applies (sha256 from stdlib; semaphore already exists). Batching N items into one call is deferred: needs a new prompt/parse contract and per-item artifact dirs — high risk, not required by the 4 ACs.
