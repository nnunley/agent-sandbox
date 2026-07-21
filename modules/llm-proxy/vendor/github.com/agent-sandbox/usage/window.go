package usage

import (
	"sort"
	"time"
)

// WindowClass is a usage window of a fixed length, anchored on first-use-after-idle.
type WindowClass struct {
	Name   string
	Length time.Duration
}

var (
	// Window5h is the Claude Code Max ~5-hour session window (anchored on first use).
	Window5h = WindowClass{Name: "5h", Length: 5 * time.Hour}
	// WindowWeekly is the longer weekly window, tracked the same way.
	WindowWeekly = WindowClass{Name: "weekly", Length: 7 * 24 * time.Hour}
)

// CurrentWindow derives the active window's anchor and reset for class wc from events,
// at time now. Anchor floats: it starts at the earliest event and re-anchors to any event
// at/after anchor+Length. The window is active iff now is before anchor+Length.
func CurrentWindow(events []UsageEvent, wc WindowClass, now time.Time) (anchor, reset time.Time, active bool) {
	if len(events) == 0 {
		return time.Time{}, time.Time{}, false
	}
	ts := make([]time.Time, 0, len(events))
	for _, e := range events {
		if !e.Ts.After(now) { // only events up to now
			ts = append(ts, e.Ts)
		}
	}
	if len(ts) == 0 {
		return time.Time{}, time.Time{}, false
	}
	sort.Slice(ts, func(i, j int) bool { return ts[i].Before(ts[j]) })
	anchor = ts[0]
	for _, t := range ts[1:] {
		if !t.Before(anchor.Add(wc.Length)) { // t >= anchor+Length → re-anchor
			anchor = t
		}
	}
	reset = anchor.Add(wc.Length)
	active = now.Before(reset)
	return anchor, reset, active
}
