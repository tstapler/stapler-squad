package wakes

import (
	"fmt"
	"strings"

	"github.com/tstapler/stapler-squad/session/domain"
)

// Describe is the operator-facing, plain-language condition of spec ("when PR
// #901 CI passes"), never raw JSON. titleOf names a session by uuid; an "at"
// spec returns "" because the row's next fire time says it. zone is the cron
// zone, "" for every other type.
func Describe(spec domain.WakeSpec, titleOf func(uuid string) string) (condition, zone string) {
	switch s := spec.(type) {
	case domain.CronSpec:
		zone, expr := cronParts(s.Expr)
		return "on schedule " + expr, zone
	case domain.BacklogEventSpec:
		return describeBacklog(s), ""
	case domain.PREventSpec:
		return fmt.Sprintf("when PR #%d (%s) is %s", s.Number, s.Repo, s.Kind), ""
	case domain.CIEventSpec:
		return fmt.Sprintf("when PR #%d (%s) %s", s.Number, s.Repo, ciPhrase(s.Kind)), ""
	case domain.SessionStateSpec:
		return fmt.Sprintf("when %s is %s", titleOf(s.SessionUUID), s.State), ""
	case domain.GuidanceAnsweredSpec:
		return "when the request it asked is answered", ""
	}
	return "", ""
}

func cronParts(expr string) (zone, rest string) {
	fields := strings.Fields(expr)
	if len(fields) > 0 {
		if name, ok := cronZonePrefix(fields[0]); ok {
			return name, strings.Join(fields[1:], " ")
		}
	}
	return DefaultCronZone, strings.Join(fields, " ")
}

func ciPhrase(kind string) string {
	switch kind {
	case "success":
		return "CI passes"
	case "failure":
		return "CI fails"
	case "completed":
		return "CI finishes"
	}
	return "CI reports " + kind
}

func describeBacklog(s domain.BacklogEventSpec) string {
	name, arg, _ := strings.Cut(s.Kind, ":")
	item := "backlog item " + shortID(s.ItemID)
	switch name {
	case "status_reached":
		return fmt.Sprintf("when %s reaches %s", item, arg)
	case "verdict_recorded":
		return fmt.Sprintf("when %s gets a review verdict", item)
	case "item_archived":
		return fmt.Sprintf("when %s is archived", item)
	case "item_removed":
		return fmt.Sprintf("when %s is removed", item)
	case "session_attached":
		return fmt.Sprintf("when a %s session attaches to %s", arg, item)
	}
	return fmt.Sprintf("when %s has event %s", item, s.Kind)
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
