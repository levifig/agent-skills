package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	FactKindEnrollment = "enrollment"
	defaultHLCSkewRefusalMS = int64((24 * time.Hour) / time.Millisecond)
)

func init() {
	RegisterFactKind(FactKindEnrollment, "ledger")
}

// HLCSkewError is returned when a remote fact HLC is grossly skewed from the local stream.
type HLCSkewError struct {
	RemoteHLC string
	LocalMax  string
	SkewMS    int64
	LimitMS   int64
}

func (e *HLCSkewError) Error() string {
	return fmt.Sprintf("receive fact: hlc skew %dms exceeds limit %dms (remote=%s local_max=%s)",
		e.SkewMS, e.LimitMS, e.RemoteHLC, e.LocalMax)
}

// ReceiveFactOptions controls remote fact ingestion.
type ReceiveFactOptions struct {
	MaxHLCSkewMS int64
}

// ReceiveFact ingests one remote fact into the grow-only store. Duplicate ids are
// ignored (idempotent). Returns inserted=true when a new row was written.
func ReceiveFact(ctx context.Context, store *Store, envelope FactEnvelope, opts ReceiveFactOptions) (bool, error) {
	if store == nil || store.db == nil {
		return false, fmt.Errorf("receive fact: store is nil")
	}
	id := strings.TrimSpace(envelope.ID)
	if id == "" {
		return false, fmt.Errorf("receive fact: id is empty")
	}
	projectID := strings.TrimSpace(envelope.ProjectID)
	if projectID == "" {
		return false, fmt.Errorf("receive fact: project id is empty")
	}
	if _, err := validateFactKind(envelope.Kind); err != nil {
		return false, err
	}
	payload := strings.TrimSpace(envelope.Payload)
	if payload == "" {
		return false, fmt.Errorf("receive fact: payload cannot be empty")
	}
	envID := strings.TrimSpace(envelope.EnvID)
	if envID == "" {
		return false, fmt.Errorf("receive fact: env id is empty")
	}
	if envelope.Seq < 1 {
		return false, fmt.Errorf("receive fact: seq must be >= 1")
	}
	remoteHLC, err := parseHLC(envelope.HLC)
	if err != nil {
		return false, fmt.Errorf("receive fact: %w", err)
	}
	if envelope.EnvelopeV != factEnvelopeVersion {
		return false, fmt.Errorf("receive fact: unsupported envelope_v %d", envelope.EnvelopeV)
	}

	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return false, fmt.Errorf("begin receive fact transaction: %w", err)
	}
	defer tx.Rollback()

	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM facts WHERE id = ?`, id).Scan(&existing); err != nil {
		return false, fmt.Errorf("inspect fact id %q: %w", id, err)
	}
	if existing > 0 {
		return false, nil
	}

	if err := refuseGrossHLCSkewTx(ctx, tx, projectID, remoteHLC, opts.MaxHLCSkewMS); err != nil {
		return false, err
	}

	if _, err := tx.ExecContext(ctx, `
INSERT INTO facts (id, project_id, kind, payload, env_id, seq, hlc, envelope_v)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, id, projectID, envelope.Kind, payload, envID, envelope.Seq, remoteHLC.String(), envelope.EnvelopeV); err != nil {
		return false, fmt.Errorf("insert received fact %q: %w", id, err)
	}
	if err := observeFactHLCTx(ctx, tx, projectID, remoteHLC); err != nil {
		return false, err
	}
	if err := reconcileEnvClockAfterReceiveTx(ctx, tx, projectID, envID, envelope.Seq, remoteHLC); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, fmt.Errorf("commit received fact: %w", err)
	}
	return true, nil
}

func refuseGrossHLCSkewTx(ctx context.Context, tx *sql.Tx, projectID string, remote HLC, maxSkewMS int64) error {
	if maxSkewMS <= 0 {
		maxSkewMS = defaultHLCSkewRefusalMS
	}
	localMax, err := maxFactHLCTx(ctx, tx, projectID)
	if err != nil {
		return err
	}
	if localMax.WallMS == 0 {
		return nil
	}
	skew := remote.WallMS - localMax.WallMS
	if skew < 0 {
		skew = -skew
	}
	if skew > maxSkewMS {
		return &HLCSkewError{
			RemoteHLC: remote.String(),
			LocalMax:  localMax.String(),
			SkewMS:    skew,
			LimitMS:   maxSkewMS,
		}
	}
	return nil
}

func maxFactHLCTx(ctx context.Context, tx *sql.Tx, projectID string) (HLC, error) {
	var maxHLC string
	err := tx.QueryRowContext(ctx, `
SELECT hlc
FROM facts
WHERE project_id = ?
ORDER BY hlc DESC, env_id DESC, id DESC
LIMIT 1
`, projectID).Scan(&maxHLC)
	if errors.Is(err, sql.ErrNoRows) {
		return HLC{}, nil
	}
	if err != nil {
		return HLC{}, fmt.Errorf("read max fact hlc: %w", err)
	}
	return parseHLC(maxHLC)
}

func reconcileEnvClockAfterReceiveTx(ctx context.Context, tx *sql.Tx, projectID, envID string, seq int64, hlc HLC) error {
	var wallMS, logical, nextSeq int64
	err := tx.QueryRowContext(ctx, `
SELECT hlc_wall_ms, hlc_logical, next_seq
FROM fact_env_clocks
WHERE project_id = ? AND env_id = ?
`, projectID, envID).Scan(&wallMS, &logical, &nextSeq)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := tx.ExecContext(ctx, `
INSERT INTO fact_env_clocks (project_id, env_id, hlc_wall_ms, hlc_logical, next_seq)
VALUES (?, ?, ?, ?, ?)
`, projectID, envID, hlc.WallMS, hlc.Logical, seq+1); err != nil {
			return fmt.Errorf("seed env clock after receive: %w", err)
		}
		return nil
	case err != nil:
		return fmt.Errorf("read env clock after receive: %w", err)
	default:
		current := HLC{WallMS: wallMS, Logical: logical}
		advanced := advanceHLC(current, hlc.WallMS, hlc)
		if seq+1 > nextSeq {
			nextSeq = seq + 1
		}
		if _, err := tx.ExecContext(ctx, `
UPDATE fact_env_clocks
SET hlc_wall_ms = ?, hlc_logical = ?, next_seq = ?
WHERE project_id = ? AND env_id = ?
`, advanced.WallMS, advanced.Logical, nextSeq, projectID, envID); err != nil {
			return fmt.Errorf("update env clock after receive: %w", err)
		}
		return nil
	}
}

// GetFactByID returns one fact envelope when present.
func GetFactByID(ctx context.Context, store *Store, factID string) (FactEnvelope, bool, error) {
	if store == nil || store.db == nil {
		return FactEnvelope{}, false, fmt.Errorf("get fact: store is nil")
	}
	factID = strings.TrimSpace(factID)
	var envelope FactEnvelope
	err := store.db.QueryRowContext(ctx, `
SELECT id, project_id, kind, payload, env_id, seq, hlc, envelope_v
FROM facts
WHERE id = ?
`, factID).Scan(&envelope.ID, &envelope.ProjectID, &envelope.Kind, &envelope.Payload,
		&envelope.EnvID, &envelope.Seq, &envelope.HLC, &envelope.EnvelopeV)
	if errors.Is(err, sql.ErrNoRows) {
		return FactEnvelope{}, false, nil
	}
	if err != nil {
		return FactEnvelope{}, false, fmt.Errorf("get fact %q: %w", factID, err)
	}
	permanence, ok := FactPermanenceClass(envelope.Kind)
	if ok {
		envelope.Permanence = permanence
	}
	return envelope, true, nil
}
