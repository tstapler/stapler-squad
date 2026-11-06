package wakes

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tstapler/stapler-squad/session/domain"
)

func TestDescribe_should_RenderPlainLanguageNeverRawJSON(t *testing.T) {
	t.Parallel()
	title := func(id string) string { return "diagnose:" + id }
	cases := []struct {
		name      string
		spec      domain.WakeSpec
		condition string
		zone      string
	}{
		{"at has no condition text", domain.AtSpec{}, "", ""},
		{"cron default zone", domain.CronSpec{Expr: "0 9 * * *"}, "on schedule 0 9 * * *", "UTC"},
		{"cron named zone", domain.CronSpec{Expr: "CRON_TZ=America/Chicago 0 9 * * *"}, "on schedule 0 9 * * *", "America/Chicago"},
		{"ci passes", domain.CIEventSpec{Repo: "o/r", Number: 901, Kind: "success"}, "when PR #901 (o/r) CI passes", ""},
		{"pr merged", domain.PREventSpec{Repo: "o/r", Number: 7, Kind: "merged"}, "when PR #7 (o/r) is merged", ""},
		{"backlog status", domain.BacklogEventSpec{ItemID: "0123456789abcdef", Kind: "status_reached:review"}, "when backlog item 01234567 reaches review", ""},
		{"session state", domain.SessionStateSpec{SessionUUID: "abc", State: "idle"}, "when diagnose:abc is idle", ""},
		{"guidance", domain.GuidanceAnsweredSpec{GuidanceRequestID: "g"}, "when the request it asked is answered", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			condition, zone := Describe(c.spec, title)
			assert.Equal(t, c.condition, condition)
			assert.Equal(t, c.zone, zone)
		})
	}
}
