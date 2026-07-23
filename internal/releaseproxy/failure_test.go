package releaseproxy

import (
	"context"
	"errors"
	"net"
	"net/url"
	"syscall"
	"testing"
)

func TestDescribe(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "HTTP status", err: HTTPStatus(502), want: "returned HTTP 502"},
		{name: "invalid JSON", err: InvalidResponse("returned invalid JSON", errors.New("bad token")), want: "returned invalid JSON"},
		{name: "missing field", err: InvalidResponse("response is missing tag_name", nil), want: "response is missing tag_name"},
		{name: "timeout", err: context.DeadlineExceeded, want: "timed out"},
		{name: "canceled", err: context.Canceled, want: "request was canceled"},
		{
			name: "DNS lookup",
			err: &url.Error{Op: "Get", URL: "https://svm1.saltbox.dev/version", Err: &net.DNSError{
				Name: "svm1.saltbox.dev",
				Err:  "no such host",
			}},
			want: "DNS lookup failed for svm1.saltbox.dev (no such host)",
		},
		{
			name: "connection refused",
			err: &url.Error{Op: "Get", URL: "https://svm.saltbox.dev/version", Err: &net.OpError{
				Op:   "dial",
				Net:  "tcp",
				Addr: testAddr("127.0.0.1:443"),
				Err:  syscall.ECONNREFUSED,
			}},
			want: "connection was refused (127.0.0.1:443)",
		},
		{name: "unknown", err: errors.New("boom"), want: "request failed (boom)"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := Describe(test.err); got != test.want {
				t.Fatalf("Describe() = %q, want %q", got, test.want)
			}
		})
	}
}

type testAddr string

func (a testAddr) Network() string {
	return "tcp"
}

func (a testAddr) String() string {
	return string(a)
}

var _ net.Addr = testAddr("")
