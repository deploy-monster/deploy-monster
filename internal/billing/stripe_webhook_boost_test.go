package billing

import (
	"context"
	"testing"
)

func TestResolveTenantID_FromMetadata(t *testing.T) {
	h := &StripeEventHandler{}
	got := h.resolveTenantID(context.Background(), "tenant-123", "cus_1", "sub_1")
	if got != "tenant-123" {
		t.Errorf("resolveTenantID = %q, want tenant-123", got)
	}
}

func TestResolveTenantID_EmptyMetadata(t *testing.T) {
	h := &StripeEventHandler{}
	got := h.resolveTenantID(context.Background(), "", "cus_1", "sub_1")
	if got != "" {
		t.Errorf("resolveTenantID = %q, want empty string", got)
	}
}
