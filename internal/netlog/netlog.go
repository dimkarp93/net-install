package netlog

import (
	"fmt"
	"io"
	"time"
)

const TimeLayout = "2006-01-02T15:04:05Z"

type Logger struct {
	w   io.Writer
	now func() time.Time
}

func New(w io.Writer, now func() time.Time) *Logger {
	if now == nil {
		now = time.Now
	}
	return &Logger{w: w, now: now}
}

func (l *Logger) Log(s string) {
	fmt.Fprintf(l.w, "[net] ts=%s %s\n", l.now().UTC().Format(TimeLayout), s)
}

func (l *Logger) Logf(format string, args ...any) {
	l.Log(fmt.Sprintf(format, args...))
}
