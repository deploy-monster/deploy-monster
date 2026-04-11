package providers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// TestSignV4_AWSVector_GetVanilla exercises signV4 against the official
// AWS Signature V4 Test Suite "get-vanilla" case.
//
// The test suite fixes the input (GET / HTTP/1.1 with host +
// x-amz-date) and the expected signature so any deviation in canonical
// request, string-to-sign, signing-key chain, or final HMAC byte order
// is caught immediately.
//
// Source fixtures (public AWS docs, not imported as data):
//
//	AccessKey:    AKIDEXAMPLE
//	SecretKey:    wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY
//	Region:       us-east-1
//	Service:      service
//	Host:         example.amazonaws.com
//	Date:         20150830T123600Z
//
//	Expected Authorization header:
//	  AWS4-HMAC-SHA256 Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request,
//	  SignedHeaders=host;x-amz-date,
//	  Signature=5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31
func TestSignV4_AWSVector_GetVanilla(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.amazonaws.com/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	now := time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)
	signV4(req, nil, "AKIDEXAMPLE",
		"wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
		"us-east-1", "service", now)

	want := "AWS4-HMAC-SHA256 " +
		"Credential=AKIDEXAMPLE/20150830/us-east-1/service/aws4_request, " +
		"SignedHeaders=host;x-amz-date, " +
		"Signature=5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"
	got := req.Header.Get("Authorization")
	if got != want {
		t.Errorf("Authorization mismatch\n got: %s\nwant: %s", got, want)
	}

	if ts := req.Header.Get("X-Amz-Date"); ts != "20150830T123600Z" {
		t.Errorf("X-Amz-Date = %q, want 20150830T123600Z", ts)
	}
}

// TestSignV4_Idempotent — signing the same request twice with the same
// clock must yield the same Authorization header. Catches hidden map
// iteration nondeterminism in the canonical headers list.
func TestSignV4_Idempotent(t *testing.T) {
	now := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		req, _ := http.NewRequest(http.MethodGet, "https://example.amazonaws.com/", nil)
		signV4(req, nil, "AKIDEXAMPLE",
			"wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
			"us-east-1", "service", now)
		a := req.Header.Get("Authorization")

		req2, _ := http.NewRequest(http.MethodGet, "https://example.amazonaws.com/", nil)
		signV4(req2, nil, "AKIDEXAMPLE",
			"wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
			"us-east-1", "service", now)
		b := req2.Header.Get("Authorization")

		if a != b {
			t.Fatalf("run %d: non-idempotent signing\n a=%s\n b=%s", i, a, b)
		}
	}
}

// TestSignV4_WithBody — POST with an XML body should hash the payload,
// set Content-Type in the canonical headers, and produce a signature
// that changes when the body changes.
func TestSignV4_WithBody(t *testing.T) {
	body1 := []byte("<foo>bar</foo>")
	body2 := []byte("<foo>baz</foo>")
	now := time.Date(2024, 6, 15, 10, 30, 0, 0, time.UTC)

	req1, _ := http.NewRequest(http.MethodPost, "https://route53.amazonaws.com/2013-04-01/hostedzone/Z123/rrset", nil)
	req1.Header.Set("Content-Type", "application/xml")
	signV4(req1, body1, "ak", "sk", "us-east-1", "route53", now)

	req2, _ := http.NewRequest(http.MethodPost, "https://route53.amazonaws.com/2013-04-01/hostedzone/Z123/rrset", nil)
	req2.Header.Set("Content-Type", "application/xml")
	signV4(req2, body2, "ak", "sk", "us-east-1", "route53", now)

	sig1 := req1.Header.Get("Authorization")
	sig2 := req2.Header.Get("Authorization")
	if sig1 == sig2 {
		t.Error("signatures must differ for different bodies")
	}

	if !strings.Contains(sig1, "SignedHeaders=content-type;host;x-amz-date") {
		t.Errorf("expected content-type in signed headers, got: %s", sig1)
	}

	// Body must be attached for the HTTP client.
	if req1.Body == nil {
		t.Error("req.Body must be set after signing with body")
	}
	if req1.ContentLength != int64(len(body1)) {
		t.Errorf("ContentLength = %d, want %d", req1.ContentLength, len(body1))
	}
}

// TestSignV4_EmptyPath — an empty URL path must canonicalize to "/"
// before signing or the AWS side will return a signature mismatch.
func TestSignV4_EmptyPath(t *testing.T) {
	req := &http.Request{
		Method: http.MethodGet,
		URL:    &url.URL{Scheme: "https", Host: "example.amazonaws.com", Path: ""},
		Header: make(http.Header),
	}
	signV4(req, nil, "AKIDEXAMPLE",
		"wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
		"us-east-1", "service",
		time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC))

	// Same input as the AWS get-vanilla vector → same signature.
	want := "Signature=5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"
	if !strings.Contains(req.Header.Get("Authorization"), want) {
		t.Errorf("empty path did not canonicalize to \"/\"; Authorization=%s",
			req.Header.Get("Authorization"))
	}
}

// TestSigV4Encode — unreserved chars pass through untouched, reserved
// chars are percent-encoded with uppercase hex, and space becomes %20
// (not "+" as url.QueryEscape would produce).
func TestSigV4Encode(t *testing.T) {
	cases := map[string]string{
		"abcXYZ123":      "abcXYZ123",
		"a-b_c.d~e":      "a-b_c.d~e",
		"hello world":    "hello%20world",
		"key=val&x=y":    "key%3Dval%26x%3Dy",
		"tildé":          "tild%C3%A9",
		"/path/with:col": "%2Fpath%2Fwith%3Acol",
	}
	for in, want := range cases {
		if got := sigV4Encode(in); got != want {
			t.Errorf("sigV4Encode(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestCanonicalQueryString — keys sorted lexicographically, values
// within a repeated key also sorted, key=value joined with "&".
func TestCanonicalQueryString(t *testing.T) {
	v := url.Values{}
	v.Add("b", "2")
	v.Add("a", "1")
	v.Add("c", "z")
	v.Add("c", "a")
	got := canonicalQueryString(v)
	want := "a=1&b=2&c=a&c=z"
	if got != want {
		t.Errorf("canonicalQueryString = %q, want %q", got, want)
	}

	if canonicalQueryString(url.Values{}) != "" {
		t.Error("empty url.Values must produce empty canonical query")
	}
}

// TestRoute53_SignsRequests — end-to-end check that every Route53 HTTP
// call (findHostedZone GET and changeRecord POST) carries a SigV4
// Authorization header with the expected scope. Without this the stub
// implementation that shipped before would silently 403 against real
// AWS and leave operators unable to publish DNS records.
func TestRoute53_SignsRequests(t *testing.T) {
	var seenAuth []string
	var seenAmzDate []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		seenAuth = append(seenAuth, req.Header.Get("Authorization"))
		seenAmzDate = append(seenAmzDate, req.Header.Get("X-Amz-Date"))

		if req.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/xml")
			fmt.Fprint(w, `<?xml version="1.0"?>
<ListHostedZonesResponse xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <HostedZones>
    <HostedZone>
      <Id>/hostedzone/Z-SIG</Id>
      <Name>example.com</Name>
    </HostedZone>
  </HostedZones>
</ListHostedZonesResponse>`)
			return
		}
		// POST: drain body and return OK so the retry loop exits.
		_, _ = io.ReadAll(req.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	r := NewRoute53("AKIDEXAMPLE",
		"wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY", "eu-west-1")
	r.client = srv.Client()
	r.client.Transport = &rewriteTransport{base: srv.URL}

	err := r.CreateRecord(context.Background(), core.DNSRecord{
		Type:  "A",
		Name:  "app.example.com",
		Value: "1.2.3.4",
		TTL:   300,
	})
	if err != nil {
		t.Fatalf("CreateRecord: %v", err)
	}

	if len(seenAuth) < 2 {
		t.Fatalf("expected >= 2 requests (findHostedZone + changeRecord), got %d", len(seenAuth))
	}

	for i, auth := range seenAuth {
		if auth == "" {
			t.Errorf("request %d: no Authorization header — unsigned request escaped", i)
			continue
		}
		if !strings.HasPrefix(auth, "AWS4-HMAC-SHA256 ") {
			t.Errorf("request %d: Authorization does not start with AWS4-HMAC-SHA256: %q", i, auth)
		}
		if !strings.Contains(auth, "Credential=AKIDEXAMPLE/") {
			t.Errorf("request %d: Authorization missing access key: %q", i, auth)
		}
		// Route53 signs against us-east-1 regardless of the configured
		// Route53.region field ("eu-west-1" above) — this is the main
		// invariant a regression would trip.
		if !strings.Contains(auth, "/us-east-1/route53/aws4_request") {
			t.Errorf("request %d: Authorization missing us-east-1 Route53 scope: %q", i, auth)
		}
		if !strings.Contains(auth, "Signature=") {
			t.Errorf("request %d: Authorization missing Signature: %q", i, auth)
		}
	}

	// POST (changeRecord) signs content-type in addition to host +
	// x-amz-date, because the request has a body.
	postAuth := seenAuth[len(seenAuth)-1]
	if !strings.Contains(postAuth, "SignedHeaders=content-type;host;x-amz-date") {
		t.Errorf("POST request must sign content-type; got: %s", postAuth)
	}

	for i, ts := range seenAmzDate {
		if ts == "" {
			t.Errorf("request %d: X-Amz-Date missing", i)
		}
		if _, err := time.Parse("20060102T150405Z", ts); err != nil {
			t.Errorf("request %d: X-Amz-Date %q not in SigV4 format: %v", i, ts, err)
		}
	}
}

// TestSignV4_RewriteHostRespected — if the caller has pre-set req.Host
// (e.g. to disambiguate a virtual-host routing), that wins over URL.Host
// for the canonical header.
func TestSignV4_RewriteHostRespected(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://ignored.example/", nil)
	req.Host = "example.amazonaws.com"
	signV4(req, nil, "AKIDEXAMPLE",
		"wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY",
		"us-east-1", "service",
		time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC))

	// Same signature as get-vanilla because req.Host overrode URL.Host.
	want := "Signature=5fa00fa31553b73ebf1942676e86291e8372ff2a2260956d9b8aae1d763fbf31"
	if !strings.Contains(req.Header.Get("Authorization"), want) {
		t.Errorf("req.Host did not override URL.Host; Authorization=%s",
			req.Header.Get("Authorization"))
	}
}
