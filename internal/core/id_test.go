package core

import (
	"testing"
)

func TestGenerateID_Length(t *testing.T) {
	id := GenerateID()
	if len(id) != 16 {
		t.Errorf("expected 16-char ID, got %d chars: %q", len(id), id)
	}
}

func TestGenerateID_Unique(t *testing.T) {
	seen := make(map[string]bool)
	for i := 0; i < 1000; i++ {
		id := GenerateID()
		if seen[id] {
			t.Fatalf("duplicate ID generated: %s", id)
		}
		seen[id] = true
	}
}

func TestGenerateSecret_NotEmpty(t *testing.T) {
	s := GenerateSecret(32)
	if s == "" {
		t.Error("expected non-empty secret")
	}
	if len(s) < 32 {
		t.Errorf("secret too short: %d chars", len(s))
	}
}

func TestGeneratePassword_Length(t *testing.T) {
	p := GeneratePassword(20)
	if len(p) != 20 {
		t.Errorf("expected 20-char password, got %d", len(p))
	}
}

func TestGeneratePassword_Alphanumeric(t *testing.T) {
	p := GeneratePassword(100)
	for _, r := range p {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			t.Errorf("non-alphanumeric character in password: %c", r)
		}
	}
}
