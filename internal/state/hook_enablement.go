package state

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// Enablement value vocabulary stored in hook_enablements.enablement.
const (
	HookEnablementEnabled  = "enabled"
	HookEnablementDisabled = "disabled"
)

// HookEnablementRef identifies one catalog hook for a target.
type HookEnablementRef struct {
	Target string
	Event  string
	HookID string
}

// HookEnablement is one user-scoped enablement record.
// Absence of a record means the hook is enabled; this type is only returned
// when a row exists (or after a set that materializes one).
type HookEnablement struct {
	ID         string  `json:"id"`
	Target     string  `json:"target"`
	Event      string  `json:"event"`
	HookID     string  `json:"hook_id"`
	Enablement string  `json:"enablement"`
	AbsorbedAt *string `json:"absorbed_at,omitempty"`
	CreatedAt  string  `json:"created_at"`
	UpdatedAt  string  `json:"updated_at"`
}

// HookAbsorptionMarker is the durable per-target absorption gate.
type HookAbsorptionMarker struct {
	ID                  string `json:"id"`
	Target              string `json:"target"`
	AbsorbedFromVersion string `json:"absorbed_from_version"`
	AbsorbedAt          string `json:"absorbed_at"`
	CreatedAt           string `json:"created_at"`
}

// HookTrustedPath is one recorded install path for a target's Loaf executable.
type HookTrustedPath struct {
	ID        string `json:"id"`
	Target    string `json:"target"`
	Path      string `json:"path"`
	IsCurrent bool   `json:"is_current"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// EffectiveHookEnabled reports whether a hook should project as enabled.
// Absence of a record means enabled.
func (s *Store) EffectiveHookEnabled(ctx context.Context, target, event, hookID string) (bool, error) {
	target, event, hookID, err := normalizeHookEnablementKey(target, event, hookID)
	if err != nil {
		return false, err
	}
	var enablement string
	err = s.db.QueryRowContext(ctx, `
SELECT enablement
FROM hook_enablements
WHERE target = ? AND event = ? AND hook_id = ?
`, target, event, hookID).Scan(&enablement)
	if err == sql.ErrNoRows {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("read hook enablement %s/%s/%s: %w", target, event, hookID, err)
	}
	return enablement == HookEnablementEnabled, nil
}

// GetHookEnablement returns the stored enablement row when present.
func (s *Store) GetHookEnablement(ctx context.Context, target, event, hookID string) (HookEnablement, bool, error) {
	target, event, hookID, err := normalizeHookEnablementKey(target, event, hookID)
	if err != nil {
		return HookEnablement{}, false, err
	}
	row, err := scanHookEnablement(s.db.QueryRowContext(ctx, `
SELECT id, target, event, hook_id, enablement, absorbed_at, created_at, updated_at
FROM hook_enablements
WHERE target = ? AND event = ? AND hook_id = ?
`, target, event, hookID))
	if err == sql.ErrNoRows {
		return HookEnablement{}, false, nil
	}
	if err != nil {
		return HookEnablement{}, false, fmt.Errorf("get hook enablement %s/%s/%s: %w", target, event, hookID, err)
	}
	return row, true, nil
}

// SetHookEnablement upserts enablement through the natural key. The opaque ID
// is minted only on insert; absorbed_at is never updated by toggles.
func (s *Store) SetHookEnablement(ctx context.Context, target, event, hookID string, enabled bool) (HookEnablement, error) {
	target, event, hookID, err := normalizeHookEnablementKey(target, event, hookID)
	if err != nil {
		return HookEnablement{}, err
	}
	enablement := HookEnablementDisabled
	if enabled {
		enablement = HookEnablementEnabled
	}
	now := time.Now().UTC().Format(time.RFC3339)
	id, err := newOpaqueStateID("hen")
	if err != nil {
		return HookEnablement{}, err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HookEnablement{}, fmt.Errorf("begin hook enablement set: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO hook_enablements (id, target, event, hook_id, enablement, absorbed_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, NULL, ?, ?)
ON CONFLICT(target, event, hook_id) DO UPDATE SET
  enablement = excluded.enablement,
  updated_at = excluded.updated_at
`, id, target, event, hookID, enablement, now, now); err != nil {
		return HookEnablement{}, fmt.Errorf("set hook enablement %s/%s/%s: %w", target, event, hookID, err)
	}

	row, err := scanHookEnablement(tx.QueryRowContext(ctx, `
SELECT id, target, event, hook_id, enablement, absorbed_at, created_at, updated_at
FROM hook_enablements
WHERE target = ? AND event = ? AND hook_id = ?
`, target, event, hookID))
	if err != nil {
		return HookEnablement{}, fmt.Errorf("reload hook enablement %s/%s/%s: %w", target, event, hookID, err)
	}
	if err := tx.Commit(); err != nil {
		return HookEnablement{}, fmt.Errorf("commit hook enablement set: %w", err)
	}
	return row, nil
}

// ListHookEnablements returns all enablement records for a target, ordered by
// event then hook_id. Tombstone rows for retired hook IDs are included.
func (s *Store) ListHookEnablements(ctx context.Context, target string) ([]HookEnablement, error) {
	target, err := normalizeNonEmpty("target", target)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, target, event, hook_id, enablement, absorbed_at, created_at, updated_at
FROM hook_enablements
WHERE target = ?
ORDER BY event, hook_id, id
`, target)
	if err != nil {
		return nil, fmt.Errorf("list hook enablements for %s: %w", target, err)
	}
	defer rows.Close()

	out := []HookEnablement{}
	for rows.Next() {
		row, err := scanHookEnablement(rows)
		if err != nil {
			return nil, fmt.Errorf("scan hook enablement: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hook enablements for %s: %w", target, err)
	}
	return out, nil
}

// GetHookAbsorptionMarker returns the absorption marker for a target when set.
func (s *Store) GetHookAbsorptionMarker(ctx context.Context, target string) (HookAbsorptionMarker, bool, error) {
	target, err := normalizeNonEmpty("target", target)
	if err != nil {
		return HookAbsorptionMarker{}, false, err
	}
	marker, err := scanHookAbsorptionMarker(s.db.QueryRowContext(ctx, `
SELECT id, target, absorbed_from_version, absorbed_at, created_at
FROM hook_absorption_markers
WHERE target = ?
`, target))
	if err == sql.ErrNoRows {
		return HookAbsorptionMarker{}, false, nil
	}
	if err != nil {
		return HookAbsorptionMarker{}, false, fmt.Errorf("get hook absorption marker for %s: %w", target, err)
	}
	return marker, true, nil
}

// AbsorbAndMarkHooks writes disabled enablement records for the given refs and
// the per-target absorption marker in one transaction. All-or-nothing: failure
// leaves neither the records nor the marker committed.
func (s *Store) AbsorbAndMarkHooks(ctx context.Context, target, absorbedFromVersion string, disabled []HookEnablementRef) (HookAbsorptionMarker, error) {
	return s.absorbAndMarkHooksWithHooks(ctx, target, absorbedFromVersion, disabled, nil)
}

// absorbAndMarkWriteHooks injects failures between transactional stages in tests.
type absorbAndMarkWriteHooks struct {
	afterEnablements func(*sql.Tx) error
	afterMarker      func(*sql.Tx) error
	beforeCommit     func(*sql.Tx) error
}

func (s *Store) absorbAndMarkHooksWithHooks(ctx context.Context, target, absorbedFromVersion string, disabled []HookEnablementRef, hooks *absorbAndMarkWriteHooks) (HookAbsorptionMarker, error) {
	target, err := normalizeNonEmpty("target", target)
	if err != nil {
		return HookAbsorptionMarker{}, err
	}
	absorbedFromVersion, err = normalizeNonEmpty("absorbed_from_version", absorbedFromVersion)
	if err != nil {
		return HookAbsorptionMarker{}, err
	}
	normalized := make([]HookEnablementRef, 0, len(disabled))
	for i, ref := range disabled {
		event, err := normalizeNonEmpty("event", ref.Event)
		if err != nil {
			return HookAbsorptionMarker{}, fmt.Errorf("disabled[%d]: %w", i, err)
		}
		hookID, err := normalizeNonEmpty("hook_id", ref.HookID)
		if err != nil {
			return HookAbsorptionMarker{}, fmt.Errorf("disabled[%d]: %w", i, err)
		}
		refTarget := strings.TrimSpace(ref.Target)
		if refTarget == "" {
			refTarget = target
		} else if refTarget != target {
			return HookAbsorptionMarker{}, fmt.Errorf("disabled[%d]: target %q does not match absorption target %q", i, refTarget, target)
		}
		normalized = append(normalized, HookEnablementRef{Target: target, Event: event, HookID: hookID})
	}

	now := time.Now().UTC().Format(time.RFC3339)
	markerID, err := newOpaqueStateID("ham")
	if err != nil {
		return HookAbsorptionMarker{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HookAbsorptionMarker{}, fmt.Errorf("begin absorb-and-mark: %w", err)
	}
	defer tx.Rollback()

	for _, ref := range normalized {
		id, err := newOpaqueStateID("hen")
		if err != nil {
			return HookAbsorptionMarker{}, err
		}
		// Mint ID only on insert. On conflict keep the existing ID and leave
		// absorbed_at immutable once set (COALESCE prefers the existing value).
		if _, err := tx.ExecContext(ctx, `
INSERT INTO hook_enablements (id, target, event, hook_id, enablement, absorbed_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(target, event, hook_id) DO UPDATE SET
  enablement = excluded.enablement,
  absorbed_at = COALESCE(hook_enablements.absorbed_at, excluded.absorbed_at),
  updated_at = excluded.updated_at
`, id, ref.Target, ref.Event, ref.HookID, HookEnablementDisabled, now, now, now); err != nil {
			return HookAbsorptionMarker{}, fmt.Errorf("absorb disable %s/%s/%s: %w", ref.Target, ref.Event, ref.HookID, err)
		}
	}
	if err := runAbsorbAndMarkHook(hooks, "after enablements", func(h *absorbAndMarkWriteHooks) func(*sql.Tx) error {
		return h.afterEnablements
	}, tx); err != nil {
		return HookAbsorptionMarker{}, err
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO hook_absorption_markers (id, target, absorbed_from_version, absorbed_at, created_at)
VALUES (?, ?, ?, ?, ?)
`, markerID, target, absorbedFromVersion, now, now); err != nil {
		return HookAbsorptionMarker{}, fmt.Errorf("write hook absorption marker for %s: %w", target, err)
	}
	if err := runAbsorbAndMarkHook(hooks, "after marker", func(h *absorbAndMarkWriteHooks) func(*sql.Tx) error {
		return h.afterMarker
	}, tx); err != nil {
		return HookAbsorptionMarker{}, err
	}

	marker, err := scanHookAbsorptionMarker(tx.QueryRowContext(ctx, `
SELECT id, target, absorbed_from_version, absorbed_at, created_at
FROM hook_absorption_markers
WHERE target = ?
`, target))
	if err != nil {
		return HookAbsorptionMarker{}, fmt.Errorf("reload hook absorption marker for %s: %w", target, err)
	}
	if err := runAbsorbAndMarkHook(hooks, "before commit", func(h *absorbAndMarkWriteHooks) func(*sql.Tx) error {
		return h.beforeCommit
	}, tx); err != nil {
		return HookAbsorptionMarker{}, err
	}
	if err := tx.Commit(); err != nil {
		return HookAbsorptionMarker{}, fmt.Errorf("commit absorb-and-mark: %w", err)
	}
	return marker, nil
}

func runAbsorbAndMarkHook(hooks *absorbAndMarkWriteHooks, stage string, pick func(*absorbAndMarkWriteHooks) func(*sql.Tx) error, tx *sql.Tx) error {
	if hooks == nil {
		return nil
	}
	hook := pick(hooks)
	if hook == nil {
		return nil
	}
	if err := hook(tx); err != nil {
		return fmt.Errorf("absorb-and-mark failed at %s: %w", stage, err)
	}
	return nil
}

// RecordHookTrustedPath records path as the current trusted executable for
// target. The previous current path, if any, becomes historical and is retained.
func (s *Store) RecordHookTrustedPath(ctx context.Context, target, path string) (HookTrustedPath, error) {
	target, err := normalizeNonEmpty("target", target)
	if err != nil {
		return HookTrustedPath{}, err
	}
	path, err = normalizeNonEmpty("path", path)
	if err != nil {
		return HookTrustedPath{}, err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	id, err := newOpaqueStateID("htp")
	if err != nil {
		return HookTrustedPath{}, err
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return HookTrustedPath{}, fmt.Errorf("begin record hook trusted path: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx, `
UPDATE hook_trusted_paths
SET is_current = 0, updated_at = ?
WHERE target = ? AND is_current = 1 AND path != ?
`, now, target, path); err != nil {
		return HookTrustedPath{}, fmt.Errorf("demote previous trusted path for %s: %w", target, err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO hook_trusted_paths (id, target, path, is_current, created_at, updated_at)
VALUES (?, ?, ?, 1, ?, ?)
ON CONFLICT(target, path) DO UPDATE SET
  is_current = 1,
  updated_at = excluded.updated_at
`, id, target, path, now, now); err != nil {
		return HookTrustedPath{}, fmt.Errorf("record trusted path for %s: %w", target, err)
	}

	row, err := scanHookTrustedPath(tx.QueryRowContext(ctx, `
SELECT id, target, path, is_current, created_at, updated_at
FROM hook_trusted_paths
WHERE target = ? AND path = ?
`, target, path))
	if err != nil {
		return HookTrustedPath{}, fmt.Errorf("reload trusted path for %s: %w", target, err)
	}
	if err := tx.Commit(); err != nil {
		return HookTrustedPath{}, fmt.Errorf("commit record hook trusted path: %w", err)
	}
	return row, nil
}

// ListHookTrustedPaths returns every recorded path for a target, current first.
func (s *Store) ListHookTrustedPaths(ctx context.Context, target string) ([]HookTrustedPath, error) {
	target, err := normalizeNonEmpty("target", target)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, target, path, is_current, created_at, updated_at
FROM hook_trusted_paths
WHERE target = ?
ORDER BY is_current DESC, path, id
`, target)
	if err != nil {
		return nil, fmt.Errorf("list hook trusted paths for %s: %w", target, err)
	}
	defer rows.Close()

	out := []HookTrustedPath{}
	for rows.Next() {
		row, err := scanHookTrustedPath(rows)
		if err != nil {
			return nil, fmt.Errorf("scan hook trusted path: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate hook trusted paths for %s: %w", target, err)
	}
	return out, nil
}

// CurrentHookTrustedPath returns the current install path for a target, if any.
func (s *Store) CurrentHookTrustedPath(ctx context.Context, target string) (HookTrustedPath, bool, error) {
	target, err := normalizeNonEmpty("target", target)
	if err != nil {
		return HookTrustedPath{}, false, err
	}
	row, err := scanHookTrustedPath(s.db.QueryRowContext(ctx, `
SELECT id, target, path, is_current, created_at, updated_at
FROM hook_trusted_paths
WHERE target = ? AND is_current = 1
`, target))
	if err == sql.ErrNoRows {
		return HookTrustedPath{}, false, nil
	}
	if err != nil {
		return HookTrustedPath{}, false, fmt.Errorf("current hook trusted path for %s: %w", target, err)
	}
	return row, true, nil
}

type hookEnablementScanner interface {
	Scan(dest ...any) error
}

func scanHookEnablement(row hookEnablementScanner) (HookEnablement, error) {
	var (
		out        HookEnablement
		absorbedAt sql.NullString
	)
	if err := row.Scan(&out.ID, &out.Target, &out.Event, &out.HookID, &out.Enablement, &absorbedAt, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return HookEnablement{}, err
	}
	if absorbedAt.Valid {
		value := absorbedAt.String
		out.AbsorbedAt = &value
	}
	return out, nil
}

func scanHookAbsorptionMarker(row hookEnablementScanner) (HookAbsorptionMarker, error) {
	var out HookAbsorptionMarker
	if err := row.Scan(&out.ID, &out.Target, &out.AbsorbedFromVersion, &out.AbsorbedAt, &out.CreatedAt); err != nil {
		return HookAbsorptionMarker{}, err
	}
	return out, nil
}

func scanHookTrustedPath(row hookEnablementScanner) (HookTrustedPath, error) {
	var (
		out       HookTrustedPath
		isCurrent int
	)
	if err := row.Scan(&out.ID, &out.Target, &out.Path, &isCurrent, &out.CreatedAt, &out.UpdatedAt); err != nil {
		return HookTrustedPath{}, err
	}
	out.IsCurrent = isCurrent == 1
	return out, nil
}

func normalizeHookEnablementKey(target, event, hookID string) (string, string, string, error) {
	target, err := normalizeNonEmpty("target", target)
	if err != nil {
		return "", "", "", err
	}
	event, err = normalizeNonEmpty("event", event)
	if err != nil {
		return "", "", "", err
	}
	hookID, err = normalizeNonEmpty("hook_id", hookID)
	if err != nil {
		return "", "", "", err
	}
	return target, event, hookID, nil
}

func normalizeNonEmpty(field, value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%s must be nonempty", field)
	}
	return trimmed, nil
}

// newOpaqueStateID mints a once-random opaque primary key with a short prefix.
// IDs are never derived from natural keys or other mutable inputs.
func newOpaqueStateID(prefix string) (string, error) {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", fmt.Errorf("generate %s id: %w", prefix, err)
	}
	return prefix + "_" + hex.EncodeToString(raw[:]), nil
}
