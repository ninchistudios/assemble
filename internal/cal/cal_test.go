package cal

import (
	"testing"
	"time"
)

func TestParseDay(t *testing.T) {
	for s, ok := range map[string]bool{
		"2026-11-14": true,
		"2026-2-3":   false,
		"2026-02-30": false,
		"14/11/2026": false,
		"":           false,
	} {
		if _, err := ParseDay(s); (err == nil) != ok {
			t.Errorf("ParseDay(%q) err = %v, want ok=%v", s, err, ok)
		}
	}
}

// Today follows the group's clock, not the server's: 23:30 UTC on the 1st is
// already the 2nd in Brisbane.
func TestTodayUsesGroupTimezone(t *testing.T) {
	bne, _ := time.LoadLocation("Australia/Brisbane")
	now := time.Date(2026, 11, 1, 23, 30, 0, 0, time.UTC)
	if got := Today(now, bne); got != "2026-11-02" {
		t.Errorf("Today in Brisbane = %s, want 2026-11-02", got)
	}
	if got := Today(now, time.UTC); got != "2026-11-01" {
		t.Errorf("Today in UTC = %s", got)
	}
}

// Sydney moves to daylight saving on the first Sunday of October. Day
// arithmetic must not skip or repeat a date across it.
func TestDayArithmeticAcrossDST(t *testing.T) {
	d := Day("2026-10-03")
	if got := d.Add(1); got != "2026-10-04" {
		t.Errorf("Add(1) across DST = %s", got)
	}
	if got := d.Add(2); got != "2026-10-05" {
		t.Errorf("Add(2) across DST = %s", got)
	}
	if got := Day("2026-03-01").Add(-1); got != "2026-02-28" {
		t.Errorf("Add(-1) into February = %s", got)
	}
}

func TestWindow(t *testing.T) {
	w := WindowFor("2026-11-30")
	if w.Last != "2027-01-30" {
		t.Errorf("horizon = %s, want 2027-01-30", w.Last)
	}
	for d, want := range map[Day][3]bool{ // editable, past, beyond
		"2026-11-29": {false, true, false},
		"2026-11-30": {true, false, false},
		"2027-01-30": {true, false, false},
		"2027-01-31": {false, false, true},
	} {
		got := [3]bool{w.Editable(d), w.Past(d), w.Beyond(d)}
		if got != want {
			t.Errorf("%s: editable/past/beyond = %v, want %v", d, got, want)
		}
	}
	days := w.Days()
	if days[0] != "2026-11-30" || days[len(days)-1] != "2027-01-30" || len(days) != 62 {
		t.Errorf("Days = %s..%s (%d)", days[0], days[len(days)-1], len(days))
	}
}

func TestLabels(t *testing.T) {
	if got := Day("2026-11-14").Label("2026-11-01"); got != "Saturday 14 November" {
		t.Errorf("Label = %q", got)
	}
	if got := Day("2027-01-02").Label("2026-12-20"); got != "Saturday 2 January 2027" {
		t.Errorf("Label next year = %q", got)
	}
	if got := Day("2026-11-14").Short(); got != "Sat 14 Nov" {
		t.Errorf("Short = %q", got)
	}
}

func TestStatusRules(t *testing.T) {
	for _, tc := range []struct {
		from, to Status
		cancels  bool
	}{
		{Yes, No, true},
		{OK, Unknown, true},
		{Yes, OK, false},
		{No, Unknown, false}, // an override-locked day isn't cancelled by this
		{Unknown, No, false},
		{No, Yes, false},
	} {
		if got := Cancels(tc.from, tc.to); got != tc.cancels {
			t.Errorf("Cancels(%q, %q) = %v", tc.from, tc.to, got)
		}
	}
	for s, want := range map[string]Status{"yes": Yes, "ok": OK, "no": No, "unknown": Unknown, "": Unknown} {
		if got, err := ParseStatus(s); err != nil || got != want {
			t.Errorf("ParseStatus(%q) = %q, %v", s, got, err)
		}
	}
	if _, err := ParseStatus("maybe"); err == nil {
		t.Error("ParseStatus accepted maybe")
	}
}
