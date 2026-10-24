package httpget

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"
)

const maxRedirects = 50

var errTooManyRedirects = errors.New("too many redirects")

type Options struct {
	ConnectTimeout time.Duration
	SpeedLimit     int
	SpeedTime      int
}

type Client struct {
	hc         *http.Client
	speedLimit int
	speedTime  int
	tick       time.Duration
}

func New(o Options) *Client {
	return &Client{
		hc:         &http.Client{Transport: newTransport(o), CheckRedirect: checkRedirect},
		speedLimit: o.SpeedLimit,
		speedTime:  o.SpeedTime,
		tick:       time.Second,
	}
}

func newTransport(o Options) *http.Transport {
	dialer := &net.Dialer{Timeout: o.ConnectTimeout, KeepAlive: 30 * time.Second}
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   o.ConnectTimeout,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		ExpectContinueTimeout: time.Second,
	}
}

func checkRedirect(_ *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return errTooManyRedirects
	}
	return nil
}

func (c *Client) Download(ctx context.Context, url string, dst io.Writer) (int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, local(err)
	}

	resp, err := c.hc.Do(req)
	if err != nil {
		return 0, classify(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return 0, statusError(resp)
	}

	n, err := c.copyBody(dst, resp.Body, cancel)
	if err != nil {
		return n, err
	}

	if resp.ContentLength >= 0 && n != resp.ContentLength {
		return n, &Error{
			Kind: KindTruncated,
			msg:  fmt.Sprintf("got %d bytes, expected %d", n, resp.ContentLength),
		}
	}
	return n, nil
}

func statusError(resp *http.Response) *Error {
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))

	return &Error{
		Kind:   KindHTTP,
		Status: resp.StatusCode,
		msg:    fmt.Sprintf("the server answered %s", resp.Status),
	}
}

func (c *Client) copyBody(dst io.Writer, body io.Reader, cancel func()) (int64, error) {
	sr := newStallReader(body, c.speedLimit, c.speedTime, c.tick, cancel)
	defer sr.stop()

	n, err := io.Copy(dst, sr)
	switch {
	case err == nil:
		return n, nil
	case sr.stalled():
		return n, &Error{
			Kind: KindTimeout,
			msg:  fmt.Sprintf("the transfer stayed below %d bytes/s for %ds", c.speedLimit, c.speedTime),
		}
	default:
		return n, classify(err)
	}
}
