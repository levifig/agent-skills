package state

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
)

func hookEnablementFixture(t *testing.T) *Store {
	t.Helper()
	root := projectRoot(t)
	stateHome := t.TempDir()
	status, err := Initialize(context.Background(), root, PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestEffectiveHookEnabledDefaultsToEnabled(t *testing.T) {
	store := hookEnablementFixture(t)
	ctx := context.Background()

	enabled, err := store.EffectiveHookEnabled(ctx, "codex", "SessionStart", "session-start-loaf")
	if err != nil {
		t.Fatalf("EffectiveHookEnabled() error = %v", err)
	}
	if !enabled {
		t.Fatal("absence of record must mean enabled")
	}
	if _, ok, err := store.GetHookEnablement(ctx, "codex", "SessionStart", "session-start-loaf"); err != nil || ok {
		t.Fatalf("GetHookEnablement() = ok=%v err=%v, want absent", ok, err)
	}
}

func TestSetHookEnablementUpsertIsIdempotentAndPreservesID(t *testing.T) {
	store := hookEnablementFixture(t)
	ctx := context.Background()

	first, err := store.SetHookEnablement(ctx, "cursor", "PreToolUse", "check-secrets", false)
	if err != nil {
		t.Fatalf("first SetHookEnablement() error = %v", err)
	}
	if first.Enablement != HookEnablementDisabled || first.ID == "" || first.AbsorbedAt != nil {
		t.Fatalf("first = %#v, want disabled with opaque ID and nil absorbed_at", first)
	}

	second, err := store.SetHookEnablement(ctx, "cursor", "PreToolUse", "check-secrets", false)
	if err != nil {
		t.Fatalf("second SetHookEnablement() error = %v", err)
	}
	if second.ID != first.ID {
		t.Fatalf("upsert re-minted ID: first=%q second=%q", first.ID, second.ID)
	}
	if second.CreatedAt != first.CreatedAt {
		t.Fatalf("upsert changed created_at: first=%q second=%q", first.CreatedAt, second.CreatedAt)
	}

	enabled, err := store.SetHookEnablement(ctx, "cursor", "PreToolUse", "check-secrets", true)
	if err != nil {
		t.Fatalf("enable SetHookEnablement() error = %v", err)
	}
	if enabled.ID != first.ID || enabled.Enablement != HookEnablementEnabled {
		t.Fatalf("enable = %#v, want same ID and enabled", enabled)
	}

	effective, err := store.EffectiveHookEnabled(ctx, "cursor", "PreToolUse", "check-secrets")
	if err != nil || !effective {
		t.Fatalf("EffectiveHookEnabled() = %v, %v; want true", effective, err)
	}
}

func TestHookEnablementNaturalKeyUniquenessIsSchemaEnforced(t *testing.T) {
	store := hookEnablementFixture(t)
	ctx := context.Background()

	mustExecSchemaSQL(t, store, `
INSERT INTO hook_enablements (id, target, event, hook_id, enablement, created_at, updated_at)
VALUES ('hen_a', 'codex', 'SessionStart', 'session-start-loaf', 'disabled', '2026-08-08T00:00:00Z', '2026-08-08T00:00:00Z')
`)
	err := execSchemaSQL(t, store, `
INSERT INTO hook_enablements (id, target, event, hook_id, enablement, created_at, updated_at)
VALUES ('hen_b', 'codex', 'SessionStart', 'session-start-loaf', 'enabled', '2026-08-08T00:00:01Z', '2026-08-08T00:00:01Z')
`)
	if err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("duplicate natural-key insert error = %v, want UNIQUE violation", err)
	}

	// Distinct hook IDs on the same target/event remain allowed.
	mustExecSchemaSQL(t, store, `
INSERT INTO hook_enablements (id, target, event, hook_id, enablement, created_at, updated_at)
VALUES ('hen_c', 'codex', 'SessionStart', 'other-hook', 'disabled', '2026-08-08T00:00:02Z', '2026-08-08T00:00:02Z')
`)

	// Prove the accessor never bypasses the constraint by re-minting on conflict.
	first, err := store.SetHookEnablement(ctx, "cursor", "Stop", "kb-staleness-nudge", false)
	if err != nil {
		t.Fatalf("SetHookEnablement() error = %v", err)
	}
	again, err := store.SetHookEnablement(ctx, "cursor", "Stop", "kb-staleness-nudge", true)
	if err != nil {
		t.Fatalf("second SetHookEnablement() error = %v", err)
	}
	if again.ID != first.ID {
		t.Fatalf("conflict path re-minted ID: %q -> %q", first.ID, again.ID)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM hook_enablements WHERE target = 'cursor' AND event = 'Stop' AND hook_id = 'kb-staleness-nudge'
`).Scan(&count); err != nil {
		t.Fatalf("count error = %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want exactly 1", count)
	}
}

func TestAbsorbedAtIsImmutableAcrossToggles(t *testing.T) {
	store := hookEnablementFixture(t)
	ctx := context.Background()

	marker, err := store.AbsorbAndMarkHooks(ctx, "codex", "0.2.20", []HookEnablementRef{
		{Event: "SessionStart", HookID: "session-start-loaf"},
	})
	if err != nil {
		t.Fatalf("AbsorbAndMarkHooks() error = %v", err)
	}
	if marker.Target != "codex" || marker.AbsorbedFromVersion != "0.2.20" || marker.AbsorbedAt == "" {
		t.Fatalf("marker = %#v", marker)
	}

	row, ok, err := store.GetHookEnablement(ctx, "codex", "SessionStart", "session-start-loaf")
	if err != nil || !ok {
		t.Fatalf("GetHookEnablement() = ok=%v err=%v", ok, err)
	}
	if row.AbsorbedAt == nil || *row.AbsorbedAt == "" || row.Enablement != HookEnablementDisabled {
		t.Fatalf("absorbed row = %#v", row)
	}
	originalAbsorbedAt := *row.AbsorbedAt
	originalID := row.ID

	enabled, err := store.SetHookEnablement(ctx, "codex", "SessionStart", "session-start-loaf", true)
	if err != nil {
		t.Fatalf("enable toggle error = %v", err)
	}
	if enabled.ID != originalID || enabled.AbsorbedAt == nil || *enabled.AbsorbedAt != originalAbsorbedAt {
		t.Fatalf("enable toggle mutated identity/provenance: %#v", enabled)
	}

	disabled, err := store.SetHookEnablement(ctx, "codex", "SessionStart", "session-start-loaf", false)
	if err != nil {
		t.Fatalf("disable toggle error = %v", err)
	}
	if disabled.ID != originalID || disabled.AbsorbedAt == nil || *disabled.AbsorbedAt != originalAbsorbedAt {
		t.Fatalf("disable toggle mutated identity/provenance: %#v", disabled)
	}

	// Explicit toggles never write absorbed_at on a fresh row.
	fresh, err := store.SetHookEnablement(ctx, "cursor", "SessionStart", "session-start-loaf", false)
	if err != nil {
		t.Fatalf("fresh disable error = %v", err)
	}
	if fresh.AbsorbedAt != nil {
		t.Fatalf("explicit toggle set absorbed_at = %v, want nil", *fresh.AbsorbedAt)
	}
}

func TestAbsorbAndMarkIsTransactionalAllOrNothing(t *testing.T) {
	store := hookEnablementFixture(t)
	ctx := context.Background()
	disabled := []HookEnablementRef{
		{Event: "SessionStart", HookID: "session-start-loaf"},
		{Event: "PreToolUse", HookID: "check-secrets"},
	}
	injected := errors.New("injected absorb failure")

	stages := []struct {
		name string
		hook func(*absorbAndMarkWriteHooks)
	}{
		{"after enablements", func(h *absorbAndMarkWriteHooks) {
			h.afterEnablements = func(*sql.Tx) error { return injected }
		}},
		{"after marker", func(h *absorbAndMarkWriteHooks) {
			h.afterMarker = func(*sql.Tx) error { return injected }
		}},
		{"before commit", func(h *absorbAndMarkWriteHooks) {
			h.beforeCommit = func(*sql.Tx) error { return injected }
		}},
	}

	for _, stage := range stages {
		t.Run(stage.name, func(t *testing.T) {
			hooks := &absorbAndMarkWriteHooks{}
			stage.hook(hooks)
			_, err := store.absorbAndMarkHooksWithHooks(ctx, "codex", "0.2.20", disabled, hooks)
			if err == nil {
				t.Fatal("error = nil, want injected failure")
			}
			if !errors.Is(err, injected) && !strings.Contains(err.Error(), "injected absorb failure") {
				t.Fatalf("error = %v, want injected failure", err)
			}

			var enablementCount, markerCount int
			if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM hook_enablements WHERE target = 'codex'`).Scan(&enablementCount); err != nil {
				t.Fatalf("count enablements: %v", err)
			}
			if err := store.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM hook_absorption_markers WHERE target = 'codex'`).Scan(&markerCount); err != nil {
				t.Fatalf("count markers: %v", err)
			}
			if enablementCount != 0 || markerCount != 0 {
				t.Fatalf("partial commit under failure: enablements=%d markers=%d", enablementCount, markerCount)
			}
			if _, ok, err := store.GetHookAbsorptionMarker(ctx, "codex"); err != nil || ok {
				t.Fatalf("marker present after failure: ok=%v err=%v", ok, err)
			}
		})
	}

	// Successful absorb-and-mark leaves both records and marker.
	marker, err := store.AbsorbAndMarkHooks(ctx, "codex", "0.2.20", disabled)
	if err != nil {
		t.Fatalf("AbsorbAndMarkHooks() error = %v", err)
	}
	if marker.AbsorbedFromVersion != "0.2.20" {
		t.Fatalf("marker version = %q", marker.AbsorbedFromVersion)
	}
	list, err := store.ListHookEnablements(ctx, "codex")
	if err != nil {
		t.Fatalf("ListHookEnablements() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list len = %d, want 2", len(list))
	}
	for _, row := range list {
		if row.Enablement != HookEnablementDisabled || row.AbsorbedAt == nil {
			t.Fatalf("absorbed row = %#v", row)
		}
	}

	// Marker uniqueness: a second absorb for the same target must fail.
	if _, err := store.AbsorbAndMarkHooks(ctx, "codex", "0.2.21", disabled[:1]); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("second absorb error = %v, want UNIQUE violation on marker", err)
	}
}

func TestFailedRepeatAbsorbPreservesEnablementIdentity(t *testing.T) {
	store := hookEnablementFixture(t)
	ctx := context.Background()
	refs := []HookEnablementRef{
		{Event: "SessionStart", HookID: "session-start-loaf"},
		{Event: "PreToolUse", HookID: "check-secrets"},
	}

	if _, err := store.AbsorbAndMarkHooks(ctx, "codex", "0.2.20", refs); err != nil {
		t.Fatalf("AbsorbAndMarkHooks() error = %v", err)
	}
	before, err := store.ListHookEnablements(ctx, "codex")
	if err != nil {
		t.Fatalf("ListHookEnablements() error = %v", err)
	}
	if len(before) != 2 {
		t.Fatalf("list len = %d, want 2", len(before))
	}
	for _, row := range before {
		if row.AbsorbedAt == nil || *row.AbsorbedAt == "" {
			t.Fatalf("absorbed row missing absorbed_at: %#v", row)
		}
	}

	// Repeat absorption for the same target must fail on the marker's
	// UNIQUE(target), regardless of version.
	if _, err := store.AbsorbAndMarkHooks(ctx, "codex", "0.2.21", refs); err == nil || !strings.Contains(err.Error(), "UNIQUE") {
		t.Fatalf("repeat absorb error = %v, want UNIQUE violation on marker", err)
	}

	after, err := store.ListHookEnablements(ctx, "codex")
	if err != nil {
		t.Fatalf("ListHookEnablements() after failed absorb error = %v", err)
	}
	if len(after) != len(before) {
		t.Fatalf("failed absorb changed row count: before=%d after=%d", len(before), len(after))
	}
	for i := range before {
		if after[i].ID != before[i].ID {
			t.Fatalf("failed absorb re-minted ID: %q -> %q", before[i].ID, after[i].ID)
		}
		if after[i].AbsorbedAt == nil || *after[i].AbsorbedAt != *before[i].AbsorbedAt {
			t.Fatalf("failed absorb mutated absorbed_at: %#v -> %#v", before[i], after[i])
		}
		if after[i].UpdatedAt != before[i].UpdatedAt {
			t.Fatalf("failed absorb mutated updated_at: %q -> %q", before[i].UpdatedAt, after[i].UpdatedAt)
		}
	}

	// A toggle upsert conflict after the failed absorb still preserves the
	// original id and absorbed_at.
	original, ok, err := store.GetHookEnablement(ctx, "codex", "SessionStart", "session-start-loaf")
	if err != nil || !ok {
		t.Fatalf("GetHookEnablement() = ok=%v err=%v", ok, err)
	}
	toggled, err := store.SetHookEnablement(ctx, "codex", "SessionStart", "session-start-loaf", true)
	if err != nil {
		t.Fatalf("toggle after failed absorb error = %v", err)
	}
	if toggled.ID != original.ID || toggled.AbsorbedAt == nil || *toggled.AbsorbedAt != *original.AbsorbedAt {
		t.Fatalf("toggle after failed absorb mutated identity/provenance: %#v", toggled)
	}
}

func TestListHookEnablementsIncludesTombstones(t *testing.T) {
	store := hookEnablementFixture(t)
	ctx := context.Background()

	if _, err := store.SetHookEnablement(ctx, "cursor", "PreToolUse", "retired-hook", false); err != nil {
		t.Fatalf("SetHookEnablement() error = %v", err)
	}
	if _, err := store.SetHookEnablement(ctx, "cursor", "Stop", "kb-staleness-nudge", true); err != nil {
		t.Fatalf("SetHookEnablement() error = %v", err)
	}
	// Other target must not leak into the list.
	if _, err := store.SetHookEnablement(ctx, "codex", "SessionStart", "session-start-loaf", false); err != nil {
		t.Fatalf("SetHookEnablement() error = %v", err)
	}

	list, err := store.ListHookEnablements(ctx, "cursor")
	if err != nil {
		t.Fatalf("ListHookEnablements() error = %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list = %#v, want 2 cursor rows including retired tombstone", list)
	}
	if list[0].HookID != "retired-hook" || list[1].HookID != "kb-staleness-nudge" {
		t.Fatalf("order/content = %#v", list)
	}
}

func TestHookTrustedPathsRecordCurrentAndPrevious(t *testing.T) {
	store := hookEnablementFixture(t)
	ctx := context.Background()

	first, err := store.RecordHookTrustedPath(ctx, "codex", "/usr/local/bin/loaf")
	if err != nil {
		t.Fatalf("first RecordHookTrustedPath() error = %v", err)
	}
	if !first.IsCurrent || first.Path != "/usr/local/bin/loaf" {
		t.Fatalf("first = %#v", first)
	}

	second, err := store.RecordHookTrustedPath(ctx, "codex", "/opt/loaf/bin/loaf")
	if err != nil {
		t.Fatalf("second RecordHookTrustedPath() error = %v", err)
	}
	if !second.IsCurrent || second.Path != "/opt/loaf/bin/loaf" {
		t.Fatalf("second = %#v", second)
	}

	current, ok, err := store.CurrentHookTrustedPath(ctx, "codex")
	if err != nil || !ok || current.Path != "/opt/loaf/bin/loaf" {
		t.Fatalf("CurrentHookTrustedPath() = %#v ok=%v err=%v", current, ok, err)
	}

	paths, err := store.ListHookTrustedPaths(ctx, "codex")
	if err != nil {
		t.Fatalf("ListHookTrustedPaths() error = %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %#v, want current + previous", paths)
	}
	if !paths[0].IsCurrent || paths[0].Path != "/opt/loaf/bin/loaf" {
		t.Fatalf("current-first order broken: %#v", paths)
	}
	if paths[1].IsCurrent || paths[1].Path != "/usr/local/bin/loaf" {
		t.Fatalf("previous path missing: %#v", paths)
	}

	// Re-recording a previous path promotes it back to current without duplicating.
	promoted, err := store.RecordHookTrustedPath(ctx, "codex", "/usr/local/bin/loaf")
	if err != nil {
		t.Fatalf("promote RecordHookTrustedPath() error = %v", err)
	}
	if promoted.ID != first.ID || !promoted.IsCurrent {
		t.Fatalf("promote = %#v, want same ID current", promoted)
	}
	paths, err = store.ListHookTrustedPaths(ctx, "codex")
	if err != nil {
		t.Fatalf("ListHookTrustedPaths() after promote error = %v", err)
	}
	if len(paths) != 2 {
		t.Fatalf("paths after promote = %#v, want still 2", paths)
	}
	var currentCount int
	for _, path := range paths {
		if path.IsCurrent {
			currentCount++
		}
	}
	if currentCount != 1 {
		t.Fatalf("current count = %d, want 1", currentCount)
	}
}

func TestHookTrustedPathsSingleCurrentPerTargetIsSchemaEnforced(t *testing.T) {
	store := hookEnablementFixture(t)

	mustExecSchemaSQL(t, store, `
INSERT INTO hook_trusted_paths (id, target, path, is_current, created_at, updated_at)
VALUES ('htp_a', 'codex', '/usr/local/bin/loaf', 1, '2026-08-08T00:00:00Z', '2026-08-08T00:00:00Z')
`)
	err := execSchemaSQL(t, store, `
INSERT INTO hook_trusted_paths (id, target, path, is_current, created_at, updated_at)
VALUES ('htp_b', 'codex', '/opt/loaf/bin/loaf', 1, '2026-08-08T00:00:01Z', '2026-08-08T00:00:01Z')
`)
	// The driver reports the violated column set, not the index name. A
	// UNIQUE failure on target alone can only come from the partial index
	// idx_hook_trusted_paths_current; the table-level constraint would name
	// hook_trusted_paths.path as well.
	if err == nil || !strings.Contains(err.Error(), "UNIQUE constraint failed: hook_trusted_paths.target") || strings.Contains(err.Error(), "hook_trusted_paths.path") {
		t.Fatalf("second current insert error = %v, want UNIQUE violation from idx_hook_trusted_paths_current", err)
	}

	// A non-current row for the same target and a current row for another
	// target remain allowed.
	mustExecSchemaSQL(t, store, `
INSERT INTO hook_trusted_paths (id, target, path, is_current, created_at, updated_at)
VALUES ('htp_c', 'codex', '/opt/loaf/bin/loaf', 0, '2026-08-08T00:00:02Z', '2026-08-08T00:00:02Z')
`)
	mustExecSchemaSQL(t, store, `
INSERT INTO hook_trusted_paths (id, target, path, is_current, created_at, updated_at)
VALUES ('htp_d', 'cursor', '/usr/local/bin/loaf', 1, '2026-08-08T00:00:03Z', '2026-08-08T00:00:03Z')
`)
}

func TestHookEnablementOpaqueIDsAreNotDerivedFromNaturalKey(t *testing.T) {
	store := hookEnablementFixture(t)
	ctx := context.Background()

	a, err := store.SetHookEnablement(ctx, "codex", "SessionStart", "session-start-loaf", false)
	if err != nil {
		t.Fatalf("SetHookEnablement a error = %v", err)
	}
	b, err := store.SetHookEnablement(ctx, "cursor", "SessionStart", "session-start-loaf", false)
	if err != nil {
		t.Fatalf("SetHookEnablement b error = %v", err)
	}
	if a.ID == b.ID {
		t.Fatalf("distinct natural keys produced identical IDs %q", a.ID)
	}
	if !strings.HasPrefix(a.ID, "hen_") || !strings.HasPrefix(b.ID, "hen_") {
		t.Fatalf("IDs = %q, %q; want hen_ prefix", a.ID, b.ID)
	}
	if strings.Contains(a.ID, "codex") || strings.Contains(a.ID, "session-start-loaf") {
		t.Fatalf("ID appears derived from natural key: %q", a.ID)
	}
}
