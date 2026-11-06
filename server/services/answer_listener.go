package services

import "context"

// AnswerListener is called after a guidance request is answered successfully.
// server/services owns the type and wiring sets the function, so the wake
// dispatcher can react to an answer without server/services importing
// server/wakes (ADR-006 rule 3; precedent: AdmissionGate in server/workflows).
type AnswerListener func(ctx context.Context, guidanceRequestID string)
