package vnextcontinuitysqlitecontract

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/levifig/loaf/vnext/continuity"
	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
)

const sqliteSourceRoot = "../../vnext/continuity/sqlite"

func TestContinuitySQLiteContractOracleIsTestOnlyAndImportClosed(t *testing.T) {
	t.Parallel()

	allowedNonStandard := map[string]struct{}{
		"github.com/levifig/loaf/vnext/continuity":        {},
		"github.com/levifig/loaf/vnext/continuity/sqlite": {},
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read contract package: %v", err)
	}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			t.Errorf("%s is a symlink", entry.Name())
			continue
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		if !strings.HasSuffix(entry.Name(), "_test.go") {
			t.Errorf("%s is production code; the oracle must remain test-only", entry.Name())
		}
		file := parseGoFile(t, entry.Name())
		for _, specification := range file.Imports {
			importPath, err := strconv.Unquote(specification.Path.Value)
			if err != nil {
				t.Errorf("unquote import in %s: %v", entry.Name(), err)
				continue
			}
			if _, allowed := allowedNonStandard[importPath]; allowed {
				continue
			}
			if strings.Contains(strings.SplitN(importPath, "/", 2)[0], ".") {
				t.Errorf("%s imports unadmitted non-standard package %q", entry.Name(), importPath)
			}
		}
	}
}

func TestContinuitySQLiteContractHasExactSourceAndAPI(t *testing.T) {
	t.Parallel()

	wantFiles := []string{
		"admission.go",
		"append_kernel.go",
		"append_methods.go",
		"authority.go",
		"codec_v1.go",
		"context_v1.go",
		"driver.go",
		"filesystem_attributes_windows.go",
		"filesystem_unix.go",
		"filesystem_windows.go",
		"prune.go",
		"read.go",
		"schema.go",
		"snapshot_fold_v1.go",
		"snapshot_records_v1.go",
		"snapshot_references_v1.go",
		"snapshot_scratchpad_v1.go",
		"store.go",
		"sync.go",
		"terminal_candidate.go",
		"terminal_candidate_codec.go",
		"terminal_candidate_promotion.go",
		"terminal_history.go",
		"wire_v1.go",
		"wire_validation_v1.go",
	}
	wantExports := []string{
		"OpaqueSyncFrame",
		"OpaqueSyncFrame.ArrivalSequence",
		"OpaqueSyncFrame.EnvelopeDigest",
		"OpaqueSyncFrame.PrunedArrival",
		"OpaqueSyncFrame.Quarantined",
		"OpaqueSyncFrame.SealedEnvelope",
		"Open",
		"SealedOutboxFrame",
		"SealedOutboxFrame.CertificateID",
		"SealedOutboxFrame.EnvelopeDigest",
		"SealedOutboxFrame.FactID",
		"SealedOutboxFrame.KeyGeneration",
		"SealedOutboxFrame.Nonce",
		"SealedOutboxFrame.PreviousEnvelopeDigest",
		"SealedOutboxFrame.SealedEnvelope",
		"Store",
		"Store.ActivateStagedSync",
		"Store.ApplySyncBatch",
		"Store.ApplyVerifiedPrune",
		"Store.ArchiveIdea",
		"Store.AttachExternalReference",
		"Store.CaptureSpark",
		"Store.Close",
		"Store.CloseScratchpad",
		"Store.CorrectFinding",
		"Store.CorrectJournalEntry",
		"Store.CreateIdea",
		"Store.CurrentSyncAuthority",
		"Store.CurrentSyncProgress",
		"Store.CurrentTerminalCandidate",
		"Store.DeriveContext",
		"Store.DetachExternalReference",
		"Store.DiscardStagedSync",
		"Store.DiscardTerminalCandidate",
		"Store.DismissSpark",
		"Store.ExportFact",
		"Store.InstallVerifiedSyncAuthority",
		"Store.IntroduceScratchpadParticipant",
		"Store.NextUnsealedLocalFact",
		"Store.OpenDecision",
		"Store.OpenScratchpad",
		"Store.PendingSealedOutbox",
		"Store.PendingSyncFrames",
		"Store.PendingSyncFramesAfter",
		"Store.PersistSealedOutbox",
		"Store.PromoteIdeaToExternalReference",
		"Store.PromoteSparkToIdea",
		"Store.PromoteTerminalCandidate",
		"Store.RecordCheckpoint",
		"Store.RecordFinding",
		"Store.RecordHandoff",
		"Store.RecordJournalEntry",
		"Store.RecordScratchpadClaim",
		"Store.RecordScratchpadMessage",
		"Store.RecordVerificationEvidence",
		"Store.RecordWrap",
		"Store.RegisterExternalReference",
		"Store.RegisterProject",
		"Store.ReleaseScratchpadClaim",
		"Store.ResolveDecision",
		"Store.ResolveIdea",
		"Store.RetractFinding",
		"Store.ReviseIdea",
		"Store.ReviseProjectLabel",
		"Store.Snapshot",
		"Store.StageSyncPage",
		"Store.StageVerifiedTerminalCandidateChunk",
		"Store.StartExploration",
		"Store.SupersedeDecision",
		"SyncActivationAttached",
		"SyncActivationStaging",
		"SyncActivationState",
		"SyncAuthority",
		"SyncAuthority.AdminPublicKey",
		"SyncAuthority.ChannelID",
		"SyncAuthority.Environments",
		"SyncAuthority.InventoryArrivalHead",
		"SyncAuthority.MembershipGeneration",
		"SyncAuthority.RelayGeneration",
		"SyncChannelID",
		"SyncEnvironmentCertificate",
		"SyncEnvironmentCertificate.CertificateBytes",
		"SyncEnvironmentCertificate.CertificateID",
		"SyncEnvironmentCertificate.EnvironmentID",
		"SyncEnvironmentCertificate.ExpiresAtMillis",
		"SyncEnvironmentCertificate.JoinMembershipGeneration",
		"SyncEnvironmentCertificate.Mode",
		"SyncEnvironmentCertificate.Retirement",
		"SyncEnvironmentEphemeral",
		"SyncEnvironmentMode",
		"SyncEnvironmentRetirement",
		"SyncEnvironmentRetirement.FinalEnvelopeDigest",
		"SyncEnvironmentRetirement.FinalEnvironmentSequence",
		"SyncEnvironmentRetirement.MembershipGeneration",
		"SyncEnvironmentRetirement.RelayGeneration",
		"SyncEnvironmentRetirement.RetirementBytes",
		"SyncEnvironmentRetirement.RetirementID",
		"SyncEnvironmentTrusted",
		"SyncError",
		"SyncError.Code",
		"SyncError.Detail",
		"SyncError.Error",
		"SyncError.Field",
		"SyncErrorActivation",
		"SyncErrorArrivalGap",
		"SyncErrorCandidate",
		"SyncErrorCertificate",
		"SyncErrorCode",
		"SyncErrorConflict",
		"SyncErrorCursor",
		"SyncErrorEnvelopeChain",
		"SyncErrorEnvironmentGap",
		"SyncErrorHLC",
		"SyncErrorInvalid",
		"SyncErrorNonceReuse",
		"SyncErrorNotAttached",
		"SyncErrorNotFound",
		"SyncErrorRecoveryRequired",
		"SyncErrorStore",
		"SyncErrorTerminalHistoryRequired",
		"SyncProgress",
		"SyncProgress.ActivationState",
		"SyncProgress.AppliedCursor",
		"SyncProgress.ChannelID",
		"SyncProgress.DownloadedCursor",
		"SyncProgress.ProjectID",
		"SyncProgress.RelayHead",
		"TerminalCandidate",
		"TerminalCandidate.AuthorityDigest",
		"TerminalCandidate.CandidateID",
		"TerminalCandidate.ChannelID",
		"TerminalCandidate.FrameCount",
		"TerminalCandidate.MembershipGeneration",
		"TerminalCandidate.ProjectID",
		"TerminalCandidate.RelayGeneration",
		"TerminalCandidate.RollingCandidateDigest",
		"TerminalCandidate.StartArrivalSequence",
		"TerminalCandidate.ThroughArrivalSequence",
		"TerminalCandidateCheckpoint",
		"TerminalCandidateCheckpoint.CandidateID",
		"TerminalCandidateCheckpoint.FrameCount",
		"TerminalCandidateCheckpoint.RollingCandidateDigest",
		"TerminalCandidateCheckpoint.ThroughArrivalSequence",
		"TerminalCandidateReceipt",
		"TerminalCandidateReceipt.AuthorityDigest",
		"TerminalCandidateReceipt.CandidateID",
		"TerminalCandidateReceipt.ChannelID",
		"TerminalCandidateReceipt.FrameCount",
		"TerminalCandidateReceipt.MembershipGeneration",
		"TerminalCandidateReceipt.PostPromotionCorpusDigest",
		"TerminalCandidateReceipt.ProjectID",
		"TerminalCandidateReceipt.RelayGeneration",
		"TerminalCandidateReceipt.ResultingAppliedCursor",
		"TerminalCandidateReceipt.RollingCandidateDigest",
		"TerminalCandidateReceipt.StartArrivalSequence",
		"TerminalCandidateReceipt.ThroughArrivalSequence",
		"UnsealedLocalFact",
		"UnsealedLocalFact.Fact",
		"UnsealedLocalFact.PreviousEnvelopeDigest",
		"VerifiedPrunePlan",
		"VerifiedPrunePlan.BarrierArrivalSequence",
		"VerifiedPrunePlan.ChannelID",
		"VerifiedPrunePlan.Closure",
		"VerifiedPrunePlan.MembershipGeneration",
		"VerifiedPrunePlan.PruneCertificateID",
		"VerifiedPrunePlan.Targets",
		"VerifiedPruneReference",
		"VerifiedPruneReference.ArrivalSequence",
		"VerifiedPruneReference.CertificateID",
		"VerifiedPruneReference.EnvelopeDigest",
		"VerifiedPruneReference.EnvironmentID",
		"VerifiedPruneReference.EnvironmentSequence",
		"VerifiedPruneReference.FactID",
		"VerifiedPruneReference.KeyGeneration",
		"VerifiedPruneReference.Nonce",
		"VerifiedPruneReference.PreviousEnvelopeDigest",
		"VerifiedSyncFrame",
		"VerifiedSyncFrame.ArrivalSequence",
		"VerifiedSyncFrame.CertificateID",
		"VerifiedSyncFrame.EnvelopeDigest",
		"VerifiedSyncFrame.Fact",
		"VerifiedSyncFrame.KeyGeneration",
		"VerifiedSyncFrame.Nonce",
		"VerifiedSyncFrame.PreviousEnvelopeDigest",
		"VerifiedTerminalCandidateFrame",
		"VerifiedTerminalCandidateFrame.Inbox",
		"VerifiedTerminalCandidateFrame.Pruned",
		"VerifiedTerminalCandidateFrame.Sealed",
		"VerifiedTerminalPrunedFrame",
		"VerifiedTerminalPrunedFrame.FactKind",
		"VerifiedTerminalPrunedFrame.HLC",
		"VerifiedTerminalPrunedFrame.PruneCertificateID",
		"VerifiedTerminalPrunedFrame.Reference",
	}
	files, exports := inspectSQLiteSource(t)
	if !reflect.DeepEqual(files, wantFiles) {
		t.Fatalf("SQLite production source inventory = %v, want %v", files, wantFiles)
	}
	if !reflect.DeepEqual(exports, wantExports) {
		t.Fatalf("SQLite exported API = %v, want %v", exports, wantExports)
	}
}

func TestContinuitySQLiteContractPinsDriverBoundary(t *testing.T) {
	t.Parallel()

	wantImports := map[string][]string{
		"admission.go": {
			"context",
			"database/sql",
			"errors",
			"github.com/levifig/loaf/vnext/continuity",
		},
		"append_kernel.go": {
			"context",
			"database/sql",
			"errors",
			"github.com/levifig/loaf/vnext/continuity",
			"math",
		},
		"append_methods.go": {
			"context",
			"github.com/levifig/loaf/vnext/continuity",
		},
		"authority.go": {
			"bytes",
			"context",
			"crypto/sha256",
			"database/sql",
			"errors",
			"fmt",
			"github.com/levifig/loaf/vnext/continuity",
			"math",
		},
		"codec_v1.go": {
			"bytes",
			"encoding/json",
			"github.com/levifig/loaf/vnext/continuity",
			"io",
			"strings",
		},
		"context_v1.go":                    {"context", "github.com/levifig/loaf/vnext/continuity"},
		"driver.go":                        {"_:github.com/ncruces/go-sqlite3/driver"},
		"filesystem_attributes_windows.go": {"fmt", "os", "syscall"},
		"filesystem_unix.go":               {"errors", "fmt", "os", "path/filepath", "strings"},
		"filesystem_windows.go":            {"errors", "fmt", "os", "path/filepath", "strings"},
		"prune.go": {
			"bytes",
			"context",
			"database/sql",
			"errors",
			"fmt",
			"github.com/levifig/loaf/vnext/continuity",
		},
		"read.go":                   {"context", "database/sql", "github.com/levifig/loaf/vnext/continuity"},
		"schema.go":                 {"context", "crypto/sha256", "database/sql", "encoding/hex", "fmt", "github.com/levifig/loaf/vnext/continuity", "strings"},
		"snapshot_fold_v1.go":       {"context", "github.com/levifig/loaf/vnext/continuity", "strings"},
		"snapshot_records_v1.go":    {"github.com/levifig/loaf/vnext/continuity", "sort"},
		"snapshot_references_v1.go": {"github.com/levifig/loaf/vnext/continuity", "sort", "strings"},
		"snapshot_scratchpad_v1.go": {"github.com/levifig/loaf/vnext/continuity", "sort", "strings"},
		"store.go": {
			"context",
			"database/sql",
			"errors",
			"fmt",
			"github.com/levifig/loaf/vnext/continuity",
			"net/url",
			"os",
			"path/filepath",
			"strings",
			"sync",
			"time",
		},
		"sync.go": {
			"bytes",
			"context",
			"crypto/sha256",
			"database/sql",
			"errors",
			"fmt",
			"github.com/levifig/loaf/vnext/continuity",
			"github.com/levifig/loaf/vnext/internal/continuitywire",
			"math",
			"sort",
		},
		"terminal_candidate.go": {
			"bytes",
			"context",
			"crypto/sha256",
			"database/sql",
			"errors",
			"github.com/levifig/loaf/vnext/continuity",
			"github.com/levifig/loaf/vnext/internal/continuitywire",
			"math",
		},
		"terminal_candidate_codec.go": {
			"bytes",
			"crypto/sha256",
			"encoding/binary",
			"errors",
			"github.com/levifig/loaf/vnext/continuity",
			"github.com/levifig/loaf/vnext/internal/continuitywire",
			"math",
		},
		"terminal_candidate_promotion.go": {
			"bytes",
			"context",
			"crypto/sha256",
			"database/sql",
			"encoding/binary",
			"errors",
			"github.com/levifig/loaf/vnext/continuity",
			"github.com/levifig/loaf/vnext/internal/continuitywire",
			"math",
			"sort",
		},
		"terminal_history.go": {
			"context",
			"database/sql",
			"github.com/levifig/loaf/vnext/continuity",
		},
		"wire_v1.go":            nil,
		"wire_validation_v1.go": {"github.com/levifig/loaf/vnext/continuity"},
	}

	for fileName, want := range wantImports {
		file := parseGoFile(t, filepath.Join(sqliteSourceRoot, fileName))
		var got []string
		for _, specification := range file.Imports {
			importPath, err := strconv.Unquote(specification.Path.Value)
			if err != nil {
				t.Fatalf("unquote import in %s: %v", fileName, err)
			}
			if specification.Name != nil {
				importPath = specification.Name.Name + ":" + importPath
			}
			got = append(got, importPath)
		}
		sort.Strings(got)
		sort.Strings(want)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s imports = %v, want %v", fileName, got, want)
		}
	}
}

func TestContinuitySQLiteContractRejectsRawAndExternalAuthoritySurfaces(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(sqliteSourceRoot)
	if err != nil {
		t.Fatalf("read SQLite source root: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		contents, err := os.ReadFile(filepath.Join(sqliteSourceRoot, entry.Name()))
		if err != nil {
			t.Errorf("read %s: %v", entry.Name(), err)
			continue
		}
		lower := strings.ToLower(string(contents))
		for _, forbidden := range []string{
			"json.rawmessage", " provider", " credential", " secret", " api_key", " oauth",
			" tracker", " linear", " jira", " assignment", " hierarchy", " dependency",
			"definition_of_done", "acceptance_criteria", "retry_queue", "retry queue", " endpoint",
		} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s contains forbidden external authority surface %q", entry.Name(), strings.TrimSpace(forbidden))
			}
		}
		upper := strings.Join(strings.Fields(strings.ToUpper(string(contents))), " ")
		for _, forbidden := range []string{"UPDATE CONTINUITY_FACTS", "DROP TABLE CONTINUITY_FACTS"} {
			if strings.Contains(upper, forbidden) {
				t.Errorf("%s contains forbidden canonical-fact mutation %q", entry.Name(), forbidden)
			}
		}
		if entry.Name() != "prune.go" && strings.Contains(upper, "DELETE FROM CONTINUITY_FACTS") {
			t.Errorf("%s contains forbidden canonical-fact mutation %q", entry.Name(), "DELETE FROM CONTINUITY_FACTS")
		}
	}
}

func TestContinuitySQLiteContractPinsExactSchema(t *testing.T) {
	t.Parallel()

	db := openContractDatabase(t)
	defer db.Close()

	var applicationID, userVersion int
	if err := db.QueryRow(`PRAGMA application_id`).Scan(&applicationID); err != nil {
		t.Fatalf("read application id: %v", err)
	}
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&userVersion); err != nil {
		t.Fatalf("read user version: %v", err)
	}
	if applicationID != 1280267825 || userVersion != 4 {
		t.Fatalf("schema pragmas = application_id %d, user_version %d", applicationID, userVersion)
	}

	var line, checksum string
	var version int
	if err := db.QueryRow(`SELECT schema_line, schema_version, schema_checksum FROM continuity_schema WHERE singleton = 1`).Scan(&line, &version, &checksum); err != nil {
		t.Fatalf("read schema identity: %v", err)
	}
	if line != "vnext" || version != 4 || checksum != "150cc2b8dfcaecda0fefcfbdff02aa924644984ae7bdef4480f884dc63fe95ca" {
		t.Fatalf("schema identity = %q, %d, %q", line, version, checksum)
	}

	wantObjects := []objectDigest{
		{kind: "index", name: "ix_continuity_facts_project_order", table: "continuity_facts", digest: "576f27dbbd45eb321a02382151c8caa7c3253d13bae84e780bbb8beb3e622c5b"},
		{kind: "index", name: "ix_continuity_facts_subject_order", table: "continuity_facts", digest: "4e442bf181f90b62b48372a200f3eb7ad1ac2863efc36f25d23db11e062d0ab9"},
		{kind: "index", name: "ux_continuity_project_identity", table: "continuity_facts", digest: "53c7715e0a375cff5f23cf76cc860b1df863e89050e451ffcfce720e67d28720"},
		{kind: "index", name: "ux_continuity_sync_authority_candidate_pages_final", table: "continuity_sync_authority_candidate_pages", digest: "fc66e9d7051940d8e042a891fcbf094c31c9869dc83a93a63f8dc5618359a340"},
		{kind: "index", name: "ux_continuity_sync_authority_candidates_active_project", table: "continuity_sync_authority_candidates", digest: "2e880d95fb29ab11f1ba2bafd5e17120b9942893d4d62f0fee7d2964db694967"},
		{kind: "index", name: "ux_continuity_sync_terminal_candidates_staging_project", table: "continuity_sync_terminal_candidates", digest: "b0aa78a63d19c8503b7df73fb9ddecb2c4bc15ef9789c3577d3ecc6eb5aec495"},
		{kind: "table", name: "continuity_facts", table: "continuity_facts", digest: "cc2165e3ec85a50478f7ee550bf4a25db6b3282b56de7b1332a007961c75555f"},
		{kind: "table", name: "continuity_schema", table: "continuity_schema", digest: "e9ab86f558e51912758b0f063195bc96deb43b9e7466ea8fadb40794fbdb505d"},
		{kind: "table", name: "continuity_sync_authorities", table: "continuity_sync_authorities", digest: "a8506a59db4d2cb96fa1ac8c30dbeb271c1e2b0dd5182370994a478865d35746"},
		{kind: "table", name: "continuity_sync_authority_candidate_environments", table: "continuity_sync_authority_candidate_environments", digest: "6f5f5d4d8f0a97657ec116b57ababf2f84e1588608e9447c671dca8b056caa89"},
		{kind: "table", name: "continuity_sync_authority_candidate_membership_events", table: "continuity_sync_authority_candidate_membership_events", digest: "9df1d2916a6fc809a41620f24e3e04edf74efcd7771b045d64ed39969c6a7c4e"},
		{kind: "table", name: "continuity_sync_authority_candidate_pages", table: "continuity_sync_authority_candidate_pages", digest: "6215f0f2eb502421980bde44b2aaed53bc23a14436eff1eec3b387274f70e629"},
		{kind: "table", name: "continuity_sync_authority_candidates", table: "continuity_sync_authority_candidates", digest: "6c5a0dbcc5817f6173ecfb178e8e4d03cfe9f86815c8f9a10b0ab74556a8e1f1"},
		{kind: "table", name: "continuity_sync_environment_certificates", table: "continuity_sync_environment_certificates", digest: "65ed5ac0d095a7eb155793bd043b3080e2822cb98ce32e3a1a7c44e6059e0856"},
		{kind: "table", name: "continuity_sync_environment_heads", table: "continuity_sync_environment_heads", digest: "542dc7124d2db185a983d6a997274f975870521524daaf82d940f824ae7b69e3"},
		{kind: "table", name: "continuity_sync_inbox", table: "continuity_sync_inbox", digest: "008be5c6e34b28d39c1a47d6be64f49b82c105ff4c02947e1ab5b94bf7046ab9"},
		{kind: "table", name: "continuity_sync_outbox", table: "continuity_sync_outbox", digest: "2702a3d8ce8bd29fc5556db6065e053b26f1263fb534ecb3dd22c94bdf5cd1e5"},
		{kind: "table", name: "continuity_sync_projects", table: "continuity_sync_projects", digest: "94841daff839eb50034602cd7aa59c5804152f2501a517a91a4ec81801f1fbb2"},
		{kind: "table", name: "continuity_sync_receipts", table: "continuity_sync_receipts", digest: "46460af061b8950c529da755f66f2d4d05fb24f0a1df5c95dee9f97ef6ebe35b"},
		{kind: "table", name: "continuity_sync_terminal_candidate_frames", table: "continuity_sync_terminal_candidate_frames", digest: "5306f8ef4814ab7f8be7cd65fba1c0d93d81caf41ce489d3dd9aa37e219fa52d"},
		{kind: "table", name: "continuity_sync_terminal_candidates", table: "continuity_sync_terminal_candidates", digest: "96c0e64f67ea269bf4d618dcc0fb45316be718e8f7af95ed995dedf0760608d3"},
		{kind: "table", name: "continuity_sync_tombstones", table: "continuity_sync_tombstones", digest: "f9cd6d9cfba708fbdfab1a5447cc7395762bcb0ec85c36d29cda2aea84a8fb4b"},
		{kind: "trigger", name: "continuity_facts_require_project_identity", table: "continuity_facts", digest: "2efe75c94d4bd6bdce39f25536d6856e61b681c127ddb60b11ae32420b9163da"},
	}
	if got := readObjectDigests(t, db); !reflect.DeepEqual(got, wantObjects) {
		t.Fatalf("schema objects = %#v, want %#v", got, wantObjects)
	}

	wantColumns := map[string][]columnSpec{
		"continuity_schema": {
			{name: "singleton", dataType: "INTEGER", notNull: 1, primaryKey: 1},
			{name: "schema_line", dataType: "TEXT", notNull: 1},
			{name: "schema_version", dataType: "INTEGER", notNull: 1},
			{name: "schema_checksum", dataType: "TEXT", notNull: 1},
		},
		"continuity_facts": {
			{name: "fact_id", dataType: "TEXT", notNull: 1, primaryKey: 1},
			{name: "project_id", dataType: "TEXT", notNull: 1},
			{name: "subject_kind", dataType: "TEXT", notNull: 1},
			{name: "subject_id", dataType: "TEXT", notNull: 1},
			{name: "fact_kind", dataType: "TEXT", notNull: 1},
			{name: "payload_version", dataType: "INTEGER", notNull: 1},
			{name: "content_json", dataType: "TEXT", notNull: 1},
			{name: "environment_id", dataType: "TEXT", notNull: 1},
			{name: "environment_sequence", dataType: "INTEGER", notNull: 1},
			{name: "hlc_wall_millis", dataType: "INTEGER", notNull: 1},
			{name: "hlc_logical", dataType: "INTEGER", notNull: 1},
			{name: "envelope_version", dataType: "INTEGER", notNull: 1},
		},
	}
	for tableName, want := range wantColumns {
		if got := readColumns(t, db, tableName); !reflect.DeepEqual(got, want) {
			t.Errorf("%s columns = %#v, want %#v", tableName, got, want)
		}
	}

	wantIndexes := []indexSpec{
		{name: "ix_continuity_facts_project_order", unique: 0, origin: "c", partial: 0, keyColumns: []string{"project_id", "hlc_wall_millis", "hlc_logical", "environment_id", "fact_id"}},
		{name: "ix_continuity_facts_subject_order", unique: 0, origin: "c", partial: 0, keyColumns: []string{"project_id", "subject_kind", "subject_id", "hlc_wall_millis", "hlc_logical", "environment_id", "fact_id"}},
		{name: "sqlite_autoindex_continuity_facts_1", unique: 1, origin: "pk", partial: 0, keyColumns: []string{"fact_id"}},
		{name: "sqlite_autoindex_continuity_facts_2", unique: 1, origin: "u", partial: 0, keyColumns: []string{"project_id", "environment_id", "environment_sequence"}},
		{name: "ux_continuity_project_identity", unique: 1, origin: "c", partial: 1, keyColumns: []string{"project_id"}},
	}
	if got := readIndexes(t, db); !reflect.DeepEqual(got, wantIndexes) {
		t.Fatalf("fact indexes = %#v, want %#v", got, wantIndexes)
	}

	for _, tableName := range []string{"continuity_schema", "continuity_facts"} {
		rows, err := db.Query(`PRAGMA foreign_key_list(` + tableName + `)`)
		if err != nil {
			t.Fatalf("read %s foreign keys: %v", tableName, err)
		}
		if rows.Next() {
			rows.Close()
			t.Fatalf("%s unexpectedly declares a foreign key", tableName)
		}
		rows.Close()
	}
}

func TestContinuitySQLiteContractPinsOpenSignature(t *testing.T) {
	t.Parallel()

	var open func(string, continuity.EnvironmentID) (*continuitysqlite.Store, error) = continuitysqlite.Open
	var closeable interface{ Close() error } = (*continuitysqlite.Store)(nil)
	if open == nil || closeable == nil {
		t.Fatal("continuity SQLite API is unexpectedly nil")
	}
}

func TestContinuitySQLiteContractPinsStoreRepresentation(t *testing.T) {
	t.Parallel()

	type fieldSpec struct {
		name      string
		typeName  string
		exported  bool
		anonymous bool
	}
	storeType := reflect.TypeOf(continuitysqlite.Store{})
	gotFields := make([]fieldSpec, 0, storeType.NumField())
	for index := 0; index < storeType.NumField(); index++ {
		field := storeType.Field(index)
		gotFields = append(gotFields, fieldSpec{
			name:      field.Name,
			typeName:  field.Type.String(),
			exported:  field.IsExported(),
			anonymous: field.Anonymous,
		})
	}
	wantFields := []fieldSpec{
		{name: "mu", typeName: "sync.RWMutex"},
		{name: "db", typeName: "*sql.DB"},
		{name: "environmentID", typeName: "continuity.EnvironmentID"},
		{name: "wallMillis", typeName: "func() int64"},
		{name: "closed", typeName: "bool"},
	}
	if !reflect.DeepEqual(gotFields, wantFields) {
		t.Fatalf("Store fields = %#v, want %#v", gotFields, wantFields)
	}

	pointerType := reflect.TypeOf((*continuitysqlite.Store)(nil))
	wantMethods := map[string]string{
		"ActivateStagedSync":                  "func(*sqlite.Store, context.Context, continuity.ProjectID, sqlite.SyncChannelID) (sqlite.SyncProgress, error)",
		"ApplySyncBatch":                      "func(*sqlite.Store, context.Context, continuity.ProjectID, []sqlite.VerifiedSyncFrame, int64, int64) (sqlite.SyncProgress, error)",
		"ApplyVerifiedPrune":                  "func(*sqlite.Store, context.Context, continuity.ProjectID, sqlite.VerifiedPrunePlan) error",
		"ArchiveIdea":                         "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.IdeaArchivePayload) (continuity.AppendReceipt, error)",
		"AttachExternalReference":             "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.ExternalReferenceAttachmentPayload) (continuity.AppendReceipt, error)",
		"CaptureSpark":                        "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.SparkCapturedPayload) (continuity.AppendReceipt, error)",
		"Close":                               "func(*sqlite.Store) error",
		"CloseScratchpad":                     "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.ScratchpadClosePayload) (continuity.AppendReceipt, error)",
		"CorrectFinding":                      "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.FindingCorrectionPayload) (continuity.AppendReceipt, error)",
		"CorrectJournalEntry":                 "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.JournalCorrectionPayload) (continuity.AppendReceipt, error)",
		"CreateIdea":                          "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.IdeaCreatedPayload) (continuity.AppendReceipt, error)",
		"CurrentSyncAuthority":                "func(*sqlite.Store, context.Context, continuity.ProjectID) (sqlite.SyncAuthority, error)",
		"CurrentSyncProgress":                 "func(*sqlite.Store, context.Context, continuity.ProjectID) (sqlite.SyncProgress, error)",
		"CurrentTerminalCandidate":            "func(*sqlite.Store, context.Context, continuity.ProjectID) (sqlite.TerminalCandidate, bool, error)",
		"DeriveContext":                       "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.ContextRequest) (continuity.ContextDigest, error)",
		"DetachExternalReference":             "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.ExternalReferenceDetachmentPayload) (continuity.AppendReceipt, error)",
		"DiscardStagedSync":                   "func(*sqlite.Store, context.Context, continuity.ProjectID, sqlite.SyncChannelID) error",
		"DiscardTerminalCandidate":            "func(*sqlite.Store, context.Context, continuity.ProjectID, sqlite.TerminalCandidateCheckpoint) error",
		"DismissSpark":                        "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.SparkDismissedPayload) (continuity.AppendReceipt, error)",
		"ExportFact":                          "func(*sqlite.Store, context.Context, continuity.FactID) (continuitywire.Fact, error)",
		"InstallVerifiedSyncAuthority":        "func(*sqlite.Store, context.Context, continuity.ProjectID, sqlite.SyncAuthority) (sqlite.SyncProgress, error)",
		"IntroduceScratchpadParticipant":      "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.ScratchpadParticipantPayload) (continuity.AppendReceipt, error)",
		"NextUnsealedLocalFact":               "func(*sqlite.Store, context.Context, continuity.ProjectID) (sqlite.UnsealedLocalFact, bool, error)",
		"OpenDecision":                        "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.DecisionOpenedPayload) (continuity.AppendReceipt, error)",
		"OpenScratchpad":                      "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.ScratchpadOpenedPayload) (continuity.AppendReceipt, error)",
		"PendingSealedOutbox":                 "func(*sqlite.Store, context.Context, continuity.ProjectID, int) ([]sqlite.SealedOutboxFrame, error)",
		"PendingSyncFrames":                   "func(*sqlite.Store, context.Context, continuity.ProjectID, int) ([]sqlite.OpaqueSyncFrame, error)",
		"PendingSyncFramesAfter":              "func(*sqlite.Store, context.Context, continuity.ProjectID, int64, int) ([]sqlite.OpaqueSyncFrame, error)",
		"PersistSealedOutbox":                 "func(*sqlite.Store, context.Context, continuity.ProjectID, sqlite.SyncChannelID, sqlite.SealedOutboxFrame) error",
		"PromoteIdeaToExternalReference":      "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.IdeaPromotionPayload) (continuity.AppendReceipt, error)",
		"PromoteSparkToIdea":                  "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.SparkPromotionPayload) (continuity.AppendReceipt, error)",
		"PromoteTerminalCandidate":            "func(*sqlite.Store, context.Context, continuity.ProjectID, sqlite.TerminalCandidateCheckpoint) (sqlite.TerminalCandidateReceipt, error)",
		"RecordCheckpoint":                    "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.CheckpointRecordedPayload) (continuity.AppendReceipt, error)",
		"RecordFinding":                       "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.FindingRecordedPayload) (continuity.AppendReceipt, error)",
		"RecordHandoff":                       "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.HandoffRecordedPayload) (continuity.AppendReceipt, error)",
		"RecordJournalEntry":                  "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.JournalRecordedPayload) (continuity.AppendReceipt, error)",
		"RecordScratchpadClaim":               "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.ScratchpadClaimPayload) (continuity.AppendReceipt, error)",
		"RecordScratchpadMessage":             "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.ScratchpadMessagePayload) (continuity.AppendReceipt, error)",
		"RecordVerificationEvidence":          "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.VerificationEvidencePayload) (continuity.AppendReceipt, error)",
		"RecordWrap":                          "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.WrapRecordedPayload) (continuity.AppendReceipt, error)",
		"RegisterExternalReference":           "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.ExternalReferenceRegistrationPayload) (continuity.AppendReceipt, error)",
		"RegisterProject":                     "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.ProjectRegistrationPayload) (continuity.AppendReceipt, error)",
		"ReleaseScratchpadClaim":              "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.ScratchpadClaimReleasePayload) (continuity.AppendReceipt, error)",
		"ResolveDecision":                     "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.DecisionResolutionPayload) (continuity.AppendReceipt, error)",
		"ResolveIdea":                         "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.IdeaResolutionPayload) (continuity.AppendReceipt, error)",
		"RetractFinding":                      "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.FindingRetractionPayload) (continuity.AppendReceipt, error)",
		"ReviseIdea":                          "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.IdeaRevisionPayload) (continuity.AppendReceipt, error)",
		"ReviseProjectLabel":                  "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.ProjectLabelRevisionPayload) (continuity.AppendReceipt, error)",
		"Snapshot":                            "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.SnapshotRequest) (continuity.Snapshot, error)",
		"StageSyncPage":                       "func(*sqlite.Store, context.Context, continuity.ProjectID, sqlite.SyncChannelID, int64, int64, []sqlite.OpaqueSyncFrame) (sqlite.SyncProgress, error)",
		"StageVerifiedTerminalCandidateChunk": "func(*sqlite.Store, context.Context, continuity.ProjectID, sqlite.SyncAuthority, []sqlite.VerifiedTerminalCandidateFrame, int64, int64) (sqlite.TerminalCandidate, error)",
		"StartExploration":                    "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.ExplorationStartedPayload) (continuity.AppendReceipt, error)",
		"SupersedeDecision":                   "func(*sqlite.Store, context.Context, continuity.ProjectID, continuity.FactID, continuity.SubjectID, continuity.DecisionSupersessionPayload) (continuity.AppendReceipt, error)",
	}
	gotMethods := make(map[string]string, pointerType.NumMethod())
	for index := 0; index < pointerType.NumMethod(); index++ {
		method := pointerType.Method(index)
		gotMethods[method.Name] = method.Type.String()
	}
	if !reflect.DeepEqual(gotMethods, wantMethods) {
		t.Fatalf("*Store methods = %#v, want %#v", gotMethods, wantMethods)
	}
}

func TestContinuitySQLiteContractPinsSnapshotReadBoundary(t *testing.T) {
	t.Parallel()

	contents, err := os.ReadFile(filepath.Join(sqliteSourceRoot, "read.go"))
	if err != nil {
		t.Fatalf("read Snapshot implementation: %v", err)
	}
	normalized := strings.Join(strings.Fields(string(contents)), " ")
	for _, required := range []string{
		"return store.snapshotV1(ctx, projectID, request.AtMillis)",
		"store.loadSnapshotFactsV1(ctx, projectID)",
		"foldProjectSnapshotV1(ctx, projectID, atMillis, facts)",
		"BeginTx(ctx, &sql.TxOptions{ReadOnly: true})",
		"WHERE project_id = ? ORDER BY hlc_wall_millis ASC, hlc_logical ASC, environment_id COLLATE BINARY ASC, fact_id COLLATE BINARY ASC",
	} {
		if !strings.Contains(normalized, required) {
			t.Errorf("read.go does not contain pinned Snapshot boundary %q", required)
		}
	}
	for _, forbidden := range []string{"Isolation:", "time.Now", "ExecContext(", "tx.Exec(", "store.db.Exec("} {
		if strings.Contains(normalized, forbidden) {
			t.Errorf("read.go contains forbidden Snapshot read behavior %q", forbidden)
		}
	}
}

func TestContinuitySQLiteContractPinsContextReadBoundaryAndCaps(t *testing.T) {
	t.Parallel()

	readContents, err := os.ReadFile(filepath.Join(sqliteSourceRoot, "read.go"))
	if err != nil {
		t.Fatalf("read context boundary: %v", err)
	}
	readNormalized := strings.Join(strings.Fields(string(readContents)), " ")
	for _, required := range []string{
		"return store.deriveContextV1(ctx, projectID, request)",
		"store.loadSnapshotFactsV1(ctx, projectID)",
		"foldProjectSnapshotV1(ctx, projectID, request.AtMillis, facts)",
		"resolveContextFocusRelationsV1(ctx, facts, request.Focus)",
		"deriveContextDigestV1(ctx, snapshot, request, relations)",
	} {
		if !strings.Contains(readNormalized, required) {
			t.Errorf("read.go does not contain pinned context boundary %q", required)
		}
	}

	readFile := parseGoFile(t, filepath.Join(sqliteSourceRoot, "read.go"))
	var deriveContext *ast.FuncDecl
	for _, declaration := range readFile.Decls {
		candidate, ok := declaration.(*ast.FuncDecl)
		if !ok || candidate.Name.Name != "deriveContextV1" || candidate.Recv == nil || len(candidate.Recv.List) != 1 || receiverName(candidate.Recv.List[0].Type) != "Store" {
			continue
		}
		if deriveContext != nil {
			t.Fatal("read.go has multiple Store.deriveContextV1 methods")
		}
		deriveContext = candidate
	}
	if deriveContext == nil || deriveContext.Body == nil {
		t.Fatal("read.go does not define Store.deriveContextV1")
	}
	deriveBody := strings.Join(strings.Fields(compactNode(t, deriveContext.Body)), " ")
	contextCalls := []string{
		"store.loadSnapshotFactsV1(ctx, projectID)",
		"foldProjectSnapshotV1(ctx, projectID, request.AtMillis, facts)",
		"resolveContextFocusRelationsV1(ctx, facts, request.Focus)",
		"deriveContextDigestV1(ctx, snapshot, request, relations)",
	}
	callPositions := make([]int, len(contextCalls))
	for index, call := range contextCalls {
		if count := strings.Count(deriveBody, call); count != 1 {
			t.Errorf("Store.deriveContextV1 contains %d calls to %q, want exactly one", count, call)
		}
		callPositions[index] = strings.Index(deriveBody, call)
		if index > 0 && callPositions[index-1] >= callPositions[index] {
			t.Errorf("Store.deriveContextV1 call order = %v, want load, fold, relations, digest", callPositions)
		}
	}
	for _, forbidden := range []string{"store.db", "BeginTx(", "QueryContext(", "QueryRowContext(", "ExecContext("} {
		if strings.Contains(deriveBody, forbidden) {
			t.Errorf("Store.deriveContextV1 contains forbidden direct persistence behavior %q", forbidden)
		}
	}
	if callPositions[0] >= 0 {
		afterMaterialization := deriveBody[callPositions[0]+len(contextCalls[0]):]
		if strings.Contains(afterMaterialization, "store.") {
			t.Error("Store.deriveContextV1 accesses Store after its one snapshot materialization")
		}
	}

	contextContents, err := os.ReadFile(filepath.Join(sqliteSourceRoot, "context_v1.go"))
	if err != nil {
		t.Fatalf("read context implementation: %v", err)
	}
	contextNormalized := strings.Join(strings.Fields(string(contextContents)), " ")
	for _, required := range []string{
		"contextFocusedJournalLimitV1 = 1",
		"contextProjectJournalLimitV1 = 10",
		"contextWrapLimitV1 = 2",
		"contextSparkLimitV1 = 10",
		"contextIdeaLimitV1 = 10",
		"contextDecisionLimitV1 = 10",
		"contextCheckpointLimitV1 = 1",
		"contextFindingLimitV1 = 10",
		"contextHandoffLimitV1 = 1",
		"contextExternalReferenceLimitV1 = 10",
		"contextVerificationEvidenceLimitV1 = 10",
		"entry.HeadObservation.Branch == request.Branch",
		"finding.Content.Scope == request.Scope",
		"optionalSubjectEqualV1(wrap.Focus, focus)",
		"optionalSubjectEqualV1(handoff.Focus, focus)",
	} {
		if !strings.Contains(contextNormalized, required) {
			t.Errorf("context_v1.go does not contain pinned context semantic %q", required)
		}
	}
	for _, forbidden := range []string{"database/sql", "time.Now", "BeginTx(", "QueryContext(", "ExecContext(", "store.db", "loadSnapshotFactsV1("} {
		if strings.Contains(contextNormalized, forbidden) {
			t.Errorf("context_v1.go contains forbidden context behavior %q", forbidden)
		}
	}
}

func TestContinuitySQLiteContractSnapshotQueryUsesProjectOrderIndex(t *testing.T) {
	t.Parallel()

	db := openContractDatabase(t)
	defer db.Close()
	rows, err := db.Query(`
EXPLAIN QUERY PLAN
SELECT
  fact_id,
  project_id,
  subject_kind,
  subject_id,
  fact_kind,
  payload_version,
  content_json,
  environment_id,
  environment_sequence,
  hlc_wall_millis,
  hlc_logical,
  envelope_version
FROM continuity_facts
WHERE project_id = ?
ORDER BY
  hlc_wall_millis ASC,
  hlc_logical ASC,
  environment_id COLLATE BINARY ASC,
  fact_id COLLATE BINARY ASC`, "project-plan")
	if err != nil {
		t.Fatalf("explain Snapshot query: %v", err)
	}
	defer rows.Close()
	var details []string
	for rows.Next() {
		var id, parent, notUsed int
		var detail string
		if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
			t.Fatalf("scan Snapshot query plan: %v", err)
		}
		details = append(details, detail)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("read Snapshot query plan: %v", err)
	}
	joined := strings.Join(details, " | ")
	if !strings.Contains(joined, "ix_continuity_facts_project_order") {
		t.Fatalf("Snapshot query plan = %q, want project-order index", joined)
	}
	if strings.Contains(strings.ToUpper(joined), "TEMP B-TREE") {
		t.Fatalf("Snapshot query plan uses a temporary sort: %q", joined)
	}
}

func TestContinuitySQLiteContractPinsV1WireShapesAndClosedUnion(t *testing.T) {
	t.Parallel()

	wantShapes := map[string][]string{
		"wireCheckpointItemV1":                {"Kind:string:`json:\"kind\"`", "Text:string:`json:\"text\"`"},
		"wireCheckpointRecordedV1":            {"Observation:wireObservationV1:`json:\"observation\"`", "ExplorationID:string:`json:\"exploration_id\"`", "CurrentFraming:string:`json:\"current_framing\"`", "Conclusions:string:`json:\"conclusions\"`", "UnresolvedQuestion:string:`json:\"unresolved_question\"`", "NextAction:string:`json:\"next_action\"`", "Items:[]wireCheckpointItemV1:`json:\"items\"`"},
		"wireDecisionOpenedV1":                {"Observation:wireObservationV1:`json:\"observation\"`", "Scope:string:`json:\"scope\"`", "Question:string:`json:\"question\"`", "Context:string:`json:\"context\"`"},
		"wireDecisionResolutionV1":            {"Observation:wireObservationV1:`json:\"observation\"`", "Predecessor:string:`json:\"predecessor\"`", "Resolution:string:`json:\"resolution\"`", "Rationale:string:`json:\"rationale\"`"},
		"wireDecisionSupersessionV1":          {"Observation:wireObservationV1:`json:\"observation\"`", "Predecessor:string:`json:\"predecessor\"`", "SuccessorID:string:`json:\"successor_id\"`", "Rationale:string:`json:\"rationale\"`"},
		"wireExplorationStartedV1":            {"Observation:wireObservationV1:`json:\"observation\"`", "Label:string:`json:\"label\"`", "Purpose:string:`json:\"purpose\"`"},
		"wireExternalReferenceAttachmentV1":   {"Observation:wireObservationV1:`json:\"observation\"`", "Target:wireSubjectRefV1:`json:\"target\"`", "Predecessor:string:`json:\"predecessor\"`"},
		"wireExternalReferenceDetachmentV1":   {"Observation:wireObservationV1:`json:\"observation\"`", "Target:wireSubjectRefV1:`json:\"target\"`", "Predecessor:string:`json:\"predecessor\"`", "Reason:string:`json:\"reason\"`"},
		"wireExternalReferenceRegistrationV1": {"Observation:wireObservationV1:`json:\"observation\"`", "Locator:string:`json:\"locator\"`"},
		"wireFindingContentV1":                {"Scope:string:`json:\"scope\"`", "Summary:string:`json:\"summary\"`", "Detail:string:`json:\"detail\"`", "Recommendation:string:`json:\"recommendation\"`"},
		"wireFindingCorrectionV1":             {"Observation:wireObservationV1:`json:\"observation\"`", "Corrects:string:`json:\"corrects\"`", "Content:wireFindingContentV1:`json:\"content\"`"},
		"wireFindingRecordedV1":               {"Observation:wireObservationV1:`json:\"observation\"`", "Content:wireFindingContentV1:`json:\"content\"`"},
		"wireFindingRetractionV1":             {"Observation:wireObservationV1:`json:\"observation\"`", "Predecessor:string:`json:\"predecessor\"`", "Reason:string:`json:\"reason\"`"},
		"wireHandoffRecordedV1":               {"Observation:wireObservationV1:`json:\"observation\"`", "Focus:*wireSubjectRefV1:`json:\"focus\"`", "Purpose:string:`json:\"purpose\"`", "Situation:string:`json:\"situation\"`", "NextActions:string:`json:\"next_actions\"`", "QuestionsAndRisks:string:`json:\"questions_and_risks\"`", "SuggestedSkills:[]string:`json:\"suggested_skills\"`"},
		"wireIdeaArchiveV1":                   {"Observation:wireObservationV1:`json:\"observation\"`", "Predecessor:string:`json:\"predecessor\"`", "Reason:string:`json:\"reason\"`"},
		"wireIdeaContentV1":                   {"Label:string:`json:\"label\"`", "Text:string:`json:\"text\"`"},
		"wireIdeaCreatedV1":                   {"Observation:wireObservationV1:`json:\"observation\"`", "Content:wireIdeaContentV1:`json:\"content\"`"},
		"wireIdeaPromotionV1":                 {"Observation:wireObservationV1:`json:\"observation\"`", "Predecessor:string:`json:\"predecessor\"`", "ReferenceID:string:`json:\"reference_id\"`"},
		"wireIdeaResolutionV1":                {"Observation:wireObservationV1:`json:\"observation\"`", "Predecessor:string:`json:\"predecessor\"`", "Resolution:string:`json:\"resolution\"`"},
		"wireIdeaRevisionV1":                  {"Observation:wireObservationV1:`json:\"observation\"`", "Revises:string:`json:\"revises\"`", "Content:wireIdeaContentV1:`json:\"content\"`"},
		"wireJournalContentV1":                {"Category:string:`json:\"category\"`", "Scope:string:`json:\"scope\"`", "Text:string:`json:\"text\"`"},
		"wireJournalCorrectionV1":             {"Observation:wireObservationV1:`json:\"observation\"`", "Corrects:string:`json:\"corrects\"`", "Content:wireJournalContentV1:`json:\"content\"`"},
		"wireJournalRecordedV1":               {"Observation:wireObservationV1:`json:\"observation\"`", "Content:wireJournalContentV1:`json:\"content\"`"},
		"wireObservationV1":                   {"ObservedAtMillis:int64:`json:\"observed_at_millis\"`", "HarnessSessionID:string:`json:\"harness_session_id\"`", "Branch:string:`json:\"branch\"`", "Worktree:string:`json:\"worktree\"`"},
		"wireProjectLabelRevisionV1":          {"Observation:wireObservationV1:`json:\"observation\"`", "Revises:string:`json:\"revises\"`", "Label:string:`json:\"label\"`"},
		"wireProjectRegistrationV1":           {"Observation:wireObservationV1:`json:\"observation\"`", "Label:string:`json:\"label\"`"},
		"wireScratchpadClaimReleaseV1":        {"Observation:wireObservationV1:`json:\"observation\"`", "ClaimID:string:`json:\"claim_id\"`", "ReleasedBy:string:`json:\"released_by\"`", "Reason:string:`json:\"reason\"`"},
		"wireScratchpadClaimV1":               {"Observation:wireObservationV1:`json:\"observation\"`", "ClaimID:string:`json:\"claim_id\"`", "ParticipantID:string:`json:\"participant_id\"`", "Resource:string:`json:\"resource\"`", "ExpiresAtMillis:int64:`json:\"expires_at_millis\"`"},
		"wireScratchpadCloseV1":               {"Observation:wireObservationV1:`json:\"observation\"`", "ClosedBy:string:`json:\"closed_by\"`", "Reason:string:`json:\"reason\"`"},
		"wireScratchpadMessageV1":             {"Observation:wireObservationV1:`json:\"observation\"`", "ParticipantID:string:`json:\"participant_id\"`", "Text:string:`json:\"text\"`"},
		"wireScratchpadOpenedV1":              {"Observation:wireObservationV1:`json:\"observation\"`", "Focus:*wireSubjectRefV1:`json:\"focus\"`", "Label:string:`json:\"label\"`"},
		"wireScratchpadParticipantV1":         {"Observation:wireObservationV1:`json:\"observation\"`", "ParticipantID:string:`json:\"participant_id\"`", "Name:string:`json:\"name\"`", "Focus:*wireSubjectRefV1:`json:\"focus\"`"},
		"wireSparkCapturedV1":                 {"Observation:wireObservationV1:`json:\"observation\"`", "Scope:string:`json:\"scope\"`", "Text:string:`json:\"text\"`"},
		"wireSparkDismissedV1":                {"Observation:wireObservationV1:`json:\"observation\"`", "Predecessor:string:`json:\"predecessor\"`", "Reason:string:`json:\"reason\"`"},
		"wireSparkPromotionV1":                {"Observation:wireObservationV1:`json:\"observation\"`", "Predecessor:string:`json:\"predecessor\"`", "IdeaID:string:`json:\"idea_id\"`"},
		"wireSubjectRefV1":                    {"Kind:string:`json:\"kind\"`", "ID:string:`json:\"id\"`"},
		"wireVerificationEvidenceV1":          {"Observation:wireObservationV1:`json:\"observation\"`", "Target:wireSubjectRefV1:`json:\"target\"`", "Check:string:`json:\"check\"`", "Method:string:`json:\"method\"`", "Outcome:string:`json:\"outcome\"`", "Detail:string:`json:\"detail\"`"},
		"wireWrapRecordedV1":                  {"Observation:wireObservationV1:`json:\"observation\"`", "Focus:*wireSubjectRefV1:`json:\"focus\"`", "Scope:string:`json:\"scope\"`", "Synthesis:string:`json:\"synthesis\"`"},
	}
	gotShapes := inspectPrivateStructShapes(t, filepath.Join(sqliteSourceRoot, "wire_v1.go"))
	if !reflect.DeepEqual(gotShapes, wantShapes) {
		t.Fatalf("V1 wire shapes = %#v, want %#v", gotShapes, wantShapes)
	}

	wantUnion := []string{
		"wireCheckpointRecordedV1",
		"wireDecisionOpenedV1",
		"wireDecisionResolutionV1",
		"wireDecisionSupersessionV1",
		"wireExplorationStartedV1",
		"wireExternalReferenceAttachmentV1",
		"wireExternalReferenceDetachmentV1",
		"wireExternalReferenceRegistrationV1",
		"wireFindingCorrectionV1",
		"wireFindingRecordedV1",
		"wireFindingRetractionV1",
		"wireHandoffRecordedV1",
		"wireIdeaArchiveV1",
		"wireIdeaCreatedV1",
		"wireIdeaPromotionV1",
		"wireIdeaResolutionV1",
		"wireIdeaRevisionV1",
		"wireJournalCorrectionV1",
		"wireJournalRecordedV1",
		"wireProjectLabelRevisionV1",
		"wireProjectRegistrationV1",
		"wireScratchpadClaimReleaseV1",
		"wireScratchpadClaimV1",
		"wireScratchpadCloseV1",
		"wireScratchpadMessageV1",
		"wireScratchpadOpenedV1",
		"wireScratchpadParticipantV1",
		"wireSparkCapturedV1",
		"wireSparkDismissedV1",
		"wireSparkPromotionV1",
		"wireVerificationEvidenceV1",
		"wireWrapRecordedV1",
	}
	gotUnion, methodSignature := inspectClosedWireUnion(t, filepath.Join(sqliteSourceRoot, "wire_v1.go"))
	if !reflect.DeepEqual(gotUnion, wantUnion) || methodSignature != "func() error" {
		t.Fatalf("V1 wire union = %v with %q, want %v with func() error", gotUnion, methodSignature, wantUnion)
	}
}

func TestContinuitySQLiteContractPinsEveryPublicMethodToOneV1EncoderAndFactKind(t *testing.T) {
	t.Parallel()

	want := map[string]methodBinding{
		"ArchiveIdea":                    {encoder: "encodeIdeaArchiveV1", record: "RecordIdea", fact: "FactIdeaArchived"},
		"AttachExternalReference":        {encoder: "encodeExternalReferenceAttachmentV1", record: "RecordExternalReference", fact: "FactExternalReferenceAttached"},
		"CaptureSpark":                   {encoder: "encodeSparkCapturedV1", record: "RecordSpark", fact: "FactSparkCaptured"},
		"CloseScratchpad":                {encoder: "encodeScratchpadCloseV1", record: "RecordScratchpad", fact: "FactScratchpadClosed"},
		"CorrectFinding":                 {encoder: "encodeFindingCorrectionV1", record: "RecordFinding", fact: "FactFindingCorrected"},
		"CorrectJournalEntry":            {encoder: "encodeJournalCorrectionV1", record: "RecordJournalEntry", fact: "FactJournalCorrectionRecorded"},
		"CreateIdea":                     {encoder: "encodeIdeaCreatedV1", record: "RecordIdea", fact: "FactIdeaCreated"},
		"DetachExternalReference":        {encoder: "encodeExternalReferenceDetachmentV1", record: "RecordExternalReference", fact: "FactExternalReferenceDetached"},
		"DismissSpark":                   {encoder: "encodeSparkDismissedV1", record: "RecordSpark", fact: "FactSparkDismissed"},
		"IntroduceScratchpadParticipant": {encoder: "encodeScratchpadParticipantV1", record: "RecordScratchpad", fact: "FactScratchpadParticipantIntroduced"},
		"OpenDecision":                   {encoder: "encodeDecisionOpenedV1", record: "RecordDecision", fact: "FactDecisionOpened"},
		"OpenScratchpad":                 {encoder: "encodeScratchpadOpenedV1", record: "RecordScratchpad", fact: "FactScratchpadOpened"},
		"PromoteIdeaToExternalReference": {encoder: "encodeIdeaPromotionV1", record: "RecordIdea", fact: "FactIdeaPromotedToExternalReference"},
		"PromoteSparkToIdea":             {encoder: "encodeSparkPromotionV1", record: "RecordSpark", fact: "FactSparkPromotedToIdea"},
		"RecordCheckpoint":               {encoder: "encodeCheckpointRecordedV1", record: "RecordCheckpoint", fact: "FactCheckpointRecorded"},
		"RecordFinding":                  {encoder: "encodeFindingRecordedV1", record: "RecordFinding", fact: "FactFindingRecorded"},
		"RecordHandoff":                  {encoder: "encodeHandoffRecordedV1", record: "RecordHandoff", fact: "FactHandoffRecorded"},
		"RecordJournalEntry":             {encoder: "encodeJournalRecordedV1", record: "RecordJournalEntry", fact: "FactJournalRecorded"},
		"RecordScratchpadClaim":          {encoder: "encodeScratchpadClaimV1", record: "RecordScratchpad", fact: "FactScratchpadClaimRecorded"},
		"RecordScratchpadMessage":        {encoder: "encodeScratchpadMessageV1", record: "RecordScratchpad", fact: "FactScratchpadMessageRecorded"},
		"RecordVerificationEvidence":     {encoder: "encodeVerificationEvidenceV1", record: "RecordVerificationEvidence", fact: "FactVerificationEvidenceRecorded"},
		"RecordWrap":                     {encoder: "encodeWrapRecordedV1", record: "RecordWrap", fact: "FactWrapRecorded"},
		"RegisterExternalReference":      {encoder: "encodeExternalReferenceRegistrationV1", record: "RecordExternalReference", fact: "FactExternalReferenceRegistered"},
		"RegisterProject":                {encoder: "encodeProjectRegistrationV1", record: "RecordProjectIdentity", fact: "FactProjectRegistered"},
		"ReleaseScratchpadClaim":         {encoder: "encodeScratchpadClaimReleaseV1", record: "RecordScratchpad", fact: "FactScratchpadClaimReleased"},
		"ResolveDecision":                {encoder: "encodeDecisionResolutionV1", record: "RecordDecision", fact: "FactDecisionResolved"},
		"ResolveIdea":                    {encoder: "encodeIdeaResolutionV1", record: "RecordIdea", fact: "FactIdeaResolved"},
		"RetractFinding":                 {encoder: "encodeFindingRetractionV1", record: "RecordFinding", fact: "FactFindingRetracted"},
		"ReviseIdea":                     {encoder: "encodeIdeaRevisionV1", record: "RecordIdea", fact: "FactIdeaRevised"},
		"ReviseProjectLabel":             {encoder: "encodeProjectLabelRevisionV1", record: "RecordProjectIdentity", fact: "FactProjectLabelRevised"},
		"StartExploration":               {encoder: "encodeExplorationStartedV1", record: "RecordExploration", fact: "FactExplorationStarted"},
		"SupersedeDecision":              {encoder: "encodeDecisionSupersessionV1", record: "RecordDecision", fact: "FactDecisionSuperseded"},
	}
	got := inspectMethodBindings(t, filepath.Join(sqliteSourceRoot, "append_methods.go"))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("public method bindings = %#v, want %#v", got, want)
	}
}

func TestContinuitySQLiteContractPinsEveryFactKindToOneObservationDecoder(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"FactCheckpointRecorded":              "wireCheckpointRecordedV1",
		"FactDecisionOpened":                  "wireDecisionOpenedV1",
		"FactDecisionResolved":                "wireDecisionResolutionV1",
		"FactDecisionSuperseded":              "wireDecisionSupersessionV1",
		"FactExplorationStarted":              "wireExplorationStartedV1",
		"FactExternalReferenceAttached":       "wireExternalReferenceAttachmentV1",
		"FactExternalReferenceDetached":       "wireExternalReferenceDetachmentV1",
		"FactExternalReferenceRegistered":     "wireExternalReferenceRegistrationV1",
		"FactFindingCorrected":                "wireFindingCorrectionV1",
		"FactFindingRecorded":                 "wireFindingRecordedV1",
		"FactFindingRetracted":                "wireFindingRetractionV1",
		"FactHandoffRecorded":                 "wireHandoffRecordedV1",
		"FactIdeaArchived":                    "wireIdeaArchiveV1",
		"FactIdeaCreated":                     "wireIdeaCreatedV1",
		"FactIdeaPromotedToExternalReference": "wireIdeaPromotionV1",
		"FactIdeaResolved":                    "wireIdeaResolutionV1",
		"FactIdeaRevised":                     "wireIdeaRevisionV1",
		"FactJournalCorrectionRecorded":       "wireJournalCorrectionV1",
		"FactJournalRecorded":                 "wireJournalRecordedV1",
		"FactProjectLabelRevised":             "wireProjectLabelRevisionV1",
		"FactProjectRegistered":               "wireProjectRegistrationV1",
		"FactScratchpadClaimRecorded":         "wireScratchpadClaimV1",
		"FactScratchpadClaimReleased":         "wireScratchpadClaimReleaseV1",
		"FactScratchpadClosed":                "wireScratchpadCloseV1",
		"FactScratchpadMessageRecorded":       "wireScratchpadMessageV1",
		"FactScratchpadOpened":                "wireScratchpadOpenedV1",
		"FactScratchpadParticipantIntroduced": "wireScratchpadParticipantV1",
		"FactSparkCaptured":                   "wireSparkCapturedV1",
		"FactSparkDismissed":                  "wireSparkDismissedV1",
		"FactSparkPromotedToIdea":             "wireSparkPromotionV1",
		"FactVerificationEvidenceRecorded":    "wireVerificationEvidenceV1",
		"FactWrapRecorded":                    "wireWrapRecordedV1",
	}
	got := inspectObservationDecoderBindings(t, filepath.Join(sqliteSourceRoot, "snapshot_fold_v1.go"))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("observation decoder bindings = %#v, want %#v", got, want)
	}
}

func TestContinuitySQLiteContractRejectsDynamicPersistenceEscapeHatches(t *testing.T) {
	t.Parallel()

	wantGenericFunctions := []string{
		"codec_v1.go:canonicalizeWireV1",
		"codec_v1.go:decodeStoredWireV1",
		"codec_v1.go:decodeWireV1",
		"codec_v1.go:encodeWireV1",
		"codec_v1.go:requireCanonicalV1",
	}
	wantInterfaces := []string{"schema.go:schemaQuerier", "wire_v1.go:wireValueV1"}

	entries, err := os.ReadDir(sqliteSourceRoot)
	if err != nil {
		t.Fatalf("read SQLite source root: %v", err)
	}
	var genericFunctions, interfaces []string
	for _, entry := range entries {
		fileName := entry.Name()
		if entry.IsDir() || filepath.Ext(fileName) != ".go" || strings.HasSuffix(fileName, "_test.go") {
			continue
		}
		file := parseGoFile(t, filepath.Join(sqliteSourceRoot, fileName))
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				if declaration.Tok == token.VAR {
					t.Errorf("%s declares package state; persistence behavior must remain Store-owned or constant", fileName)
				}
				for _, specification := range declaration.Specs {
					typeSpecification, ok := specification.(*ast.TypeSpec)
					if !ok {
						continue
					}
					if typeSpecification.TypeParams != nil {
						t.Errorf("%s:%s is an unadmitted generic type", fileName, typeSpecification.Name.Name)
					}
					if _, ok := typeSpecification.Type.(*ast.InterfaceType); ok {
						interfaces = append(interfaces, fileName+":"+typeSpecification.Name.Name)
					}
				}
			case *ast.FuncDecl:
				if declaration.Name.Name == "init" {
					t.Errorf("%s declares init; persistence setup must remain explicit", fileName)
				}
				if declaration.Type.TypeParams != nil {
					genericFunctions = append(genericFunctions, fileName+":"+declaration.Name.Name)
				}
				if declaration.Name.Name == "MarshalJSON" || declaration.Name.Name == "UnmarshalJSON" {
					t.Errorf("%s declares %s; the V1 codec owns JSON behavior", fileName, declaration.Name.Name)
				}
			}
		}

		if fileName != "codec_v1.go" && fileName != "wire_v1.go" && fileName != "wire_validation_v1.go" {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			switch node := node.(type) {
			case *ast.Ident:
				if node.Name == "any" {
					t.Errorf("%s uses any in the closed V1 codec", fileName)
				}
			case *ast.MapType:
				t.Errorf("%s uses a map in the closed V1 codec", fileName)
			case *ast.SelectorExpr:
				if identifier, ok := node.X.(*ast.Ident); ok && identifier.Name == "json" && node.Sel.Name == "RawMessage" {
					t.Errorf("%s uses json.RawMessage in the closed V1 codec", fileName)
				}
			case *ast.Field:
				if node.Tag != nil && strings.Contains(node.Tag.Value, "omitempty") {
					t.Errorf("%s uses omitempty in the closed V1 codec", fileName)
				}
			}
			return true
		})
	}
	sort.Strings(genericFunctions)
	sort.Strings(interfaces)
	if !reflect.DeepEqual(genericFunctions, wantGenericFunctions) {
		t.Fatalf("generic persistence functions = %v, want %v", genericFunctions, wantGenericFunctions)
	}
	if !reflect.DeepEqual(interfaces, wantInterfaces) {
		t.Fatalf("persistence interfaces = %v, want %v", interfaces, wantInterfaces)
	}
}

func TestContinuitySQLiteContractPinsOpenSafetyImplementation(t *testing.T) {
	t.Parallel()

	wantSourceDigests := map[string]string{
		"driver.go":                        "bd8fc591dd2d5c9c2c8f5f9ea6320b913a08ad8b9e1f9243c4e1a93538970a44",
		"filesystem_attributes_windows.go": "1e3b5f12e4debbc5432f2153818862cdc39ec617409e526b750e6db093dcb0a0",
		"filesystem_unix.go":               "4a06e1818660145b50a3a28a7c0b4e994df0eb5fc46bed56d1f424259000b0e2",
		"filesystem_windows.go":            "0770148405c2b7f215a92e3706443bd6bf6438790c926f6c5a18461a09628818",
		"schema.go":                        "5009592f61bb11d2d0035a4ead6c6757e07522e36143dce9087bbfce605557a3",
		"store.go":                         "25a31b8efe29c41bbc6dcac1a7e2dcb2fd5cfca38863c4b4ee56660115a856a2",
	}
	for fileName, wantDigest := range wantSourceDigests {
		gotDigest := digestSourceFile(t, filepath.Join(sqliteSourceRoot, fileName))
		if gotDigest != wantDigest {
			t.Errorf("%s source digest = %q, want %q", fileName, gotDigest, wantDigest)
		}
	}

	want := map[string]map[string]string{
		"driver.go": {},
		"filesystem_attributes_windows.go": {
			"windowsReparsePoint": "81f5d3403c7fb9f2fbb37255d5061ae761a50c56fadc53b5aa52c048cc7c0440",
		},
		"filesystem_unix.go": {
			"databaseURLPath":                    "a4fa2923449759d339aa49022c0068bf2cc615e18d558762954c9be63da9365a",
			"securePrivateFilePlatform":          "358bedcdc24452ef3951e35358f97c1656f2490fc78cb734c4e7eb0ecb080133",
			"validateExistingUnixPathComponents": "e6c2bc130ca8d6742805bac6a9d7b518f01eecac2f8060ba3c78a2439d2f13a6",
			"validatePrivateDirectoryPlatform":   "73ddadd3b323b7ff9872b82e41b643d2acf5b1229a86714b7dc406b61d0737a6",
			"validatePrivateFilePlatform":        "bce83a861542a75dc778d2b7f252d19820868a18c7e92acc20052ac41abde4fc",
			"validateStateRootLocationPlatform":  "818cc5d8212da884024e801df3c8273bc5d71ff3a3c856d0c05d7c82377af3ef",
			"validateStateRootPlatform":          "d465119f3cd961c818f763b8540fb4544733f08dc5542953ba8fbeb215dacfc7",
		},
		"filesystem_windows.go": {
			"databaseURLPath":                       "6539227ce439034da79cd08ef3df63b5e89821bd0b8200b75d528fc7dda7e35e",
			"isWindowsUNC":                          "9fc97df4a9d797e9d464c94a93891271ec59c83beafa825db11c28e9b58b8737",
			"pathWithinWindowsRoot":                 "8d2439757c50d9ba3e2f43708cdc0c6ef09469c0113be2f0211231b129076237",
			"securePrivateFilePlatform":             "cfb416d3d80180bb04a15e94dc97241c9eb50016aa988af4c488dee89e0f17eb",
			"validateExistingWindowsPathComponents": "362c51d2e892771ea2de3b2a69a5299777bea8d58f843377af4f117e9aa9dfa9",
			"validateObservedWindowsPath":           "655fc09ffbd4d1633c0c74a7214b0e483ca2156b6d72dcc13a06d09fe3884ea8",
			"validatePrivateDirectoryPlatform":      "b62ff8dbdae49190bf22a1779d687fd53d161792a98d8a9f50e5af732ac89d63",
			"validatePrivateFilePlatform":           "9fd50adfb65959ed2c792e0c3c84dc4b97f16fb677494776f04f02331a6ab1d1",
			"validateStateRootLocationPlatform":     "4b10e1964a7edd4c2b5b0b9bd2169f4f3bf7f893d4bd43716a897a4d4bea644b",
			"validateStateRootPlatform":             "db32888248aa99a7a5ee8f72060f1fc1ae8eb6e07e6c9ac7b19aa6f8eebc6751",
			"validateWindowsPathComponents":         "7613a329580b81601f4f6d08cc74abcab3d132504ed9e7ccebb075a7c38a2a0a",
			"windowsUserDataRoot":                   "97b6595bfcf4d216279ef990542b9b02509a1763f09ac0cfdd1898a69b5bc3d9",
		},
		"schema.go": {
			"checksumDDL":                  "8bb7ecf518009f014254a71f4ada47cf49decb32a805691cee121476566b1d4a",
			"checksumSchema":               "57e891a96e8bcc3f1889cc71c530afe367d1f097717d25389e206b6b24e74907",
			"checksumSchemaV1":             "caceb1d21c21d048cbaca49bf393f3d37a3290fa02cf84b3a2933a183c8dd054",
			"checksumSchemaV2":             "89dd6cdd8c6947ca7f6e649675dd82d4456a2ea2db0c6c9107510f36bd0b21be",
			"checksumSchemaV3":             "50f038bbbda2f56c6725ef28690e987c57e04d3d59e9fd6c066db3b148a81d3f",
			"closeMigrationRows":           "1a1b6a77c23eef79fb5730df7674519c954e253ec51fd8f9a1c60ac793d1861f",
			"expectedSchemaObjects":        "0094c2ecdf12cb79b06eef38f89607fed191e7cd3f470455e2b0ca03f4692cb3",
			"expectedSchemaV1Objects":      "690fbda019e83a41451abc381d9d1c10f56f9597c2b55a0809bf977abd4b6da1",
			"expectedSchemaV2Objects":      "f8a369c7a168ff5a4a5228fbc0888af213e0d18b36dcc996334eb4ec3a936e9d",
			"expectedSchemaV3Objects":      "e0ed9b1701a662c2d23dc02f338b8bc42799b477b215841ab696cf2c0a4ea3f1",
			"initializeSchemaIfEmpty":      "ca3df10057c06362dbf790b8dcfcc2f7865bca40374099eed9012f7523ac491a",
			"migrateSchema":                "3658f72330d0977a45f8851753c64dc26415602a056ef104f5ada0db829f74cf",
			"migrateSchemaV1ToV2":          "3f1ee78cc1b2a56a036b03a390207ea7d27a911867efdad1fc4f162623925293",
			"migrateSchemaV2ToV3":          "3b640231ba853caec59017133ddcc87d3e20d0c3419abb6027b3b447f450fef9",
			"migrateSchemaV3ToV4":          "38ad8482a0a9411c1db9afb2c231518e965348f979631afa1ccd27f330d8f5db",
			"normalizeSQL":                 "ba395a0d4dddb73b4b9669ad5a6cd12d28494ba464560910074abbddd0d96e98",
			"seedV3SyncAuthorityMetadata":  "61d5a908b0fee56af16bd14bba221c70ffe1f100259b3335beae1a43452e9e55",
			"validateForeignKeys":          "5d789c539f43fdb2be215784eeb7074c290023cc6ef12dbe92511c1d335163f1",
			"validateKnownSchemaVersion":   "f803ada484269862fc656896c65d71431fb7a593a8273f0556ef742962dc88ba",
			"validateMigrationPreflight":   "70843e4fedff7b293fc588c53ea29d8a20e0598dc0b2a5fb3dfd6f8a44e74026",
			"validateSchema":               "03e687e88be737459ecbdde22172fd82222a45c8b6c6bd9942fb83a1da1c1e93",
			"validateSchemaVersion":        "56b5625f087b8809fd6abc6e9e67326dac4e35d71ba77e32cf6cad00c5ec41f4",
			"validateV1EnvironmentHistory": "fea75b609db237d482ce9cf8769c8bec944a7a4532692d7d87ddf3f7d02e5528",
			"validateV2InboxCopy":          "57c4e8da2c1ec025021f3a4c545b382b800d954f0b01d67da227cac9337b4359",
		},
		"store.go": {
			"Close":                     "69f3a1056e0ce920d07dbdbf96c0559b9edf375218c904d29ea307d6623006f1",
			"Open":                      "074877062be5bb129401c555bb8734fb952a284d8a8a00aaed2d20ca7f06f357",
			"databaseDSN":               "31b0b9b4bd1269c1e5f8c521c17fcac5ed99cfe1609e48e1937744a729788e19",
			"inspectRegularPrivateFile": "177aaa7ec2087d38ca322829f4a54d9c333a51aea70be68823d1d9d34772e64a",
			"openDatabase":              "63865fcf446d578fad398587c9c01c6e7330f6117673125cd4afd351b57c3dfc",
			"prepareDatabaseFile":       "22eb5fd9fe6e793d6f846255ffd83be7e43748b2d41fcac76e4bc6f4322e74fb",
			"preparePrivateDirectory":   "5024c5c56c7bf3b92ba98eaf138d8c78542146bdc992051583772566fb629fa0",
			"retryableOpenError":        "b359c6f459c7490b1f8aed8df8e3f137456003c3a1d5b0a842cc639f53e437ca",
			"schemaIsEmpty":             "d96549881a4a329b5815e73d4b144cbe6b8c3cb49a56b5e98c7736d7532ac4fc",
			"secureSQLiteSidecars":      "403d22fdce31128b08551f01f6cb68102332930ed3c6ebaf0cceaf0d32fb0e89",
			"validOpaqueID":             "c0072295d2bd1701a690020b9a9b3791eba2c22429671460ffac401896d1bdb1",
			"validateEnvironmentID":     "6f4006fba6c237bb97b739529c6a82f739cf3a75758fa97625b36da20389fb67",
			"verifySQLiteFiles":         "32e01c16e83df7de6a0034055df7ff29d169823a2d268fc412b824609ad8891d",
		},
	}

	for fileName, wantFunctions := range want {
		got := digestFunctions(t, filepath.Join(sqliteSourceRoot, fileName))
		if len(got) != len(wantFunctions) {
			t.Errorf("%s function inventory = %#v, want %#v", fileName, got, wantFunctions)
		}
		for functionName, wantDigest := range wantFunctions {
			if got[functionName] != wantDigest {
				t.Errorf("%s %s digest = %q, want %q", fileName, functionName, got[functionName], wantDigest)
			}
		}
	}
}

func TestContinuitySQLiteContractPinsOpenBehavior(t *testing.T) {
	t.Parallel()

	t.Run("special URI path and persistent WAL", func(t *testing.T) {
		stateRoot := filepath.Join(contractTempDir(t), "state #% ü")
		store, err := continuitysqlite.Open(stateRoot, "oracle-environment")
		if err != nil {
			t.Fatalf("open continuity store: %v", err)
		}
		if err := store.Close(); err != nil {
			t.Fatalf("close continuity store: %v", err)
		}

		db := openContractDatabaseAt(t, stateRoot, "ro")
		defer db.Close()
		var journalMode string
		if err := db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
			t.Fatalf("read journal mode: %v", err)
		}
		if journalMode != "wal" {
			t.Fatalf("journal mode = %q, want wal", journalMode)
		}

		if runtime.GOOS != "windows" {
			assertContractMode(t, filepath.Join(stateRoot, "vnext"), 0o700)
			assertContractMode(t, filepath.Join(stateRoot, "vnext", "continuity.sqlite"), 0o600)
		}
	})

	t.Run("invalid paths", func(t *testing.T) {
		root := filepath.Join(contractTempDir(t), "state")
		assertContractOpenRefused(t, "relative")
		assertContractOpenRefused(t, root+string(filepath.Separator)+".")
	})

	t.Run("symlinked state surfaces", func(t *testing.T) {
		t.Run("state root", func(t *testing.T) {
			link := filepath.Join(contractTempDir(t), "state-link")
			if err := os.Symlink(contractTempDir(t), link); err != nil {
				t.Skipf("create symlink: %v", err)
			}
			assertContractOpenRefused(t, link)
		})

		t.Run("state root ancestor", func(t *testing.T) {
			parent := contractTempDir(t)
			target := contractTempDir(t)
			link := filepath.Join(parent, "linked-parent")
			if err := os.Symlink(target, link); err != nil {
				t.Skipf("create symlink: %v", err)
			}
			assertContractOpenRefused(t, filepath.Join(link, "new-state"))
			if _, err := os.Lstat(filepath.Join(target, "new-state")); !os.IsNotExist(err) {
				t.Fatalf("Open created state through a symlink ancestor: %v", err)
			}
		})

		t.Run("private directory", func(t *testing.T) {
			root := contractTempDir(t)
			if err := os.Symlink(contractTempDir(t), filepath.Join(root, "vnext")); err != nil {
				t.Skipf("create symlink: %v", err)
			}
			assertContractOpenRefused(t, root)
		})

		t.Run("database", func(t *testing.T) {
			root := contractTempDir(t)
			privateDirectory := filepath.Join(root, "vnext")
			if err := os.Mkdir(privateDirectory, 0o700); err != nil {
				t.Fatalf("create private directory: %v", err)
			}
			target := filepath.Join(contractTempDir(t), "target.sqlite")
			file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
			if err != nil {
				t.Fatalf("create target: %v", err)
			}
			file.Close()
			if err := os.Symlink(target, filepath.Join(privateDirectory, "continuity.sqlite")); err != nil {
				t.Skipf("create symlink: %v", err)
			}
			assertContractOpenRefused(t, root)
		})

		for _, suffix := range []string{"-wal", "-shm", "-journal"} {
			suffix := suffix
			t.Run("sidecar "+suffix, func(t *testing.T) {
				root := contractTempDir(t)
				privateDirectory := filepath.Join(root, "vnext")
				if err := os.Mkdir(privateDirectory, 0o700); err != nil {
					t.Fatalf("create private directory: %v", err)
				}
				databasePath := filepath.Join(privateDirectory, "continuity.sqlite")
				file, err := os.OpenFile(databasePath, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
				if err != nil {
					t.Fatalf("create database: %v", err)
				}
				file.Close()
				if err := os.Symlink(filepath.Join(contractTempDir(t), "target"), databasePath+suffix); err != nil {
					t.Skipf("create symlink: %v", err)
				}
				assertContractOpenRefused(t, root)
			})
		}
	})

	t.Run("insecure Unix modes", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("POSIX mode contract")
		}

		t.Run("state root", func(t *testing.T) {
			root := filepath.Join(contractTempDir(t), "state")
			if err := os.Mkdir(root, 0o700); err != nil {
				t.Fatalf("create state root: %v", err)
			}
			if err := os.Chmod(root, 0o770); err != nil {
				t.Fatalf("weaken state root: %v", err)
			}
			assertContractOpenRefused(t, root)
		})

		t.Run("private directory", func(t *testing.T) {
			root := contractTempDir(t)
			if err := os.Mkdir(filepath.Join(root, "vnext"), 0o755); err != nil {
				t.Fatalf("create private directory: %v", err)
			}
			assertContractOpenRefused(t, root)
		})

		t.Run("database", func(t *testing.T) {
			root := contractTempDir(t)
			privateDirectory := filepath.Join(root, "vnext")
			if err := os.Mkdir(privateDirectory, 0o700); err != nil {
				t.Fatalf("create private directory: %v", err)
			}
			file, err := os.OpenFile(filepath.Join(privateDirectory, "continuity.sqlite"), os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o644)
			if err != nil {
				t.Fatalf("create database: %v", err)
			}
			file.Close()
			assertContractOpenRefused(t, root)
		})
	})

	t.Run("concurrent bootstrap", func(t *testing.T) {
		const openers = 8
		stateRoot := filepath.Join(contractTempDir(t), "state")
		start := make(chan struct{})
		errorsByOpener := make([]error, openers)
		var wait sync.WaitGroup
		wait.Add(openers)
		for opener := 0; opener < openers; opener++ {
			opener := opener
			go func() {
				defer wait.Done()
				<-start
				store, err := continuitysqlite.Open(stateRoot, continuity.EnvironmentID(fmt.Sprintf("oracle-%d", opener)))
				if err == nil {
					err = store.Close()
				}
				errorsByOpener[opener] = err
			}()
		}
		close(start)
		wait.Wait()
		for opener, err := range errorsByOpener {
			if err != nil {
				t.Errorf("concurrent Open() caller %d: %v", opener, err)
			}
		}
	})
}

func TestContinuitySQLiteContractMatchesEveryCatalogFactToExactlyOneSubjectKind(t *testing.T) {
	t.Parallel()

	persistedKinds := []continuity.RecordKind{
		continuity.RecordProjectIdentity,
		continuity.RecordJournalEntry,
		continuity.RecordWrap,
		continuity.RecordSpark,
		continuity.RecordIdea,
		continuity.RecordDecision,
		continuity.RecordExploration,
		continuity.RecordCheckpoint,
		continuity.RecordFinding,
		continuity.RecordHandoff,
		continuity.RecordScratchpad,
		continuity.RecordExternalReference,
		continuity.RecordVerificationEvidence,
	}
	definitions := continuity.FactCatalog()
	if len(definitions) != 32 {
		t.Fatalf("fact catalog has %d definitions, want 32", len(definitions))
	}

	for definitionIndex, definition := range definitions {
		definitionIndex, definition := definitionIndex, definition
		t.Run(string(definition.Kind), func(t *testing.T) {
			db := openWritableContractDatabase(t)
			defer db.Close()

			projectID := fmt.Sprintf("correct-project-%d", definitionIndex)
			sequence := 1
			if definition.Kind != continuity.FactProjectRegistered {
				if err := insertContractFact(db, "correct-registration", projectID, continuity.RecordProjectIdentity, continuity.SubjectID(projectID), continuity.FactProjectRegistered, sequence); err != nil {
					t.Fatalf("insert project identity: %v", err)
				}
				sequence++
			}
			subjectID := continuity.SubjectID("correct-subject")
			if definition.Record == continuity.RecordProjectIdentity {
				subjectID = continuity.SubjectID(projectID)
			}
			if err := insertContractFact(db, "correct-fact", projectID, definition.Record, subjectID, definition.Kind, sequence); err != nil {
				t.Fatalf("schema rejected catalog pair (%s, %s): %v", definition.Record, definition.Kind, err)
			}

			for wrongIndex, wrongKind := range persistedKinds {
				if wrongKind == definition.Record {
					continue
				}
				wrongProjectID := fmt.Sprintf("wrong-project-%d-%d", definitionIndex, wrongIndex)
				wrongSequence := 1
				if definition.Kind != continuity.FactProjectRegistered {
					if err := insertContractFact(db, fmt.Sprintf("wrong-registration-%d", wrongIndex), wrongProjectID, continuity.RecordProjectIdentity, continuity.SubjectID(wrongProjectID), continuity.FactProjectRegistered, wrongSequence); err != nil {
						t.Fatalf("insert wrong-case project identity: %v", err)
					}
					wrongSequence++
				}
				wrongSubjectID := continuity.SubjectID(fmt.Sprintf("wrong-subject-%d", wrongIndex))
				if wrongKind == continuity.RecordProjectIdentity {
					wrongSubjectID = continuity.SubjectID(wrongProjectID)
				}
				tx, err := db.Begin()
				if err != nil {
					t.Fatalf("begin wrong-kind probe: %v", err)
				}
				err = insertContractFact(tx, fmt.Sprintf("wrong-fact-%d", wrongIndex), wrongProjectID, wrongKind, wrongSubjectID, definition.Kind, wrongSequence)
				if rollbackErr := tx.Rollback(); rollbackErr != nil {
					t.Fatalf("rollback wrong-kind probe: %v", rollbackErr)
				}
				if err == nil {
					t.Errorf("schema accepted %s under wrong subject kind %s; want only %s", definition.Kind, wrongKind, definition.Record)
				}
			}
		})
	}
}

type objectDigest struct {
	kind   string
	name   string
	table  string
	digest string
}

type methodBinding struct {
	encoder string
	record  string
	fact    string
}

type columnSpec struct {
	name       string
	dataType   string
	notNull    int
	primaryKey int
	hidden     int
}

type indexSpec struct {
	name       string
	unique     int
	origin     string
	partial    int
	keyColumns []string
}

func openContractDatabase(t *testing.T) *sql.DB {
	t.Helper()

	stateRoot := filepath.Join(contractTempDir(t), "state")
	store, err := continuitysqlite.Open(stateRoot, continuity.EnvironmentID("oracle-environment"))
	if err != nil {
		t.Fatalf("open continuity store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close continuity store: %v", err)
	}

	return openContractDatabaseAt(t, stateRoot, "ro")
}

func openWritableContractDatabase(t *testing.T) *sql.DB {
	t.Helper()

	stateRoot := filepath.Join(contractTempDir(t), "state")
	store, err := continuitysqlite.Open(stateRoot, continuity.EnvironmentID("oracle-environment"))
	if err != nil {
		t.Fatalf("open continuity store: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close continuity store: %v", err)
	}
	return openContractDatabaseAt(t, stateRoot, "rw")
}

func openContractDatabaseAt(t *testing.T, stateRoot, mode string) *sql.DB {
	t.Helper()

	values := url.Values{}
	values.Set("mode", mode)
	databasePath := filepath.Join(stateRoot, "vnext", "continuity.sqlite")
	urlPath := filepath.ToSlash(databasePath)
	if runtime.GOOS == "windows" && filepath.VolumeName(databasePath) != "" && !strings.HasPrefix(urlPath, "/") {
		urlPath = "/" + urlPath
	}
	dsn := (&url.URL{
		Scheme:   "file",
		Path:     urlPath,
		RawQuery: values.Encode(),
	}).String()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open continuity database for inspection: %v", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		t.Fatalf("ping continuity database for inspection: %v", err)
	}
	return db
}

func contractTempDir(t *testing.T) string {
	t.Helper()

	if runtime.GOOS != "windows" {
		return t.TempDir()
	}
	root, err := os.UserCacheDir()
	if err != nil {
		t.Fatalf("resolve LocalAppData contract-test root: %v", err)
	}
	directory, err := os.MkdirTemp(root, "loaf-continuity-contract-")
	if err != nil {
		t.Fatalf("create LocalAppData contract-test directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(directory); err != nil {
			t.Errorf("remove LocalAppData contract-test directory: %v", err)
		}
	})
	return directory
}

type contractExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func insertContractFact(executor contractExecer, factID, projectID string, subjectKind continuity.RecordKind, subjectID continuity.SubjectID, factKind continuity.FactKind, sequence int) error {
	_, err := executor.Exec(`
INSERT INTO continuity_facts(
  fact_id,
  project_id,
  subject_kind,
  subject_id,
  fact_kind,
  payload_version,
  content_json,
  environment_id,
  environment_sequence,
  hlc_wall_millis,
  hlc_logical,
  envelope_version
) VALUES(?, ?, ?, ?, ?, 1, '{}', 'oracle-environment', ?, 1, 0, 1)`,
		factID,
		projectID,
		string(subjectKind),
		string(subjectID),
		string(factKind),
		sequence,
	)
	return err
}

func assertContractOpenRefused(t *testing.T, stateRoot string) {
	t.Helper()

	store, err := continuitysqlite.Open(stateRoot, "oracle-environment")
	if err == nil {
		store.Close()
		t.Fatal("Open() error = nil, want refusal")
	}
}

func assertContractMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("inspect %s: %v", path, err)
	}
	if info.Mode().Perm() != want {
		t.Fatalf("mode for %s = %o, want %o", path, info.Mode().Perm(), want)
	}
}

func digestFunctions(t *testing.T, path string) map[string]string {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	digests := make(map[string]string)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok {
			continue
		}
		var formatted bytes.Buffer
		if err := format.Node(&formatted, fileSet, function); err != nil {
			t.Fatalf("format %s.%s: %v", path, function.Name.Name, err)
		}
		if _, duplicate := digests[function.Name.Name]; duplicate {
			t.Fatalf("%s has multiple functions or methods named %s", path, function.Name.Name)
		}
		digests[function.Name.Name] = fmt.Sprintf("%x", sha256.Sum256(formatted.Bytes()))
	}
	return digests
}

func digestSourceFile(t *testing.T, path string) string {
	t.Helper()

	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return fmt.Sprintf("%x", sha256.Sum256(contents))
}

func inspectPrivateStructShapes(t *testing.T, path string) map[string][]string {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	shapes := make(map[string][]string)
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			typeSpecification, ok := specification.(*ast.TypeSpec)
			if !ok {
				continue
			}
			structure, ok := typeSpecification.Type.(*ast.StructType)
			if !ok {
				continue
			}
			var fields []string
			for _, field := range structure.Fields.List {
				if len(field.Names) == 0 || field.Tag == nil {
					t.Fatalf("%s has an anonymous or untagged V1 wire field", typeSpecification.Name.Name)
				}
				var formatted bytes.Buffer
				if err := format.Node(&formatted, fileSet, field.Type); err != nil {
					t.Fatalf("format %s field type: %v", typeSpecification.Name.Name, err)
				}
				for _, name := range field.Names {
					fields = append(fields, name.Name+":"+formatted.String()+":"+field.Tag.Value)
				}
			}
			shapes[typeSpecification.Name.Name] = fields
		}
	}
	return shapes
}

func inspectClosedWireUnion(t *testing.T, path string) ([]string, string) {
	t.Helper()

	fileSet := token.NewFileSet()
	file, err := parser.ParseFile(fileSet, path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	var members []string
	var methodSignature string
	for _, declaration := range file.Decls {
		general, ok := declaration.(*ast.GenDecl)
		if !ok || general.Tok != token.TYPE {
			continue
		}
		for _, specification := range general.Specs {
			typeSpecification, ok := specification.(*ast.TypeSpec)
			if !ok || typeSpecification.Name.Name != "wireValueV1" {
				continue
			}
			contract, ok := typeSpecification.Type.(*ast.InterfaceType)
			if !ok {
				t.Fatal("wireValueV1 is not an interface constraint")
			}
			for _, field := range contract.Methods.List {
				if len(field.Names) == 0 {
					members = append(members, flattenUnionNames(t, field.Type)...)
					continue
				}
				if len(field.Names) != 1 || field.Names[0].Name != "validate" {
					t.Fatalf("wireValueV1 has unexpected method field %#v", field.Names)
				}
				var formatted bytes.Buffer
				if err := format.Node(&formatted, fileSet, field.Type); err != nil {
					t.Fatalf("format wireValueV1 method: %v", err)
				}
				methodSignature = formatted.String()
			}
		}
	}
	sort.Strings(members)
	return members, methodSignature
}

func inspectMethodBindings(t *testing.T, path string) map[string]methodBinding {
	t.Helper()

	wantSubjectByRecord := map[string]string{
		"RecordProjectIdentity":      "continuity.SubjectID(projectID)",
		"RecordJournalEntry":         "journalID",
		"RecordWrap":                 "wrapID",
		"RecordSpark":                "sparkID",
		"RecordIdea":                 "ideaID",
		"RecordDecision":             "decisionID",
		"RecordExploration":          "explorationID",
		"RecordCheckpoint":           "checkpointID",
		"RecordFinding":              "findingID",
		"RecordHandoff":              "handoffID",
		"RecordScratchpad":           "scratchpadID",
		"RecordExternalReference":    "referenceID",
		"RecordVerificationEvidence": "evidenceID",
	}
	file := parseGoFile(t, path)
	bindings := make(map[string]methodBinding)
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || !ast.IsExported(function.Name.Name) || function.Recv == nil || len(function.Recv.List) != 1 {
			continue
		}
		if receiverName(function.Recv.List[0].Type) != "Store" {
			continue
		}
		if len(function.Recv.List[0].Names) != 1 || function.Recv.List[0].Names[0].Name != "store" {
			t.Fatalf("%s receiver is not the canonical store receiver", function.Name.Name)
		}
		if function.Body == nil || len(function.Body.List) != 3 {
			t.Fatalf("%s body has %d statements, want canonical encode/guard/append", function.Name.Name, len(function.Body.List))
		}

		assignment, ok := function.Body.List[0].(*ast.AssignStmt)
		if !ok || assignment.Tok != token.DEFINE || len(assignment.Lhs) != 2 || compactNode(t, assignment.Lhs[0]) != "content" || compactNode(t, assignment.Lhs[1]) != "err" || len(assignment.Rhs) != 1 {
			t.Fatalf("%s does not begin with canonical content/error assignment", function.Name.Name)
		}
		encoderCall, ok := assignment.Rhs[0].(*ast.CallExpr)
		if !ok || len(encoderCall.Args) != 1 || compactNode(t, encoderCall.Args[0]) != "payload" {
			t.Fatalf("%s encoder does not consume exactly payload", function.Name.Name)
		}
		encoder, ok := encoderCall.Fun.(*ast.Ident)
		if !ok || !strings.HasPrefix(encoder.Name, "encode") || !strings.HasSuffix(encoder.Name, "V1") {
			t.Fatalf("%s uses non-V1 encoder %s", function.Name.Name, compactNode(t, encoderCall.Fun))
		}
		if got := strings.Join(strings.Fields(compactNode(t, function.Body.List[1])), " "); got != "if err != nil { return continuity.AppendReceipt{}, err }" {
			t.Fatalf("%s error guard = %q, want canonical fail-fast guard", function.Name.Name, got)
		}

		returnStatement, ok := function.Body.List[2].(*ast.ReturnStmt)
		if !ok || len(returnStatement.Results) != 1 {
			t.Fatalf("%s does not end in one append return", function.Name.Name)
		}
		appendCall, ok := returnStatement.Results[0].(*ast.CallExpr)
		if !ok || len(appendCall.Args) != 6 {
			t.Fatalf("%s append return does not have six closed arguments", function.Name.Name)
		}
		appendSelector, ok := appendCall.Fun.(*ast.SelectorExpr)
		if !ok || compactNode(t, appendSelector.X) != "store" || appendSelector.Sel.Name != "appendFactV1" {
			t.Fatalf("%s does not delegate exactly once to store.appendFactV1", function.Name.Name)
		}
		for index, wantArgument := range []string{"ctx", "projectID", "factID"} {
			if got := compactNode(t, appendCall.Args[index]); got != wantArgument {
				t.Fatalf("%s append argument %d = %q, want %q", function.Name.Name, index, got, wantArgument)
			}
		}
		if got := compactNode(t, appendCall.Args[5]); got != "content" {
			t.Fatalf("%s append content argument = %q, want encoded content", function.Name.Name, got)
		}

		subject, ok := appendCall.Args[3].(*ast.CompositeLit)
		if !ok || compactNode(t, subject.Type) != "continuity.SubjectRef" || len(subject.Elts) != 2 {
			t.Fatalf("%s does not construct one closed continuity.SubjectRef", function.Name.Name)
		}
		kindField, ok := subject.Elts[0].(*ast.KeyValueExpr)
		if !ok || compactNode(t, kindField.Key) != "Kind" {
			t.Fatalf("%s subject does not begin with Kind", function.Name.Name)
		}
		recordSelector, ok := kindField.Value.(*ast.SelectorExpr)
		if !ok || compactNode(t, recordSelector.X) != "continuity" || !strings.HasPrefix(recordSelector.Sel.Name, "Record") {
			t.Fatalf("%s subject kind is not a closed continuity record", function.Name.Name)
		}
		record := recordSelector.Sel.Name
		idField, ok := subject.Elts[1].(*ast.KeyValueExpr)
		if !ok || compactNode(t, idField.Key) != "ID" {
			t.Fatalf("%s subject does not end with ID", function.Name.Name)
		}
		wantSubject, ok := wantSubjectByRecord[record]
		if !ok || compactNode(t, idField.Value) != wantSubject {
			t.Fatalf("%s subject id = %q for %s, want %q", function.Name.Name, compactNode(t, idField.Value), record, wantSubject)
		}

		factSelector, ok := appendCall.Args[4].(*ast.SelectorExpr)
		if !ok || compactNode(t, factSelector.X) != "continuity" || !strings.HasPrefix(factSelector.Sel.Name, "Fact") {
			t.Fatalf("%s fact kind is not a closed continuity fact", function.Name.Name)
		}
		if _, exists := bindings[function.Name.Name]; exists {
			t.Fatalf("duplicate Store method %s", function.Name.Name)
		}
		bindings[function.Name.Name] = methodBinding{encoder: encoder.Name, record: record, fact: factSelector.Sel.Name}
	}
	return bindings
}

func inspectObservationDecoderBindings(t *testing.T, path string) map[string]string {
	t.Helper()

	file := parseGoFile(t, path)
	var function *ast.FuncDecl
	for _, declaration := range file.Decls {
		candidate, ok := declaration.(*ast.FuncDecl)
		if ok && candidate.Name.Name == "observationForStoredFactV1" {
			function = candidate
			break
		}
	}
	if function == nil {
		t.Fatal("observationForStoredFactV1 not found")
	}
	bindings := make(map[string]string)
	defaultFound := false
	for _, statement := range function.Body.List {
		switchStatement, ok := statement.(*ast.SwitchStmt)
		if !ok {
			continue
		}
		for _, caseStatement := range switchStatement.Body.List {
			clause := caseStatement.(*ast.CaseClause)
			if len(clause.List) == 0 {
				defaultFound = true
				continue
			}
			if len(clause.List) != 1 {
				t.Fatalf("observation decoder case has %d fact kinds", len(clause.List))
			}
			selector, ok := clause.List[0].(*ast.SelectorExpr)
			if !ok {
				t.Fatalf("observation decoder case is %#v, want continuity selector", clause.List[0])
			}
			var decoders []string
			for _, bodyStatement := range clause.Body {
				ast.Inspect(bodyStatement, func(node ast.Node) bool {
					index, ok := node.(*ast.IndexExpr)
					if !ok {
						return true
					}
					callee, ok := index.X.(*ast.Ident)
					if !ok || callee.Name != "decodeStoredWireV1" {
						return true
					}
					wireType, ok := index.Index.(*ast.Ident)
					if !ok {
						t.Fatalf("decoder for %s uses non-identifier type %#v", selector.Sel.Name, index.Index)
					}
					decoders = append(decoders, wireType.Name)
					return true
				})
			}
			if len(decoders) != 1 {
				t.Fatalf("decoder for %s = %v, want exactly one", selector.Sel.Name, decoders)
			}
			bindings[selector.Sel.Name] = decoders[0]
		}
	}
	if !defaultFound {
		t.Fatal("observation decoder has no rejecting default")
	}
	return bindings
}

func compactNode(t *testing.T, node ast.Node) string {
	t.Helper()

	var formatted bytes.Buffer
	if err := format.Node(&formatted, token.NewFileSet(), node); err != nil {
		t.Fatalf("format AST node: %v", err)
	}
	return formatted.String()
}

func flattenUnionNames(t *testing.T, expression ast.Expr) []string {
	t.Helper()
	switch expression := expression.(type) {
	case *ast.Ident:
		return []string{expression.Name}
	case *ast.BinaryExpr:
		if expression.Op != token.OR {
			t.Fatalf("wire union uses operator %s", expression.Op)
		}
		return append(flattenUnionNames(t, expression.X), flattenUnionNames(t, expression.Y)...)
	default:
		t.Fatalf("wire union contains %T", expression)
		return nil
	}
}

func readObjectDigests(t *testing.T, db *sql.DB) []objectDigest {
	t.Helper()

	rows, err := db.Query(`
SELECT type, name, tbl_name, sql
FROM sqlite_schema
WHERE name NOT LIKE 'sqlite_%'
ORDER BY type, name`)
	if err != nil {
		t.Fatalf("read schema objects: %v", err)
	}
	defer rows.Close()

	var objects []objectDigest
	for rows.Next() {
		var object objectDigest
		var definition string
		if err := rows.Scan(&object.kind, &object.name, &object.table, &definition); err != nil {
			t.Fatalf("scan schema object: %v", err)
		}
		normalized := strings.Join(strings.Fields(definition), " ")
		object.digest = fmt.Sprintf("%x", sha256.Sum256([]byte(normalized)))
		objects = append(objects, object)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate schema objects: %v", err)
	}
	return objects
}

func readColumns(t *testing.T, db *sql.DB, tableName string) []columnSpec {
	t.Helper()

	rows, err := db.Query(`PRAGMA table_xinfo(` + tableName + `)`)
	if err != nil {
		t.Fatalf("read %s columns: %v", tableName, err)
	}
	defer rows.Close()

	var columns []columnSpec
	for rows.Next() {
		var column columnSpec
		var cid int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &column.name, &column.dataType, &column.notNull, &defaultValue, &column.primaryKey, &column.hidden); err != nil {
			t.Fatalf("scan %s column: %v", tableName, err)
		}
		if defaultValue.Valid {
			t.Fatalf("%s.%s has unexpected default %q", tableName, column.name, defaultValue.String)
		}
		columns = append(columns, column)
	}
	return columns
}

func readIndexes(t *testing.T, db *sql.DB) []indexSpec {
	t.Helper()

	rows, err := db.Query(`PRAGMA index_list(continuity_facts)`)
	if err != nil {
		t.Fatalf("read fact indexes: %v", err)
	}
	defer rows.Close()

	var indexes []indexSpec
	for rows.Next() {
		var index indexSpec
		var sequence int
		if err := rows.Scan(&sequence, &index.name, &index.unique, &index.origin, &index.partial); err != nil {
			t.Fatalf("scan fact index: %v", err)
		}
		index.keyColumns = readIndexKeyColumns(t, db, index.name)
		indexes = append(indexes, index)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate fact indexes: %v", err)
	}
	sort.Slice(indexes, func(left, right int) bool { return indexes[left].name < indexes[right].name })
	return indexes
}

func readIndexKeyColumns(t *testing.T, db *sql.DB, indexName string) []string {
	t.Helper()

	rows, err := db.Query(`PRAGMA index_xinfo(` + indexName + `)`)
	if err != nil {
		t.Fatalf("read %s columns: %v", indexName, err)
	}
	defer rows.Close()

	var columns []string
	for rows.Next() {
		var sequence, cid, descending, key int
		var name sql.NullString
		var collation string
		if err := rows.Scan(&sequence, &cid, &name, &descending, &collation, &key); err != nil {
			t.Fatalf("scan %s column: %v", indexName, err)
		}
		if key == 1 {
			if !name.Valid {
				t.Fatalf("%s has unnamed key column", indexName)
			}
			columns = append(columns, name.String)
		}
	}
	return columns
}

func inspectSQLiteSource(t *testing.T) ([]string, []string) {
	t.Helper()

	entries, err := os.ReadDir(sqliteSourceRoot)
	if err != nil {
		t.Fatalf("read SQLite source root: %v", err)
	}
	var files []string
	var exports []string
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 {
			t.Errorf("%s is a symlink", entry.Name())
			continue
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		files = append(files, entry.Name())
		file := parseGoFile(t, filepath.Join(sqliteSourceRoot, entry.Name()))
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				for _, specification := range declaration.Specs {
					switch specification := specification.(type) {
					case *ast.TypeSpec:
						if !ast.IsExported(specification.Name.Name) {
							continue
						}
						exports = append(exports, specification.Name.Name)
						if structure, ok := specification.Type.(*ast.StructType); ok {
							publicData := specification.Name.Name != "Store"
							for _, field := range structure.Fields.List {
								if len(field.Names) == 0 {
									t.Errorf("%s embeds an anonymous field", specification.Name.Name)
									continue
								}
								for _, name := range field.Names {
									if ast.IsExported(name.Name) {
										if publicData {
											exports = append(exports, specification.Name.Name+"."+name.Name)
										} else {
											t.Errorf("%s.%s is an exported field", specification.Name.Name, name.Name)
										}
									}
								}
							}
						}
					case *ast.ValueSpec:
						for _, name := range specification.Names {
							if ast.IsExported(name.Name) {
								exports = append(exports, name.Name)
							}
						}
					}
				}
			case *ast.FuncDecl:
				if !ast.IsExported(declaration.Name.Name) {
					continue
				}
				name := declaration.Name.Name
				if declaration.Recv != nil && len(declaration.Recv.List) == 1 {
					name = receiverName(declaration.Recv.List[0].Type) + "." + name
				}
				exports = append(exports, name)
			}
		}
	}
	sort.Strings(files)
	sort.Strings(exports)
	return files, exports
}

func receiverName(expression ast.Expr) string {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression.Name
	case *ast.StarExpr:
		return receiverName(expression.X)
	default:
		return "unknown"
	}
}

func parseGoFile(t *testing.T, path string) *ast.File {
	t.Helper()

	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}
