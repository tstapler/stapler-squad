package stagecfg

// StageDefinition is this package's plain-data view of one stage graph node.
// Field names/semantics mirror ent's BacklogStage schema
// (session/ent/schema/backlog_stage.go) exactly.
type StageDefinition struct {
	Slug       string
	IsEntry    bool
	IsTerminal bool
}

// TransitionDefinition is this package's plain-data view of one (fromStage,
// toStage) edge, mirroring ent's StageTransition schema
// (session/ent/schema/stage_transition.go). GateCount is the number of gates
// attached to this edge — Story 2.6.2's cycle-escape lint only needs "does
// this edge have at least one gate," never gate kind/config, so a count
// (rather than a slice of gate detail) keeps this struct storage-free and
// small.
type TransitionDefinition struct {
	FromSlug  string
	ToSlug    string
	Enabled   bool
	GateCount int
}
