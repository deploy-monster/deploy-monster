package handlers

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

// SSLStatusHandler checks SSL certificate status for domains.
type SSLStatusHandler struct{}

func NewSSLStatusHandler() *SSLStatusHandler {
	return &SSLStatusHandler{}
}

// SSLCheckResult holds SSL verification details.
type SSLCheckResult struct {
	FQDN       string    `json:"fqdn"`
	Valid      bool      `json:"valid"`
	Issuer     string    `json:"issuer,omitempty"`
	Subject    string    `json:"subject,omitempty"`
	ExpiresAt  time.Time `json:"expires_at,omitempty"`
	DaysLeft   int       `json:"days_left,omitempty"`
	Error      string    `json:"error,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

// Check handles GET /api/v1/domains/{id}/ssl-status
func (h *SSLStatusHandler) Check(w http.ResponseWriter, r *http.Request) {
	fqdn := r.URL.Query().Get("fqdn")
	if fqdn == "" {
		writeError(w, http.StatusBadRequest, "fqdn query param required")
		return
	}

	result := checkSSL(fqdn)
	writeJSON(w, http.StatusOK, result)
}

func checkSSL(fqdn string) SSLCheckResult {
	result := SSLCheckResult{
		FQDN:      fqdn,
		CheckedAt: time.Now(),
	}

	conn, err := tls.DialWithDialer(
		&net.Dialer{Timeout: 5 * time.Second},
		"tcp", fqdn+":443",
		&tls.Config{InsecureSkipVerify: false},
	)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer conn.Close()

	certs := conn.ConnectionState().PeerCertificates
	if len(certs) > 0 {
		cert := certs[0]
		result.Valid = true
		result.Issuer = cert.Issuer.CommonName
		result.Subject = cert.Subject.CommonName
		result.ExpiresAt = cert.NotAfter
		result.DaysLeft = int(time.Until(cert.NotAfter).Hours() / 24)
	}

	return result
}
