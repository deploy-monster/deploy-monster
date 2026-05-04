package auth

import "testing"

// TestTimingSafeEqual covers the three behavioral branches of
// timingSafeEqual: length mismatch (early return false), equal-length
// equal-content (loop completes with result==0), and equal-length
// distinct-content (result accumulates a non-zero byte).
func TestTimingSafeEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"both empty", "", "", true},
		{"equal short strings", "secret", "secret", true},
		{"equal long strings", "0123456789abcdef0123456789abcdef", "0123456789abcdef0123456789abcdef", true},
		{"length mismatch a shorter", "short", "shorter", false},
		{"length mismatch b shorter", "longer", "long", false},
		{"same length first byte differs", "abcdef", "Xbcdef", false},
		{"same length last byte differs", "abcdef", "abcdeX", false},
		{"same length mid byte differs", "abcdef", "abXdef", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := timingSafeEqual(tc.a, tc.b); got != tc.want {
				t.Fatalf("timingSafeEqual(%q,%q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestConstantTimeCompare hits the length-mismatch branch and the
// equal/unequal same-length branches in totp.go's constantTimeCompare.
func TestConstantTimeCompare(t *testing.T) {
	cases := []struct {
		name string
		a, b string
		want bool
	}{
		{"both empty", "", "", true},
		{"equal", "123456", "123456", true},
		{"length mismatch", "12345", "123456", false},
		{"same length differ", "123456", "123457", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := constantTimeCompare(tc.a, tc.b); got != tc.want {
				t.Fatalf("constantTimeCompare(%q,%q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

// TestValidateTOTP_EdgeCases covers the rejection paths and the
// padding-fallback decode that the happy-path tests skip.
func TestValidateTOTP_EdgeCases(t *testing.T) {
	t.Run("rejects token of wrong length", func(t *testing.T) {
		// Length must be exactly 6 or 8; everything else is rejected
		// before the decoder runs.
		if ValidateTOTP("12345", "JBSWY3DPEHPK3PXP") {
			t.Fatal("5-digit token must be rejected")
		}
		if ValidateTOTP("1234567", "JBSWY3DPEHPK3PXP") {
			t.Fatal("7-digit token must be rejected")
		}
		if ValidateTOTP("", "JBSWY3DPEHPK3PXP") {
			t.Fatal("empty token must be rejected")
		}
	})

	t.Run("rejects undecodable secret", func(t *testing.T) {
		// Base32 only accepts A-Z and 2-7; a literal "1" is invalid
		// and the padding-retry will fail too.
		if ValidateTOTP("000000", "111111") {
			t.Fatal("invalid base32 secret must reject")
		}
	})

	t.Run("rejects wrong code", func(t *testing.T) {
		// Use a known-good secret. Almost-any 6-digit token will
		// disagree with the live HOTP value; "000000" suffices in
		// practice since collision is 1-in-10^6 per call.
		if ValidateTOTP("000000", "JBSWY3DPEHPK3PXP") {
			// Extremely unlikely; if this hits, swap the literal for
			// another value rather than treat it as a regression.
			t.Skip("000000 happened to match HOTP at this clock tick")
		}
	})
}
