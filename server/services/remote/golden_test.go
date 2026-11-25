package remote

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"github.com/tstapler/stapler-squad/config"
	sessionv1 "github.com/tstapler/stapler-squad/gen/proto/go/session/v1"
)

// goldenPath holds the transcript recorded from origin/main (the handler in
// package services, before the move). REMOTE_GOLDEN_RECORD=1 rewrites it.
const goldenPath = "testdata/remote.golden.txt"

var (
	goldenAddrRE = regexp.MustCompile(`127\.0\.0\.1:\d{4,5}`)
	goldenFPRE   = regexp.MustCompile(`SHA256:[A-Za-z0-9+/=]+`)
	goldenKeyRE  = regexp.MustCompile(`ssh-ed25519 [A-Za-z0-9+/=]+`)
	goldenPortRE = regexp.MustCompile(`port=\d{5}`)
)

func goldenScrub(s string) string {
	s = goldenAddrRE.ReplaceAllString(s, "ADDR")
	s = goldenFPRE.ReplaceAllString(s, "FINGERPRINT")
	s = goldenPortRE.ReplaceAllString(s, "port=EPHEMERAL")
	return goldenKeyRE.ReplaceAllString(s, "ssh-ed25519 KEY")
}

type goldenLog struct{ sb strings.Builder }

func (g *goldenLog) line(label string, format string, args ...any) {
	fmt.Fprintf(&g.sb, "%s: %s\n", label, goldenScrub(fmt.Sprintf(format, args...)))
}

func (g *goldenLog) rpcErr(label string, err error) {
	if err == nil {
		g.line(label, "OK")
		return
	}
	g.line(label, "code=%s msg=%s", connect.CodeOf(err), err.Error())
}

func TestGolden_HandlerTranscript_MatchesOriginMain(t *testing.T) {
	ctx := context.Background()
	g := &goldenLog{}

	for _, h := range []string{"example.com", "example.com:2222", "10.0.0.5", "[::1]:2200", "::1", ""} {
		g.line("RemoteAddr", "%q -> %q", h, RemoteAddr(h))
	}

	addr, _ := startRemoteTestSSHServer(t)
	cfg := &config.Config{Remotes: []config.RemoteConfig{
		{Name: "live", Host: addr, User: "tester", IdentityRef: "live"},
		{Name: "dead", Host: "127.0.0.1:1", User: "tester"},
		{Name: "portful", Host: "box.example:2200", User: "u", BasePath: "/srv", IdentityRef: "portful"},
		{Name: "plain", Host: "box.example", User: "u", BasePath: "/srv"},
		{Name: "badport", Host: "box.example:99999", User: "u"},
	}}
	svc, knownHosts, keyStore := newTestRemoteService(t, cfg)

	tc := func(label string, req *sessionv1.TestRemoteConnectionRequest) *sessionv1.TestRemoteConnectionResponse {
		resp, err := svc.TestRemoteConnection(ctx, connect.NewRequest(req))
		g.rpcErr(label, err)
		if err != nil {
			return nil
		}
		m := resp.Msg
		g.line(label, "success=%v unknown=%v fp=%q err=%q", m.Success, m.HostKeyUnknown, m.Fingerprint, m.ErrorMessage)
		return m
	}
	trust := func(label string, req *sessionv1.TrustRemoteHostKeyRequest) *sessionv1.TrustRemoteHostKeyResponse {
		resp, err := svc.TrustRemoteHostKey(ctx, connect.NewRequest(req))
		g.rpcErr(label, err)
		if err != nil {
			return nil
		}
		g.line(label, "success=%v err=%q", resp.Msg.Success, resp.Msg.ErrorMessage)
		return resp.Msg
	}

	// TestRemoteConnection: argument validation, then the TOFU states.
	tc("tc/empty", &sessionv1.TestRemoteConnectionRequest{})
	tc("tc/both", &sessionv1.TestRemoteConnectionRequest{RemoteName: "live", Draft: &sessionv1.DraftRemoteTarget{Name: "d", Host: "h", User: "u"}})
	tc("tc/unknown", &sessionv1.TestRemoteConnectionRequest{RemoteName: "nope"})
	for i, d := range []*sessionv1.DraftRemoteTarget{{Host: "h", User: "u"}, {Name: "n", User: "u"}, {Name: "n", Host: "h"}} {
		tc(fmt.Sprintf("tc/draft-incomplete-%d", i), &sessionv1.TestRemoteConnectionRequest{Draft: d})
	}
	tc("tc/dead", &sessionv1.TestRemoteConnectionRequest{RemoteName: "dead"})
	first := tc("tc/live-first", &sessionv1.TestRemoteConnectionRequest{RemoteName: "live"})

	// TrustRemoteHostKey: validation, unreachable, mismatch, then the real trust.
	trust("trust/no-fp", &sessionv1.TrustRemoteHostKeyRequest{RemoteName: "live"})
	trust("trust/unknown", &sessionv1.TrustRemoteHostKeyRequest{RemoteName: "nope", Fingerprint: "SHA256:x"})
	trust("trust/both", &sessionv1.TrustRemoteHostKeyRequest{RemoteName: "live", Draft: &sessionv1.DraftRemoteTarget{Name: "d", Host: "h", User: "u"}, Fingerprint: "SHA256:x"})
	trust("trust/dead", &sessionv1.TrustRemoteHostKeyRequest{RemoteName: "dead", Fingerprint: "SHA256:x"})
	trust("trust/mismatch", &sessionv1.TrustRemoteHostKeyRequest{RemoteName: "live", Fingerprint: "SHA256:wrong"})
	trusted, err := knownHosts.IsHostTrusted(addr)
	require.NoError(t, err)
	g.line("trusted-after-mismatch", "%v", trusted)
	trust("trust/ok", &sessionv1.TrustRemoteHostKeyRequest{RemoteName: "live", Fingerprint: first.Fingerprint})
	trusted, err = knownHosts.IsHostTrusted(addr)
	require.NoError(t, err)
	g.line("trusted-after-trust", "%v", trusted)

	// Identity generation: validation, then idempotence (the stored key never rotates).
	_, err = svc.GenerateRemoteIdentity(ctx, connect.NewRequest(&sessionv1.GenerateRemoteIdentityRequest{}))
	g.rpcErr("gen/empty", err)
	gen1, err := svc.GenerateRemoteIdentity(ctx, connect.NewRequest(&sessionv1.GenerateRemoteIdentityRequest{Name: "live"}))
	g.rpcErr("gen/live", err)
	gen2, err := svc.GenerateRemoteIdentity(ctx, connect.NewRequest(&sessionv1.GenerateRemoteIdentityRequest{Name: "live"}))
	g.rpcErr("gen/live-again", err)
	g.line("gen/pub", "%q", gen1.Msg.PublicKeyText)
	g.line("gen/auth", "%q", gen1.Msg.AuthorizedKeysLine)
	g.line("gen/stable", "%v", gen1.Msg.PublicKeyText == gen2.Msg.PublicKeyText && gen1.Msg.AuthorizedKeysLine == gen2.Msg.AuthorizedKeysLine)

	tc("tc/live-after-trust", &sessionv1.TestRemoteConnectionRequest{RemoteName: "live"})
	tc("tc/draft-live", &sessionv1.TestRemoteConnectionRequest{Draft: &sessionv1.DraftRemoteTarget{Name: "live", Host: "127.0.0.1", User: "tester", Port: 1}})

	// ListRemotes: host/port split, bad ports and the identity flag.
	list, err := svc.ListRemotes(ctx, connect.NewRequest(&sessionv1.ListRemotesRequest{}))
	g.rpcErr("list", err)
	for _, r := range list.Msg.Remotes {
		g.line("list/item", "name=%s host=%s user=%s port=%d base=%s identity=%v", r.Name, r.Host, r.User, r.Port, r.BasePath, r.HasIdentity)
	}

	// CreateRemote: every precondition in order, then a trusted save.
	create := func(label string, req *sessionv1.CreateRemoteRequest) {
		resp, err := svc.CreateRemote(ctx, connect.NewRequest(req))
		g.rpcErr(label, err)
		if err == nil {
			r := resp.Msg.Remote
			g.line(label, "name=%s host=%s user=%s port=%d base=%s identity=%v", r.Name, r.Host, r.User, r.Port, r.BasePath, r.HasIdentity)
		}
	}
	create("create/empty", &sessionv1.CreateRemoteRequest{})
	create("create/no-host", &sessionv1.CreateRemoteRequest{Name: "n", User: "u", BasePath: "/b"})
	create("create/no-user", &sessionv1.CreateRemoteRequest{Name: "n", Host: "h", BasePath: "/b"})
	create("create/no-base", &sessionv1.CreateRemoteRequest{Name: "n", Host: "h", User: "u"})
	create("create/dup", &sessionv1.CreateRemoteRequest{Name: "live", Host: "h", User: "u", BasePath: "/b"})
	create("create/untrusted", &sessionv1.CreateRemoteRequest{Name: "fresh", Host: "untrusted.example", User: "u", BasePath: "/b"})
	require.NoError(t, knownHosts.Trust("trusted.example:2201", newRemoteTestKey(t)))
	create("create/trusted-port", &sessionv1.CreateRemoteRequest{Name: "fresh", Host: "trusted.example", Port: 2201, User: "u", BasePath: "/b"})
	_, _, idErr := keyStore.GetIdentity(ctx, "fresh")
	g.line("create/identity-generated", "%v", idErr == nil)

	// DeleteRemote: validation, config+identity, orphan identity, nothing.
	del := func(label, name string) {
		resp, err := svc.DeleteRemote(ctx, connect.NewRequest(&sessionv1.DeleteRemoteRequest{Name: name}))
		g.rpcErr(label, err)
		if err == nil {
			g.line(label, "%+v", resp.Msg.String())
		}
	}
	del("del/empty", "")
	del("del/nothing", "ghost")
	_, err = keyStore.GenerateOrDescribeIdentity(ctx, "orphan")
	require.NoError(t, err)
	del("del/orphan", "orphan")
	del("del/orphan-again", "orphan")

	got := g.sb.String()
	if os.Getenv("REMOTE_GOLDEN_RECORD") != "" {
		require.NoError(t, os.WriteFile(goldenPath, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(goldenPath)
	require.NoError(t, err)
	require.Equal(t, string(want), got)
}
