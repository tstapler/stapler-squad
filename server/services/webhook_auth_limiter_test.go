package services

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type limiterClock struct {
	mu sync.Mutex
	t  time.Time
}

func newFakeClock() *limiterClock {
	return &limiterClock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *limiterClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *limiterClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

func limiterWith(clock *limiterClock, max int, window time.Duration, maxKeys int) *authFailureLimiter {
	l := newAuthFailureLimiter()
	l.now, l.max, l.window, l.maxKeys = clock.Now, max, window, maxKeys
	return l
}

func TestAuthFailureLimiter_should_BlockUntilWindowEnds_When_BudgetIsExhausted(t *testing.T) {
	clock := newFakeClock()
	l := limiterWith(clock, 3, time.Minute, 100)

	for i := 1; i <= 3; i++ {
		_, blocked := l.blocked("a")
		require.False(t, blocked, "attempt %d must still be allowed", i)
		assert.Equal(t, i == 3, l.recordFailure("a"), "only the failure that exhausts the budget reports justBlocked")
	}

	retry, blocked := l.blocked("a")
	assert.True(t, blocked)
	assert.Equal(t, time.Minute, retry)
	clock.Advance(20 * time.Second)
	retry, blocked = l.blocked("a")
	assert.True(t, blocked)
	assert.Equal(t, 40*time.Second, retry)
	clock.Advance(40 * time.Second)
	_, blocked = l.blocked("a")
	assert.False(t, blocked, "the block ends with the window")
}

func TestAuthFailureLimiter_should_StartAFreshBudget_When_TheWindowHasPassed(t *testing.T) {
	clock := newFakeClock()
	l := limiterWith(clock, 2, time.Minute, 100)
	l.recordFailure("a")
	clock.Advance(time.Minute + time.Second)

	assert.False(t, l.recordFailure("a"), "the old failure must have aged out, so this is failure 1 of 2")
	_, blocked := l.blocked("a")
	assert.False(t, blocked)
}

func TestAuthFailureLimiter_should_NotAffectOtherClients_When_OneIsBlocked(t *testing.T) {
	l := limiterWith(newFakeClock(), 1, time.Minute, 100)
	l.recordFailure("noisy")

	_, noisyBlocked := l.blocked("noisy")
	_, quietBlocked := l.blocked("quiet")

	assert.True(t, noisyBlocked)
	assert.False(t, quietBlocked)
}

func TestAuthFailureLimiter_should_StayBounded_When_ManyDistinctClientsFail(t *testing.T) {
	clock := newFakeClock()
	l := limiterWith(clock, 5, time.Minute, 50)

	for i := 0; i < 500; i++ {
		l.recordFailure(fmt.Sprintf("10.0.%d.%d", i/250, i%250))
		clock.Advance(time.Millisecond)
	}

	assert.LessOrEqual(t, len(l.entries), 50)
}

func TestAuthFailureLimiter_should_BeSafe_When_UsedConcurrently(t *testing.T) {
	l := newAuthFailureLimiter()
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			key := fmt.Sprintf("k%d", i%4)
			for j := 0; j < 200; j++ {
				l.blocked(key)
				l.recordFailure(key)
			}
		}(i)
	}
	wg.Wait()
	_, blocked := l.blocked("k0")
	assert.True(t, blocked)
}

func TestClientLimiterKey_should_NormalizeAddresses_When_DerivingTheKey(t *testing.T) {
	assert.Equal(t, "192.0.2.7", clientLimiterKey("192.0.2.7:51234"))
	assert.Equal(t, "192.0.2.7", clientLimiterKey("192.0.2.7"), "no port")
	assert.Equal(t, "192.0.2.7", clientLimiterKey("[::ffff:192.0.2.7]:80"), "an IPv4-mapped address is the IPv4 address")
	assert.Equal(t, "2001:db8:1:2::/64", clientLimiterKey("[2001:db8:1:2::1]:443"))
	assert.Equal(t, clientLimiterKey("[2001:db8:1:2::1]:443"), clientLimiterKey("[2001:db8:1:2:ffff:ffff:ffff:ffff]:443"),
		"addresses in one /64 must share a key so rotating within it cannot evade the limit")
	assert.NotEqual(t, clientLimiterKey("[2001:db8:1:2::1]:443"), clientLimiterKey("[2001:db8:1:3::1]:443"))
	assert.Equal(t, "2001:db8:1:2::/64", clientLimiterKey("[2001:db8:1:2::1%eth0]:443"), "zone is dropped")
	assert.Equal(t, "not-an-address", clientLimiterKey("not-an-address"), "garbage still yields a stable key")
}

// A client that has burned its failure budget is refused before its credential is even
// looked at, so presenting the VALID token during a block still gets 429.
func TestWebhookManagement_should_Return429BeforeCheckingCredential_When_FailureBudgetIsSpent(t *testing.T) {
	h := newMgmtHarness(t)
	clock := newFakeClock()
	h.handler.limiter = limiterWith(clock, 10, time.Minute, 100)
	h.remoteAddr = "198.51.100.9:5000"
	capability := webhookManagementBasePath + "/capability"

	for i := 0; i < 10; i++ {
		rec := h.do(t, http.MethodGet, capability, "bad-token", nil)
		require.Equal(t, http.StatusUnauthorized, rec.Code, "failure %d is still an ordinary 401", i+1)
	}

	rec := h.do(t, http.MethodGet, capability, h.token, nil)
	assert.Equal(t, http.StatusTooManyRequests, rec.Code, "a valid token must not bypass a block")
	assert.Equal(t, "RATE_LIMITED", errCode(t, rec))
	requireValid(t, "error-response.schema.json", rec.Body.Bytes())
	retry, err := strconv.Atoi(rec.Header().Get("Retry-After"))
	require.NoError(t, err)
	assert.Equal(t, 60, retry)

	clock.Advance(time.Minute)
	assert.Equal(t, http.StatusOK, h.do(t, http.MethodGet, capability, h.token, nil).Code, "service resumes once the window ends")
}

func TestWebhookManagement_should_NotConsumeBudget_When_RequestsSucceed(t *testing.T) {
	h := newMgmtHarness(t)
	h.handler.limiter = limiterWith(newFakeClock(), 3, time.Minute, 100)
	h.remoteAddr = "198.51.100.9:5000"
	capability := webhookManagementBasePath + "/capability"

	for i := 0; i < 100; i++ {
		require.Equal(t, http.StatusOK, h.do(t, http.MethodGet, capability, h.token, nil).Code)
	}

	for i := 0; i < 3; i++ {
		assert.Equal(t, http.StatusUnauthorized, h.do(t, http.MethodGet, capability, "bad", nil).Code)
	}
	assert.Equal(t, http.StatusTooManyRequests, h.do(t, http.MethodGet, capability, h.token, nil).Code)
}

func TestWebhookManagement_should_ShareOneBudgetAcrossRoutesAndIgnoreForwardedHeaders_When_ProbingDifferentPaths(t *testing.T) {
	h := newMgmtHarness(t)
	h.handler.limiter = limiterWith(newFakeClock(), 4, time.Minute, 100)
	h.remoteAddr = "198.51.100.9:5000"
	paths := []struct{ method, path string }{
		{http.MethodGet, webhookManagementBasePath + "/capability"},
		{http.MethodGet, mgmtBase + testInstance},
		{http.MethodPost, mgmtBase + testInstance + "/reconcile"},
		{http.MethodPost, mgmtBase + testInstance + "/disable"},
	}
	for i, p := range paths {
		req := httptest.NewRequest(p.method, p.path, nil)
		req.RemoteAddr = h.remoteAddr
		req.Header.Set("X-Forwarded-For", fmt.Sprintf("203.0.113.%d", i)) // an attempt to pick its own key
		rec := httptest.NewRecorder()
		h.mux.ServeHTTP(rec, req)
		require.Equal(t, http.StatusUnauthorized, rec.Code)
	}

	for _, p := range paths {
		req := httptest.NewRequest(p.method, p.path, nil)
		req.RemoteAddr = h.remoteAddr
		req.Header.Set("X-Forwarded-For", "203.0.113.200")
		req.Header.Set("Authorization", "Bearer "+h.token)
		rec := httptest.NewRecorder()
		h.mux.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusTooManyRequests, rec.Code, "%s %s", p.method, p.path)
	}

	h.remoteAddr = "198.51.100.77:5000"
	assert.Equal(t, http.StatusOK, h.do(t, http.MethodGet, paths[0].path, h.token, nil).Code, "a different peer is unaffected")
}
