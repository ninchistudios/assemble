package web

import (
	"bytes"
	"log"
	"strings"
	"testing"
	"time"
)

func TestLogLimiterCapsAndSummarises(t *testing.T) {
	var buf bytes.Buffer
	now := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	l := newLogLimiter(3, time.Hour)
	l.out = log.New(&buf, "", 0)
	l.now = func() time.Time { return now }

	for i := range 10 {
		l.Printf("refused %d", i)
	}
	if got := strings.Count(buf.String(), "refused"); got != 3 {
		t.Fatalf("lines in first window = %d, want 3\n%s", got, buf.String())
	}

	now = now.Add(time.Hour)
	l.Printf("refused again")
	out := buf.String()
	if !strings.Contains(out, "(7 similar log lines suppressed") || !strings.Contains(out, "refused again") {
		t.Errorf("new window did not summarise and reset:\n%s", out)
	}
}
