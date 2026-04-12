package backup

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/core"
)

// ErrNoBackupsFound is returned by RestoreApp when storage contains
// no backup payloads for the requested tenant/app. Callers can use
// errors.Is to distinguish "nothing to restore" from a storage or
// unmarshal error.
var ErrNoBackupsFound = errors.New("backup: no backups found")

// RestoreApp restores a single application from the most recent
// backup payload in storage. The payload format matches what
// scheduler.backupApp writes: json.Marshal of core.Application at
// key "{tenantID}/{appID}/{backupID}.json".
//
// Behavior:
//   - Lists storage under "{tenantID}/{appID}/", filters to .json
//     payloads only (DB snapshot .db files and other artifacts are
//     ignored), and picks the entry with the newest CreatedAt.
//   - Downloads, unmarshals, and validates that the payload's
//     TenantID/ID (when present) match the requested identifiers so
//     a mis-keyed upload cannot poison a different tenant.
//   - Writes via the Store interface: UpdateApp when the app still
//     exists ("reset to last backup"), CreateApp otherwise ("disaster
//     recovery"). UpdatedAt is refreshed to time.Now so the restore
//     is visible as a fresh write downstream.
//
// RestoreApp is the read-side counterpart to the Tier 73-77 backup
// hardening pass. Before this function the backup path was write-only
// and untested end-to-end: nothing in the codebase actually loaded a
// backup back into a Store, so a silently broken Upload would only
// surface during a real disaster.
func RestoreApp(ctx context.Context, store core.Store, storage core.BackupStorage, tenantID, appID string) (*core.Application, error) {
	if store == nil {
		return nil, errors.New("backup: store is nil")
	}
	if storage == nil {
		return nil, errors.New("backup: storage is nil")
	}
	if tenantID == "" || appID == "" {
		return nil, errors.New("backup: tenantID and appID are required")
	}

	prefix := fmt.Sprintf("%s/%s/", tenantID, appID)
	entries, err := storage.List(ctx, prefix)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}

	var newest *core.BackupEntry
	for i := range entries {
		if !strings.HasSuffix(entries[i].Key, ".json") {
			continue
		}
		if newest == nil || entries[i].CreatedAt > newest.CreatedAt {
			newest = &entries[i]
		}
	}
	if newest == nil {
		return nil, fmt.Errorf("%w: tenant=%s app=%s", ErrNoBackupsFound, tenantID, appID)
	}

	rc, err := storage.Download(ctx, newest.Key)
	if err != nil {
		return nil, fmt.Errorf("download backup %q: %w", newest.Key, err)
	}
	payload, readErr := io.ReadAll(rc)
	// Close before returning so a late Close error on the underlying
	// file doesn't leak a handle if unmarshal fails downstream.
	if closeErr := rc.Close(); closeErr != nil && readErr == nil {
		readErr = closeErr
	}
	if readErr != nil {
		return nil, fmt.Errorf("read backup payload: %w", readErr)
	}

	var restored core.Application
	if err := json.Unmarshal(payload, &restored); err != nil {
		return nil, fmt.Errorf("unmarshal backup: %w", err)
	}

	if restored.TenantID != "" && restored.TenantID != tenantID {
		return nil, fmt.Errorf("backup tenant mismatch: payload=%q expected=%q",
			restored.TenantID, tenantID)
	}
	if restored.ID != "" && restored.ID != appID {
		return nil, fmt.Errorf("backup app ID mismatch: payload=%q expected=%q",
			restored.ID, appID)
	}

	restored.UpdatedAt = time.Now().UTC()
	existing, getErr := store.GetApp(ctx, restored.ID)
	if getErr == nil && existing != nil {
		if err := store.UpdateApp(ctx, &restored); err != nil {
			return nil, fmt.Errorf("update app: %w", err)
		}
	} else {
		if err := store.CreateApp(ctx, &restored); err != nil {
			return nil, fmt.Errorf("create app: %w", err)
		}
	}

	return &restored, nil
}

// RestoreTenant restores every application under a tenant from the
// newest backup found for each app. appIDs must be supplied by the
// caller (typically from a records-only scan of the original Store
// before wipe, or from walking the on-disk backup tree).
//
// Returns the number of apps successfully restored and a slice of
// per-app errors encountered. A non-empty error slice does not abort
// the sweep — disaster recovery wants partial progress, not an
// all-or-nothing failure.
func RestoreTenant(ctx context.Context, store core.Store, storage core.BackupStorage, tenantID string, appIDs []string) (int, []error) {
	var restored int
	var errs []error
	for _, id := range appIDs {
		if err := ctx.Err(); err != nil {
			errs = append(errs, fmt.Errorf("restore cancelled mid-sweep: %w", err))
			return restored, errs
		}
		if _, err := RestoreApp(ctx, store, storage, tenantID, id); err != nil {
			errs = append(errs, fmt.Errorf("app %s: %w", id, err))
			continue
		}
		restored++
	}
	return restored, errs
}
