package retry

import (
	"errors"
	"time"

	"github.com/dimkarp93/net-install/internal/netlog"
)

type Coded interface {
	ExitCode() int
	Retryable() bool
}

type Options struct {
	Attempts int
	Delay    int
	All      bool
	Sleep    func(time.Duration)
}

type run struct {
	log      *netlog.Logger
	all      bool
	delay    int
	label    string
	attempts int
}

func Do(log *netlog.Logger, o Options, label string, fn func() error) error {
	sleep := sleepFunc(o.Sleep)
	r := run{log: log, all: o.All, delay: o.Delay, label: label, attempts: max(o.Attempts, 1)}

	for attempt := 1; ; attempt++ {
		log.Logf("event=start what=%s attempt=%d/%d", label, attempt, r.attempts)
		started := time.Now()

		err := fn()
		seconds := time.Now().Unix() - started.Unix()

		if !r.retry(attempt, err, seconds) {
			return err
		}
		sleep(time.Duration(o.Delay) * time.Second)
	}
}

func sleepFunc(sleep func(time.Duration)) func(time.Duration) {
	if sleep == nil {
		return time.Sleep
	}

	return sleep
}

func (r run) retry(attempt int, err error, seconds int64) bool {
	if err == nil {
		r.log.Logf("event=ok what=%s attempt=%d seconds=%d", r.label, attempt, seconds)
		return false
	}

	rc := ExitCode(err)
	if attempt >= r.attempts || !(r.all || Retryable(err)) {
		r.log.Logf("event=fail what=%s attempts=%d rc=%d seconds=%d", r.label, attempt, rc, seconds)
		return false
	}

	r.log.Logf("event=retry what=%s attempt=%d/%d rc=%d seconds=%d sleep=%ds",
		r.label, attempt, r.attempts, rc, seconds, r.delay)
	return true
}

func ExitCode(err error) int {
	var c Coded
	if errors.As(err, &c) {
		return c.ExitCode()
	}
	if err == nil {
		return 0
	}
	return 1
}

func Retryable(err error) bool {
	var c Coded
	if errors.As(err, &c) {
		return c.Retryable()
	}
	return false
}
