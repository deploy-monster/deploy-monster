package providers

import (
	"net/http"
	"net/url"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/awsauth"
)

// The SigV4 algorithm and its canonicalization helpers live in
// internal/awsauth so both Route53 and S3 share a single
// implementation. These package-local wrappers exist so the existing
// tests (sigv4_test.go) can continue to call signV4, canonicalQueryString,
// and sigV4Encode by their lowercase names.

func signV4(req *http.Request, body []byte, accessKey, secretKey, region, service string, now time.Time) {
	awsauth.SignV4(req, body, accessKey, secretKey, region, service, now)
}

func canonicalQueryString(values url.Values) string {
	return awsauth.CanonicalQueryString(values)
}

func sigV4Encode(s string) string {
	return awsauth.SigV4Encode(s)
}
