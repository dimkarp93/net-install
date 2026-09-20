package retry

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/dimkarp93/net-install/internal/netlog"
)

type coded struct {
	rc    int
	again bool
}

func (c *coded) Error() string   { return "boom" }
func (c *coded) ExitCode() int   { return c.rc }
func (c *coded) Retryable() bool { return c.again }

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
	if err := Do(log, opts(5), "fetch u", func() error { return nil }); err != nil {
		t.Fatal(err)
	}

	want := "[net] ts=2026-09-20T01:54:11Z event=start what=fetch u attempt=1/5\n" +
		"[net] ts=2026-09-20T01:54:11Z event=ok what=fetch u attempt=1 seconds=0\n"
	if buf.String() != want {
		t.Fatalf("got:\n%swant:\n%s", buf.String(), want)
	}
}

func TestRetriesThenSucceeds(t *testing.T) {
	log, buf := newLog()
	calls := 0
	err := Do(log, opts(5), "fetch u", func() error {
		calls++
		if calls < 3 {
			return &coded{rc: 7, again: true}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 3 {
		t.Fatalf("calls=%d, want 3", calls)
	}

	want := "[net] ts=2026-09-20T01:54:11Z event=start what=fetch u attempt=1/5\n" +
		"[net] ts=2026-09-20T01:54:11Z event=retry what=fetch u attempt=1/5 rc=7 seconds=0 sleep=5s\n" +
		"[net] ts=2026-09-20T01:54:11Z event=start what=fetch u attempt=2/5\n" +
		"[net] ts=2026-09-20T01:54:11Z event=retry what=fetch u attempt=2/5 rc=7 seconds=0 sleep=5s\n" +
		"[net] ts=2026-09-20T01:54:11Z event=start what=fetch u attempt=3/5\n" +
		"[net] ts=2026-09-20T01:54:11Z event=ok what=fetch u attempt=3 seconds=0\n"
	if buf.String() != want {
		t.Fatalf("got:\n%swant:\n%s", buf.String(), want)
	}
}

func TestNonRetryableFailsImmediately(t *testing.T) {
	log, buf := newLog()
	calls := 0
	err := Do(log, opts(5), "fetch u", func() error {
		calls++
		return &coded{rc: 22}
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	if calls != 1 {
		t.Fatalf("calls=%d, want 1", calls)
	}

	want := "[net] ts=2026-09-20T01:54:11Z event=start what=fetch u attempt=1/5\n" +
		"[net] ts=2026-09-20T01:54:11Z event=fail what=fetch u attempts=1 rc=22 seconds=0\n"
	if buf.String() != want {
		t.Fatalf("got:\n%swant:\n%s", buf.String(), want)
	}
}

func TestRetryAllForcesTheLoop(t *testing.T) {
	log, _ := newLog()
	o := opts(3)
	o.All = true
	calls := 0
	err := Do(log, o, "fetch u", func() error {
		calls++
		return &coded{rc: 22}
	})
	if err == nil || calls != 3 {
		t.Fatalf("calls=%d, err=%v", calls, err)
	}
}

func TestExhaustsAttempts(t *testing.T) {
	log, buf := newLog()
	calls := 0
	err := Do(log, opts(3), "clone u", func() error {
		calls++
		return &coded{rc: 128, again: true}
	})
	if err == nil || calls != 3 {
		t.Fatalf("calls=%d, err=%v", calls, err)
	}
	if ExitCode(err) != 128 {
		t.Fatalf("exit code=%d", ExitCode(err))
	}
	if want := "event=fail what=clone u attempts=3 rc=128 seconds=0\n"; !bytes.Contains(buf.Bytes(), []byte(want)) {
		t.Fatalf("missing %q in:\n%s", want, buf.String())
	}
}

func TestExitCodeOfAPlainError(t *testing.T) {
	if got := ExitCode(errors.New("x")); got != 1 {
		t.Fatalf("got %d, want 1", got)
	}
	if got := ExitCode(nil); got != 0 {
		t.Fatalf("got %d, want 0", got)
	}
}
