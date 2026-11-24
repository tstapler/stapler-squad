package hostclaim

import "testing"

func TestVerdictConstructors_should_PairKindWithRecord(t *testing.T) {
	t.Parallel()
	rec := ClaimRecord{ItemDeepLink: "item-1"}

	if v := NewUnclaimedVerdict(); v.Kind != ClaimUnclaimed || v.Record.ItemDeepLink != "" {
		t.Errorf("unclaimed: %+v", v)
	}
	if v := NewIndeterminateVerdict(); v.Kind != ClaimCheckIndeterminate || v.Record.ItemDeepLink != "" {
		t.Errorf("indeterminate: %+v", v)
	}
	if v := NewHeldByOtherVerdict(rec); v.Kind != ClaimHeldByOther || v.Record.ItemDeepLink != rec.ItemDeepLink {
		t.Errorf("held by other: %+v", v)
	}
}
