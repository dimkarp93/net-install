package netlog

import (
	"bytes"
	"testing"
	"time"
)

func frozen() func() time.Time {
	t := time.Date(2026, 9, 20, 1, 54, 11, 0, time.UTC)
	return func() time.Time { return t }
}

func TestLogMatchesShellFormat(t *testing.T) {
	var buf bytes.Buffer
	l := New(&buf, frozen())

	l.Log("event=skip what=apt-retries reason=exists")
	l.Logf("event=retry what=fetch %s attempt=%d/%d rc=%d seconds=%d sleep=%ds",
		"https://example.com/x", 1, 5, 7, 80, 5)

	want := "[net] ts=2026-09-20T01:54:11Z event=skip what=apt-retries reason=exists\n" +
		"[net] ts=2026-09-20T01:54:11Z event=retry what=fetch https://example.com/x attempt=1/5 rc=7 seconds=80 sleep=5s\n"

	if got := buf.String(); got != want {
		t.Fatalf("log mismatch\ngot:\n%s\nwant:\n%s", got, want)
	}
}

func TestTimestampNeverCarriesAnOffset(t *testing.T) {
	var buf bytes.Buffer
	zone := time.FixedZone("YEKT", 5*60*60)
	at := time.Date(2026, 9, 20, 6, 54, 11, 0, zone)
	New(&buf, func() time.Time { return at }).Log("event=ok")

	want := "[net] ts=2026-09-20T01:54:11Z event=ok\n"
	if got := buf.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
