package services

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
	"github.com/tstapler/stapler-squad/server/deliverygate"
)

// gateStatsView is everything GetDeliveryGateStats reports, gathered once.
type gateStatsView struct {
	Snapshot    deliverygate.StatsSnapshot
	Series      []deliverygate.Series
	File        deliverygate.StatsFileStatus
	ExplicitOff []string
}

// explicitOffScopes lists the scopes with a persisted explicit false for the
// gate flag (Stage 3 prerequisite 1). Per-kind scopes join it with Story 2.11.
func explicitOffScopes() []string {
	if v, ok := config.LoadConfig().GetFeatureFlagOverride(config.HiddenSessionGateFeatureFlag); ok && !v {
		return []string{"global"}
	}
	return nil
}

func gateCounterProto(s deliverygate.Series) *sessionv1.DeliveryGateCounter {
	var extra []string
	for k, v := range s.Labels {
		if k != "kind" && k != "class" {
			extra = append(extra, k+"="+v)
		}
	}
	sort.Strings(extra)
	name := deliverygate.ShortCounterName(s.Name)
	if len(extra) > 0 {
		name += "{" + strings.Join(extra, ",") + "}"
	}
	return &sessionv1.DeliveryGateCounter{Counter: name, Kind: s.Labels["kind"], Class: s.Labels["class"], Count: int64(s.Count)}
}

func gateBucketProto(b deliverygate.BucketSnapshot) *sessionv1.DeliveryGateBucket {
	out := &sessionv1.DeliveryGateBucket{
		HourStart: timestamppb.New(b.HourStart), UptimeSeconds: b.UptimeSeconds, GateOnSeconds: b.GateOnSeconds,
		GateOnSecondsByKind: b.GateOnByKind, HiddenEventsSeen: b.Seen, HiddenEventsWhileOn: b.WhileOn,
		RoutineEventsWhileOn: b.RoutineWhileOn,
	}
	for _, c := range b.Counters {
		out.Counters = append(out.Counters, &sessionv1.DeliveryGateCounter{Counter: c.Counter, Kind: c.Kind, Class: c.Class, Count: c.Count})
	}
	return out
}

func gateMutationProto(name string) sessionv1.FlagMutation {
	return sessionv1.FlagMutation(sessionv1.FlagMutation_value["FLAG_MUTATION_"+name])
}

func buildGateStatsResponse(v gateStatsView) *sessionv1.GetDeliveryGateStatsResponse {
	snap := v.Snapshot
	resp := &sessionv1.GetDeliveryGateStatsResponse{
		ProcessStart:     timestamppb.New(snap.ProcessStart),
		UptimeSeconds:    int64(snap.Uptime / time.Second),
		EventsByKind_24H: snap.EventsByKind24h,
		StatsFileStatus: &sessionv1.DeliveryGateStatsFileStatus{
			Loaded: v.File.Loaded, Quarantined: v.File.Quarantined, Writable: v.File.Writable,
		},
		Soak: &sessionv1.DeliveryGateSoak{
			GateOnHours:                snap.Soak.GateOnHours,
			RoutineEventsWhileOn:       snap.Soak.RoutineEventsWhileOn,
			FailureDeliveredWhileOn:    snap.Soak.FailureDeliveredWhileOn,
			NeedsHumanDeliveredWhileOn: snap.Soak.NeedsHumanDeliveredWhileOn,
			ProbeDeliveredWhileOn:      snap.Soak.ProbeDeliveredWhileOn,
			ExplicitOffScopes:          v.ExplicitOff,
			SoakStreakHours:            snap.Soak.SoakStreakHours,
			SoakStreakHoursByKind:      snap.Soak.SoakStreakHoursByKind,
		},
	}
	if !snap.Soak.LastOffFlipAt.IsZero() {
		resp.Soak.LastOffFlipAt = timestamppb.New(snap.Soak.LastOffFlipAt)
	}
	for _, s := range v.Series {
		resp.SinceProcessStart = append(resp.SinceProcessStart, gateCounterProto(s))
	}
	sort.Slice(resp.SinceProcessStart, func(i, j int) bool {
		a, b := resp.SinceProcessStart[i], resp.SinceProcessStart[j]
		return fmt.Sprint(a.Counter, a.Kind, a.Class) < fmt.Sprint(b.Counter, b.Kind, b.Class)
	})
	for _, b := range snap.Buckets {
		resp.Buckets = append(resp.Buckets, gateBucketProto(b))
	}
	for _, c := range snap.FlagHistory {
		resp.FlagHistory = append(resp.FlagHistory, &sessionv1.DeliveryGateFlagChange{
			Scope: c.Scope, Value: c.Value, At: timestamppb.New(c.At), Mutation: gateMutationProto(c.Mutation),
		})
	}
	return resp
}
