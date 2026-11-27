package services

// Differential golden for SessionService.ScanLeaseWedges (event bytes and gate
// counter), recorded from origin/main before the logic moved.

import (
	"context"
	"flag"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/server/deliverygate"
	"github.com/tstapler/stapler-squad/session/domain/backlogdto"
	"github.com/tstapler/stapler-squad/session/instance"
)

var updateLeaseWedgeGolden = flag.Bool("update-lease-wedge-golden", false, "rewrite testdata/lease_wedge.golden")

var leaseWedgeAcquisitionRe = regexp.MustCompile(`(lease-wedge-uuid-golden-[a-z]+)-\d+`)

func TestLeaseWedgeGolden(t *testing.T) {
	var out strings.Builder
	for _, name := range []string{"hidden", "visible", "linked"} {
		e := newCrashEnv(t, true)
		uuid := "uuid-golden-" + name
		inst := wedgeTarget(t, uuid)
		inst.Hidden = name != "visible"
		itemID := ""
		if name == "linked" {
			ctx := context.Background()
			item, err := e.svc.concStorage.CreateBacklogItem(ctx, backlogdto.BacklogItemData{Title: "wedge item", Status: "in_progress"})
			require.NoError(t, err)
			_, err = e.svc.concStorage.CreateItemSession(ctx, backlogdto.ItemSessionData{ItemID: item.ID, SessionUUID: uuid, SessionRole: instance.SessionRoleWork})
			require.NoError(t, err)
			itemID = item.ID
		}
		e.svc.DeliveryGate().Index().Replace([]deliverygate.Entry{
			{UUID: uuid, Title: inst.Title, TmuxName: "ssq_golden_" + name, Hidden: inst.Hidden, Kind: deliverygate.KindReview},
		})
		lease, ok := inst.TryTerminalWriteLease(instance.LeaseWriterReply)
		require.True(t, ok)

		// ScanLeaseWedges sweeps every lease in the process, so only this
		// instance's events are recorded.
		e.svc.ScanLeaseWedges(time.Now().Add(instance.LeaseWedgeWarnAfter + time.Second))
		lease.Release()

		fmt.Fprintf(&out, "=== %s\n", name)
		for _, ev := range e.collect(t) {
			if ev.SessionID != uuid {
				continue
			}
			keys := make([]string, 0, len(ev.NotificationMetadata))
			for k, v := range ev.NotificationMetadata {
				keys = append(keys, k+"="+v)
			}
			slices.Sort(keys)
			line := fmt.Sprintf("%s session=%q id=%q type=%d prio=%d title=%q msg=%q ctx=%q meta=%v",
				ev.Type, ev.SessionID, ev.NotificationID, ev.NotificationType, ev.NotificationPriority,
				ev.NotificationTitle, ev.NotificationMessage, ev.Context, keys)
			if itemID != "" {
				line = strings.ReplaceAll(line, itemID, "<item>")
			}
			out.WriteString(leaseWedgeAcquisitionRe.ReplaceAllString(line, "${1}-<n>") + "\n")
		}
		assert.Positive(t, e.svc.DeliveryGate().Metrics().Total(deliverygate.CounterLeaseWedgeNotified))
	}
	const path = "testdata/lease_wedge.golden"
	if *updateLeaseWedgeGolden {
		require.NoError(t, os.WriteFile(path, []byte(out.String()), 0o600))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, string(want), out.String())
}
