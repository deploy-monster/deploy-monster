package ingress

import (
	"crypto/tls"
	"testing"
)

func TestCertStore_GetCertificate_Found(t *testing.T) {
	cs := NewCertStore()

	cert, err := GenerateSelfSigned("secure.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	cs.Put("secure.example.com", cert)

	hello := &tls.ClientHelloInfo{ServerName: "secure.example.com"}

	got, err := cs.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate returned error: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil certificate")
	}
	if got != cert {
		t.Error("expected the same certificate that was Put")
	}
}

func TestCertStore_GetCertificate_NotFound(t *testing.T) {
	cs := NewCertStore()

	hello := &tls.ClientHelloInfo{ServerName: "unknown.example.com"}

	got, err := cs.GetCertificate(hello)
	if err == nil {
		t.Error("expected error for unknown domain")
	}
	if got != nil {
		t.Error("expected nil certificate for unknown domain")
	}
}

func TestCertStore_GetCertificate_AfterRemove(t *testing.T) {
	cs := NewCertStore()

	cert, _ := GenerateSelfSigned("removed.example.com")
	cs.Put("removed.example.com", cert)
	cs.Remove("removed.example.com")

	hello := &tls.ClientHelloInfo{ServerName: "removed.example.com"}

	got, err := cs.GetCertificate(hello)
	if err == nil {
		t.Error("expected error after certificate removal")
	}
	if got != nil {
		t.Error("expected nil certificate after removal")
	}
}

func TestCertStore_GetCertificate_MultipleDomains(t *testing.T) {
	cs := NewCertStore()

	cert1, _ := GenerateSelfSigned("one.example.com")
	cert2, _ := GenerateSelfSigned("two.example.com")
	cs.Put("one.example.com", cert1)
	cs.Put("two.example.com", cert2)

	hello1 := &tls.ClientHelloInfo{ServerName: "one.example.com"}
	got1, err := cs.GetCertificate(hello1)
	if err != nil {
		t.Fatalf("unexpected error for one.example.com: %v", err)
	}
	if got1 != cert1 {
		t.Error("expected cert1 for one.example.com")
	}

	hello2 := &tls.ClientHelloInfo{ServerName: "two.example.com"}
	got2, err := cs.GetCertificate(hello2)
	if err != nil {
		t.Fatalf("unexpected error for two.example.com: %v", err)
	}
	if got2 != cert2 {
		t.Error("expected cert2 for two.example.com")
	}
}

func TestCertStore_GetCertificate_OverwriteDomain(t *testing.T) {
	cs := NewCertStore()

	oldCert, _ := GenerateSelfSigned("update.example.com")
	newCert, _ := GenerateSelfSigned("update.example.com")
	cs.Put("update.example.com", oldCert)
	cs.Put("update.example.com", newCert) // overwrite

	hello := &tls.ClientHelloInfo{ServerName: "update.example.com"}
	got, err := cs.GetCertificate(hello)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != newCert {
		t.Error("expected the new certificate after overwrite")
	}
}
