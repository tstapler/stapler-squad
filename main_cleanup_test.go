package main

import "testing"

func TestCleanupOrphanedControlModeClients(t *testing.T) {
	for _, tc := range []struct {
		name       string
		isolated   bool
		wantCalled bool
	}{
		{"isolated skips kill", true, false},
		{"live instance kills", false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called := false
			cleanupOrphanedControlModeClients(tc.isolated, func(socket string) (int, error) {
				called = true
				if socket != "" {
					t.Errorf("socket = %q, want default", socket)
				}
				return 1, nil
			})
			if called != tc.wantCalled {
				t.Fatalf("called=%v, want %v", called, tc.wantCalled)
			}
		})
	}
}
