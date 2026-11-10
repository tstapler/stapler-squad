package svctest

import (
	"github.com/tstapler/stapler-squad/server/events"
)

func NotificationTitles(evs []*events.Event) []string {
	var titles []string
	for _, ev := range evs {
		if ev.Type == events.EventNotification {
			titles = append(titles, ev.NotificationTitle)
		}
	}
	return titles
}
