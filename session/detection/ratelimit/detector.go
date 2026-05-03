package ratelimit

import (
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"
	_ "time/tzdata" // Embed IANA timezone database (~500KB); needed for time.LoadLocation on systems without system tzdata.

	"github.com/tstapler/stapler-squad/log"
)

const (
	DefaultCooldown      = 30 * time.Second
	DefaultResetBuffer   = 5
	DefaultRecoveryInput = "1\n"
	// DefaultFallbackWait is how long to wait before retrying when no reset time is known.
	DefaultFallbackWait = 30 * time.Minute
)

type Provider string

const (
	ProviderAnthropic Provider = "anthropic"
	ProviderOpenAI    Provider = "openai"
	ProviderGoogle    Provider = "google"
	ProviderAider     Provider = "aider"
	ProviderUnknown   Provider = "unknown"
)

type RateLimitState int

const (
	StateNone RateLimitState = iota
	StateWaiting
	StateRecovering
	StateRecovered
	StateFailed
)

type Detection struct {
	Provider    Provider
	State       RateLimitState
	ResetTime   time.Time
	InputToSend []byte
	DetectedAt  time.Time
}

type Detector struct {
	mu sync.Mutex

	sessionID string

	rateLimitPatterns []*regexp.Regexp
	continuePatterns  []*regexp.Regexp
	timestampPatterns []*regexp.Regexp
	providerPatterns  map[Provider][]*regexp.Regexp

	currentState     RateLimitState
	currentResetTime time.Time
	lastDetection    time.Time
	cooldown         time.Duration
	resetBufferSecs  int

	onDetection func(Detection)
}

var defaultRateLimitPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)/rate-limit-options`),
	regexp.MustCompile(`(?i)rate limit.*exceeded`),
	regexp.MustCompile(`(?i)429.*Too Many Requests`),
	regexp.MustCompile(`(?i)rate_limit_error`),
	regexp.MustCompile(`(?i)Usage limit reached`),
	regexp.MustCompile(`(?i)rate limit reached`),
	regexp.MustCompile(`(?i)quota exceeded`),
	regexp.MustCompile(`(?i)you'?ve hit your (usage )?limit`),
}

var defaultContinuePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)1\.\s*Keep trying`),
	regexp.MustCompile(`(?i)press.*enter.*continue`),
	regexp.MustCompile(`(?i)continue.*\?.*\[y/n\]`),
	regexp.MustCompile(`(?i)\*?\s*\d+\.\s*(Keep|Try|Continue|Retry)`),
	regexp.MustCompile(`(?i)Access resets at`),
	regexp.MustCompile(`(?i)/extra-usage`),
}

var defaultTimestampPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:reset at|Access resets at) (.+?)(?:\s*$|PT|PDT)`),
	regexp.MustCompile(`(?i)retry\s*after\s*(\d+)\s*(second|minute|hour)s?`),
	regexp.MustCompile(`(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})`),
	// Two-group pattern: captures time and timezone separately, e.g. "resets 11pm (America/Los_Angeles)"
	regexp.MustCompile(`(?i)resets\s+(\d{1,2}(?::\d{2})?(?:am|pm))\s*\(?([^)\s]+)\)?`),
}

// tzFixedOffsets maps timezone abbreviations to their fixed UTC offsets in seconds.
// Abbreviations denote a specific offset, not a DST-aware region — "PST" always
// means UTC-8, whether or not it is currently summer in Los Angeles.
var tzFixedOffsets = map[string]int{
	"PST": -8 * 3600,
	"PDT": -7 * 3600,
	"MST": -7 * 3600,
	"MDT": -6 * 3600,
	"CST": -6 * 3600,
	"CDT": -5 * 3600,
	"EST": -5 * 3600,
	"EDT": -4 * 3600,
	"UTC": 0,
	"GMT": 0,
}

// tzCommonNames maps informal timezone names to IANA location names.
// These use DST-aware regions because the common name (e.g. "Pacific")
// is understood to follow local daylight rules.
var tzCommonNames = map[string]string{
	"Pacific":  "America/Los_Angeles",
	"Mountain": "America/Denver",
	"Central":  "America/Chicago",
	"Eastern":  "America/New_York",
}

var providerSpecificPatterns = map[Provider][]*regexp.Regexp{
	ProviderAnthropic: {
		regexp.MustCompile(`(?i)/rate-limit-options`),
		regexp.MustCompile(`(?i)Usage limit reached for`),
		regexp.MustCompile(`(?i)Access resets at`),
	},
	ProviderOpenAI: {
		regexp.MustCompile(`(?i)Rate limit exceeded`),
		regexp.MustCompile(`(?i)exceeded retry limit.*429`),
		regexp.MustCompile(`(?i)openai.*rate limit`),
	},
	ProviderGoogle: {
		regexp.MustCompile(`(?i)rate limit.*gemini`),
		regexp.MustCompile(`(?i)429.*Too Many Requests`),
	},
	ProviderAider: {
		regexp.MustCompile(`(?i)RateLimitError`),
		regexp.MustCompile(`(?i)rate limit exceeded`),
	},
}

var providerRecoveryInputs = map[Provider][]byte{
	ProviderAnthropic: []byte("1\n"),
	ProviderOpenAI:    []byte("1\n"),
	ProviderGoogle:    []byte("1\n"),
	ProviderAider:     []byte("\n"),
}

func NewDetector(sessionID string) *Detector {
	return &Detector{
		sessionID:         sessionID,
		rateLimitPatterns: defaultRateLimitPatterns,
		continuePatterns:  defaultContinuePatterns,
		timestampPatterns: defaultTimestampPatterns,
		providerPatterns:  providerSpecificPatterns,
		currentState:      StateNone,
		cooldown:          DefaultCooldown,
		resetBufferSecs:   DefaultResetBuffer,
	}
}

func (d *Detector) SetDetectionCallback(callback func(Detection)) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.onDetection = callback
}

func (d *Detector) SetCooldown(cooldown time.Duration) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.cooldown = cooldown
}

func (d *Detector) SetResetBuffer(seconds int) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.resetBufferSecs = seconds
}

func (d *Detector) ProcessOutput(data []byte) {
	d.mu.Lock()
	defer d.mu.Unlock()

	output := string(data)

	if d.currentState != StateNone && d.currentState != StateWaiting {
		return
	}

	if time.Since(d.lastDetection) < d.cooldown {
		return
	}

	detection := d.detectInOutput(output)
	if detection != nil {
		d.lastDetection = time.Now()
		d.currentState = StateWaiting

		log.InfoLog.Printf("[RateLimit] Detected rate limit for session %s: provider=%s, reset at %v",
			d.sessionID, detection.Provider, detection.ResetTime)

		log.DebugLog.Printf("[RateLimit] Pattern matched in session %s: provider=%s, input=%q, detected_at=%v",
			d.sessionID, detection.Provider, string(detection.InputToSend), detection.DetectedAt)

		if d.onDetection != nil {
			go d.onDetection(*detection)
		}
	}
}

func (d *Detector) detectInOutput(output string) *Detection {
	output = stripANSI(output)

	hasRateLimitPattern := d.matchAny(d.rateLimitPatterns, output)
	if !hasRateLimitPattern {
		return nil
	}

	provider := d.identifyProvider(output)

	resetTime := d.parseResetTime(output)

	continueMatch := d.matchAny(d.continuePatterns, output)
	if !continueMatch {
		return nil
	}

	input := providerRecoveryInputs[provider]
	if len(input) == 0 {
		input = []byte("\n")
	}

	return &Detection{
		Provider:    provider,
		State:       StateWaiting,
		ResetTime:   resetTime,
		InputToSend: input,
		DetectedAt:  time.Now(),
	}
}

func (d *Detector) matchAny(patterns []*regexp.Regexp, output string) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(output) {
			return true
		}
	}
	return false
}

func (d *Detector) identifyProvider(output string) Provider {
	for provider, patterns := range d.providerPatterns {
		for _, pattern := range patterns {
			if pattern.MatchString(output) {
				return provider
			}
		}
	}
	return ProviderUnknown
}

func (d *Detector) parseResetTime(output string) time.Time {
	for _, pattern := range d.timestampPatterns {
		matches := pattern.FindStringSubmatch(output)
		// Two-group match: time string + timezone string (e.g. "11pm" + "America/Los_Angeles")
		if len(matches) == 3 && matches[1] != "" && matches[2] != "" {
			parsed := parseTimeWithTZ(matches[1], matches[2])
			if !parsed.IsZero() {
				d.currentResetTime = parsed
				return parsed
			}
		}
		if len(matches) > 1 && matches[1] != "" {
			parsed := d.parseTimestamp(matches[1])
			if !parsed.IsZero() {
				d.currentResetTime = parsed
				return parsed
			}
		}
	}
	return time.Time{}
}

// parseTimeWithTZ parses a time-of-day string with an explicit timezone.
// tzStr can be an IANA location name ("America/Los_Angeles"), a common abbreviation
// ("PDT", "EST"), or a common name ("Pacific"). Falls back to time.Local if unknown.
// If the resulting time is in the past, it adds 24h (next-day rollover).
func parseTimeWithTZ(timeStr, tzStr string) time.Time {
	// Strip surrounding parentheses from tzStr
	tzStr = strings.Trim(tzStr, "()")
	tzStr = strings.TrimSpace(tzStr)

	// Resolve the location. Priority:
	//   1. IANA name directly (e.g. "America/Los_Angeles")
	//   2. Fixed-offset abbreviation (e.g. "PST" → UTC-8, always)
	//   3. Common informal name (e.g. "Pacific" → DST-aware IANA zone)
	//   4. Unknown → return zero so the scheduler falls back to 30-min wait
	var loc *time.Location
	if l, err := time.LoadLocation(tzStr); err == nil {
		loc = l
	} else if offsetSecs, ok := tzFixedOffsets[tzStr]; ok {
		loc = time.FixedZone(tzStr, offsetSecs)
	} else if ianaName, ok := tzCommonNames[tzStr]; ok {
		if l, err := time.LoadLocation(ianaName); err == nil {
			loc = l
		}
	}
	if loc == nil {
		return time.Time{}
	}

	// Try parsing with various time-of-day formats (case-insensitive)
	normalizedTime := strings.ToUpper(strings.TrimSpace(timeStr))
	formats := []string{"3PM", "3:04PM", "3 PM", "3:04 PM"}

	now := time.Now().In(loc)
	for _, format := range formats {
		parsed, parseErr := time.ParseInLocation(format, normalizedTime, loc)
		if parseErr != nil {
			continue
		}
		// Anchor result to today in the target location
		result := time.Date(now.Year(), now.Month(), now.Day(),
			parsed.Hour(), parsed.Minute(), parsed.Second(), 0, loc)
		// If the time has already passed today, roll to tomorrow
		if result.Before(now) {
			result = result.AddDate(0, 0, 1)
		}
		return result
	}

	return time.Time{}
}

func (d *Detector) parseTimestamp(input string) time.Time {
	input = strings.TrimSpace(input)

	baseTime := time.Now()

	retryMatchNumber := regexp.MustCompile(`^(\d+)$`).FindStringSubmatch(input)
	if len(retryMatchNumber) == 2 {
		var amount int
		fmt.Sscanf(retryMatchNumber[1], "%d", &amount)
		return baseTime.Add(time.Duration(amount) * time.Second)
	}

	retryMatch := regexp.MustCompile(`(?i)^(\d+)\s*(second|minute|hour)s?$`).FindStringSubmatch(input)
	if len(retryMatch) > 2 {
		var duration time.Duration
		var amount int
		fmt.Sscanf(retryMatch[1], "%d", &amount)
		switch strings.ToLower(retryMatch[2]) {
		case "second", "seconds":
			duration = time.Duration(amount) * time.Second
		case "minute", "minutes":
			duration = time.Duration(amount) * time.Minute
		case "hour", "hours":
			duration = time.Duration(amount) * time.Hour
		}
		return baseTime.Add(duration)
	}

	retryMatchFull := regexp.MustCompile(`(?i)retry\s*after\s*(\d+)\s*(second|minute|hour)s?`).FindStringSubmatch(input)
	if len(retryMatchFull) > 2 {
		var duration time.Duration
		var amount int
		fmt.Sscanf(retryMatchFull[1], "%d", &amount)
		switch strings.ToLower(retryMatchFull[2]) {
		case "second", "seconds":
			duration = time.Duration(amount) * time.Second
		case "minute", "minutes":
			duration = time.Duration(amount) * time.Minute
		case "hour", "hours":
			duration = time.Duration(amount) * time.Hour
		}
		return baseTime.Add(duration)
	}

	timeFormats := []string{
		"3:04 PM",
		"3:04:05 PM",
		"15:04",
		"15:04:05",
		"2006-01-02T15:04:05",
	}

	for _, format := range timeFormats {
		if parsed, err := time.Parse(format, input); err == nil {
			if parsed.Year() == 0 {
				parsed = time.Date(baseTime.Year(), baseTime.Month(), baseTime.Day(),
					parsed.Hour(), parsed.Minute(), parsed.Second(), 0, baseTime.Location())
			}
			if parsed.Before(baseTime) {
				parsed = parsed.AddDate(0, 0, 1)
			}
			return parsed
		}
	}

	return time.Time{}
}

func (d *Detector) GetState() RateLimitState {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.currentState
}

func (d *Detector) SetState(state RateLimitState) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.currentState = state
	// Clear the reset time when returning to StateNone so stale time does not
	// leak into the proto after recovery.
	if state == StateNone {
		d.currentResetTime = time.Time{}
	}
}

func (d *Detector) GetResetTime() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.currentResetTime
}

func stripANSI(input string) string {
	ansiRegex := regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)
	return ansiRegex.ReplaceAllString(input, "")
}

func init() {
	for _, patterns := range providerSpecificPatterns {
		for _, p := range patterns {
			_ = p.String()
		}
	}
}
