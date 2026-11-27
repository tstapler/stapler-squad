package services

import (
	"github.com/tstapler/stapler-squad/server/services/contentsearch"
)

// Range read caps (Task 1.6.2c, ADR-036): 4 concurrent range calls per client, 8 per
// remote IP. "Client" is the (remote host, session) pair, the finest identity the
// unauthenticated local listener can see; the IP key is the shared contentsearch.RemoteHost seam.
const (
	maxRangesPerIP = 8
	rangeNoTotal   = 1 << 30
)

// RangeBusyReason says which cap refused a range call; it travels in RangeBusyDetail.
type RangeBusyReason string

const (
	rangeBusyPerClient RangeBusyReason = "per_client"
	rangeBusyPerIP     RangeBusyReason = "per_ip"
)

// rangeLimiter counts in-flight ReadFileRange calls per remote IP and per client.
type rangeLimiter struct{ perIP, perClient *contentsearch.IPLimiter }

func newRangeLimiter() *rangeLimiter {
	return &rangeLimiter{
		perIP:     contentsearch.NewIPLimiter(contentsearch.IPCaps{PerHost: maxRangesPerIP, Total: rangeNoTotal}),
		perClient: contentsearch.NewIPLimiter(contentsearch.IPCaps{PerHost: maxRangesPerClient, Total: rangeNoTotal}),
	}
}

// Acquire takes a slot on both counters or reports which one refused.
func (l *rangeLimiter) Acquire(remoteAddr, sessionID string) (release func(), refused RangeBusyReason) {
	releaseIP, busy := l.perIP.Acquire(remoteAddr)
	if busy != "" {
		return nil, rangeBusyPerIP
	}
	releaseClient, busy := l.perClient.Acquire(contentsearch.RemoteHost(remoteAddr) + "|" + sessionID)
	if busy != "" {
		releaseIP()
		return nil, rangeBusyPerClient
	}
	return func() { releaseClient(); releaseIP() }, ""
}
