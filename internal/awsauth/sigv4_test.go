package awsauth

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestSignV4_AWSVector_GetVanilla exercises SignV4 against the AWS
// Signature V4 Test Suite "get-vanilla" case. This is the same fixture
// used by internal/dns/providers/sigv4_test.go — keeping it here
// guarantees the shared package stays self-verifying even if the
// Route53 wrapper is removed.
func TestSignV4_AWSVector_GetVanilla(t *testing.T) {
	req, err := http.NewRequest(http.MethodGet, "https://example.amazonaws.com/", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	now := time.Date(2015, 8, 30, 12, 36, 0, 0, time.UTC)
	SignV4(req, nil, "AKIDEXAMPLE",
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
}

// TestSignV4_S3_IncludesContentSha256 — S3 requires x-amz-content-sha256
// to appear in the signed header set. The non-S3 path must not add it.
func TestSignV4_S3_IncludesContentSha256(t *testing.T) {
	body := []byte("hello world")
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	req, _ := http.NewRequest(http.MethodPut, "https://bucket.s3.us-east-1.amazonaws.com/key", nil)
	SignV4(req, body, "ak", "sk", "us-east-1", "s3", now)

	auth := req.Header.Get("Authorization")
	if !strings.Contains(auth, "x-amz-content-sha256") {
		t.Errorf("S3 Authorization must sign x-amz-content-sha256: %s", auth)
	}
	if got := req.Header.Get("X-Amz-Content-Sha256"); got == "" {
		t.Error("SignV4 should set X-Amz-Content-Sha256 for s3")
	}

	// Non-S3 path: same body but service="route53" should NOT add the header.
	req2, _ := http.NewRequest(http.MethodPost, "https://route53.amazonaws.com/", nil)
	req2.Header.Set("Content-Type", "application/xml")
	SignV4(req2, body, "ak", "sk", "us-east-1", "route53", now)
	if req2.Header.Get("X-Amz-Content-Sha256") != "" {
		t.Error("non-S3 service must not set X-Amz-Content-Sha256")
	}
	if strings.Contains(req2.Header.Get("Authorization"), "x-amz-content-sha256") {
		t.Error("non-S3 Authorization must not include x-amz-content-sha256 in SignedHeaders")
	}
}

// TestSignV4_BodyAttached — passing a non-nil body must leave req.Body
// rewindable so the HTTP client can actually transmit it, and the
// ContentLength must match.
func TestSignV4_BodyAttached(t *testing.T) {
	body := []byte(`{"foo":"bar"}`)
	req, _ := http.NewRequest(http.MethodPost, "https://example.amazonaws.com/", nil)
	SignV4(req, body, "ak", "sk", "us-east-1", "service", time.Now())

	if req.Body == nil {
		t.Fatal("req.Body must be set after signing with body")
	}
	if req.ContentLength != int64(len(body)) {
		t.Errorf("ContentLength = %d, want %d", req.ContentLength, len(body))
	}
	if req.GetBody == nil {
		t.Error("req.GetBody must be set so retries can rewind")
	}
}

// TestSigV4Encode_RFC3986 verifies the percent-encoding matches the SigV4
// unreserved set. Differs from url.QueryEscape in that space must encode
// as %20 (never "+") and the unreserved punctuation set stays literal.
func TestSigV4Encode_RFC3986(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"", ""},
		{"abc", "abc"},
		{"ABC_123", "ABC_123"},
		{"a-._~z", "a-._~z"},
		{"hello world", "hello%20world"},
		{"a+b", "a%2Bb"},
		{"a/b", "a%2Fb"},
		{"a:b", "a%3Ab"},
		{"\x00\xff", "%00%FF"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := sigV4Encode(tc.in); got != tc.want {
				t.Errorf("sigV4Encode(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestcanonicalQueryString covers the sorted-key, sorted-value, percent-
// encoded pairing. Repeated keys must have their VALUES sorted, and empty
// input must return an empty string (not panic).
func TestCanonicalQueryString(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		if got := canonicalQueryString(url.Values{}); got != "" {
			t.Errorf("empty values should return empty string, got %q", got)
		}
	})

	t.Run("sorted keys", func(t *testing.T) {
		v := url.Values{}
		v.Set("z", "1")
		v.Set("a", "2")
		v.Set("m", "3")
		got := canonicalQueryString(v)
		want := "a=2&m=3&z=1"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("repeated key values sorted", func(t *testing.T) {
		v := url.Values{}
		v["tag"] = []string{"banana", "apple", "cherry"}
		got := canonicalQueryString(v)
		want := "tag=apple&tag=banana&tag=cherry"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("percent-encoded key and value", func(t *testing.T) {
		v := url.Values{}
		v.Set("name with space", "val/slash")
		got := canonicalQueryString(v)
		want := "name%20with%20space=val%2Fslash"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
