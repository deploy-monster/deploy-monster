package vps

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// discardLogger returns a logger that discards output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestModule_Init(t *testing.T) {
	m := New()
	c := &core.Core{
		Store:  nil,
		Logger: discardLogger(),
	}

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}

	if m.core != c {
		t.Error("core reference not set")
	}
	if m.store != nil {
		t.Error("store should be nil since Core.Store is nil")
	}
	if m.logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestModule_Init_WithStore(t *testing.T) {
	m := New()
	c := &core.Core{
		Store:  nil,
		Logger: discardLogger(),
	}

	if err := m.Init(context.Background(), c); err != nil {
		t.Fatalf("Init: %v", err)
	}
}
