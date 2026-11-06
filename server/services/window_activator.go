package services

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/tstapler/stapler-squad/executor/safeexec"
)

// ErrAppNotRunning means the target app has no running process, so activating it
// would launch a fresh instance instead of focusing an existing window.
var ErrAppNotRunning = errors.New("application is not running")

// WindowActivator brings an already-running application to the foreground.
type WindowActivator interface {
	// Activate focuses the app identified by bundleID, or by appName when bundleID is empty.
	// It returns ErrAppNotRunning rather than launching the app.
	Activate(ctx context.Context, bundleID, appName string) error
}

// nonDeveloperBundleIDs are media apps that macOS gates behind their own privacy
// prompt ("would like to access Apple Music…"); a stale notification must never wake them.
var nonDeveloperBundleIDs = map[string]struct{}{
	"com.apple.Music":    {},
	"com.apple.TV":       {},
	"com.apple.Photos":   {},
	"com.apple.iTunesU":  {},
	"com.apple.podcasts": {},
}

// nonDeveloperAppNames covers requests that carry only an app name (no bundle ID).
var nonDeveloperAppNames = map[string]struct{}{
	"music": {}, "tv": {}, "photos": {}, "podcasts": {},
}

func isBlockedBundleID(bundleID string) bool {
	_, blocked := nonDeveloperBundleIDs[bundleID]
	return blocked
}

// isBlockedTarget applies the media-app blocklist to whichever identifier the request carries.
func isBlockedTarget(bundleID, appName string) bool {
	if bundleID != "" {
		return isBlockedBundleID(bundleID)
	}
	_, blocked := nonDeveloperAppNames[strings.ToLower(strings.TrimSpace(appName))]
	return blocked
}

var bundleIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

type commandRunner func(ctx context.Context, name string, args ...string) ([]byte, error)

func runSafeexec(ctx context.Context, name string, args ...string) ([]byte, error) {
	return safeexec.CommandContext(ctx, name, args...).CombinedOutput()
}

// launchServicesActivator uses lsappinfo (running check) and open (activation).
// Both go through Launch Services, so macOS raises no per-app Automation (Apple
// Events) consent dialog, unlike `osascript … to activate`.
type launchServicesActivator struct {
	run commandRunner
}

func newLaunchServicesActivator() *launchServicesActivator {
	return &launchServicesActivator{run: runSafeexec}
}

func (a *launchServicesActivator) Activate(ctx context.Context, bundleID, appName string) error {
	var findKey, openFlag, target string
	switch {
	case bundleID != "":
		if !bundleIDPattern.MatchString(bundleID) {
			return fmt.Errorf("invalid bundle id %q", bundleID)
		}
		findKey, openFlag, target = "bundleid="+bundleID, "-b", bundleID
	default:
		findKey, openFlag, target = "name="+appName, "-a", appName
	}

	// open launches apps that aren't running, so an unconfirmed check must fail
	// rather than fall through.
	out, err := a.run(ctx, "/usr/bin/lsappinfo", "find", findKey)
	if err != nil {
		return fmt.Errorf("cannot confirm %q is running: %w", target, err)
	}
	if strings.TrimSpace(string(out)) == "" {
		return ErrAppNotRunning
	}

	if out, err := a.run(ctx, "/usr/bin/open", openFlag, target); err != nil {
		return fmt.Errorf("open %s %s: %w: %s", openFlag, target, err, strings.TrimSpace(string(out)))
	}
	return nil
}
