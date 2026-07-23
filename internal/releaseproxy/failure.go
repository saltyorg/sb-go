package releaseproxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"syscall"
)

// Failure describes why release metadata from the SVM proxy could not be used.
type Failure struct {
	StatusCode int
	Detail     string
	Err        error
}

func (f *Failure) Error() string {
	switch {
	case f.StatusCode != 0:
		return fmt.Sprintf("proxy returned HTTP %d", f.StatusCode)
	case f.Detail != "" && f.Err != nil:
		return fmt.Sprintf("%s: %v", f.Detail, f.Err)
	case f.Detail != "":
		return f.Detail
	default:
		return f.Err.Error()
	}
}

func (f *Failure) Unwrap() error {
	return f.Err
}

// HTTPStatus returns a structured proxy HTTP status failure.
func HTTPStatus(statusCode int) error {
	return &Failure{StatusCode: statusCode}
}

// InvalidResponse returns a structured failure for malformed or incomplete proxy data.
func InvalidResponse(detail string, err error) error {
	return &Failure{Detail: detail, Err: err}
}

// Describe returns a concise, safe reason suitable for normal user output.
func Describe(err error) string {
	if err == nil {
		return "unknown failure"
	}

	var failure *Failure
	if errors.As(err, &failure) {
		if failure.StatusCode != 0 {
			return fmt.Sprintf("returned HTTP %d", failure.StatusCode)
		}
		if failure.Detail != "" {
			return failure.Detail
		}
	}

	if errors.Is(err, context.Canceled) {
		return "request was canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timed out"
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "timed out"
		}

		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) {
			if dnsErr.Name != "" {
				return fmt.Sprintf("DNS lookup failed for %s (%s)", dnsErr.Name, cleanDetail(dnsErr.Err))
			}
			return fmt.Sprintf("DNS lookup failed (%s)", cleanDetail(dnsErr.Err))
		}

		switch {
		case errors.Is(err, syscall.ECONNREFUSED):
			return fmt.Sprintf("connection was refused (%s)", networkEndpoint(err))
		case errors.Is(err, syscall.ENETUNREACH):
			return fmt.Sprintf("network is unreachable (%s)", networkEndpoint(err))
		case errors.Is(err, syscall.EHOSTUNREACH):
			return fmt.Sprintf("host is unreachable (%s)", networkEndpoint(err))
		}

		return fmt.Sprintf("network request failed (%s)", cleanDetail(unwrapNetworkError(err).Error()))
	}

	return fmt.Sprintf("request failed (%s)", cleanDetail(err.Error()))
}

func networkEndpoint(err error) string {
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Addr != nil {
		return opErr.Addr.String()
	}
	return cleanDetail(unwrapNetworkError(err).Error())
}

func unwrapNetworkError(err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		err = urlErr.Err
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) && opErr.Err != nil {
		err = opErr.Err
	}
	return err
}

func cleanDetail(detail string) string {
	detail = strings.Join(strings.Fields(detail), " ")
	const maxLength = 240
	if len(detail) > maxLength {
		return detail[:maxLength] + "..."
	}
	return detail
}
