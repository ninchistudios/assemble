package words

import "testing"

func TestClean(t *testing.T) {
	for in, want := range map[string]string{
		"  Jo   the\tDrummer\n":         "Jo the Drummer",
		"evil\u202ename":                "evilname", // right-to-left override
		"a\u0000b\u200bc":               "abc",      // NUL, zero width space
		"family 👨\u200d👩\u200d👧":        "family 👨\u200d👩\u200d👧",
		"Robert'); DROP TABLE users;--": "Robert'); DROP TABLE users;--",
	} {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFilter(t *testing.T) {
	f := New("darn, Heck , bad apple,,")
	for _, tc := range []struct {
		in, masked string
		blocked    bool
	}{
		{"oh darn it", "oh *** it", true},
		{"DARN!", "***!", true},
		{"what the heck-heck", "what the ***-***", true},
		{"a bad apple pie", "a *** pie", true},
		{"a bad pie", "a bad pie", false},       // phrase needs both words
		{"darned socks", "darned socks", false}, // whole words only
		{"Scunthorpe", "Scunthorpe", false},
		{"", "", false},
	} {
		if got := f.Mask(tc.in); got != tc.masked {
			t.Errorf("Mask(%q) = %q, want %q", tc.in, got, tc.masked)
		}
		if got := f.Blocked(tc.in); got != tc.blocked {
			t.Errorf("Blocked(%q) = %v", tc.in, got)
		}
	}
}

func TestEmptyFilterPassesEverything(t *testing.T) {
	for _, f := range []*Filter{New(""), New(" , ,"), nil} {
		if f.Blocked("anything at all") || f.Mask("anything") != "anything" {
			t.Errorf("empty filter %v blocked or masked", f)
		}
	}
}
