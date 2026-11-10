package services

// Story 5.1.3: runtime proofs for the transition-gate CRUD request fields. Each
// proof flips the field through CreateTransitionGate or UpdateTransitionGate on
// a real BacklogService and observes the operator transition handler's answer
// (allowed, or the text of the first unsatisfied gate it names).
