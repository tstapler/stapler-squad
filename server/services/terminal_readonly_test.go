package services

// Story 5.1 (PR 5, streams): a hidden session's terminal stream refuses input
// and resize at every stream site. The capture-pane, shell and fallback rows are
// observed on the tmuxInputSender seam, never on an exec the test cannot see, so
// a "0 calls" assertion cannot pass vacuously: each row also asserts the visible
// attach reaches the same seam.
