// Package cal holds the calendar rules: what a day is, which days can be
// seen and changed, and what each availability status means. It knows
// nothing about storage or HTTP. See docs/calendar.md.
package cal

import (
	"fmt"
	"time"
)

// Day is a calendar date in the group's timezone, "2006-01-02". It is not an
// instant: a day stays the same day when daylight saving changes or the admin
// changes the timezone.
type Day string

const layout = "2006-01-02"

// HorizonMonths is how far ahead availability can be set.
const HorizonMonths = 2

// ParseDay validates a YYYY-MM-DD string.
func ParseDay(s string) (Day, error) {
	t, err := time.Parse(layout, s)
	if err != nil {
		return "", fmt.Errorf("not a date: %q", s)
	}
	// Round-trip so 2026-2-3 style variants can't create a second key for
	// the same day.
	if t.Format(layout) != s {
		return "", fmt.Errorf("not a date: %q", s)
	}
	return Day(s), nil
}

// Today is the current date in loc.
func Today(now time.Time, loc *time.Location) Day {
	return Day(now.In(loc).Format(layout))
}

// civil is the day as midnight UTC: a fixed point for date arithmetic that
// no timezone rule can shift.
func (d Day) civil() time.Time {
	t, _ := time.Parse(layout, string(d))
	return t
}

// Add returns the day n days later (or earlier, for negative n).
func (d Day) Add(n int) Day { return Day(d.civil().AddDate(0, 0, n).Format(layout)) }

// Before reports whether d is earlier than o. Dates in this layout sort as
// strings.
func (d Day) Before(o Day) bool { return d < o }

func (d Day) String() string { return string(d) }

// Weekday is the day of the week the date falls on.
func (d Day) Weekday() time.Weekday { return d.civil().Weekday() }

// Label is how a day reads in headings: "Saturday 14 November", with the year
// added when it is not the year of today.
func (d Day) Label(today Day) string {
	t := d.civil()
	if t.Year() != today.civil().Year() {
		return t.Format("Monday 2 January 2006")
	}
	return t.Format("Monday 2 January")
}

// MonthLabel is the month and year the day falls in: "November 2026".
func (d Day) MonthLabel() string { return d.civil().Format("January 2006") }

// Short is the compact form for lists: "Sat 14 Nov".
func (d Day) Short() string { return d.civil().Format("Mon 2 Jan") }

// Window is the span of days that can be viewed and changed: from today to
// the end of the horizon, inclusive. Earlier days are read-only history;
// later ones don't exist yet.
type Window struct{ First, Last Day }

// WindowFor is the window as of today. The horizon is the same day of the
// month HorizonMonths on, clamped to that month's last day: AddDate alone
// would roll 31 December into 3 March and then 1 January back to 1 March,
// moving the horizon backwards overnight.
func WindowFor(today Day) Window {
	t := today.civil()
	first := time.Date(t.Year(), t.Month()+HorizonMonths, 1, 0, 0, 0, 0, time.UTC)
	lastDay := first.AddDate(0, 1, -1).Day()
	day := t.Day()
	if day > lastDay {
		day = lastDay
	}
	return Window{First: today, Last: Day(time.Date(first.Year(), first.Month(), day, 0, 0, 0, 0, time.UTC).Format(layout))}
}

// Editable reports whether availability can still be set for d. A day stays
// editable until it has ended in the group's timezone, i.e. while it is today
// or later.
func (w Window) Editable(d Day) bool { return !d.Before(w.First) && !w.Last.Before(d) }

// Past reports whether d has ended.
func (w Window) Past(d Day) bool { return d.Before(w.First) }

// Beyond reports whether d is past the horizon.
func (w Window) Beyond(d Day) bool { return w.Last.Before(d) }

// Days lists every day in the window, in order.
func (w Window) Days() []Day {
	var out []Day
	for d := w.First; !w.Last.Before(d); d = d.Add(1) {
		out = append(out, d)
	}
	return out
}

// Status is one member's availability for one day. Unknown is the absence of
// an answer, and is never stored.
type Status string

const (
	Yes     Status = "yes"
	OK      Status = "ok"
	No      Status = "no"
	Unknown Status = ""
)

// ParseStatus accepts the four statuses, with "unknown" naming [?].
func ParseStatus(s string) (Status, error) {
	switch Status(s) {
	case Yes, OK, No:
		return Status(s), nil
	case "unknown", Unknown:
		return Unknown, nil
	}
	return "", fmt.Errorf("not a status: %q", s)
}

// Available reports whether the status counts towards locking a day in:
// [YES] and [OK] do; [NO] and [?] do not.
func (s Status) Available() bool { return s == Yes || s == OK }

// Label is the status as the UI names it.
func (s Status) Label() string {
	switch s {
	case Yes:
		return "YES"
	case OK:
		return "OK"
	case No:
		return "NO"
	}
	return "?"
}

// Icon is the status's name in the icon templates and CSS.
func (s Status) Icon() string {
	if s == Unknown {
		return "unknown"
	}
	return string(s)
}

// All is the four statuses in the order the picker shows them.
var All = []Status{Yes, OK, No, Unknown}

// Cancels reports whether changing from one status to another on a
// locked-in day cancels it: only pulling out (available to unavailable)
// does. A day locked by override is not cancelled by NO -> ?.
func Cancels(from, to Status) bool { return from.Available() && !to.Available() }
