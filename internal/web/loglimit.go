package web

import (
	"log"
	"sync"
	"time"
)

// logLimiter caps how many lines a noisy source may write per window, then
// summarises what it dropped. Refusals are the only log lines a stranger can
// trigger, and an unattended instance should not let a scanner fill the disk
// over months.
type logLimiter struct {
	max    int
	window time.Duration
	out    *log.Logger
	now    func() time.Time

	mu      sync.Mutex
	start   time.Time
	n       int
	dropped int
}

func newLogLimiter(max int, window time.Duration) *logLimiter {
	return &logLimiter{max: max, window: window, out: log.Default(), now: time.Now}
}

func (l *logLimiter) Printf(format string, args ...any) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	if now.Sub(l.start) >= l.window {
		if l.dropped > 0 {
			l.out.Printf("(%d similar log lines suppressed in the last %s)", l.dropped, l.window)
		}
		l.start, l.n, l.dropped = now, 0, 0
	}
	if l.n >= l.max {
		l.dropped++
		return
	}
	l.n++
	l.out.Printf(format, args...)
}
