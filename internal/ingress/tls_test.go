package ingress

import "testing"

func TestCertStore_PutAndGet(t *testing.T) {
	cs := NewCertStore()

	cert, err := GenerateSelfSigned("test.example.com")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}

	cs.Put("test.example.com", cert)

	got := cs.Get("test.example.com")
	if got == nil {
		t.Error("expected cert, got nil")
	}

	if cs.Get("other.com") != nil {
		t.Error("expected nil for unknown domain")
	}
}

func TestCertStore_Remove(t *testing.T) {
	cs := NewCertStore()
	cert, _ := GenerateSelfSigned("remove.com")
	cs.Put("remove.com", cert)

	cs.Remove("remove.com")
	if cs.Get("remove.com") != nil {
		t.Error("expected nil after remove")
	}
}

func TestGenerateSelfSigned(t *testing.T) {
	cert, err := GenerateSelfSigned("test.local")
	if err != nil {
		t.Fatalf("GenerateSelfSigned: %v", err)
	}
	if cert == nil {
		t.Fatal("expected certificate")
	}
	if len(cert.Certificate) == 0 {
		t.Error("certificate should have DER bytes")
	}
}
