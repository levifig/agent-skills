package vnextcontinuitycontract

import (
	"bytes"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
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
		"identity.go",
		"payloads.go",
		"semantics.go",
		"sqlite/admission.go",
		"sqlite/append_kernel.go",
		"sqlite/append_methods.go",
		"sqlite/codec_v1.go",
		"sqlite/driver.go",
		"sqlite/filesystem_attributes_windows.go",
		"sqlite/filesystem_unix.go",
		"sqlite/filesystem_windows.go",
		"sqlite/schema.go",
		"sqlite/store.go",
		"sqlite/wire_v1.go",
		"sqlite/wire_validation_v1.go",
		"validation.go",
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
	wantExports = append(wantExports,
		"AppendReceipt",
		"CheckpointCandidate", "CheckpointConstraint", "CheckpointEvidence", "CheckpointItem", "CheckpointItemKind", "CheckpointRecordedPayload",
		"DecisionOpenedPayload", "DecisionResolutionPayload", "DecisionSupersessionPayload",
		"ExplorationStartedPayload",
		"ExternalReferenceAttachmentPayload", "ExternalReferenceDetachmentPayload", "ExternalReferenceRegistrationPayload",
		"FindingContent", "FindingCorrectionPayload", "FindingRecordedPayload", "FindingRetractionPayload",
		"HandoffRecordedPayload", "HybridTime",
		"IdeaArchivePayload", "IdeaContent", "IdeaCreatedPayload", "IdeaPromotionPayload", "IdeaResolutionPayload", "IdeaRevisionPayload",
		"JournalBlock", "JournalCategory", "JournalCommit", "JournalContent", "JournalCorrectionPayload", "JournalDecision", "JournalDiscover", "JournalFinding", "JournalNote", "JournalRecordedPayload", "JournalSkill", "JournalSpark", "JournalTodo", "JournalUnblock", "JournalWrap",
		"NewEnvironmentID", "NewFactID", "NewProjectID", "NewSubjectID",
		"Observation",
		"Problem", "ProblemClockExhausted", "ProblemCode", "ProblemCommitUnknown", "ProblemCorruptFact", "ProblemFactConflict", "ProblemInvalid", "ProblemPreconditionFailed", "ProblemProjectAlreadyRegistered", "ProblemProjectNotRegistered", "ProblemReferenceMismatch", "ProblemReferenceNotFound", "ProblemStoreClosed", "ProblemStoreUnavailable", "ProblemSubjectAlreadyRegistered", "ProblemSubjectNotRegistered",
		"ProjectLabelRevisionPayload", "ProjectRegistrationPayload",
		"ScratchpadClaimPayload", "ScratchpadClaimReleasePayload", "ScratchpadClosePayload", "ScratchpadMessagePayload", "ScratchpadOpenedPayload", "ScratchpadParticipantPayload",
		"SparkCapturedPayload", "SparkDismissedPayload", "SparkPromotionPayload",
		"SubjectRef",
		"VerificationEvidencePayload", "VerificationFailed", "VerificationIndeterminate", "VerificationOutcome", "VerificationPassed",
		"WrapRecordedPayload",
	)
	sort.Strings(wantExports)

	production, exports := inspectContinuitySource(t)
	if strings.Join(production, "\n") != strings.Join(wantProduction, "\n") {
		t.Fatalf("production source inventory = %v, want %v", production, wantProduction)
	}
	if strings.Join(exports, "\n") != strings.Join(wantExports, "\n") {
		t.Fatalf("exported API = %v, want %v", exports, wantExports)
	}
}

func TestContinuityPayloadContractHasExactConcreteShapes(t *testing.T) {
	t.Parallel()

	want := map[string][]string{
		"AppendReceipt":                        {"FactID:FactID", "ProjectID:ProjectID", "Subject:SubjectRef", "Kind:FactKind", "EnvironmentID:EnvironmentID", "EnvironmentSequence:int64", "Clock:HybridTime", "Replayed:bool"},
		"CheckpointItem":                       {"Kind:CheckpointItemKind", "Text:string"},
		"CheckpointRecordedPayload":            {"Observation:Observation", "ExplorationID:SubjectID", "CurrentFraming:string", "Conclusions:string", "UnresolvedQuestion:string", "NextAction:string", "Items:[]CheckpointItem"},
		"DecisionOpenedPayload":                {"Observation:Observation", "Scope:string", "Question:string", "Context:string"},
		"DecisionResolutionPayload":            {"Observation:Observation", "Predecessor:FactID", "Resolution:string", "Rationale:string"},
		"DecisionSupersessionPayload":          {"Observation:Observation", "Predecessor:FactID", "SuccessorID:SubjectID", "Rationale:string"},
		"ExplorationStartedPayload":            {"Observation:Observation", "Label:string", "Purpose:string"},
		"ExternalReferenceAttachmentPayload":   {"Observation:Observation", "Target:SubjectRef", "Predecessor:FactID"},
		"ExternalReferenceDetachmentPayload":   {"Observation:Observation", "Target:SubjectRef", "Predecessor:FactID", "Reason:string"},
		"ExternalReferenceRegistrationPayload": {"Observation:Observation", "Locator:string"},
		"FindingContent":                       {"Scope:string", "Summary:string", "Detail:string", "Recommendation:string"},
		"FindingCorrectionPayload":             {"Observation:Observation", "Corrects:FactID", "Content:FindingContent"},
		"FindingRecordedPayload":               {"Observation:Observation", "Content:FindingContent"},
		"FindingRetractionPayload":             {"Observation:Observation", "Predecessor:FactID", "Reason:string"},
		"HandoffRecordedPayload":               {"Observation:Observation", "Focus:*SubjectRef", "Purpose:string", "Situation:string", "NextActions:string", "QuestionsAndRisks:string", "SuggestedSkills:[]string"},
		"HybridTime":                           {"WallMillis:int64", "Logical:int32"},
		"IdeaArchivePayload":                   {"Observation:Observation", "Predecessor:FactID", "Reason:string"},
		"IdeaContent":                          {"Label:string", "Text:string"},
		"IdeaCreatedPayload":                   {"Observation:Observation", "Content:IdeaContent"},
		"IdeaPromotionPayload":                 {"Observation:Observation", "Predecessor:FactID", "ReferenceID:SubjectID"},
		"IdeaResolutionPayload":                {"Observation:Observation", "Predecessor:FactID", "Resolution:string"},
		"IdeaRevisionPayload":                  {"Observation:Observation", "Revises:FactID", "Content:IdeaContent"},
		"JournalContent":                       {"Category:JournalCategory", "Scope:string", "Text:string"},
		"JournalCorrectionPayload":             {"Observation:Observation", "Corrects:FactID", "Content:JournalContent"},
		"JournalRecordedPayload":               {"Observation:Observation", "Content:JournalContent"},
		"Observation":                          {"ObservedAtMillis:int64", "HarnessSessionID:string", "Branch:string", "Worktree:string"},
		"Problem":                              {"Code:ProblemCode", "Field:string", "Detail:string"},
		"ProjectLabelRevisionPayload":          {"Observation:Observation", "Revises:FactID", "Label:string"},
		"ProjectRegistrationPayload":           {"Observation:Observation", "Label:string"},
		"ScratchpadClaimPayload":               {"Observation:Observation", "ClaimID:SubjectID", "ParticipantID:SubjectID", "Resource:string", "ExpiresAtMillis:int64"},
		"ScratchpadClaimReleasePayload":        {"Observation:Observation", "ClaimID:SubjectID", "ReleasedBy:SubjectID", "Reason:string"},
		"ScratchpadClosePayload":               {"Observation:Observation", "ClosedBy:SubjectID", "Reason:string"},
		"ScratchpadMessagePayload":             {"Observation:Observation", "ParticipantID:SubjectID", "Text:string"},
		"ScratchpadOpenedPayload":              {"Observation:Observation", "Focus:*SubjectRef", "Label:string"},
		"ScratchpadParticipantPayload":         {"Observation:Observation", "ParticipantID:SubjectID", "Name:string", "Focus:*SubjectRef"},
		"SparkCapturedPayload":                 {"Observation:Observation", "Scope:string", "Text:string"},
		"SparkDismissedPayload":                {"Observation:Observation", "Predecessor:FactID", "Reason:string"},
		"SparkPromotionPayload":                {"Observation:Observation", "Predecessor:FactID", "IdeaID:SubjectID"},
		"SubjectRef":                           {"Kind:RecordKind", "ID:SubjectID"},
		"VerificationEvidencePayload":          {"Observation:Observation", "Target:SubjectRef", "Check:string", "Method:string", "Outcome:VerificationOutcome", "Detail:string"},
		"WrapRecordedPayload":                  {"Observation:Observation", "Focus:*SubjectRef", "Scope:string", "Synthesis:string"},
	}

	got := inspectExportedStructShapes(t, filepath.Join(continuityRoot, "payloads.go"))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("payload struct shapes = %#v, want %#v", got, want)
	}
}

func TestContinuityDomainContractHasExactExportedMethods(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"*Problem.Error":                                "func() string",
		"CheckpointRecordedPayload.Validate":            "func() error",
		"DecisionOpenedPayload.Validate":                "func() error",
		"DecisionResolutionPayload.Validate":            "func() error",
		"DecisionSupersessionPayload.Validate":          "func() error",
		"EnvironmentID.Validate":                        "func() error",
		"ExplorationStartedPayload.Validate":            "func() error",
		"ExternalReferenceAttachmentPayload.Validate":   "func() error",
		"ExternalReferenceDetachmentPayload.Validate":   "func() error",
		"ExternalReferenceRegistrationPayload.Validate": "func() error",
		"FactID.Validate":                               "func() error",
		"FindingContent.Validate":                       "func() error",
		"FindingCorrectionPayload.Validate":             "func() error",
		"FindingRecordedPayload.Validate":               "func() error",
		"FindingRetractionPayload.Validate":             "func() error",
		"HandoffRecordedPayload.Validate":               "func() error",
		"IdeaArchivePayload.Validate":                   "func() error",
		"IdeaContent.Validate":                          "func() error",
		"IdeaCreatedPayload.Validate":                   "func() error",
		"IdeaPromotionPayload.Validate":                 "func() error",
		"IdeaResolutionPayload.Validate":                "func() error",
		"IdeaRevisionPayload.Validate":                  "func() error",
		"JournalContent.Validate":                       "func() error",
		"JournalCorrectionPayload.Validate":             "func() error",
		"JournalRecordedPayload.Validate":               "func() error",
		"Observation.Validate":                          "func() error",
		"ProjectID.Validate":                            "func() error",
		"ProjectLabelRevisionPayload.Validate":          "func() error",
		"ProjectRegistrationPayload.Validate":           "func() error",
		"ScratchpadClaimPayload.Validate":               "func() error",
		"ScratchpadClaimReleasePayload.Validate":        "func() error",
		"ScratchpadClosePayload.Validate":               "func() error",
		"ScratchpadMessagePayload.Validate":             "func() error",
		"ScratchpadOpenedPayload.Validate":              "func() error",
		"ScratchpadParticipantPayload.Validate":         "func() error",
		"SparkCapturedPayload.Validate":                 "func() error",
		"SparkDismissedPayload.Validate":                "func() error",
		"SparkPromotionPayload.Validate":                "func() error",
		"SubjectID.Validate":                            "func() error",
		"VerificationEvidencePayload.Validate":          "func() error",
		"WrapRecordedPayload.Validate":                  "func() error",
	}

	got := inspectExportedMethodSignatures(t)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("exported method signatures = %#v, want %#v", got, want)
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

func inspectExportedStructShapes(t *testing.T, path string) map[string][]string {
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
			if !ok || !ast.IsExported(typeSpecification.Name.Name) {
				continue
			}
			structure, ok := typeSpecification.Type.(*ast.StructType)
			if !ok {
				continue
			}
			var fields []string
			for _, field := range structure.Fields.List {
				if len(field.Names) == 0 {
					t.Fatalf("%s embeds an anonymous field", typeSpecification.Name.Name)
				}
				var formatted bytes.Buffer
				if err := format.Node(&formatted, fileSet, field.Type); err != nil {
					t.Fatalf("format %s field type: %v", typeSpecification.Name.Name, err)
				}
				fieldType := formatted.String()
				if field.Tag != nil {
					fieldType += ":" + field.Tag.Value
				}
				for _, name := range field.Names {
					fields = append(fields, name.Name+":"+fieldType)
				}
			}
			shapes[typeSpecification.Name.Name] = fields
		}
	}
	return shapes
}

func inspectExportedMethodSignatures(t *testing.T) map[string]string {
	t.Helper()

	methods := make(map[string]string)
	entries, err := os.ReadDir(continuityRoot)
	if err != nil {
		t.Fatalf("read continuity root: %v", err)
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(continuityRoot, entry.Name())
		fileSet := token.NewFileSet()
		file, err := parser.ParseFile(fileSet, path, nil, parser.AllErrors)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Recv == nil || !ast.IsExported(function.Name.Name) {
				continue
			}
			if len(function.Recv.List) != 1 {
				t.Fatalf("%s has %d receivers", function.Name.Name, len(function.Recv.List))
			}
			var receiver bytes.Buffer
			if err := format.Node(&receiver, fileSet, function.Recv.List[0].Type); err != nil {
				t.Fatalf("format %s receiver: %v", function.Name.Name, err)
			}
			var signature bytes.Buffer
			if err := format.Node(&signature, fileSet, function.Type); err != nil {
				t.Fatalf("format %s signature: %v", function.Name.Name, err)
			}
			key := receiver.String() + "." + function.Name.Name
			if _, duplicate := methods[key]; duplicate {
				t.Fatalf("duplicate exported method %s", key)
			}
			methods[key] = signature.String()
		}
	}
	return methods
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
