package secretscan

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRedactErrorDetail_ShouldMaskEachCredentialShape_When_ItRidesAlongInAnErrorString(t *testing.T) {
	t.Parallel()
	cases := map[string]struct{ in, want string }{
		"url userinfo":   {"push https://user:pw@host.invalid/r failed", "push https://[REDACTED]@host.invalid/r failed"},
		"vendor prefix":  {"key sk-abcdefgh1234 rejected", "key [REDACTED] rejected"},
		"github token":   {"ghp_abcdefghij0123456789 expired", "[REDACTED] expired"},
		"slack token":    {"xoxb-1234567890 revoked", "[REDACTED] revoked"},
		"aws key id":     {"AKIAABCDEFGHIJKLMNOP denied", "[REDACTED] denied"},
		"jwt":            {"jwt eyJhbGci.eyJzdWIi.sig bad", "jwt [REDACTED] bad"},
		"bearer":         {"sent Bearer abcdefgh12345678 got 401", "sent [REDACTED] got 401"},
		"key value":      {"api_key=hunter2 refused", "[REDACTED] refused"},
		"flag and space": {"run --token hunter2 failed", "run [REDACTED] failed"},
		"nothing secret": {"exit status 128: not a git repository", "exit status 128: not a git repository"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tc.want, RedactErrorDetail(tc.in))
		})
	}
}
