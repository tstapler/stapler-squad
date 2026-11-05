package schema

import (
	"fmt"
	"strings"
	"time"

	"entgo.io/ent"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"
)

// PullRequestState holds the persisted facts about one pull request, unique per
// canonical (host, owner, repo, number). Links from sessions and backlog items
// live in SessionPR. Its columns after insert are written only by
// EntRepository.UpdatePullRequestState (and ResetVerifyCounters for seeded rows).
type PullRequestState struct {
	ent.Schema
}

var pullRequestStates = []string{"open", "closed", "merged", "unknown"}

// pollErrorKinds is the poll_error enum; "" and "none" both mean no error.
var pollErrorKinds = []string{"", "none", "auth", "rate_limited", "not_found", "repo_inaccessible", "network"}

func oneOf(name string, allowed []string) func(string) error {
	return func(s string) error {
		for _, a := range allowed {
			if s == a {
				return nil
			}
		}
		return fmt.Errorf("%s %q is not one of %v", name, s, allowed)
	}
}

// canonicalLower rejects an upper-case identity column: identity is stored in
// the NewPRRef canonical form, so a raw write of "Acme" is a bug.
func canonicalLower(name string) func(string) error {
	return func(s string) error {
		if s != strings.ToLower(s) {
			return fmt.Errorf("%s %q must be lower-case (canonical PRRef form)", name, s)
		}
		return nil
	}
}

// Fields of the PullRequestState.
func (PullRequestState) Fields() []ent.Field {
	return []ent.Field{
		field.String("host").NotEmpty().Validate(canonicalLower("host")),
		field.String("owner").NotEmpty().Validate(canonicalLower("owner")),
		field.String("repo").NotEmpty().Validate(canonicalLower("repo")),
		field.Int("number").Positive(),
		field.String("url").Default("").Comment("Display URL; keeps the original case"),
		field.String("title").Default("").Comment("PR title as last polled; additive column, empty until the first successful poll"),
		field.String("head_branch").Default(""),
		field.String("base_branch").Default(""),
		field.String("state").Default("unknown").Validate(oneOf("state", pullRequestStates)),
		field.Bool("is_draft").Default(false),
		field.String("check_conclusion").Default(""),
		field.Int("approved_count").Default(0),
		field.Int("changes_req_count").Default(0),
		field.Bool("terminal").Default(false).
			Comment("With state=unknown this is an unverified terminal row seeded by the backfill"),
		field.Time("opened_at").Optional().Nillable(),
		field.Time("merged_at").Optional().Nillable(),
		field.Time("closed_at").Optional().Nillable(),
		field.Time("created_at").Default(time.Now).Immutable().Comment("Age basis of the orphan GC"),
		field.Time("last_checked_at").Optional().Nillable(),
		field.Time("last_polled_at").Optional().Nillable().Comment("The applied observed_at (fetch start), not completion time"),
		field.Int("poll_fail_count").Default(0),
		field.Time("first_fail_at").Optional().Nillable().Comment("Start of the current counted-failure streak"),
		field.String("poll_error").Default("").Validate(oneOf("poll_error", pollErrorKinds)),
		field.Time("rate_limit_reset_at").Optional().Nillable(),
	}
}

// Edges of the PullRequestState.
func (PullRequestState) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("links", SessionPR.Type).Ref("pull_request_state"),
	}
}

// Indexes of the PullRequestState.
func (PullRequestState) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("host", "owner", "repo", "number").Unique(),
	}
}
