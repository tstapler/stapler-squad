// Command baseline_report computes the attention-inbox baselines B1 (needs-operator
// episodes that were detected but missed, plus the sampled undetected-question
// estimate) and U1 (daily /review-queue opens). All data sources are opened read-only.
// Method and limitations: docs/reference/attention-inbox.md.
package main

import (
	"bufio"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite" // pure-Go driver, already a module dependency
)

// Notification type values mirror session/v1/types.proto (see server/notifications/store.go).
const (
	typeApprovalNeeded     int32 = 1
	typeInputRequired      int32 = 2
	typeConfirmationNeeded int32 = 3
	typeError              int32 = 7
	typeFailure            int32 = 9
)

const (
	defaultWindowDays  = 30
	defaultMissMinutes = 30
	// relativeTargetFloor is the B1 level below which "at most 5%" would pass vacuously.
	relativeTargetFloor = 0.05
)

// Notification is the subset of a notification-history record the baseline needs.
type Notification struct {
	SessionID        string            `json:"session_id"`
	NotificationType int32             `json:"notification_type"`
	Metadata         map[string]string `json:"metadata"`
	CreatedAt        time.Time         `json:"created_at"`
	LastOccurredAt   *time.Time        `json:"last_occurred_at"`
	ReadAt           *time.Time        `json:"read_at"`
}

// EpisodeTime is the most recent occurrence: history dedups repeats into one record, and the
// operator-touch columns only hold the latest value, so the latest occurrence is the only
// consistent comparison point.
func (n Notification) EpisodeTime() time.Time {
	if n.LastOccurredAt != nil {
		return *n.LastOccurredAt
	}
	return n.CreatedAt
}

// Touch holds the latest operator-touch timestamps recorded for a session.
type Touch struct {
	Program          string
	LastViewed       time.Time
	LastUserResponse time.Time
	LastAcknowledged time.Time
}

// ProgramResult is the detected-but-missed tally for one program.
type ProgramResult struct {
	Episodes int
	Missed   int
}

// Rate returns missed/episodes, or 0 when there are no episodes.
func (p ProgramResult) Rate() float64 {
	if p.Episodes == 0 {
		return 0
	}
	return float64(p.Missed) / float64(p.Episodes)
}

// DetectedReport is baseline (a): detected-but-missed episodes per program.
type DetectedReport struct {
	RequestedDays int
	CoveredFrom   time.Time
	CoveredTo     time.Time
	CoveredDays   float64
	PerProgram    map[string]ProgramResult
	Total         ProgramResult
	Limitations   []string
}

func isNeedsOperator(n Notification) bool {
	switch n.NotificationType {
	case typeApprovalNeeded, typeInputRequired, typeConfirmationNeeded, typeError, typeFailure:
		return n.Metadata["auto_remediating"] != "true"
	}
	return false
}

func touchedWithin(t Touch, n Notification, cutoff time.Duration) bool {
	start := n.EpisodeTime()
	end := start.Add(cutoff)
	in := func(ts time.Time) bool { return !ts.IsZero() && !ts.Before(start) && !ts.After(end) }
	if n.ReadAt != nil && in(*n.ReadAt) {
		return true
	}
	return in(t.LastViewed) || in(t.LastUserResponse) || in(t.LastAcknowledged)
}

func programOf(touches map[string]Touch, sessionID string) string {
	t, ok := touches[sessionID]
	if !ok || t.Program == "" {
		return "unknown (session deleted)"
	}
	return t.Program
}

// ComputeDetectedMissed counts needs-operator episodes in the trailing windowDays and how many
// had no operator touch within cutoff. The covered window is the span of data actually seen.
func ComputeDetectedMissed(records []Notification, touches map[string]Touch, now time.Time, windowDays int, cutoff time.Duration) DetectedReport {
	rep := DetectedReport{RequestedDays: windowDays, PerProgram: map[string]ProgramResult{}}
	from := now.AddDate(0, 0, -windowDays)
	for _, n := range records {
		at := n.EpisodeTime()
		if at.Before(from) || !isNeedsOperator(n) {
			continue
		}
		prog := programOf(touches, n.SessionID)
		res := rep.PerProgram[prog]
		res.Episodes++
		missed := !touchedWithin(touches[n.SessionID], n, cutoff)
		res.Missed += boolToInt(missed)
		rep.PerProgram[prog] = res
		rep.Total.Episodes++
		rep.Total.Missed += boolToInt(missed)
		if rep.CoveredFrom.IsZero() || at.Before(rep.CoveredFrom) {
			rep.CoveredFrom = at
		}
		if at.After(rep.CoveredTo) {
			rep.CoveredTo = at
		}
	}
	if !rep.CoveredFrom.IsZero() {
		rep.CoveredDays = rep.CoveredTo.Sub(rep.CoveredFrom).Hours() / 24
	}
	rep.Limitations = limitations(rep)
	return rep
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func limitations(rep DetectedReport) []string {
	out := []string{
		"Notification history only holds what was detected, so (a) understates misses; (b) is a sample, not a census.",
		"History dedups repeats into one record (first/last occurrence only), so an episode is one record.",
		"The 30-minute 'missed' cutoff is arbitrary.",
		"Touch columns hold only the latest value; a touch inside the cutoff that was later overwritten is not seen, so misses may be overstated.",
		"A view without input counts as touched only through last_viewed/read_at, where recorded.",
	}
	if rep.CoveredDays < float64(rep.RequestedDays) {
		out = append(out, fmt.Sprintf("Covered window is %.1f days (%s to %s), not the requested %d days; retained history is shorter than requested.",
			rep.CoveredDays, rep.CoveredFrom.Format("2006-01-02"), rep.CoveredTo.Format("2006-01-02"), rep.RequestedDays))
	}
	return out
}

// LabelledTurn is one owner-labelled transcript turn end.
type LabelledTurn struct {
	ID              string
	IsQuestion      bool
	TouchedWithin30 bool
}

// SampleReport is baseline (b); it is reported separately from (a) because it is a sample.
type SampleReport struct {
	N                  int
	Questions          int
	QuestionsUntouched int
	Pending            bool
}

// SummarizeSample tallies owner labels. With no labels the estimate is pending.
func SummarizeSample(labelled []LabelledTurn) SampleReport {
	rep := SampleReport{N: len(labelled), Pending: len(labelled) == 0}
	for _, l := range labelled {
		if !l.IsQuestion {
			continue
		}
		rep.Questions++
		if !l.TouchedWithin30 {
			rep.QuestionsUntouched++
		}
	}
	return rep
}

// TargetRule is the coverage target selected from B1.
type TargetRule struct {
	Description string
}

// SelectTarget returns the relative target, switching rule when B1 is below the 5% floor.
func SelectTarget(b1 float64) TargetRule {
	if b1 < relativeTargetFloor {
		return TargetRule{Description: "missed share does not exceed B1 AND prose-question recall on the owner-labelled corpus >= 0.80"}
	}
	return TargetRule{Description: "missed share falls to at most half of B1 (or at most 5%, whichever is larger)"}
}

// PageViewBaseline is U1: the mean daily opens over the trailing days that ended before today.
type PageViewBaseline struct {
	Days  int
	Total int
	Mean  float64
}

// ComputeU1 sums per-day counts (keyed YYYY-MM-DD) over the last `days` complete days before now.
func ComputeU1(perDay map[string]int, now time.Time, days int) PageViewBaseline {
	res := PageViewBaseline{Days: days}
	for i := 1; i <= days; i++ {
		res.Total += perDay[now.AddDate(0, 0, -i).Format("2006-01-02")]
	}
	res.Mean = float64(res.Total) / float64(days)
	return res
}

func loadNotifications(path string) ([]Notification, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- operator-supplied data dir
	if err != nil {
		return nil, fmt.Errorf("read notifications %s: %w", path, err)
	}
	var file struct {
		Notifications []Notification `json:"notifications"`
	}
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse notifications %s: %w", path, err)
	}
	return file.Notifications, nil
}

func openReadOnly(path string) (*sql.DB, error) {
	u := url.URL{Scheme: "file", Opaque: (&url.URL{Path: path}).EscapedPath(), RawQuery: "mode=ro&_pragma=busy_timeout(5000)"}
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	return db, nil
}

// parseSQLTime returns the zero time for NULL and an error for a value no known layout parses.
func parseSQLTime(s sql.NullString) (time.Time, error) {
	if !s.Valid {
		return time.Time{}, nil
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999999-07:00", "2006-01-02T15:04:05.999999999Z07:00", "2006-01-02 15:04:05.999999999 -0700 MST"} {
		if t, err := time.Parse(layout, strings.SplitN(s.String, " m=", 2)[0]); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unparseable timestamp %q", s.String)
}

var programNameRe = regexp.MustCompile(`[^/\s]+$`)

func programName(raw string) string {
	first := strings.Fields(raw)
	if len(first) == 0 {
		return ""
	}
	return programNameRe.FindString(first[0])
}

// loadTouches reads per-session operator touches. unparsed counts timestamp values no known
// layout parsed; those touches are treated as absent, so misses are overstated by that amount.
func loadTouches(dbPath string) (touches map[string]Touch, unparsed int, err error) {
	db, err := openReadOnly(dbPath)
	if err != nil {
		return nil, 0, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT uuid, program, last_viewed, last_user_response, last_acknowledged FROM sessions`)
	if err != nil {
		return nil, 0, fmt.Errorf("query sessions in %s: %w", dbPath, err)
	}
	defer rows.Close()
	touches = map[string]Touch{}
	for rows.Next() {
		var uuid, program string
		var v, r, a sql.NullString
		if err := rows.Scan(&uuid, &program, &v, &r, &a); err != nil {
			return nil, 0, fmt.Errorf("scan sessions in %s: %w", dbPath, err)
		}
		t := Touch{Program: programName(program)}
		for _, f := range []struct {
			raw sql.NullString
			dst *time.Time
		}{{v, &t.LastViewed}, {r, &t.LastUserResponse}, {a, &t.LastAcknowledged}} {
			parsed, perr := parseSQLTime(f.raw)
			if perr != nil {
				unparsed++
			}
			*f.dst = parsed
		}
		touches[uuid] = t
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read sessions in %s: %w", dbPath, err)
	}
	return touches, unparsed, nil
}

func loadPageViewsPerDay(dbPath string, pages []string) (map[string]int, error) {
	db, err := openReadOnly(dbPath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	args := make([]any, len(pages))
	for i, p := range pages {
		args[i] = p
	}
	q := `SELECT substr(created_at,1,10), count(*) FROM analytics_events WHERE event_name='page_view' AND page IN (` +
		strings.TrimSuffix(strings.Repeat("?,", len(pages)), ",") + `) GROUP BY 1`
	rows, err := db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("query page views in %s: %w", dbPath, err)
	}
	defer rows.Close()
	out := map[string]int{}
	for rows.Next() {
		var day string
		var n int
		if err := rows.Scan(&day, &n); err != nil {
			return nil, fmt.Errorf("scan page views in %s: %w", dbPath, err)
		}
		out[day] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read page views in %s: %w", dbPath, err)
	}
	return out, nil
}

// lastAssistantText returns the text of the final assistant text block in a transcript JSONL.
func lastAssistantText(r io.Reader) string {
	type block struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type line struct {
		Type    string `json:"type"`
		Message struct {
			Content json.RawMessage `json:"content"`
		} `json:"message"`
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 1<<20), 64<<20)
	last := ""
	for sc.Scan() {
		var l line
		if json.Unmarshal(sc.Bytes(), &l) != nil || l.Type != "assistant" {
			continue
		}
		var blocks []block
		if json.Unmarshal(l.Message.Content, &blocks) != nil {
			continue
		}
		for _, b := range blocks {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				last = b.Text
			}
		}
	}
	return last
}

type transcriptFile struct {
	path string
	mod  time.Time
}

// recentTranscripts returns up to n transcripts, newest first.
func recentTranscripts(root string, n int) []transcriptFile {
	var files []transcriptFile
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".jsonl") || strings.Contains(p, "subagents") {
			return nil
		}
		if info, e := d.Info(); e == nil {
			files = append(files, transcriptFile{p, info.ModTime()})
		}
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].mod.After(files[j].mod) })
	if len(files) > n {
		files = files[:n]
	}
	return files
}

// HarvestSample writes the last assistant message of up to n recent transcripts to outCSV
// (id, mtime, text, is_question, touched_within_30m; the last two blank for the owner) and
// returns how many were harvested. Nothing is written inside the repository by default.
func HarvestSample(root string, n int, outCSV string) (int, error) {
	f, err := os.OpenFile(outCSV, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open harvest output %s: %w", outCSV, err)
	}
	defer f.Close()
	w := csv.NewWriter(f)
	_ = w.Write([]string{"id", "mtime", "last_assistant_text", "is_question", "touched_within_30m"})
	count := 0
	for _, tf := range recentTranscripts(root, n) {
		if count >= n {
			break
		}
		in, err := os.Open(tf.path) // #nosec G304 -- walked under operator-supplied root
		if err != nil {
			continue
		}
		text := lastAssistantText(in)
		_ = in.Close()
		if text == "" {
			continue
		}
		_ = w.Write([]string{strings.TrimSuffix(filepath.Base(tf.path), ".jsonl"), tf.mod.Format(time.RFC3339), text, "", ""})
		count++
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return count, fmt.Errorf("write harvest output %s: %w", outCSV, err)
	}
	return count, nil
}

// LoadLabels reads an owner-labelled harvest CSV; rows with a blank is_question are skipped.
func LoadLabels(r io.Reader) ([]LabelledTurn, error) {
	rows, err := csv.NewReader(r).ReadAll()
	if err != nil {
		return nil, fmt.Errorf("read labels csv: %w", err)
	}
	var out []LabelledTurn
	for i, row := range rows {
		if i == 0 || len(row) < 5 || strings.TrimSpace(row[3]) == "" {
			continue
		}
		out = append(out, LabelledTurn{ID: row[0], IsQuestion: truthy(row[3]), TouchedWithin30: truthy(row[4])})
	}
	return out, nil
}

func truthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "y", "yes":
		return true
	}
	return false
}

func renderDetected(w io.Writer, rep DetectedReport) {
	fmt.Fprintf(w, "B1(a) detected-but-missed: window %s to %s (%.1f days covered of %d requested)\n",
		rep.CoveredFrom.Format("2006-01-02"), rep.CoveredTo.Format("2006-01-02"), rep.CoveredDays, rep.RequestedDays)
	progs := make([]string, 0, len(rep.PerProgram))
	for p := range rep.PerProgram {
		progs = append(progs, p)
	}
	sort.Strings(progs)
	for _, p := range progs {
		r := rep.PerProgram[p]
		fmt.Fprintf(w, "  %-28s episodes=%d missed=%d rate=%.1f%%\n", p, r.Episodes, r.Missed, 100*r.Rate())
	}
	fmt.Fprintf(w, "  %-28s episodes=%d missed=%d rate=%.1f%%\n", "ALL", rep.Total.Episodes, rep.Total.Missed, 100*rep.Total.Rate())
	for _, l := range rep.Limitations {
		fmt.Fprintf(w, "  limitation: %s\n", l)
	}
}

func renderSample(w io.Writer, s SampleReport, harvested int) {
	if s.Pending {
		fmt.Fprintf(w, "B1(b) undetected-question sample: pending owner labelling (N harvested=%d, N labelled=0)\n", harvested)
		return
	}
	fmt.Fprintf(w, "B1(b) undetected-question sample (a sample, not a census): N labelled=%d questions=%d untouched-within-30m=%d\n",
		s.N, s.Questions, s.QuestionsUntouched)
}

func logWindow(dir string) string {
	files, _ := filepath.Glob(filepath.Join(dir, "staplersquad-*.log.gz"))
	if len(files) == 0 {
		return "no rotated logs"
	}
	sort.Strings(files)
	return fmt.Sprintf("rotated logs %d files, first %s", len(files), strings.TrimSuffix(strings.TrimPrefix(filepath.Base(files[0]), "staplersquad-"), ".log.gz"))
}

func run(w io.Writer, args []string) error {
	fs := flag.NewFlagSet("baseline_report", flag.ContinueOnError)
	dataDir := fs.String("data-dir", "", "state dir holding notifications.json, sessions.db, analytics.db (read-only)")
	logDir := fs.String("log-dir", "", "log dir (defaults to <data-dir>/logs)")
	windowDays := fs.Int("window-days", defaultWindowDays, "requested window in days")
	missMin := fs.Int("miss-minutes", defaultMissMinutes, "operator-touch cutoff in minutes")
	transcripts := fs.String("transcripts-root", "", "Claude projects dir to harvest last assistant messages from")
	sampleN := fs.Int("sample-n", 100, "transcripts to harvest")
	harvestOut := fs.String("harvest-out", "", "CSV path (outside the repo) for harvested text to label")
	labels := fs.String("labels", "", "owner-labelled harvest CSV")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dataDir == "" {
		return fmt.Errorf("--data-dir is required")
	}
	now := time.Now()
	recs, err := loadNotifications(filepath.Join(*dataDir, "notifications.json"))
	if err != nil {
		return err
	}
	touches, unparsed, err := loadTouches(filepath.Join(*dataDir, "sessions.db"))
	if err != nil {
		return err
	}
	if unparsed > 0 {
		fmt.Fprintf(w, "warning: %d unparseable touch timestamps in sessions.db treated as absent; detected-but-missed is overstated by up to that many touches\n", unparsed)
	}
	renderDetected(w, ComputeDetectedMissed(recs, touches, now, *windowDays, time.Duration(*missMin)*time.Minute))
	ld := *logDir
	if ld == "" {
		ld = filepath.Join(*dataDir, "logs")
	}
	fmt.Fprintf(w, "logs: %s\n", logWindow(ld))
	harvested := 0
	if *transcripts != "" && *harvestOut != "" {
		if harvested, err = HarvestSample(*transcripts, *sampleN, *harvestOut); err != nil {
			return fmt.Errorf("harvest transcripts from %s: %w", *transcripts, err)
		}
	}
	var labelled []LabelledTurn
	if *labels != "" {
		f, err := os.Open(*labels) // #nosec G304 -- operator-supplied
		if err != nil {
			return fmt.Errorf("open labels %s: %w", *labels, err)
		}
		defer f.Close()
		if labelled, err = LoadLabels(f); err != nil {
			return fmt.Errorf("load labels %s: %w", *labels, err)
		}
	}
	renderSample(w, SummarizeSample(labelled), harvested)
	return renderU1(w, filepath.Join(*dataDir, "analytics.db"), now)
}

func renderU1(w io.Writer, dbPath string, now time.Time) error {
	for _, pages := range [][]string{{"/review-queue"}, {"/review-queue", "/review-queue/"}} {
		perDay, err := loadPageViewsPerDay(dbPath, pages)
		if err != nil {
			return err
		}
		u := ComputeU1(perDay, now, 14)
		fmt.Fprintf(w, "U1 pages=%v: total=%d over %d complete days, mean/day=%.2f\n", pages, u.Total, u.Days, u.Mean)
	}
	return nil
}

func main() {
	if err := run(os.Stdout, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "baseline_report:", err)
		os.Exit(1)
	}
}
