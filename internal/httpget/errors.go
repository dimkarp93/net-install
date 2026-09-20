package httpget

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"syscall"
)

type Kind int

const (
	KindLocal Kind = iota
	KindDNS
	KindConnect
	KindTruncated
	KindHTTP
	KindTimeout
	KindTLS
	KindRedirect
)

type Error struct {
	Kind   Kind
	Status int
	msg    string
}

func (e *Error) Error() string { return e.msg }

func (e *Error) ExitCode() int {
	switch e.Kind {
	case KindDNS:
		return 6
	case KindConnect:
		return 7
	case KindTruncated:
		return 18
	case KindHTTP:
		return 22
	case KindTimeout:
		return 28
	case KindTLS:
		return 35
	case KindRedirect:
		return 47
	}
	return 1
}

func (e *Error) Retryable() bool {
	switch e.Kind {
	case KindDNS, KindConnect, KindTruncated, KindTimeout:
		return true
	case KindHTTP:
		return e.Status >= 500 || e.Status == 408 || e.Status == 429
	}
	return false
}

func local(err error) *Error {
	return &Error{Kind: KindLocal, msg: err.Error()}
}

func classify(err error) *Error {
	if errors.Is(err, errTooManyRedirects) {
		return &Error{Kind: KindRedirect, msg: errTooManyRedirects.Error()}
	}

	var dns *net.DNSError
	if errors.As(err, &dns) {
		return &Error{Kind: KindDNS, msg: err.Error()}
	}

	var cert *tls.CertificateVerificationError
	var hostname x509.HostnameError
	var unknown x509.UnknownAuthorityError
	var record tls.RecordHeaderError
	if errors.As(err, &cert) || errors.As(err, &hostname) || errors.As(err, &unknown) || errors.As(err, &record) {
		return &Error{Kind: KindTLS, msg: err.Error()}
	}

	if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
		return &Error{Kind: KindTruncated, msg: err.Error()}
	}

	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return &Error{Kind: KindTimeout, msg: err.Error()}
	}

	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) ||
		errors.Is(err, syscall.EHOSTUNREACH) || errors.Is(err, syscall.ENETUNREACH) ||
		errors.Is(err, syscall.EPIPE) {
		return &Error{Kind: KindConnect, msg: err.Error()}
	}

	return &Error{Kind: KindConnect, msg: err.Error()}
}
