package core

import (
	"log/slog"
	"runtime/debug"
)

// SafeGo launches a goroutine with panic recovery. If the goroutine panics,
// it logs the error with a stack trace. Use this for all background goroutines
// to prevent a single panic from crashing the entire server.
func SafeGo(logger *slog.Logger, name string, fn func()) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				if logger == nil {
					logger = slog.Default()
				}
				logger.Error("goroutine panic recovered",
					"goroutine", name,
					"error", r,
					"stack", string(debug.Stack()),
				)
			}
		}()
		fn()
	}()
}
