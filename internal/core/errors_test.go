package core

import (
	"errors"
	"testing"
)

func TestAppError(t *testing.T) {
	err := NewAppError(404, "not found", ErrNotFound)

	if err.Code != 404 {
		t.Errorf("code = %d, want 404", err.Code)
	}
	if err.Message != "not found" {
		t.Errorf("message = %q", err.Message)
	}
	if err.Error() != "not found: not found" {
		t.Errorf("Error() = %q", err.Error())
	}
	if !errors.Is(err.Unwrap(), ErrNotFound) {
		t.Error("Unwrap should return ErrNotFound")
	}
}

func TestAppError_NilInner(t *testing.T) {
	err := NewAppError(500, "server error", nil)
	if err.Error() != "server error" {
		t.Errorf("Error() = %q", err.Error())
	}
	if err.Unwrap() != nil {
		t.Error("Unwrap should be nil")
	}
}

func TestSentinelErrors(t *testing.T) {
	sentinels := []error{
		ErrNotFound, ErrAlreadyExists, ErrUnauthorized, ErrForbidden,
		ErrQuotaExceeded, ErrBuildFailed, ErrDeployFailed,
		ErrInvalidInput, ErrExpired, ErrInvalidToken,
	}

	for _, err := range sentinels {
		if err == nil {
			t.Error("sentinel error should not be nil")
		}
		if err.Error() == "" {
			t.Error("sentinel error should have message")
		}
	}
}

func TestHealthStatus_String(t *testing.T) {
	tests := []struct {
		status HealthStatus
		want   string
	}{
		{HealthOK, "ok"},
		{HealthDegraded, "degraded"},
		{HealthDown, "down"},
		{HealthStatus(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("HealthStatus(%d).String() = %q, want %q", tt.status, got, tt.want)
		}
	}
}
