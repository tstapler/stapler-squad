package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tstapler/stapler-squad/session/tymux"
)

func TestStartTymuxdSupervisor_MonitorsNonfatalStartupFailure(t *testing.T) {
	t.Parallel()
	var worker func(context.Context)
	monitorCalls := 0
	err := startTymuxdSupervisor(context.Background(), tymux.DaemonConfig{}, false, true,
		func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
			return tymux.TymuxdReady{}, errors.New("offline")
		},
		func(string, func(context.Context) error) { t.Fatal("must not register stop for failed spawn") },
		func(_ string, fn func(context.Context)) { worker = fn },
		func(context.Context, tymux.DaemonConfig) { monitorCalls++ },
	)
	require.NoError(t, err)
	require.NotNil(t, worker)
	worker(context.Background())
	require.Equal(t, 1, monitorCalls)
}

func TestStartTymuxdSupervisor_StrictFailureDoesNotStartMonitor(t *testing.T) {
	t.Parallel()
	err := startTymuxdSupervisor(context.Background(), tymux.DaemonConfig{}, true, true,
		func(context.Context, tymux.DaemonConfig) (tymux.TymuxdReady, error) {
			return tymux.TymuxdReady{}, errors.New("offline")
		},
		func(string, func(context.Context) error) { t.Fatal("unexpected stop") },
		func(string, func(context.Context)) { t.Fatal("unexpected monitor") },
		func(context.Context, tymux.DaemonConfig) { t.Fatal("unexpected monitor") },
	)
	require.Error(t, err)
}
