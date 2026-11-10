package scenario

import "time"

// SessionRecord is a mock session written before boot (Paused or Active only,
// so the review-queue poller does not reclassify it without tmux).
type SessionRecord struct {
	ID, Title, Path, Branch, Status, Program, Category string
	Tags                                               []string
	CreatedAt, UpdatedAt                               time.Time
	Added, Removed                                     int
}

// FlagSetting turns a feature flag on or off after boot.
type FlagSetting struct {
	Name    string
	Enabled bool
}

// ApprovalRule is seeded through UpsertApprovalRule.
type ApprovalRule struct {
	ID, Name, ToolName, CommandPattern, Decision, RiskLevel string
	Priority                                                int
}

// BacklogItem is seeded through CreateBacklogItem then, when Status is not
// "idea", TransitionBacklogItemStatus.
type BacklogItem struct {
	ID, Title, Description, Status string
	Priority                       int
	CreatedAt                      time.Time
}

// Terminal is a session whose program prints a canned transcript.
type Terminal struct {
	ID, Title, Transcript string
}

// Row is one generic fixture row served from a route fixture.
type Row struct {
	ID, Kind, Title, Detail string
	Value                   float64
	Labels                  []string
	At                      time.Time
}

// RouteFixture is data the server cannot be seeded with over RPC; the
// recorder fulfils the named route with it (plan Task 0.1.1f, `seeded: route`).
type RouteFixture struct {
	ID, Route, Seeded string
	Rows              []Row
}

// Bundle is the full data of one or more scenarios.
type Bundle struct {
	Sessions      []SessionRecord
	Flags         []FlagSetting
	ApprovalRules []ApprovalRule
	Backlog       []BacklogItem
	Terminals     []Terminal
	Routes        []RouteFixture
}

func (b *Bundle) append(o Bundle) {
	b.Sessions = append(b.Sessions, o.Sessions...)
	b.Flags = append(b.Flags, o.Flags...)
	b.ApprovalRules = append(b.ApprovalRules, o.ApprovalRules...)
	b.Backlog = append(b.Backlog, o.Backlog...)
	b.Terminals = append(b.Terminals, o.Terminals...)
	b.Routes = append(b.Routes, o.Routes...)
}

// keys returns namespaced ids used for duplicate detection.
func (b Bundle) keys() []string {
	ks := make([]string, 0, len(b.Sessions)+len(b.Flags)+len(b.ApprovalRules)+len(b.Backlog)+len(b.Terminals)+len(b.Routes))
	for _, s := range b.Sessions {
		ks = append(ks, "session:"+s.ID)
	}
	for _, f := range b.Flags {
		ks = append(ks, "flag:"+f.Name)
	}
	for _, r := range b.ApprovalRules {
		ks = append(ks, "approval-rule:"+r.ID)
	}
	for _, i := range b.Backlog {
		ks = append(ks, "backlog:"+i.ID)
	}
	for _, t := range b.Terminals {
		ks = append(ks, "terminal:"+t.ID)
	}
	for _, r := range b.Routes {
		ks = append(ks, "route:"+r.ID)
	}
	return ks
}

// ago is the single way scenarios derive timestamps: a fixed offset before ref.
func ago(ref time.Time, d time.Duration) time.Time { return ref.Add(-d) }
