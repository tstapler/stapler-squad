package wakerpc

// Differential golden for the operator wake-trigger and peer-help surface,
// recorded before the package move; the body is unchanged by it.
// Regenerate with -update-wake-golden.

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/services/internal/svctest"
	"github.com/tstapler/stapler-squad/server/wakes"
	"github.com/tstapler/stapler-squad/session/domain"
)

var updateWakeGolden = flag.Bool("update-wake-golden", false, "rewrite testdata/wake.golden")

func TestWakeGolden(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	at := time.Date(2026, 3, 4, 5, 6, 7, 0, time.UTC)

	svc := NewWakeTriggerService(WakeTriggerDeps{Titles: func(id string) string {
		if id == "untitled" {
			return ""
		}
		return "title-" + id
	}})
	add("default enabled=%v clock=%v", svc.deps.Enabled(), svc.deps.Now != nil)
	add("title %s %s", svc.title("x"), svc.title("untitled"))
	add("reminder %q %q %d", reminderText(""), reminderText("label"), len([]rune(reminderText(strings.Repeat("é", 300)))))
	add("cancel text %q %q", cancelText(wakes.OperatorCancelReason), cancelText(""))

	row := &domain.WakeTriggerData{
		ID: "w1", OwnerSessionUUID: "owner", Type: domain.WakeTriggerType("at"), Spec: domain.AtSpec{At: at},
		Action: domain.WakeAction{Kind: domain.WakeActionNotifyOperator, Payload: "msg"},
		Status: domain.WakeStatusDelivered, EndReason: domain.WakeReasonExpired, CreatedAt: at,
		NextFireAt: &at, DeliveredAt: &at, EndedAt: &at,
	}
	add("info live %s", svctest.CanonicalProto(t, svc.infoOf(row, wakes.OwnerFacts{Exists: true, Archived: true})))
	add("info gone %s", svctest.CanonicalProto(t, svc.infoOf(row, wakes.OwnerFacts{})))

	facts := func(id string) wakes.OwnerFacts {
		return wakes.OwnerFacts{Exists: id != "gone", Live: id != "gone", WorkspaceKey: "repo"}
	}
	peer := func(answerer string, kind domain.AnswererKind, mutate func(*domain.GuidanceRequestData)) {
		r := &domain.GuidanceRequestData{ID: "g", AskerSessionUUID: "asker", AnswererSessionUUID: answerer, AnswererKind: kind, QuestionText: "q", CreatedAt: at}
		mutate(r)
		add("peer %s %s %s -> %s", answerer, kind, r.Status(), svctest.CanonicalProto(t, svc.peerInfoOf(r, facts)))
		add("ended %s %s %s -> %s", answerer, kind, r.Status(), svc.alreadyEnded(r))
	}
	for _, answerer := range []string{"peer", "gone"} {
		peer(answerer, domain.AnswererKindSession, func(*domain.GuidanceRequestData) {})
		peer(answerer, domain.AnswererKindSession, func(r *domain.GuidanceRequestData) { r.CancelledAt, r.CancelReason = &at, wakes.OperatorCancelReason })
		for _, kind := range []domain.AnswererKind{domain.AnswererKindSession, domain.AnswererKindHuman} {
			peer(answerer, kind, func(r *domain.GuidanceRequestData) { r.AnsweredAt, r.Answer, r.NotifiedAt = &at, "yes", &at })
		}
	}

	res, err := svc.CancelWakeTrigger(ctx, connect.NewRequest(&sessionv1.CancelWakeTriggerRequest{Id: "not-a-uuid"}))
	add("cancel bad id -> %s %s", svctest.CanonicalProto(t, res.Msg), svctest.CanonicalError(err))
	action, err := svc.CancelWake(ctx, "not-a-uuid")
	add("action cancel wake -> %s %q %s", action.Outcome, action.Detail, svctest.CanonicalError(err))
	action, err = svc.CancelPeerHelp(ctx, "any")
	add("action cancel peer unwired -> %s %q %s", action.Outcome, action.Detail, svctest.CanonicalError(err))
	for label, req := range map[string]*sessionv1.CreateOperatorReminderRequest{
		"missing":    {},
		"past":       {SessionUuid: "s", FireAt: timestamppb.New(at.AddDate(-20, 0, 0))},
		"no-backend": {SessionUuid: "s", FireAt: timestamppb.New(at.AddDate(80, 0, 0))},
	} {
		res, err := svc.CreateOperatorReminder(ctx, connect.NewRequest(req))
		msg := ""
		if err == nil {
			msg = svctest.CanonicalProto(t, res.Msg)
		}
		add("remind %s -> %s %s", label, msg, svctest.CanonicalError(err))
	}

	slices.Sort(out)
	svctest.CompareGolden(t, filepath.Join("testdata", "wake.golden"), out, *updateWakeGolden)
}
