package httpget

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var gzipABC = []byte{
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x03,
	0x4b, 0x4c, 0x4a, 0x06, 0x00, 0xc2, 0x41, 0x24, 0x35, 0x03, 0x00, 0x00, 0x00,
}

func testClient(t *testing.T) *Client {
	t.Helper()

	t.Setenv("NO_PROXY", "*")
	t.Setenv("no_proxy", "*")

	return New(Options{ConnectTimeout: 5 * time.Second, SpeedLimit: 0, SpeedTime: 0})
}

func code(t *testing.T, err error) int {
	t.Helper()

	var e *Error
	require.ErrorAs(t, err, &e)

	return e.ExitCode()
}

func serve(t *testing.T, h http.HandlerFunc) string {
	t.Helper()

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)

	return srv.URL
}

func bodyHandler(body []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) { w.Write(body) }
}

type gzipRequest struct {
	asked bool
}

func (g *gzipRequest) serveHTTP(w http.ResponseWriter, r *http.Request) {
	g.asked = r.Header.Get("Accept-Encoding") != ""
	w.Header().Set("Content-Encoding", "gzip")
	w.Write(gzipABC)
}

type redirector struct {
	url   string
	hops  int
	limit int
}

func (rd *redirector) serveHTTP(w http.ResponseWriter, r *http.Request) {
	rd.hops++
	if rd.limit > 0 && rd.hops > rd.limit {
		w.Write([]byte("done"))
		return
	}
	http.Redirect(w, r, rd.url, http.StatusFound)
}

func redirectServer(t *testing.T, limit int) string {
	t.Helper()

	rd := &redirector{limit: limit}
	rd.url = serve(t, rd.serveHTTP)

	return rd.url
}

func truncatedHandler(w http.ResponseWriter, _ *http.Request) {
	conn, bufrw, err := w.(http.Hijacker).Hijack()
	if err != nil {
		return
	}
	defer conn.Close()

	fmt.Fprint(bufrw, "HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\nshort")
	bufrw.Flush()
}

func trickleHandler(w http.ResponseWriter, r *http.Request) {
	for range 200 {
		if r.Context().Err() != nil {
			return
		}
		w.Write([]byte("x"))
		w.(http.Flusher).Flush()
		time.Sleep(50 * time.Millisecond)
	}
}

func download(t *testing.T, c *Client, url string) (int64, string, error) {
	t.Helper()

	var buf bytes.Buffer
	n, err := c.Download(context.Background(), url, &buf)

	return n, buf.String(), err
}

func downloadOK(t *testing.T, c *Client, url string) string {
	t.Helper()

	_, got, err := download(t, c, url)
	require.NoError(t, err)

	return got
}

func TestDownloadWritesTheBody(t *testing.T) {
	body := bytes.Repeat([]byte("abc"), 1000)

	n, got, err := download(t, testClient(t), serve(t, bodyHandler(body)))

	require.NoError(t, err)
	assert.Equal(t, int64(len(body)), n)
	assert.Equal(t, string(body), got)
}

func TestGzipEncodedBodyArrivesDecoded(t *testing.T) {
	g := &gzipRequest{}
	got := downloadOK(t, testClient(t), serve(t, g.serveHTTP))

	t.Run("asks for gzip", func(t *testing.T) {
		assert.True(t, g.asked)
	})
	t.Run("decodes body", func(t *testing.T) {
		assert.Equal(t, "abc", got)
	})
}

func TestStatusAboveFourHundredFailsAndWritesNothing(t *testing.T) {
	notFound := func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "nope", http.StatusNotFound) }

	_, got, err := download(t, testClient(t), serve(t, notFound))

	t.Run("exit code", func(t *testing.T) {
		assert.Equal(t, 22, code(t, err))
	})
	t.Run("writes nothing", func(t *testing.T) {
		assert.Empty(t, got)
	})
}

func TestRetryableStatuses(t *testing.T) {
	cases := map[int]bool{
		404: false, 403: false, 400: false,
		408: true, 429: true, 500: true, 503: true,
	}
	for status, want := range cases {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			assert.Equal(t, want, (&Error{Kind: KindHTTP, Status: status}).Retryable())
		})
	}
}

func TestExitCodes(t *testing.T) {
	cases := map[Kind]int{
		KindLocal: 1, KindDNS: 6, KindConnect: 7, KindTruncated: 18,
		KindHTTP: 22, KindTimeout: 28, KindTLS: 35, KindRedirect: 47,
	}
	for kind, want := range cases {
		t.Run(fmt.Sprint(kind), func(t *testing.T) {
			assert.Equal(t, want, (&Error{Kind: kind}).ExitCode())
		})
	}
}

func TestRedirectsAreFollowedAndBounded(t *testing.T) {
	_, got, err := download(t, testClient(t), redirectServer(t, 3))

	require.NoError(t, err)
	assert.Equal(t, "done", got)
}

func TestTooManyRedirects(t *testing.T) {
	_, _, err := download(t, testClient(t), redirectServer(t, 0))

	assert.Equal(t, 47, code(t, err))
}

func TestTruncatedBody(t *testing.T) {
	_, _, err := download(t, testClient(t), serve(t, truncatedHandler))

	assert.Equal(t, 18, code(t, err))
}

func TestConnectionRefused(t *testing.T) {
	_, _, err := download(t, testClient(t), "http://127.0.0.1:1/x")

	assert.Equal(t, 7, code(t, err))
}

func TestClassifyKinds(t *testing.T) {
	cases := map[string]struct {
		err  error
		want Kind
	}{
		"dns":       {err: &net.DNSError{Err: "no such host", Name: "x"}, want: KindDNS},
		"eof":       {err: io.ErrUnexpectedEOF, want: KindTruncated},
		"other":     {err: errors.New("x"), want: KindConnect},
		"redirects": {err: fmt.Errorf("get: %w", errTooManyRedirects), want: KindRedirect},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, classify(tc.err).Kind)
		})
	}
}

func speedClient(t *testing.T, limit int) *Client {
	t.Helper()

	t.Setenv("NO_PROXY", "*")
	c := New(Options{ConnectTimeout: 5 * time.Second, SpeedLimit: limit, SpeedTime: 1})
	c.tick = 50 * time.Millisecond

	return c
}

func TestStalledTransferIsAborted(t *testing.T) {
	c := speedClient(t, 1000)
	url := serve(t, trickleHandler)

	done := make(chan error, 1)
	go func() {
		_, err := c.Download(context.Background(), url, &bytes.Buffer{})
		done <- err
	}()

	select {
	case err := <-done:
		assert.Equal(t, 28, code(t, err))
	case <-time.After(10 * time.Second):
		assert.Fail(t, "the stalled transfer was not aborted")
	}
}

func TestFastTransferIsNotAborted(t *testing.T) {
	body := bytes.Repeat([]byte("y"), 512<<10)

	_, got, err := download(t, speedClient(t, 1024), serve(t, bodyHandler(body)))

	require.NoError(t, err)
	assert.Len(t, got, len(body))
}
