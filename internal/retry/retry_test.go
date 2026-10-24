package retry

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/dimkarp93/net-install/internal/netlog"
)

type coded struct {
	rc    int
	again bool
}

func (c *coded) Error() string   { return "boom" }
func (c *coded) ExitCode() int   { return c.rc }
func (c *coded) Retryable() bool { return c.again }

type flaky struct {
	calls   int
	failFor int
	err     error
}

func (f *flaky) call() error {
	f.calls++
	if f.failFor < 0 || f.calls <= f.failFor {
		return f.err
	}
	return nil
}

func newLog() (*netlog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	at := time.Date(2026, 9, 20, 1, 54, 11, 0, time.UTC)
	return netlog.New(&buf, func() time.Time { return at }), &buf
}

func opts(attempts int) Options {
	return Options{Attempts: attempts, Delay: 5, Sleep: func(time.Duration) {}}
}

func TestSuccessOnTheFirstAttempt(t *testing.T) {
	log, buf := newLog()

	err := Do(log, opts(5), "fetch u", func() error { return nil })

	require.NoError(t, err)
	want := "[net] ts=2026-09-20T01:54:11Z event=start what=fetch u attempt=1/5\n" +
		"[net] ts=2026-09-20T01:54:11Z event=ok what=fetch u attempt=1 seconds=0\n"
	assert.Equal(t, want, buf.String())
}

func TestRetriesThenSucceeds(t *testing.T) {
	log, buf := newLog()
	f := &flaky{failFor: 2, err: &coded{rc: 7, again: true}}

	err := Do(log, opts(5), "fetch u", f.call)

	t.Run("succeeds", func(t *testing.T) {
		assert.NoError(t, err)
	})
	t.Run("calls three times", func(t *testing.T) {
		assert.Equal(t, 3, f.calls)
	})
	t.Run("logs every attempt", func(t *testing.T) {
		want := "[net] ts=2026-09-20T01:54:11Z event=start what=fetch u attempt=1/5\n" +
			"[net] ts=2026-09-20T01:54:11Z event=retry what=fetch u attempt=1/5 rc=7 seconds=0 sleep=5s\n" +
			"[net] ts=2026-09-20T01:54:11Z event=start what=fetch u attempt=2/5\n" +
			"[net] ts=2026-09-20T01:54:11Z event=retry what=fetch u attempt=2/5 rc=7 seconds=0 sleep=5s\n" +
			"[net] ts=2026-09-20T01:54:11Z event=start what=fetch u attempt=3/5\n" +
			"[net] ts=2026-09-20T01:54:11Z event=ok what=fetch u attempt=3 seconds=0\n"
		assert.Equal(t, want, buf.String())
	})
}

func TestNonRetryableFailsImmediately(t *testing.T) {
	log, buf := newLog()
	f := &flaky{failFor: -1, err: &coded{rc: 22}}

	err := Do(log, opts(5), "fetch u", f.call)

	t.Run("fails", func(t *testing.T) {
		assert.Error(t, err)
	})
	t.Run("calls once", func(t *testing.T) {
		assert.Equal(t, 1, f.calls)
	})
	t.Run("logs the failure", func(t *testing.T) {
		want := "[net] ts=2026-09-20T01:54:11Z event=start what=fetch u attempt=1/5\n" +
			"[net] ts=2026-09-20T01:54:11Z event=fail what=fetch u attempts=1 rc=22 seconds=0\n"
		assert.Equal(t, want, buf.String())
	})
}

func TestRetryAllForcesTheLoop(t *testing.T) {
	log, _ := newLog()
	o := opts(3)
	o.All = true
	f := &flaky{failFor: -1, err: &coded{rc: 22}}

	err := Do(log, o, "fetch u", f.call)

	t.Run("fails", func(t *testing.T) {
		assert.Error(t, err)
	})
	t.Run("uses every attempt", func(t *testing.T) {
		assert.Equal(t, 3, f.calls)
	})
}

func TestExhaustsAttempts(t *testing.T) {
	log, buf := newLog()
	f := &flaky{failFor: -1, err: &coded{rc: 128, again: true}}

	err := Do(log, opts(3), "clone u", f.call)

	t.Run("uses every attempt", func(t *testing.T) {
		assert.Equal(t, 3, f.calls)
	})
	t.Run("keeps the exit code", func(t *testing.T) {
		assert.Equal(t, 128, ExitCode(err))
	})
	t.Run("logs the failure", func(t *testing.T) {
		assert.Contains(t, buf.String(), "event=fail what=clone u attempts=3 rc=128 seconds=0\n")
	})
}

func TestZeroAttemptsRunsOnce(t *testing.T) {
	log, _ := newLog()
	f := &flaky{failFor: -1, err: &coded{rc: 7, again: true}}

	_ = Do(log, opts(0), "fetch u", f.call)

	assert.Equal(t, 1, f.calls)
}

func TestExitCode(t *testing.T) {
	t.Run("plain error", func(t *testing.T) {
		assert.Equal(t, 1, ExitCode(errors.New("x")))
	})
	t.Run("nil", func(t *testing.T) {
		assert.Equal(t, 0, ExitCode(nil))
	})
}
