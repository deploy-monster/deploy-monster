package storage

import "testing"

// TestPackageExists verifies the storage subpackage is importable.
// The actual LocalStorage implementation lives in backup/local.go
// to keep the module self-contained; this is a marker package.
func TestPackageExists(t *testing.T) {
	// If this compiles, the package exists and is importable.
	t.Log("storage package is importable")
}
