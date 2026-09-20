package httpget

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func testClient(t *testing.T) *Client {
	t.Helper()
	t.Setenv("NO_PROXY", "*")
	t.Setenv("no_proxy", "*")
	return New(Options{ConnectTimeout: 5 * time.Second, SpeedLimit: 0, SpeedTime: 0})
}

func code(t *testing.T, err error) int {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) {
		t.Fatalf("not a httpget error: %v", err)
	}
	return e.ExitCode()
}

func TestDownloadWritesTheBody(t *testing.T) {
	body := bytes.Repeat([]byte("abc"), 1000)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	n, err := testClient(t).Download(context.Background(), srv.URL, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(body)) || !bytes.Equal(buf.Bytes(), body) {
		t.Fatalf("got %d bytes, want %d", n, len(body))
	}
}

func TestGzipEncodedBodyArrivesDecoded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Accept-Encoding") == "" {
			t.Error("the transport did not ask for gzip")
		}
		w.Header().Set("Content-Encoding", "gzip")
		w.Write([]byte{
			0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03,
			0x4b, 0x4c, 0x4a, 0x06, 0x00, 0xc2, 0x41, 0x24, 0x35, 0x03, 0x00, 0x00, 0x00,
		})
	}))
	defer srv.Close()

	var buf bytes.Buffer
	if _, err := testClient(t).Download(context.Background(), srv.URL, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "abc" {
		t.Fatalf("got %q, want %q", buf.String(), "abc")
	}
}

func TestStatusAboveFourHundredFailsAndWritesNothing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	_, err := testClient(t).Download(context.Background(), srv.URL, &buf)
	if got := code(t, err); got != 22 {
		t.Fatalf("rc=%d, want 22", got)
	}
	if buf.Len() != 0 {
		t.Fatalf("wrote %d bytes on a 404", buf.Len())
	}
}

func TestRetryableStatuses(t *testing.T) {
	cases := map[int]bool{
		404: false, 403: false, 400: false,
		408: true, 429: true, 500: true, 503: true,
	}
	for status, want := range cases {
		e := &Error{Kind: KindHTTP, Status: status}
		if e.Retryable() != want {
			t.Fatalf("status %d: retryable=%v, want %v", status, e.Retryable(), want)
		}
	}
}

func TestRedirectsAreFollowedAndBounded(t *testing.T) {
	var srv *httptest.Server
	hops := 0
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hops++
		if hops > 3 {
			w.Write([]byte("done"))
			return
		}
		http.Redirect(w, r, srv.URL, http.StatusFound)
	}))
	defer srv.Close()

	var buf bytes.Buffer
	if _, err := testClient(t).Download(context.Background(), srv.URL, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "done" {
		t.Fatalf("got %q", buf.String())
	}
}

func TestTooManyRedirects(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, srv.URL, http.StatusFound)
	}))
	defer srv.Close()

	_, err := testClient(t).Download(context.Background(), srv.URL, &bytes.Buffer{})
	if got := code(t, err); got != 47 {
		t.Fatalf("rc=%d, want 47", got)
	}
}

func TestTruncatedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Skip("no hijacker")
		}
		conn, bufrw, err := hj.Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer conn.Close()
		fmt.Fprint(bufrw, "HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\nshort")
		bufrw.Flush()
	}))
	defer srv.Close()

	_, err := testClient(t).Download(context.Background(), srv.URL, &bytes.Buffer{})
	if got := code(t, err); got != 18 {
		t.Fatalf("rc=%d, want 18: %v", got, err)
	}
}

func TestConnectionRefused(t *testing.T) {
	_, err := testClient(t).Download(context.Background(), "http://127.0.0.1:1/x", &bytes.Buffer{})
	if got := code(t, err); got != 7 {
		t.Fatalf("rc=%d, want 7: %v", got, err)
	}
}

func TestStalledTransferIsAborted(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f, ok := w.(http.Flusher)
		if !ok {
			return
		}
		for i := 0; i < 200; i++ {
			select {
			case <-r.Context().Done():
				return
			default:
			}
			w.Write([]byte("x"))
			f.Flush()
			time.Sleep(50 * time.Millisecond)
		}
	}))
	defer srv.Close()

	t.Setenv("NO_PROXY", "*")
	c := New(Options{ConnectTimeout: 5 * time.Second, SpeedLimit: 1000, SpeedTime: 1})
	c.tick = 50 * time.Millisecond

	done := make(chan error, 1)
	go func() {
		_, err := c.Download(context.Background(), srv.URL, &bytes.Buffer{})
		done <- err
	}()

	select {
	case err := <-done:
		if got := code(t, err); got != 28 {
			t.Fatalf("rc=%d, want 28: %v", got, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the stalled transfer was not aborted")
	}
}

func TestFastTransferIsNotAborted(t *testing.T) {
	body := bytes.Repeat([]byte("y"), 512<<10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(body)
	}))
	defer srv.Close()

	t.Setenv("NO_PROXY", "*")
	c := New(Options{ConnectTimeout: 5 * time.Second, SpeedLimit: 1024, SpeedTime: 1})
	c.tick = 50 * time.Millisecond

	var buf bytes.Buffer
	if _, err := c.Download(context.Background(), srv.URL, &buf); err != nil {
		t.Fatal(err)
	}
	if buf.Len() != len(body) {
		t.Fatalf("got %d bytes, want %d", buf.Len(), len(body))
	}
}
