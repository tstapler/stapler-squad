package services

// Story 5.1.3: runtime proofs for the stage and transition CRUD request fields.
// Each proof flips the field through the production RPC handler on a real
// BacklogService (enfService) and observes a different answer from either the
// operator transition handler or the graph validator the handlers run. Helpers
// carry the bkf prefix to stay clear of other test files.
