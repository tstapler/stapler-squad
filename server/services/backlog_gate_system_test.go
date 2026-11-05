package services

// Story 1.5.6: system-class writers in server/services (and the two operator
// triage writers) go through the transition gate. Each test wires a real
// ConfiguredWorkflowEngine under Enforce for every class and adds an operator
// gate or disables an edge, then drives the production function.
