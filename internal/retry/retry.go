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

func Do(log *netlog.Logger, o Options, label string, fn func() error) error {
	sleep := o.Sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	attempts := o.Attempts
	if attempts < 1 {
		attempts = 1
	}

	for attempt := 1; ; attempt++ {
		log.Logf("event=start what=%s attempt=%d/%d", label, attempt, attempts)
		started := time.Now()

		err := fn()
		seconds := time.Now().Unix() - started.Unix()

		if err == nil {
			log.Logf("event=ok what=%s attempt=%d seconds=%d", label, attempt, seconds)
			return nil
		}

		rc := ExitCode(err)
		if attempt >= attempts || !(o.All || Retryable(err)) {
			log.Logf("event=fail what=%s attempts=%d rc=%d seconds=%d", label, attempt, rc, seconds)
			return err
		}

		log.Logf("event=retry what=%s attempt=%d/%d rc=%d seconds=%d sleep=%ds",
			label, attempt, attempts, rc, seconds, o.Delay)
		sleep(time.Duration(o.Delay) * time.Second)
	}
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
