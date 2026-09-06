package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func TestTargetCapabilityEvidenceContractLoadsCurrentRecords(t *testing.T) {
	contract := loadTestTargetCapabilityEvidence(t)
	if contract.ContractVersion != TargetCapabilityEvidenceContractVersion {
		t.Fatalf("contract version = %d, want %d", contract.ContractVersion, TargetCapabilityEvidenceContractVersion)
	}
	if len(contract.Records) != 6 {
		t.Fatalf("records = %d, want six exact target surface records", len(contract.Records))
	}
	want := map[string]string{
		"claude-code\x00cli":     "2.1.226\x00plugin-dir",
		"cursor\x00ide":          "3.11.19\x00candidate-build",
		"cursor\x00cursor-agent": "2026.05.09-0afadcc\x00candidate-build",
		"codex\x00cli":           "0.147.0\x00isolated-codex-home",
		"opencode\x00cli":        "1.18.13\x00isolated-xdg",
		"amp\x00cli":             "0.0.1783873056-g278461\x00candidate-build",
	}
	for _, record := range contract.Records {
		key := strings.ToLower(record.Target) + "\x00" + record.Surface
		if got := record.Version + "\x00" + record.InstalledMode; got != want[key] {
			t.Errorf("%s version/mode = %q, want %q", key, got, want[key])
		}
		if record.Platform != "darwin-arm64" {
			t.Errorf("%s platform = %q, want darwin-arm64", key, record.Platform)
		}
		if record.Completion.Status != "unsupported" || strings.TrimSpace(record.Completion.Reason) == "" {
			t.Errorf("%s completion = %#v, want unsupported with reason", key, record.Completion)
		}
		if strings.TrimSpace(record.Completion.Evidence.Source) == "" {
			t.Errorf("%s completion evidence has no retained source", key)
		}
	}
	if len(contract.Deferred) != 1 || contract.Deferred[0].Target != "pi" || contract.Deferred[0].Status != "deferred" || !contract.Deferred[0].NotABuildTarget {
		t.Fatalf("deferred = %#v, want Pi only as not-a-build-target/deferred", contract.Deferred)
	}
}

func TestTargetCapabilityEvidenceCurrentModesAndTriggers(t *testing.T) {
	contract := loadTestTargetCapabilityEvidence(t)
	byIdentity := map[string]TargetCapabilityRecord{}
	for _, record := range contract.Records {
		byIdentity[record.Target+"\x00"+record.Surface] = record
	}
	claude := byIdentity["claude-code\x00cli"]
	if claude.Context.Adapter != "claude-session-start-v1" {
		t.Fatalf("Claude adapter = %q", claude.Context.Adapter)
	}
	claudeModes := modeEvidenceByName(claude.Context.Modes)
	if startup := claudeModes["startup"]; startup.Status != "supported" || !startup.ModelVisible || startup.Trigger != "SessionStart:startup" || startup.Evidence.Level != "installed-smoke" {
		t.Fatalf("Claude startup = %#v, want installed-smoke model-visible support", startup)
	}
	for _, name := range []string{"resume", "clear", "compact"} {
		mode := claudeModes[name]
		if mode.Status != "candidate" || mode.ModelVisible || mode.Trigger != "SessionStart:"+name || mode.Evidence.Source != "internal/cli/journal_hook_claude_test.go" || mode.Reason == "" {
			t.Fatalf("Claude %s = %#v, want candidate fixture evidence", name, mode)
		}
	}

	for _, identity := range []string{"cursor\x00ide", "cursor\x00cursor-agent"} {
		record := byIdentity[identity]
		modes := modeEvidenceByName(record.Context.Modes)
		if mode := modes["new-composer"]; mode.Status != "candidate" || mode.Trigger != "sessionStart" {
			t.Fatalf("%s new-composer = %#v, want sessionStart candidate", identity, mode)
		}
		for _, name := range []string{"resume", "compact", "cloud"} {
			if mode := modes[name]; mode.Status != "unsupported" || mode.Trigger != "" || mode.Reason == "" {
				t.Fatalf("%s %s = %#v, want explicit unsupported native-surface gap", identity, name, mode)
			}
		}
		if strings.Contains(strings.Join(record.BypassModes, "\x00"), "resume") || strings.Contains(strings.Join(record.BypassModes, "\x00"), "compact") || strings.Contains(strings.Join(record.BypassModes, "\x00"), "cloud") || strings.Contains(strings.Join(record.BypassModes, "\x00"), "background") || strings.Contains(strings.Join(record.BypassModes, "\x00"), "child") {
			t.Fatalf("%s bypass_modes duplicates runtime scopes: %#v", identity, record.BypassModes)
		}
	}

	codex := byIdentity["codex\x00cli"]
	codexModes := modeEvidenceByName(codex.Context.Modes)
	if mode := codexModes["startup"]; mode.Status != "supported" || !mode.ModelVisible || mode.Trigger != "SessionStart:startup" || mode.Evidence.Level != "installed-smoke" {
		t.Fatalf("Codex startup = %#v, want installed-smoke model-visible support", mode)
	}
	for _, name := range []string{"resume", "clear", "compact"} {
		mode := codexModes[name]
		if mode.Status != "candidate" || mode.ModelVisible || mode.Trigger != "SessionStart:"+name || mode.Evidence.Source == "" || mode.Reason == "" {
			t.Fatalf("Codex %s = %#v, want SessionStart candidate", name, mode)
		}
	}
	opencode := byIdentity["opencode\x00cli"]
	opencodeModes := modeEvidenceByName(opencode.Context.Modes)
	if mode := opencodeModes["request"]; mode.Trigger != "experimental.chat.system.transform" || mode.Status != "supported" || !mode.ModelVisible || mode.Evidence.Level != "installed-smoke" {
		t.Fatalf("OpenCode request = %#v, want installed-smoke model-visible support", mode)
	}
	if mode := opencodeModes["startup"]; mode.Trigger != "" || mode.Status != "unsupported" || mode.ModelVisible || mode.Reason == "" {
		t.Fatalf("OpenCode startup = %#v, want unsupported without distinct trigger", mode)
	}
	if mode := opencodeModes["resume"]; mode.Trigger != "experimental.chat.system.transform" || mode.Status != "candidate" || mode.ModelVisible || mode.Reason == "" {
		t.Fatalf("OpenCode resume = %#v, want transform candidate", mode)
	}
	if mode := opencodeModes["compact"]; mode.Trigger != "experimental.session.compacting" || mode.Status != "candidate" || mode.ModelVisible || mode.Reason == "" {
		t.Fatalf("OpenCode compact = %#v", mode)
	}
	amp := byIdentity["amp\x00cli"]
	ampModes := modeEvidenceByName(amp.Context.Modes)
	if mode := ampModes["foreground-turn"]; mode.Trigger != "agent.start" || mode.Status != "candidate" || mode.Reason != "root-only identity cannot be proven" {
		t.Fatalf("Amp foreground-turn = %#v", mode)
	}
	for _, name := range []string{"startup", "resume", "compact"} {
		if mode := ampModes[name]; mode.Status != "unsupported" || mode.Trigger != "" || mode.Reason == "" {
			t.Fatalf("Amp %s = %#v, want unsupported without native trigger", name, mode)
		}
	}
	if amp.Context.Adapter != "amp-plugin-v1" {
		t.Fatalf("Amp adapter = %q, want amp-plugin-v1", amp.Context.Adapter)
	}
}

func modeEvidenceByName(modes []ModeEvidence) map[string]ModeEvidence {
	result := make(map[string]ModeEvidence, len(modes))
	for _, mode := range modes {
		result[mode.Name] = mode
	}
	return result
}

func TestTargetCapabilityEvidenceHasNoAggregateContextClaims(t *testing.T) {
	typeOfContext := reflect.TypeOf(TargetContextCapabilityRecord{})
	for _, forbidden := range []string{"Status", "ModelVisible", "Evidence", "Events"} {
		if _, ok := typeOfContext.FieldByName(forbidden); ok {
			t.Fatalf("context retains forbidden aggregate field %s", forbidden)
		}
	}
	got, ok := typeOfContext.FieldByName("Modes")
	if !ok || got.Type != reflect.TypeOf([]ModeEvidence{}) {
		t.Fatalf("context Modes type = %v, want []ModeEvidence", got.Type)
	}
	data, err := os.ReadFile(testTargetCapabilityEvidencePath(t))
	if err != nil {
		t.Fatal(err)
	}
	var raw struct {
		Records []struct {
			Context map[string]json.RawMessage `json:"context"`
		} `json:"records"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for index, record := range raw.Records {
		for _, forbidden := range []string{"status", "model_visible", "evidence", "events"} {
			if _, ok := record.Context[forbidden]; ok {
				t.Errorf("records[%d] context has forbidden aggregate key %q", index, forbidden)
			}
		}
	}
}

func TestTargetCapabilityEvidenceRejectsUnknownFieldsAndTrailingJSON(t *testing.T) {
	data, err := os.ReadFile(testTargetCapabilityEvidencePath(t))
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["unknown"] = true
	unknown, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeTargetCapabilityEvidence(unknown); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("unknown field error = %v, want strict decoder rejection", err)
	}
	trailing := append(append([]byte(nil), data...), []byte("\n{}")...)
	if _, err := DecodeTargetCapabilityEvidence(trailing); err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("trailing JSON error = %v, want strict decoder rejection", err)
	}

	raw["contract_version"] = 1
	delete(raw, "unknown")
	oldVersion, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeTargetCapabilityEvidence(oldVersion); err == nil || !strings.Contains(err.Error(), "unsupported target capability contract version") {
		t.Fatalf("old version error = %v, want closed version gate", err)
	}
}

func TestTargetCapabilityEvidenceRejectsInvalidClaims(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*TargetCapabilityEvidenceContract)
		want   string
	}{
		{name: "duplicate identity", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records = append(contract.Records, contract.Records[0])
		}, want: "duplicate target/surface/version/platform/installed_mode identity"},
		{name: "missing supported target", mutate: func(contract *TargetCapabilityEvidenceContract) {
			filtered := make([]TargetCapabilityRecord, 0, len(contract.Records)-1)
			for _, record := range contract.Records {
				if record.Target != "amp" {
					filtered = append(filtered, record)
				}
			}
			contract.Records = filtered
		}, want: "no record for supported target"},
		{name: "missing deferred pi", mutate: func(contract *TargetCapabilityEvidenceContract) { contract.Deferred = nil }, want: "deferred pi boundary"},
		{name: "missing platform", mutate: func(contract *TargetCapabilityEvidenceContract) { contract.Records[0].Platform = "" }, want: "exact canonical platform"},
		{name: "duplicate mode names", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records[0].Context.Modes = append(contract.Records[0].Context.Modes, contract.Records[0].Context.Modes[0])
		}, want: "duplicate mode name"},
		{name: "wildcard version", mutate: func(contract *TargetCapabilityEvidenceContract) { contract.Records[0].Version = "2.x" }, want: "exact tested version"},
		{name: "missing fallback", mutate: func(contract *TargetCapabilityEvidenceContract) { contract.Records[0].Fallback = "" }, want: "fallback is required"},
		{name: "arbitrary installed mode", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records[0].InstalledMode = "installed-runtime"
		}, want: "not a supported artifact-loading mode"},
		{name: "missing completion reason", mutate: func(contract *TargetCapabilityEvidenceContract) { contract.Records[0].Completion.Reason = "" }, want: "reason is required"},
		{name: "supported mode not visible", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records[0].Context.Modes[0].ModelVisible = false
		}, want: "supported mode requires model_visible=true"},
		{name: "supported mode wrong evidence", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records[0].Context.Modes[0].Evidence.Level = "source"
		}, want: "supported mode requires installed-smoke evidence"},
		{name: "supported mode has reason", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records[0].Context.Modes[0].Reason = "not allowed"
		}, want: "supported mode must not include reason"},
		{name: "candidate mode visible", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records[1].Context.Modes[0].ModelVisible = true
		}, want: "candidate mode requires model_visible=false"},
		{name: "candidate mode missing reason", mutate: func(contract *TargetCapabilityEvidenceContract) { contract.Records[1].Context.Modes[0].Reason = "" }, want: "candidate mode requires reason"},
		{name: "candidate mode missing trigger", mutate: func(contract *TargetCapabilityEvidenceContract) { contract.Records[1].Context.Modes[0].Trigger = "" }, want: "candidate mode requires trigger"},
		{name: "candidate mode wrong evidence", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records[1].Context.Modes[0].Evidence.Level = "installed-smoke"
		}, want: "candidate mode requires source, fixture, or candidate-build evidence"},
		{name: "unsupported mode visible", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records[1].Context.Modes[1].ModelVisible = true
		}, want: "unsupported mode requires model_visible=false"},
		{name: "unsupported mode missing reason", mutate: func(contract *TargetCapabilityEvidenceContract) { contract.Records[1].Context.Modes[1].Reason = "" }, want: "unsupported mode requires reason"},
		{name: "missing evidence source", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records[0].Context.Modes[0].Evidence.Source = ""
		}, want: "evidence source is required"},
		{name: "missing evidence summary", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records[0].Context.Modes[0].Evidence.Summary = ""
		}, want: "evidence summary is required"},
		{name: "unknown evidence level", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records[0].Context.Modes[0].Evidence.Level = "unverified"
		}, want: "unsupported evidence level"},
		{name: "noncanonical mode", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records[0].Context.Modes[0].Name = "Startup"
		}, want: "canonical lower-kebab"},
		{name: "root mode", mutate: func(contract *TargetCapabilityEvidenceContract) { contract.Records[0].Context.Modes[0].Name = "root" }, want: "root is a policy scope"},
		{name: "candidate build support", mutate: func(contract *TargetCapabilityEvidenceContract) {
			contract.Records[1].Context.Modes[0].Status = "supported"
			contract.Records[1].Context.Modes[0].ModelVisible = true
			contract.Records[1].Context.Modes[0].Evidence.Level = "installed-smoke"
		}, want: "candidate-build installed mode cannot claim a supported mode"},
		{name: "runtime bypass", mutate: func(contract *TargetCapabilityEvidenceContract) { contract.Records[1].BypassModes[0] = "resume" }, want: "duplicates runtime mode"},
		{name: "supported completion", mutate: func(contract *TargetCapabilityEvidenceContract) { contract.Records[0].Completion.Status = "supported" }, want: "completion status must be unsupported"},
		{name: "pi record", mutate: func(contract *TargetCapabilityEvidenceContract) { contract.Records[0].Target = "pi" }, want: "pi is deferred"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			contract := loadTestTargetCapabilityEvidence(t)
			tt.mutate(&contract)
			if err := ValidateTargetCapabilityEvidence(contract); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("validation error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestTargetCapabilityEvidenceRejectsEveryCompletionClaimExceptUnsupported(t *testing.T) {
	for _, status := range []string{"candidate", "supported", "unverified"} {
		t.Run(status, func(t *testing.T) {
			contract := loadTestTargetCapabilityEvidence(t)
			contract.Records[0].Completion.Status = status
			if err := ValidateTargetCapabilityEvidence(contract); err == nil || !strings.Contains(err.Error(), "completion status must be unsupported") {
				t.Fatalf("validation error = %v, want permanent unsupported completion rejection", err)
			}
		})
	}
}

func TestTargetCapabilityEvidenceRejectsReservedExecutionScopesAsModes(t *testing.T) {
	for _, name := range []string{"root", "background", "child", "subagent", "delegated"} {
		t.Run(name, func(t *testing.T) {
			contract := loadTestTargetCapabilityEvidence(t)
			contract.Records[1].Context.Modes[0].Name = name
			if err := ValidateTargetCapabilityEvidence(contract); err == nil || !strings.Contains(err.Error(), "policy scope") {
				t.Fatalf("validation error = %v, want reserved policy-scope rejection", err)
			}
		})
	}
}

func TestTargetCapabilityEvidenceRejectsRuntimeModesHiddenInBypassPhrases(t *testing.T) {
	for _, bypass := range []string{"resume with hooks disabled", "background-agent", "child execution", "delegated worker", "resume-with-hooks-disabled", "new composer", "new_composer", "new-composer"} {
		t.Run(strings.ReplaceAll(bypass, " ", "_"), func(t *testing.T) {
			contract := loadTestTargetCapabilityEvidence(t)
			contract.Records[1].BypassModes[0] = bypass
			if err := ValidateTargetCapabilityEvidence(contract); err == nil || !strings.Contains(err.Error(), "duplicates runtime mode") {
				t.Fatalf("validation error = %v, want hidden runtime-mode collision", err)
			}
		})
	}
}

func TestTargetCapabilityEvidenceLoadRequiresRetainedRegularSources(t *testing.T) {
	root := testRepositoryRoot(t)
	for name, source := range map[string]string{
		"absolute":  filepath.Join(t.TempDir(), "evidence.md"),
		"traversal": "../outside.md",
		"missing":   "docs/changes/20260710-journal-reliability-foundation/research/missing.md",
		"directory": "internal/cli",
		"anchor":    "docs/changes/20260710-journal-reliability-foundation/research/target-capability-survey.md#survey",
	} {
		t.Run(name, func(t *testing.T) {
			err := validateEvidenceSourceFile(root, source)
			if name == "anchor" {
				if err != nil {
					t.Fatalf("validateEvidenceSourceFile() error = %v, want anchor accepted", err)
				}
			} else if err == nil {
				t.Fatalf("validateEvidenceSourceFile() error = nil, want source rejection for %q", source)
			}
		})
	}
	t.Run("symlink-escape", func(t *testing.T) {
		outside := filepath.Join(t.TempDir(), "evidence.md")
		if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		relativeLink := filepath.Join("internal", "cli", ".capability-source-symlink")
		link := filepath.Join(root, relativeLink)
		if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(outside, link); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(link)
		if err := validateEvidenceSourceFile(root, filepath.ToSlash(relativeLink)); err == nil {
			t.Fatal("validateEvidenceSourceFile() = nil, want symlink source rejection")
		}
	})
	t.Run("intermediate-symlink-escape", func(t *testing.T) {
		outsideDir := t.TempDir()
		outside := filepath.Join(outsideDir, "evidence.md")
		if err := os.WriteFile(outside, []byte("outside"), 0o600); err != nil {
			t.Fatal(err)
		}
		relativeLink := "internal/.capability-source-linkdir"
		link := filepath.Join(root, filepath.FromSlash(relativeLink))
		if err := os.Symlink(outsideDir, link); err != nil {
			t.Fatal(err)
		}
		defer os.Remove(link)
		if err := validateEvidenceSourceFile(root, filepath.ToSlash(filepath.Join(relativeLink, "evidence.md"))); err == nil {
			t.Fatal("validateEvidenceSourceFile() = nil, want intermediate symlink source rejection")
		}
	})
}

func TestExactCapabilityVersionGrammar(t *testing.T) {
	valid := []string{"2.1.220", "3.11.19", "2026.05.09-0afadcc", "0.145.0", "1.18.7", "0.0.1783873056-g278461", "1.2.3-alpha9"}
	for _, version := range valid {
		if !isExactCapabilityVersion(version) {
			t.Errorf("isExactCapabilityVersion(%q) = false, want true", version)
		}
	}
	invalid := []string{"1.2.?", "unknown", "TBD", "latest", "current", "1.2.3 || 1.2.4", "1.*", "1.2.x", " 1.2.3", "1.2.3 ", "1.2.3/4", ">=1.2.3"}
	for _, version := range invalid {
		if isExactCapabilityVersion(version) {
			t.Errorf("isExactCapabilityVersion(%q) = true, want false", version)
		}
	}
}

func TestTargetCapabilityEvidenceIsRecordOnly(t *testing.T) {
	data, err := os.ReadFile(testTargetCapabilityEvidencePath(t))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(`"dispatcher"`)) || bytes.Contains(data, []byte(`"renderer"`)) || bytes.Contains(data, []byte(`"command"`)) {
		t.Fatalf("capability evidence contains behavior-dispatch fields: %s", data)
	}
}

func loadTestTargetCapabilityEvidence(t *testing.T) TargetCapabilityEvidenceContract {
	t.Helper()
	data, err := os.ReadFile(testTargetCapabilityEvidencePath(t))
	if err != nil {
		t.Fatal(err)
	}
	contract, err := DecodeTargetCapabilityEvidence(data)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func testTargetCapabilityEvidencePath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(filename), "..", "..", "config", "target-capabilities.json")
}

func testRepositoryRoot(t *testing.T) string {
	t.Helper()
	return filepath.Dir(filepath.Dir(testTargetCapabilityEvidencePath(t)))
}

func openCodeTestRecord(t *testing.T) TargetCapabilityRecord {
	return capabilityTestRecord(t, "opencode", "cli")
}

func capabilityTestRecord(t *testing.T, target, surface string) TargetCapabilityRecord {
	t.Helper()
	data, err := os.ReadFile(testTargetCapabilityEvidencePath(t))
	if err != nil {
		t.Fatal(err)
	}
	var contract TargetCapabilityEvidenceContract
	if err := json.Unmarshal(data, &contract); err != nil {
		t.Fatal(err)
	}
	for _, record := range contract.Records {
		if record.Target == target && record.Surface == surface {
			return record
		}
	}
	t.Fatalf("%s/%s capability record is missing", target, surface)
	return TargetCapabilityRecord{}
}

func readSmokeReceiptRaw(t *testing.T, path string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return raw
}

func writeSmokeReceiptFixture(t *testing.T, root string, raw map[string]any, sourceNativePath string, nativeContent []byte) {
	t.Helper()
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "receipt.json"), encoded, 0o600); err != nil {
		t.Fatal(err)
	}
	artifacts := raw["candidate_artifacts"].(map[string]any)
	for _, artifact := range []string{"hooks_path", "native_binary_path"} {
		destination := artifacts[artifact].(string)
		path := filepath.Join(root, filepath.FromSlash(destination))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		content := nativeContent
		if artifact == "hooks_path" {
			original := filepath.Join(testRepositoryRoot(t), filepath.FromSlash(destination))
			var err error
			content, err = os.ReadFile(original)
			if err != nil {
				t.Fatal(err)
			}
		} else if sourceNativePath == "" {
			t.Fatal("source native path is required")
		}
		if err := os.WriteFile(path, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestTargetCapabilityEvidenceJSONRoundTripKeepsStrictShape(t *testing.T) {
	contract := loadTestTargetCapabilityEvidence(t)
	data, err := json.Marshal(contract)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DecodeTargetCapabilityEvidence(data); err != nil {
		t.Fatalf("round-trip validation error = %v", err)
	}
}

// TestValidateInstalledSmokeEvidencePinsHooksShimAndCandidateBinary pins the
// version-3 Claude receipt: the plugin ships a shim instead of a native
// binary, so the receipt must name the shim beside the hooks and pin the
// candidate binary the shim resolved through LOAF_BIN, and every digest must
// match the current candidate tree.
func TestValidateInstalledSmokeEvidencePinsHooksShimAndCandidateBinary(t *testing.T) {
	record := capabilityTestRecord(t, "claude-code", "cli")
	var mode ModeEvidence
	for _, candidate := range record.Context.Modes {
		if candidate.Name == "startup" {
			mode = candidate
		}
	}
	if mode.Name != "startup" {
		t.Fatal("claude-code startup mode is missing from the capability record")
	}
	mode.Evidence.Source = "receipt.json"

	root := t.TempDir()
	hooks, err := os.ReadFile(filepath.Join(testRepositoryRoot(t), "plugins", "loaf", "hooks", "hooks.json"))
	if err != nil {
		t.Fatal(err)
	}
	nativeRel := "bin/native/" + record.Platform + "/loaf"
	writeReceiptArtifact(t, root, "plugins/loaf/hooks/hooks.json", hooks)
	writeReceiptArtifact(t, root, "plugins/loaf/bin/loaf", []byte(claudePluginShim))
	writeReceiptArtifact(t, root, nativeRel, []byte("candidate binary\n"))

	receipt := func() map[string]any {
		return map[string]any{
			"evidence_version": 3,
			"timestamp":        "2026-09-06T00:00:00Z",
			"target":           record.Target,
			"surface":          record.Surface,
			"version":          record.Version,
			"platform":         record.Platform,
			"installed_mode":   record.InstalledMode,
			"context_mode":     mode.Name,
			"adapter":          record.Context.Adapter,
			"mode":             "explicit-plugin-dir",
			"invocation": map[string]any{
				"command": "claude",
				"args": []string{
					"--plugin-dir", "<repo>/plugins/loaf",
					"--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`,
					"--no-session-persistence", "--setting-sources", "", "--tools", "",
					"--include-hook-events", "--output-format", "stream-json", "-p",
					"Reply with exactly the unique marker present in Loaf continuity context, and nothing else.",
				},
				"cwd": "<disposable-repo>",
			},
			"setup":                         []string{"build candidate", "point LOAF_BIN at the candidate binary for the plugin shim"},
			"candidate_plugin_path":         "plugins/loaf",
			"exit_code":                     0,
			"stderr_empty":                  true,
			"model_visible_marker_observed": true,
			"assistant_marker_match":        true,
			"marker":                        "LOAF_CLAUDE_STARTUP_SMOKE_ABCDEF123456",
			"hook_observation": map[string]any{
				"event_name":                "SessionStart:startup",
				"native_json":               true,
				"hook_event_name":           "SessionStart",
				"additional_context_marker": true,
			},
			"candidate_artifacts": map[string]any{
				"hooks_path":           "plugins/loaf/hooks/hooks.json",
				"hooks_sha256":         sha256Hex(string(hooks)),
				"shim_path":            "plugins/loaf/bin/loaf",
				"shim_sha256":          sha256Hex(claudePluginShim),
				"native_binary_path":   nativeRel,
				"native_binary_sha256": sha256Hex("candidate binary\n"),
			},
		}
	}
	validate := func(t *testing.T, raw map[string]any) error {
		t.Helper()
		encoded, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "receipt.json"), encoded, 0o600); err != nil {
			t.Fatal(err)
		}
		return validateInstalledSmokeEvidence(root, record, mode)
	}

	if err := validate(t, receipt()); err != nil {
		t.Fatalf("valid version-3 receipt rejected: %v", err)
	}

	stale := receipt()
	stale["evidence_version"] = 2
	if err := validate(t, stale); err == nil || !strings.Contains(err.Error(), "version 2") {
		t.Fatalf("version-2 receipt error = %v, want an unsupported-version refusal", err)
	}

	wrongShim := receipt()
	wrongShim["candidate_artifacts"].(map[string]any)["shim_path"] = "plugins/loaf/bin/native/" + record.Platform + "/loaf"
	if err := validate(t, wrongShim); err == nil || !strings.Contains(err.Error(), "shim path") {
		t.Fatalf("wrong shim path error = %v, want a shim path refusal", err)
	}

	oldNativePath := receipt()
	oldNativePath["candidate_artifacts"].(map[string]any)["native_binary_path"] = "plugins/loaf/bin/native/" + record.Platform + "/loaf"
	if err := validate(t, oldNativePath); err == nil || !strings.Contains(err.Error(), "native binary path") {
		t.Fatalf("plugin-native path error = %v, want the candidate bin/native path to be required", err)
	}

	// The shim on disk changes after the receipt was recorded: the digest no
	// longer matches the candidate and the receipt is stale.
	writeReceiptArtifact(t, root, "plugins/loaf/bin/loaf", []byte(claudePluginShim+"# edited\n"))
	if err := validate(t, receipt()); err == nil || !strings.Contains(err.Error(), "shim SHA-256") {
		t.Fatalf("edited shim error = %v, want a shim digest mismatch", err)
	}
}

func writeReceiptArtifact(t *testing.T, root string, rel string, body []byte) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o755); err != nil {
		t.Fatal(err)
	}
}
