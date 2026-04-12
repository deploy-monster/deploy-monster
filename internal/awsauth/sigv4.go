// Package awsauth implements the minimum subset of AWS Signature V4
// needed to talk to Route53 and S3 without pulling in the official
// aws-sdk-go-v2 dependency. Verified against the AWS Signature V4 Test
// Suite (see sigv4_test.go).
package awsauth

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// SignV4 signs an HTTP request with AWS Signature Version 4.
//
// It mutates the request in place by adding X-Amz-Date, x-amz-content-sha256,
// and Authorization headers. If `body` is non-nil it is also assigned
// to req.Body so the caller can pass a single byte slice without
// managing a reader.
//
// `region` must match the credential scope the target service expects —
// Route53 is global and always signs as "us-east-1" regardless of where
// the caller is. `service` is the AWS service name ("route53", "s3", ...).
//
// The x-amz-content-sha256 header is included in the signed header set
// when the service is "s3", which S3 requires; other services only need
// host + x-amz-date + optional content-type.
func SignV4(req *http.Request, body []byte, accessKey, secretKey, region, service string, now time.Time) {
	const (
		algorithm  = "AWS4-HMAC-SHA256"
		timeFormat = "20060102T150405Z"
		dateFormat = "20060102"
	)

	ts := now.UTC().Format(timeFormat)
	date := now.UTC().Format(dateFormat)

	// Host header source: req.Host wins over URL.Host so tests that
	// override one or the other still produce a consistent signed host.
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}

	// Payload hash — hex(sha256(body)). Empty body still gets the empty
	// string's sha256 (e3b0c...), which is what AWS expects.
	bodySum := sha256.Sum256(body)
	payloadHash := hex.EncodeToString(bodySum[:])

	// Attach the body to the request so the HTTP client can actually
	// send it. Rewindable via bytes.Reader so retries work.
	if body != nil {
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.GetBody = func() (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(body)), nil
		}
		req.ContentLength = int64(len(body))
	}

	req.Header.Set("X-Amz-Date", ts)

	// Canonical headers: always sign host + x-amz-date; include
	// content-type if set so POST bodies are bound to the signature.
	// S3 additionally requires x-amz-content-sha256 to be signed.
	canonHeaders := map[string]string{
		"host":       host,
		"x-amz-date": ts,
	}
	if ct := req.Header.Get("Content-Type"); ct != "" {
		canonHeaders["content-type"] = ct
	}
	if service == "s3" {
		req.Header.Set("X-Amz-Content-Sha256", payloadHash)
		canonHeaders["x-amz-content-sha256"] = payloadHash
	}
	names := make([]string, 0, len(canonHeaders))
	for n := range canonHeaders {
		names = append(names, n)
	}
	sort.Strings(names)

	var headerLines strings.Builder
	for _, n := range names {
		headerLines.WriteString(n)
		headerLines.WriteString(":")
		headerLines.WriteString(strings.TrimSpace(canonHeaders[n]))
		headerLines.WriteString("\n")
	}
	signedHeaders := strings.Join(names, ";")

	canonicalURI := req.URL.EscapedPath()
	if canonicalURI == "" {
		canonicalURI = "/"
	}
	canonicalQuery := canonicalQueryString(req.URL.Query())

	canonicalRequest := req.Method + "\n" +
		canonicalURI + "\n" +
		canonicalQuery + "\n" +
		headerLines.String() + "\n" +
		signedHeaders + "\n" +
		payloadHash

	hashedCanon := sha256.Sum256([]byte(canonicalRequest))
	hashedCanonHex := hex.EncodeToString(hashedCanon[:])

	credentialScope := date + "/" + region + "/" + service + "/aws4_request"
	stringToSign := strings.Join([]string{
		algorithm,
		ts,
		credentialScope,
		hashedCanonHex,
	}, "\n")

	kDate := hmacSHA256([]byte("AWS4"+secretKey), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	kSigning := hmacSHA256(kService, "aws4_request")
	signature := hex.EncodeToString(hmacSHA256(kSigning, stringToSign))

	authHeader := algorithm +
		" Credential=" + accessKey + "/" + credentialScope +
		", SignedHeaders=" + signedHeaders +
		", Signature=" + signature
	req.Header.Set("Authorization", authHeader)
}

func hmacSHA256(key []byte, data string) []byte {
	h := hmac.New(sha256.New, key)
	h.Write([]byte(data))
	return h.Sum(nil)
}

// canonicalQueryString produces a SigV4-canonical query string from a
// url.Values map: keys sorted lex, values within a repeated key also
// sorted, name + "=" + value, joined with "&", with SigV4-style
// percent-encoding (spaces → %20, unreserved set preserved).
func canonicalQueryString(values url.Values) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var pairs []string
	for _, k := range keys {
		ek := sigV4Encode(k)
		vs := append([]string(nil), values[k]...)
		sort.Strings(vs)
		for _, v := range vs {
			pairs = append(pairs, ek+"="+sigV4Encode(v))
		}
	}
	return strings.Join(pairs, "&")
}

// sigV4Encode percent-encodes a string per RFC 3986 unreserved rules.
// url.QueryEscape is wrong for this — it encodes spaces as "+" whereas
// SigV4 requires "%20".
func sigV4Encode(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') ||
			(c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.' || c == '~' {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
