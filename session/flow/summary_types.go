package flow

import "time"

// Result types of the aggregator, frozen before the proto is written (Story
// 1.4.2, Task 1.4.2a). They carry no logic. Any later field change here is a
// change to proto/session/v1/backlog_flow.proto in the same PR.

// SectionName names one section of a FlowSummary.
type SectionName string

// Section names, in the order they appear in a FlowSummary.
const (
	SectionTiles     SectionName = "tiles"
	SectionFunnel    SectionName = "funnel"
	SectionCauses    SectionName = "causes"
	SectionReview    SectionName = "review"
	SectionPR        SectionName = "pr"
	SectionStuck     SectionName = "stuck"
	SectionAnomalies SectionName = "anomalies"
	SectionTrends    SectionName = "trends"
	SectionNotes     SectionName = "notes"
)

// SectionStatus is the one section-status enum shared by RPC and MCP.
type SectionStatus int

// SectionStatus values. OK is the zero value.
const (
	SectionOK SectionStatus = iota
	SectionPartial
	SectionFailed
	SectionSkipped
	SectionEmpty
)

// SectionMeta is the status header every section carries. Error is set when
// Status is Failed or Partial. Estimated lists the heuristic metrics in the
// section (a subset of FlowMeta.Estimated).
type SectionMeta struct {
	Status    SectionStatus
	Error     string
	Estimated []string
}

// RowCounts is how many source rows a computation read.
type RowCounts struct {
	Events         int
	Verdicts       int
	StuckIntervals int
	Items          int
}

// FlowMeta describes how a response was computed. CaptureSince is the zero
// time when no trigger capture exists yet. Clamps reports requested values the
// aggregator adjusted (for example a compare end after as-of).
type FlowMeta struct {
	AsOf         time.Time
	WindowStart  time.Time
	WindowEnd    time.Time
	RowCounts    RowCounts
	QueryMs      int64
	Estimated    []string
	SlowPath     bool
	CaptureSince time.Time
	// degraded marks a result with a Failed section; it is never cached (a stable Partial one is).
	degraded bool
	Clamps   []string
}

// ListPage is the shape of every top-N list. HasMore is TotalCount >
// ReturnedCount; Truncated is true when the server clamped the requested limit.
type ListPage[T any] struct {
	Items         []T
	TotalCount    int
	ReturnedCount int
	HasMore       bool
	Truncated     bool
}

// CountMetric is a count with the drill key naming the items behind it.
type CountMetric struct {
	Value int
	Drill DrillKey
}

// RatioMetric is Num/Den, always shown with its denominator. Ratio is 0 when Den is 0.
type RatioMetric struct {
	Num       int
	Den       int
	Ratio     float64
	LowSample bool
	Drill     DrillKey
}

// DurationMetric is a nearest-rank duration summary with its drill key.
type DurationMetric struct {
	Stats Percentiles
	Drill DrillKey
}

// NamedCount is one labelled count in a series or breakdown.
type NamedCount struct {
	Name  string
	Count int
	// Drill names the item set behind Count; empty when none exists.
	Drill DrillKey
}

// TilesSection is the headline numbers.
type TilesSection struct {
	Meta           SectionMeta
	Merged         CountMetric
	LeadTime       DurationMetric
	FirstPassYield RatioMetric
	ThrashAffected CountMetric
	// LeadTimeExcluded counts merged items with no in_progress start.
	LeadTimeExcluded int
	MedianBounces    int
}

// FunnelStageRow is one funnel stage.
type FunnelStageRow struct {
	Status   string
	Entered  CountMetric
	Advanced int
	// BackEdges is the transition total; BackEdgeTargets splits it by target stage.
	BackEdges       int
	BackEdgeTargets []BackEdgeRow
	Dwell           DurationMetric
	// AdvancedDrill names the distinct items behind Advanced.
	AdvancedDrill DrillKey
}

// BackEdgeRow is the back-edge transitions from a stage to one earlier stage;
// Drill names the distinct items that took it.
type BackEdgeRow struct {
	To    string
	Count int
	Drill DrillKey
}

// FunnelSection is the stage funnel plus the thrash dwell bucket.
type FunnelSection struct {
	Meta     SectionMeta
	Stages   []FunnelStageRow
	Other    int
	Archived int
	Thrash   DurationMetric
}

// CauseRow is one rework cause.
type CauseRow struct {
	Cause string
	Count CountMetric
	// ItemsAffected is the distinct items behind Count (the drill set size).
	ItemsAffected int
}

// TriggerRow is one PR-fix trigger count.
type TriggerRow struct {
	Trigger string
	Count   int
}

// CausesSection is the rework causes and the reopen-note classification.
type CausesSection struct {
	Meta          SectionMeta
	Rework        []CauseRow
	PRFixTriggers []TriggerRow
	// BounceBuckets[n] is merged items with n pre-merge review reopens; the
	// last bucket holds ten or more.
	BounceBuckets []int
	ZeroBounce    int
	// ZeroBounceDrill names the zero-bounce items. MergedCount equals
	// tiles.merged and MedianBounces is a statistic over it, so neither has a key.
	ZeroBounceDrill DrillKey
	MergedCount     int
	Notes           []NamedCount
	Unclassified    int
}

// VerdictClassRow is one verdict class.
type VerdictClassRow struct {
	Class   string
	Total   CountMetric
	NonPass int
	// NonPassDrill names the items with a non-PASS verdict of this class.
	NonPassDrill DrillKey
}

// DeadlockRow is one deadlock candidate. Title is the raw item title; the
// transport sanitizes it.
type DeadlockRow struct {
	ItemID               ItemID
	Title                string
	Resolution           string
	Sequence             string
	RunLength            int
	RepeatedCriterion    int
	HasRepeatedCriterion bool
	DiffUnchangedPairs   int
	AgentClaimedDone     bool
	CapExhausted         bool
	InFlight             bool
	LastVerdictAt        time.Time
}

// StreakRow is one reviewer-side UNVERIFIABLE streak.
type StreakRow struct {
	ItemID        ItemID
	Title         string
	Length        int
	LastVerdictAt time.Time
}

// ReviewSection is the review-gate analysis.
type ReviewSection struct {
	Meta         SectionMeta
	Total        CountMetric
	Classes      []VerdictClassRow
	RawFail      RatioMetric
	AdjustedFail RatioMetric
	Override     RatioMetric
	// Unverifiable is raw UNVERIFIABLE verdicts over the raw total.
	Unverifiable    RatioMetric
	Unrecognized    int
	Deadlocks       ListPage[DeadlockRow]
	ReviewerStreaks ListPage[StreakRow]
	StalledCount    int
	StalledDrill    DrillKey
	WeakFilter      string
}

// PRSection is the PR-fix health.
type PRSection struct {
	Meta      SectionMeta
	FixDwell  DurationMetric
	Cycles    CountMetric
	OpenCount int
	// OpenDrill names the items with a PR-fix cycle still open.
	OpenDrill DrillKey
	Notes     []string
	// Outcomes counts nudge outcomes recorded since capture.
	Outcomes PROutcomes
	// TriggerDwell splits dwell by joined trigger for cycles after capture.
	TriggerDwell []TriggerDwellRow
	// Nudges counts user-originated interventions (manual_nudge markers).
	Nudges ManualNudges
}

// ManualNudges is the manual-intervention tally. Recording false means no
// nudge was ever captured (capture is forward-only, no backfill), so the
// counts must not be shown as zeros; CaptureSince is the first nudge.
type ManualNudges struct {
	Recording    bool
	CaptureSince time.Time
	Total        int
	// Weekly has one bucket per ISO week in the window, oldest first.
	Weekly []ThroughputBucket
	// Drill names the items nudged in the window.
	Drill DrillKey
}

// PROutcomes is the PR-fix outcome tally. Recording false means outcome
// capture has not started; the counts are then zero and must not be shown.
type PROutcomes struct {
	Recording         bool
	Steered           int
	Respawned         int
	SkippedActive     int
	ResolvedWithin24h int
}

// TriggerDwellRow is the PR-fix dwell of cycles joined to one trigger.
type TriggerDwellRow struct {
	Trigger string
	Cycles  CountMetric
	Dwell   DurationMetric
}

// StuckReasonRow is one stuck reason.
type StuckReasonRow struct {
	Reason        string
	DistinctItems CountMetric
	Open          int
	Duration      DurationMetric
	LowerBound    bool
}

// StuckSection is current and historical stuck time per reason.
type StuckSection struct {
	Meta    SectionMeta
	Open    int
	Reasons []StuckReasonRow
	Notes   []string
}

// ThrashItemRow is one thrashing item.
type ThrashItemRow struct {
	ItemID   ItemID
	Title    string
	Episodes int
	Events   int
	FirstAt  time.Time
	LastAt   time.Time
	Pair     string
	Cause    string
	// Archived is true when the item's current status is archived.
	Archived bool
}

// AnomaliesSection lists thrash items, which are excluded from the averages.
type AnomaliesSection struct {
	Meta        SectionMeta
	ThrashItems CountMetric
	// ArchivedItems counts all thrashing items (not just the page) that are archived.
	ArchivedItems int
	Items         ListPage[ThrashItemRow]
}

// ThroughputBucket is merged items in one ISO week (Monday 00:00 UTC).
type ThroughputBucket struct {
	WeekStart time.Time
	N         int
	// Drill opens the week's items (DrillBucket of the series key).
	Drill DrillKey
}

// VerdictMixBucketRow is one week of verdict counts by class and outcome.
type VerdictMixBucketRow struct {
	WeekStart time.Time
	N         int
	Drill     DrillKey
	Cells     []VerdictMixCell
}

// VerdictMixCell is the count of one class and outcome in a week.
type VerdictMixCell struct {
	Class   string
	Outcome string
	Count   int
}

// ReworkBucket is one week of rework events by cause.
type ReworkBucket struct {
	WeekStart time.Time
	N         int
	Drill     DrillKey
	ByCause   []NamedCount
}

// TrendsSection feeds the charts. Totals are kept when Compact omits buckets.
type TrendsSection struct {
	Meta            SectionMeta
	Throughput      []ThroughputBucket
	ThroughputTotal int
	VerdictMix      []VerdictMixBucketRow
	VerdictMixTotal int
	Rework          []ReworkBucket
	ReworkTotal     int
}

// NotesSection reconciles raw rows with collapsed counts and states data quality.
type NotesSection struct {
	Meta            SectionMeta
	RawEvents       int
	RealSpans       int
	CollapsedEvents int
	Quality         DataQuality
	Notes           []string
}

// FlowSummary is the sectioned response shared by the UI and MCP.
type FlowSummary struct {
	Meta      FlowMeta
	Tiles     TilesSection
	Funnel    FunnelSection
	Causes    CausesSection
	Review    ReviewSection
	PR        PRSection
	Stuck     StuckSection
	Anomalies AnomaliesSection
	Trends    TrendsSection
	Notes     NotesSection
}

// CompareState says whether a metric could be compared.
type CompareState string

// CompareState values.
const (
	CompareOK           CompareState = "ok"
	CompareIncomparable CompareState = "incomparable"
)

// CompareMetric is one metric over two windows. A and B are raw values; Delta
// is B-A, nil when A is empty or the metric is incomparable (rendered n/a).
// No significance is claimed.
type CompareMetric struct {
	Name   string
	Unit   string
	State  CompareState
	Reason string
	A      float64
	B      float64
	Delta  *float64
	NA     int
	NB     int
	Drill  DrillKey
}

// CompareQuery selects two windows, either by day offsets or by explicit
// half-open bounds [start, end); supplying both forms is an error. Offset
// form: B is the last BDays days ending BOffsetDays before as-of, and A is the
// ADays days ending where B starts.
type CompareQuery struct {
	ADays       int
	BDays       int
	BOffsetDays int
	AStart      time.Time
	AEnd        time.Time
	BStart      time.Time
	BEnd        time.Time
	// AllTime is rejected with ErrCompareAllTime; compare needs bounded windows.
	AllTime bool
	Cohort  string
	Config  Config
}

// CompareResult holds both window summaries and the per-metric comparison.
type CompareResult struct {
	Meta    FlowMeta
	A       *FlowSummary
	B       *FlowSummary
	Metrics []CompareMetric
}

// DrillItem is one item behind a displayed number. Title is raw text. Duration
// is zero when the metric has no per-item duration.
type DrillItem struct {
	ItemID   ItemID
	Title    string
	Status   string
	Reason   string
	Duration time.Duration
}

// DrilldownQuery asks for the items behind Key under Query.
type DrilldownQuery struct {
	Key    DrillKey
	Query  FlowQuery
	Limit  int
	Offset int
}

// DrilldownPage is one page of the items behind a key.
type DrilldownPage struct {
	Key   DrillKey
	Meta  FlowMeta
	Items ListPage[DrillItem]
}

// TimelineEntry is one real transition or one collapsed thrash episode.
type TimelineEntry struct {
	Kind  string // "transition" or "thrash_episode"
	From  string
	To    string
	At    time.Time
	Last  time.Time
	Count int // events in a thrash episode, 1 for a transition
	Cause string
	Note  string // raw note, sanitized by the transport
}

// ItemTimelineQuery selects one item's timeline over a window.
type ItemTimelineQuery struct {
	ItemID ItemID
	Window WindowSpec
	Config Config
}

// ItemTimeline is one item's spans, thrash collapsed, capped at 50 entries.
type ItemTimeline struct {
	Meta           FlowMeta
	ItemID         ItemID
	Title          string
	Status         string
	Entries        []TimelineEntry
	TotalEntries   int
	Truncated      bool
	ThrashEpisodes int
}
