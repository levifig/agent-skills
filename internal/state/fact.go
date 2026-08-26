package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	FactKindJournal = "journal"

	legacyFactEnvID = "legacy-host"
)

var registeredFactKinds = map[string]string{
	FactKindJournal: "notebook",
}

// FactEnvelope is the v1 grow-only sync contract. There is no supersession field.
type FactEnvelope struct {
	ID         string `json:"id"`
	ProjectID  string `json:"project_id"`
	Kind       string `json:"kind"`
	Payload    string `json:"payload"`
	EnvID      string `json:"env_id"`
	Seq        int64  `json:"seq"`
	HLC        string `json:"hlc"`
	EnvelopeV  int    `json:"envelope_v"`
	Permanence string `json:"permanence_class,omitempty"`
}

// AppendFactInput describes one grow-only append through the single write chokepoint.
type AppendFactInput struct {
	ProjectID string
	Kind      string
	Payload   string
	EnvID     string
	ID        string
	Now       time.Time
}

// AppendFact appends one fact to the grow-only store.
func AppendFact(ctx context.Context, store *Store, input AppendFactInput) (FactEnvelope, error) {
	if store == nil || store.db == nil {
		return FactEnvelope{}, fmt.Errorf("append fact: store is nil")
	}
	tx, err := store.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		return FactEnvelope{}, fmt.Errorf("begin fact transaction: %w", err)
	}
	defer tx.Rollback()
	envelope, err := appendFactTx(ctx, tx, input)
	if err != nil {
		return FactEnvelope{}, err
	}
	if err := tx.Commit(); err != nil {
		return FactEnvelope{}, fmt.Errorf("commit fact transaction: %w", err)
	}
	return envelope, nil
}

func appendFactTx(ctx context.Context, tx *sql.Tx, input AppendFactInput) (FactEnvelope, error) {
	permanence, err := validateFactKind(input.Kind)
	if err != nil {
		return FactEnvelope{}, err
	}
	payload := strings.TrimSpace(input.Payload)
	if payload == "" {
		return FactEnvelope{}, fmt.Errorf("append fact: payload cannot be empty")
	}
	envID := strings.TrimSpace(input.EnvID)
	if envID == "" {
		envID = localFactEnvID()
	}
	now := input.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id, err = mintFactID(now)
		if err != nil {
			return FactEnvelope{}, err
		}
	}
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM facts WHERE id = ?`, id).Scan(&existing); err != nil {
		return FactEnvelope{}, fmt.Errorf("inspect fact id %q: %w", id, err)
	}
	if existing > 0 {
		return FactEnvelope{}, fmt.Errorf("append fact: id %q already exists", id)
	}

	hlc, seq, err := nextFactClockTx(ctx, tx, input.ProjectID, envID, now)
	if err != nil {
		return FactEnvelope{}, err
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO facts (id, project_id, kind, payload, env_id, seq, hlc, envelope_v)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
`, id, input.ProjectID, input.Kind, payload, envID, seq, hlc.String(), factEnvelopeVersion); err != nil {
		return FactEnvelope{}, fmt.Errorf("insert fact %q: %w", id, err)
	}
	return FactEnvelope{
		ID:         id,
		ProjectID:  input.ProjectID,
		Kind:       input.Kind,
		Payload:    payload,
		EnvID:      envID,
		Seq:        seq,
		HLC:        hlc.String(),
		EnvelopeV:  factEnvelopeVersion,
		Permanence: permanence,
	}, nil
}

func validateFactKind(kind string) (string, error) {
	kind = strings.TrimSpace(kind)
	permanence, ok := registeredFactKinds[kind]
	if !ok {
		return "", fmt.Errorf("append fact: unknown kind %q", kind)
	}
	return permanence, nil
}

func localFactEnvID() string {
	host, err := os.Hostname()
	if err != nil || strings.TrimSpace(host) == "" {
		return "local-host"
	}
	return strings.TrimSpace(host)
}

func nextFactClockTx(ctx context.Context, tx *sql.Tx, projectID, envID string, now time.Time) (HLC, int64, error) {
	nowMS := now.UTC().UnixMilli()
	var wallMS, logical, nextSeq int64
	err := tx.QueryRowContext(ctx, `
SELECT hlc_wall_ms, hlc_logical, next_seq
FROM fact_env_clocks
WHERE project_id = ? AND env_id = ?
`, projectID, envID).Scan(&wallMS, &logical, &nextSeq)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		hlc := advanceHLC(HLC{}, nowMS, HLC{})
		if _, err := tx.ExecContext(ctx, `
INSERT INTO fact_env_clocks (project_id, env_id, hlc_wall_ms, hlc_logical, next_seq)
VALUES (?, ?, ?, ?, ?)
`, projectID, envID, hlc.WallMS, hlc.Logical, int64(2)); err != nil {
			return HLC{}, 0, fmt.Errorf("seed fact env clock: %w", err)
		}
		return hlc, 1, nil
	case err != nil:
		return HLC{}, 0, fmt.Errorf("read fact env clock: %w", err)
	default:
		current := HLC{WallMS: wallMS, Logical: logical}
		hlc := advanceHLC(current, nowMS, current)
		if _, err := tx.ExecContext(ctx, `
UPDATE fact_env_clocks
SET hlc_wall_ms = ?, hlc_logical = ?, next_seq = ?
WHERE project_id = ? AND env_id = ?
`, hlc.WallMS, hlc.Logical, nextSeq+1, projectID, envID); err != nil {
			return HLC{}, 0, fmt.Errorf("advance fact env clock: %w", err)
		}
		return hlc, nextSeq, nil
	}
}

func observeFactHLCTx(ctx context.Context, tx *sql.Tx, projectID string, seen HLC) error {
	rows, err := tx.QueryContext(ctx, `
SELECT env_id, hlc_wall_ms, hlc_logical, next_seq
FROM fact_env_clocks
WHERE project_id = ?
`, projectID)
	if err != nil {
		return fmt.Errorf("list fact env clocks: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var envID string
		var wallMS, logical, nextSeq int64
		if err := rows.Scan(&envID, &wallMS, &logical, &nextSeq); err != nil {
			return fmt.Errorf("scan fact env clock: %w", err)
		}
		current := HLC{WallMS: wallMS, Logical: logical}
		if compareHLC(seen, current) <= 0 {
			continue
		}
		advanced := advanceHLC(current, seen.WallMS, seen)
		if _, err := tx.ExecContext(ctx, `
UPDATE fact_env_clocks
SET hlc_wall_ms = ?, hlc_logical = ?
WHERE project_id = ? AND env_id = ?
`, advanced.WallMS, advanced.Logical, projectID, envID); err != nil {
			return fmt.Errorf("observe fact hlc for env %q: %w", envID, err)
		}
	}
	return rows.Err()
}

const PermanenceScratchpad = "scratchpad"

// RegisterFactKind registers a closed fact kind and its permanence class.
func RegisterFactKind(kind, permanence string) {
	registeredFactKinds[strings.TrimSpace(kind)] = strings.TrimSpace(permanence)
}

// FactPermanenceClass returns the permanence class for a registered kind.
func FactPermanenceClass(kind string) (string, bool) {
	permanence, ok := registeredFactKinds[strings.TrimSpace(kind)]
	return permanence, ok
}

// LocalFactEnvID returns the pre-auth environment identity for fact writes.
func LocalFactEnvID() string {
	return localFactEnvID()
}
