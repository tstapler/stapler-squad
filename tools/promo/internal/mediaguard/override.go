package mediaguard

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/tstapler/stapler-squad/tools/promo/internal/codeowners"
	"github.com/tstapler/stapler-squad/tools/promo/internal/flags"
	"github.com/tstapler/stapler-squad/tools/promolib/gitbase"
)

// CodeownersPath is where the base CODEOWNERS file lives.
const CodeownersPath = ".github/CODEOWNERS"

// Event is one entry of the PR issue events API, reduced to what the override needs.
type Event struct {
	Event string `json:"event"`
	Label struct {
		Name string `json:"name"`
	} `json:"label"`
	Actor struct {
		Login string `json:"login"`
		Type  string `json:"type"`
	} `json:"actor"`
	CreatedAt string `json:"created_at"`
}

// Decision is the outcome of the allow-media authority check.
type Decision struct {
	Allowed bool
	Actor   string
	Reason  string
}

// ParseEvents reads one or more concatenated JSON arrays (gh api --paginate).
func ParseEvents(r io.Reader) ([]Event, error) {
	dec := json.NewDecoder(r)
	var all []Event
	for {
		var page []Event
		if err := dec.Decode(&page); errors.Is(err, io.EOF) {
			return all, nil
		} else if err != nil {
			return nil, fmt.Errorf("parse events JSON: %w", err)
		}
		all = append(all, page...)
	}
}

// Authorize applies the override rule: the latest labeled/unlabeled event for the
// label must be a `labeled` made by a non-bot User listed as an individual user
// in owners. Teams cannot be resolved with GITHUB_TOKEN, so they never count.
func Authorize(events []Event, owners []string) Decision {
	var relevant []Event
	for _, e := range events {
		if (e.Event == "labeled" || e.Event == "unlabeled") && e.Label.Name == Label {
			relevant = append(relevant, e)
		}
	}
	if len(relevant) == 0 {
		return Decision{Reason: "no labeled event for the label was found"}
	}
	sort.SliceStable(relevant, func(i, j int) bool { return relevant[i].CreatedAt < relevant[j].CreatedAt })
	latest := relevant[len(relevant)-1]
	if latest.Event != "labeled" {
		return Decision{Reason: "the label was removed (latest event is unlabeled)"}
	}
	login := latest.Actor.Login
	if latest.Actor.Type != "User" || strings.HasSuffix(login, "[bot]") {
		return Decision{Actor: login, Reason: fmt.Sprintf("actor %s is a bot, not a User", login)}
	}
	users, skipped := codeowners.Split(owners)
	for _, u := range users {
		if strings.EqualFold(u, login) {
			return Decision{Allowed: true, Actor: login}
		}
	}
	reason := fmt.Sprintf("labeller %s not listed as a user in %s", login, CodeownersPath)
	if len(skipped) > 0 {
		reason = fmt.Sprintf("labeller %s not listed as a user; teams cannot be resolved with GITHUB_TOKEN", login)
	}
	return Decision{Actor: login, Reason: reason}
}

func decideOverride(r *gitbase.Repo, base, eventsFile string) (Decision, error) {
	if eventsFile == "" {
		return Decision{Reason: "no -events-json supplied, cannot establish who applied the label"}, nil
	}
	raw, err := os.ReadFile(eventsFile)
	if err != nil {
		return Decision{}, err
	}
	events, err := ParseEvents(bytes.NewReader(raw))
	if err != nil {
		return Decision{}, err
	}
	co, err := r.ReadBlob(base, CodeownersPath)
	if errors.Is(err, gitbase.ErrNotFound) {
		return Decision{Reason: "base has no " + CodeownersPath + ", so nobody holds override authority"}, nil
	}
	if err != nil {
		return Decision{}, err
	}
	return Authorize(events, codeowners.Parse(co).Owners(flags.Path)), nil
}
