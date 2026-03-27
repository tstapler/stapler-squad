package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/publicsuffix"

	"github.com/tstapler/stapler-squad/log"
)

// urlPattern matches http/https URLs in shell commands.
var urlPattern = regexp.MustCompile(`https?://([^\s/'"\\]+)`)

// knownSafeHosts are well-known long-established hosts that we skip RDAP lookups for.
// These are always considered safe from a domain-age perspective.
var knownSafeHosts = map[string]bool{
	"github.com": true, "raw.githubusercontent.com": true,
	"npmjs.com": true, "registry.npmjs.org": true,
	"pypi.org": true, "files.pythonhosted.org": true,
	"golang.org": true, "pkg.go.dev": true,
	"crates.io": true, "static.crates.io": true,
	"hub.docker.com": true, "registry-1.docker.io": true,
	"amazonaws.com": true, "cloudfront.net": true, "s3.amazonaws.com": true,
	"googleapis.com": true, "storage.googleapis.com": true,
	"microsoft.com": true, "azure.com": true,
	"google.com": true, "stackoverflow.com": true,
	"example.com": true, "localhost": true, "127.0.0.1": true,
}

// domainCacheEntry is an in-memory cache entry for a domain registration date.
type domainCacheEntry struct {
	registrationDate time.Time
	notFound         bool // RDAP returned 404 / no data
	cachedAt         time.Time
}

// DomainAgeChecker extracts domains from Bash commands and checks their registration
// age using RDAP (Registration Data Access Protocol). Results are cached for 24h.
//
// A domain is considered "new" if its registration date is within the configured threshold
// (default 30 days). New domains from network-oriented commands are escalated for review.
type DomainAgeChecker struct {
	mu         sync.RWMutex
	cache      map[string]domainCacheEntry
	httpClient *http.Client
	cacheTTL   time.Duration
	newThresh  time.Duration // how old counts as "new"
	enabled    bool
}

// NewDomainAgeChecker creates a DomainAgeChecker with sensible defaults.
// Set enabled=false to disable RDAP lookups (no-op mode).
func NewDomainAgeChecker(enabled bool) *DomainAgeChecker {
	return &DomainAgeChecker{
		cache: make(map[string]domainCacheEntry),
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
		cacheTTL:  24 * time.Hour,
		newThresh: 30 * 24 * time.Hour, // 30 days
		enabled:   enabled,
	}
}

// ExtractDomainsFromCommand parses network-relevant Bash commands and returns the
// eTLD+1 (registered domain) for each URL found, deduplicated.
func ExtractDomainsFromCommand(cmd string) []string {
	matches := urlPattern.FindAllStringSubmatch(cmd, -1)
	seen := make(map[string]bool)
	var domains []string

	for _, m := range matches {
		host := m[1]
		// Strip port number.
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		// Skip IP addresses — no WHOIS needed.
		if net.ParseIP(host) != nil {
			continue
		}
		// Reduce to eTLD+1 (registered domain).
		registered, err := publicsuffix.EffectiveTLDPlusOne(host)
		if err != nil || registered == "" {
			registered = host
		}
		registered = strings.ToLower(registered)

		if !seen[registered] {
			seen[registered] = true
			domains = append(domains, registered)
		}
	}
	return domains
}

// IsNewlyRegistered returns true if the domain was registered within the threshold
// and the check is enabled. Returns (false, nil) for any lookup failure, to avoid
// blocking legitimate operations on RDAP outages.
func (d *DomainAgeChecker) IsNewlyRegistered(ctx context.Context, domain string) (bool, error) {
	if !d.enabled {
		return false, nil
	}
	if knownSafeHosts[domain] {
		return false, nil
	}

	regDate, err := d.lookupRegistrationDate(ctx, domain)
	if err != nil {
		// Non-blocking: lookup failures don't cause escalation.
		return false, nil
	}
	if regDate.IsZero() {
		return false, nil
	}

	age := time.Since(regDate)
	return age < d.newThresh, nil
}

// NewDomainThreshold returns how old a domain must be to be considered "established".
func (d *DomainAgeChecker) NewDomainThreshold() time.Duration {
	return d.newThresh
}

// lookupRegistrationDate returns the registration date for a domain, using the cache.
func (d *DomainAgeChecker) lookupRegistrationDate(ctx context.Context, domain string) (time.Time, error) {
	// Check cache first.
	d.mu.RLock()
	entry, ok := d.cache[domain]
	d.mu.RUnlock()

	if ok && time.Since(entry.cachedAt) < d.cacheTTL {
		if entry.notFound {
			return time.Time{}, nil
		}
		return entry.registrationDate, nil
	}

	// Perform RDAP lookup.
	regDate, notFound, err := fetchRDAPRegistrationDate(ctx, d.httpClient, domain)
	if err != nil {
		log.WarningLog.Printf("[DomainAgeChecker] RDAP lookup failed for %s: %v", domain, err)
		return time.Time{}, err
	}

	// Cache the result.
	d.mu.Lock()
	d.cache[domain] = domainCacheEntry{
		registrationDate: regDate,
		notFound:         notFound,
		cachedAt:         time.Now(),
	}
	d.mu.Unlock()

	return regDate, nil
}

// rdapResponse is the minimal structure we need from an RDAP domain response.
type rdapResponse struct {
	Events []struct {
		Action string `json:"eventAction"`
		Date   string `json:"eventDate"`
	} `json:"events"`
	// ErrorCode is present when the RDAP server returns an error response.
	ErrorCode int `json:"errorCode"`
}

// fetchRDAPRegistrationDate queries rdap.org for the registration date of domain.
// Returns (zeroTime, true, nil) if the domain has no RDAP record.
// Returns (zeroTime, false, err) on network or parse errors.
func fetchRDAPRegistrationDate(ctx context.Context, client *http.Client, domain string) (time.Time, bool, error) {
	url := fmt.Sprintf("https://rdap.org/domain/%s", domain)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return time.Time{}, false, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "claude-squad-domain-checker/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return time.Time{}, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return time.Time{}, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		return time.Time{}, false, fmt.Errorf("RDAP returned HTTP %d for %s", resp.StatusCode, domain)
	}

	var rdap rdapResponse
	if err := json.NewDecoder(resp.Body).Decode(&rdap); err != nil {
		return time.Time{}, false, fmt.Errorf("parse RDAP response: %w", err)
	}

	// Find the "registration" event.
	for _, ev := range rdap.Events {
		if strings.EqualFold(ev.Action, "registration") {
			t, err := time.Parse(time.RFC3339, ev.Date)
			if err != nil {
				// Try alternative format used by some registrars.
				t, err = time.Parse("2006-01-02T15:04:05Z0700", ev.Date)
			}
			if err == nil {
				return t, false, nil
			}
		}
	}

	// Registration event not found (unusual but possible for some TLDs).
	return time.Time{}, true, nil
}
