package core

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// tunedTransport is a shared *http.Transport with connection pooling tuned for
// outbound API calls (VPS, DNS, webhooks, git providers, notifications).
// It keeps connections alive longer and allows more idle conns per host than
// http.DefaultTransport, which only retains 2 idle connections per host.
var (
	tunedTransport     *http.Transport
	tunedTransportOnce sync.Once
)

func getTunedTransport() *http.Transport {
	tunedTransportOnce.Do(func() {
		tunedTransport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          200,
			MaxIdleConnsPerHost:   20, // default is 2 — too low for repeated API calls
			MaxConnsPerHost:       50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
		}
	})
	return tunedTransport
}

// NewHTTPClient returns an *http.Client with a shared, connection-pooled
// transport and the given overall request timeout.
//
// Use this for any outbound HTTP call to an external service (VPS APIs,
// DNS APIs, webhooks, OAuth endpoints, etc.) so that connections are reused
// across requests to the same host. The shared transport enables HTTP/2 and
// allows up to 20 idle connections per host (vs. 2 in http.DefaultTransport).
//
// The timeout applies to the full request lifecycle (connect → headers → body).
// Pass 0 to use no client-level timeout (rely on context.Context instead).
func NewHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Transport: getTunedTransport(),
		Timeout:   timeout,
	}
}
