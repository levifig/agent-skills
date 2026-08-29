package vnextcontinuitycontract

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const continuityRoot = "../../vnext/continuity"

func TestContinuityDomainContractHasExactSourceAndExports(t *testing.T) {
	t.Parallel()

	wantProduction := []string{
		"facts.go",
		"semantics.go",
		"sqlite/driver.go",
		"sqlite/filesystem_attributes_windows.go",
		"sqlite/filesystem_unix.go",
		"sqlite/filesystem_windows.go",
		"sqlite/schema.go",
		"sqlite/store.go",
	}
	wantExports := []string{
		"Catalog",
		"ContextAttachedOnly", "ContextExcluded", "ContextFocusActive", "ContextFocusAndProjectWrap", "ContextFocusDecision", "ContextFocusFinding", "ContextFocusThenProjectJournal", "ContextInclusion", "ContextLatestCheckpoint", "ContextLatestFocusHandoff", "ContextOutput", "ContextProject",
		"DefinitionFor", "EnvironmentID",
		"FactCatalog", "FactCheckpointRecorded", "FactDecisionOpened", "FactDecisionResolved", "FactDecisionSuperseded", "FactDefinition", "FactExplorationStarted", "FactExternalReferenceAttached", "FactExternalReferenceDetached", "FactExternalReferenceRegistered", "FactFindingCorrected", "FactFindingRecorded", "FactFindingRetracted", "FactHandoffRecorded", "FactID", "FactIdeaArchived", "FactIdeaCreated", "FactIdeaPromotedToExternalReference", "FactIdeaResolved", "FactIdeaRevised", "FactJournalCorrectionRecorded", "FactJournalRecorded", "FactKind", "FactProjectLabelRevised", "FactProjectRegistered", "FactScratchpadClaimRecorded", "FactScratchpadClaimReleased", "FactScratchpadClosed", "FactScratchpadMessageRecorded", "FactScratchpadOpened", "FactScratchpadParticipantIntroduced", "FactSparkCaptured", "FactSparkDismissed", "FactSparkPromotedToIdea", "FactVerificationEvidenceRecorded", "FactWrapRecorded",
		"Mutation", "MutationAdvisoryFacts", "MutationAppendOnly", "MutationAppendWithAttachmentChanges", "MutationAppendWithCorrectionRetraction", "MutationAppendWithCorrections", "MutationAppendWithDisposition", "MutationAppendWithResolutionSupersession", "MutationAppendWithRevisionDispositionPromotion", "MutationMintOnce", "MutationMintOnceWithLabelRevisions", "MutationReadTimeFold",
		"Permanence", "PermanenceDerived", "PermanenceLedger", "PermanenceNotebook", "PermanenceScratchpad",
		"Projection", "ProjectionActiveSpark", "ProjectionContextDigest", "ProjectionCoordinationState", "ProjectionCurrentDecision", "ProjectionCurrentFinding", "ProjectionCurrentIdea", "ProjectionCurrentIdentity", "ProjectionEffectiveTimeline", "ProjectionEvidenceLedger", "ProjectionExplorationIdentity", "ProjectionLatestCheckpoint", "ProjectionLatestHandoff", "ProjectionLatestWrap", "ProjectionReferenceAttachments",
		"ProjectID",
		"RecordCheckpoint", "RecordDecision", "RecordDerivedContext", "RecordExploration", "RecordExternalReference", "RecordFinding", "RecordHandoff", "RecordIdea", "RecordJournalEntry", "RecordKind", "RecordProjectIdentity", "RecordScratchpad", "RecordSemantics", "RecordSpark", "RecordVerificationEvidence", "RecordWrap",
		"ReferenceBehavior", "ReferencesEphemeralOpaqueIDs", "ReferencesEvidenceLeaf", "ReferencesIncludeAttached", "ReferencesMayAttachOpaque", "ReferencesOpaqueLeaf",
		"Retention", "RetentionForever", "RetentionRecomputed", "RetentionUntilSyncSafePoint",
		"SemanticsFor",
		"SubjectID",
	}
	sort.Strings(wantExports)

	production, exports := inspectContinuitySource(t)
	if strings.Join(production, "\n") != strings.Join(wantProduction, "\n") {
		t.Fatalf("production source inventory = %v, want %v", production, wantProduction)
	}
	if strings.Join(exports, "\n") != strings.Join(wantExports, "\n") {
		t.Fatalf("exported API = %v, want %v", exports, wantExports)
	}
}

func TestContinuityDomainContractRejectsMutableAndDynamicSurfaces(t *testing.T) {
	t.Parallel()

	entries, err := os.ReadDir(continuityRoot)
	if err != nil {
		t.Fatalf("read continuity root: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(continuityRoot, entry.Name())
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("read %s: %v", entry.Name(), err)
			continue
		}
		lower := strings.ToLower(string(contents))
		for _, forbidden := range []string{
			"json.rawmessage", " provider", " credential", " secret", " token",
			" tracker", " linear", " jira", " assignment", " hierarchy", " dependency", " work-item",
		} {
			if strings.Contains(lower, forbidden) {
				t.Errorf("%s contains forbidden dynamic or external authority surface %q", entry.Name(), strings.TrimSpace(forbidden))
			}
		}

		file, err := parser.ParseFile(token.NewFileSet(), path, contents, parser.AllErrors)
		if err != nil {
			t.Errorf("parse %s: %v", entry.Name(), err)
			continue
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				if declaration.Tok == token.VAR {
					t.Errorf("%s declares package variable state", entry.Name())
				}
			case *ast.FuncDecl:
				if declaration.Name.Name == "init" {
					t.Errorf("%s declares init; the catalog must not register dynamically", entry.Name())
				}
			}
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				for _, specification := range declaration.Specs {
					if specification, ok := specification.(*ast.TypeSpec); ok && ast.IsExported(specification.Name.Name) {
						assertNoDynamicExport(t, entry.Name(), specification.Type)
					}
				}
			case *ast.FuncDecl:
				if ast.IsExported(declaration.Name.Name) {
					assertNoDynamicExport(t, entry.Name(), declaration.Type)
				}
			}
		}
	}
}

func assertNoDynamicExport(t *testing.T, fileName string, node ast.Node) {
	t.Helper()

	ast.Inspect(node, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.MapType, *ast.InterfaceType:
			t.Errorf("%s exports a map or interface escape hatch", fileName)
		case *ast.Ident:
			if node.Name == "any" {
				t.Errorf("%s exports an any escape hatch", fileName)
			}
		}
		return true
	})
}

func TestContinuityContractOracleIsTestOnlyAndStandardLibrary(t *testing.T) {
	t.Parallel()

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
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, parser.ImportsOnly)
		if err != nil {
			t.Errorf("parse %s: %v", entry.Name(), err)
			continue
		}
		for _, spec := range file.Imports {
			importPath, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Errorf("unquote import %s in %s: %v", spec.Path.Value, entry.Name(), err)
				continue
			}
			if strings.Contains(strings.SplitN(importPath, "/", 2)[0], ".") {
				t.Errorf("%s imports non-standard package %q", entry.Name(), importPath)
			}
		}
	}
}

func inspectContinuitySource(t *testing.T) ([]string, []string) {
	t.Helper()

	var production []string
	var exports []string
	err := filepath.WalkDir(continuityRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(continuityRoot, path)
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			t.Errorf("%s is a symlink", filepath.ToSlash(relative))
			return nil
		}
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			return nil
		}
		production = append(production, filepath.ToSlash(relative))
		if filepath.Dir(relative) != "." {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.AllErrors)
		if err != nil {
			return err
		}
		for _, declaration := range file.Decls {
			switch declaration := declaration.(type) {
			case *ast.GenDecl:
				for _, specification := range declaration.Specs {
					switch specification := specification.(type) {
					case *ast.TypeSpec:
						if ast.IsExported(specification.Name.Name) {
							exports = append(exports, specification.Name.Name)
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
				if declaration.Recv == nil && ast.IsExported(declaration.Name.Name) {
					exports = append(exports, declaration.Name.Name)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("inspect continuity source: %v", err)
	}
	sort.Strings(production)
	sort.Strings(exports)
	return production, exports
}
