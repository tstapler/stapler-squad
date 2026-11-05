package services

// Story 5.1.4: runtime proofs for the transition-gate `stateful` request field.
// A structural gate with stateful=true latches once a transition commits through
// it, so a later regression of the item no longer denies; stateful=false keeps
// recomputing. A recorded kind is always stateful and rejects an explicit false.
