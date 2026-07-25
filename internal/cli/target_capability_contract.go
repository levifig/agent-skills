package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// TargetCapabilityEvidenceRecord is retained source evidence for one claim.
// It is passive data: it is not a hook implementation or dispatch instruction.
type TargetCapabilityEvidenceRecord struct {
	Source  string `json:"source"`
	Level   string `json:"level"`
	Summary string `json:"summary"`
}

// TargetCapabilitySmokeEvidence is the strict, structured receipt emitted by
// the reproducible installed-smoke runner. It authenticates only one exact
// target/surface/version/mode claim and intentionally carries no transcript.
type TargetCapabilitySmokeEvidence struct {
	EvidenceVersion            int                             `json:"evidence_version"`
	Timestamp                  string                          `json:"timestamp"`
	Target                     string                          `json:"target"`
	Surface                    string                          `json:"surface"`
	Version                    string                          `json:"version"`
	Platform                   string                          `json:"platform"`
	InstalledMode              string                          `json:"installed_mode"`
	ContextMode                string                          `json:"context_mode"`
	Adapter                    string                          `json:"adapter"`
	Mode                       string                          `json:"mode"`
	Invocation                 TargetCapabilitySmokeInvocation `json:"invocation"`
	Setup                      []string                        `json:"setup"`
	CandidatePluginPath        string                          `json:"candidate_plugin_path"`
	FailureReason              string                          `json:"failure_reason,omitempty"`
	ExitCode                   int                             `json:"exit_code"`
	StderrEmpty                bool                            `json:"stderr_empty"`
	ModelVisibleMarkerObserved bool                            `json:"model_visible_marker_observed"`
	AssistantMarkerMatch       bool                            `json:"assistant_marker_match"`
	Marker                     string                          `json:"marker"`
	HookObservation            TargetCapabilitySmokeHook       `json:"hook_observation"`
	CandidateArtifacts         TargetCapabilitySmokeArtifacts  `json:"candidate_artifacts"`
}

type TargetCapabilitySmokeInvocation struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
	CWD     string   `json:"cwd"`
}

type TargetCapabilitySmokeHook struct {
	EventName               string `json:"event_name"`
	NativeJSON              bool   `json:"native_json"`
	HookEventName           string `json:"hook_event_name"`
	AdditionalContextMarker bool   `json:"additional_context_marker"`
}

type TargetCapabilitySmokeArtifacts struct {
	HooksPath          string `json:"hooks_path"`
	HooksSHA256        string `json:"hooks_sha256"`
	NativeBinaryPath   string `json:"native_binary_path"`
	NativeBinarySHA256 string `json:"native_binary_sha256"`
}

// TargetCapabilityCodexSmokeEvidence is the sanitized receipt for the
// isolated CODEX_HOME Codex SessionStart proof. It is deliberately separate
// from Claude's plugin-dir receipt because the runtimes have different
// invocation and artifact contracts.
type TargetCapabilityCodexSmokeEvidence struct {
	EvidenceVersion            int                             `json:"evidence_version"`
	Timestamp                  string                          `json:"timestamp"`
	Target                     string                          `json:"target"`
	Surface                    string                          `json:"surface"`
	Version                    string                          `json:"version"`
	Platform                   string                          `json:"platform"`
	InstalledMode              string                          `json:"installed_mode"`
	ContextMode                string                          `json:"context_mode"`
	Adapter                    string                          `json:"adapter"`
	Mode                       string                          `json:"mode"`
	Invocation                 TargetCapabilitySmokeInvocation `json:"invocation"`
	Setup                      []string                        `json:"setup"`
	ExitCode                   int                             `json:"exit_code"`
	StderrEmpty                bool                            `json:"stderr_empty"`
	Stderr                     string                          `json:"stderr,omitempty"`
	ModelVisibleMarkerObserved bool                            `json:"model_visible_marker_observed"`
	AssistantMarkerMatch       bool                            `json:"assistant_marker_match"`
	Marker                     string                          `json:"marker"`
	HookObservation            TargetCapabilitySmokeHook       `json:"hook_observation"`
	CandidateArtifacts         TargetCapabilitySmokeArtifacts  `json:"candidate_artifacts"`
}

// TargetCapabilityOpenCodeSmokeEvidence is the strict receipt for the
// isolated-XDG OpenCode request smoke. It intentionally has its own shape:
// OpenCode exposes plugin result and identity observations that are not part
// of the Claude or Codex hook receipts.
type TargetCapabilityOpenCodeSmokeEvidence struct {
	EvidenceVersion            int                             `json:"evidence_version"`
	Timestamp                  string                          `json:"timestamp"`
	Target                     string                          `json:"target"`
	Surface                    string                          `json:"surface"`
	Version                    string                          `json:"version"`
	Platform                   string                          `json:"platform"`
	InstalledMode              string                          `json:"installed_mode"`
	ContextMode                string                          `json:"context_mode"`
	Adapter                    string                          `json:"adapter"`
	Mode                       string                          `json:"mode"`
	Invocation                 TargetCapabilitySmokeInvocation `json:"invocation"`
	Setup                      []string                        `json:"setup"`
	ExitCode                   int                             `json:"exit_code"`
	StderrEmpty                bool                            `json:"stderr_empty"`
	Stderr                     string                          `json:"stderr"`
	FailureReason              string                          `json:"failure_reason,omitempty"`
	ModelVisibleMarkerObserved bool                            `json:"model_visible_marker_observed"`
	AssistantMarkerMatch       bool                            `json:"assistant_marker_match"`
	PluginLoaded               bool                            `json:"plugin_loaded"`
	RootSessionLookupProven    bool                            `json:"root_session_lookup_proven"`
	NoAuthSupplied             bool                            `json:"no_auth_supplied"`
	CleanupSucceeded           bool                            `json:"cleanup_succeeded"`
	Marker                     string                          `json:"marker"`
	CandidateArtifacts         TargetCapabilitySmokeArtifacts  `json:"candidate_artifacts"`
}

// ModeEvidence records one independently classified context-delivery mode.
// Trigger is omitted only when an unsupported mode has no native trigger.
type ModeEvidence struct {
	Name         string                         `json:"name"`
	Trigger      string                         `json:"trigger,omitempty"`
	Status       string                         `json:"status"`
	ModelVisible bool                           `json:"model_visible"`
	Reason       string                         `json:"reason,omitempty"`
	Evidence     TargetCapabilityEvidenceRecord `json:"evidence"`
}

// TargetContextCapabilityRecord records model-context delivery policies and
// independently classified modes. Aggregate status/evidence claims are
// intentionally absent; support belongs to each mode.
type TargetContextCapabilityRecord struct {
	Adapter          string         `json:"adapter"`
	Mechanism        string         `json:"mechanism"`
	RootPolicy       string         `json:"root_policy"`
	SubagentPolicy   string         `json:"subagent_policy"`
	BackgroundPolicy string         `json:"background_policy"`
	Modes            []ModeEvidence `json:"modes"`
}

// TargetCompletionCapabilityRecord records automatic-completion evidence.
// Completion remains strictly unsupported until a result-bearing target
// signal and durable identity are independently proven.
type TargetCompletionCapabilityRecord struct {
	Status               string                         `json:"status"`
	Reason               string                         `json:"reason"`
	ResultSignalFidelity string                         `json:"result_signal_fidelity"`
	DurableIdentity      string                         `json:"durable_identity"`
	ChildSuppression     string                         `json:"child_suppression"`
	Evidence             TargetCapabilityEvidenceRecord `json:"evidence"`
}

// TargetCapabilityRecord is one exact tested harness surface/version evidence
// record. It is record-only: builders and hook adapters are not derived from
// this data.
type TargetCapabilityRecord struct {
	Target        string                           `json:"target"`
	Surface       string                           `json:"surface"`
	Version       string                           `json:"version"`
	Platform      string                           `json:"platform"`
	InstalledMode string                           `json:"installed_mode"`
	Context       TargetContextCapabilityRecord    `json:"context"`
	Completion    TargetCompletionCapabilityRecord `json:"completion"`
	BypassModes   []string                         `json:"bypass_modes"`
	Fallback      string                           `json:"fallback"`
}

// DeferredTargetCapabilityRecord keeps explicitly deferred, non-build targets
// visible without making them target capability records.
type DeferredTargetCapabilityRecord struct {
	Target          string `json:"target"`
	Status          string `json:"status"`
	NotABuildTarget bool   `json:"not_a_build_target"`
	Reason          string `json:"reason"`
}

// TargetCapabilityEvidenceContract is the versioned passive evidence-record
// registry consumed by doctor/reporting code in later slices. It is not a
// dispatcher, renderer, adapter registry, or source for generated behavior.
type TargetCapabilityEvidenceContract struct {
	ContractVersion int                              `json:"contract_version"`
	Records         []TargetCapabilityRecord         `json:"records"`
	Deferred        []DeferredTargetCapabilityRecord `json:"deferred"`
}

const (
	TargetCapabilityEvidenceContractVersion = 3
	TargetCapabilityEvidenceRecordPath      = "config/target-capabilities.json"
	targetCapabilityEvidencePath            = TargetCapabilityEvidenceRecordPath
)

var supportedCapabilityTargets = map[string]struct{}{
	"claude-code": {},
	"cursor":      {},
	"codex":       {},
	"opencode":    {},
	"amp":         {},
}

var supportedModeStatuses = map[string]struct{}{
	"candidate":   {},
	"supported":   {},
	"unsupported": {},
}

var supportedEvidenceLevels = map[string]struct{}{
	"source":          {},
	"fixture":         {},
	"candidate-build": {},
	"installed-smoke": {},
}

var supportedInstalledModes = map[string]struct{}{
	"candidate-build":     {},
	"plugin-dir":          {},
	"isolated-codex-home": {},
	"isolated-xdg":        {},
}

var canonicalCapabilityModePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

var canonicalCapabilityPlatformPattern = regexp.MustCompile(`^[a-z0-9]+-[a-z0-9]+$`)

var runtimeCapabilityModeNames = map[string]struct{}{
	"root":         {},
	"startup":      {},
	"resume":       {},
	"clear":        {},
	"compact":      {},
	"new-composer": {},
	"cloud":        {},
	"background":   {},
	"child":        {},
	"subagent":     {},
	"delegated":    {},
}

var reservedCapabilityScopeNames = map[string]struct{}{
	"root":       {},
	"background": {},
	"child":      {},
	"subagent":   {},
	"delegated":  {},
}

// LoadTargetCapabilityEvidence reads and strictly validates a passive target
// evidence-record registry. Unknown fields, trailing JSON, unknown registry
// versions, and invalid records are rejected before consumption.
func LoadTargetCapabilityEvidence(path string) (TargetCapabilityEvidenceContract, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return TargetCapabilityEvidenceContract{}, fmt.Errorf("read target capability evidence %q: %w", path, err)
	}
	contract, err := DecodeTargetCapabilityEvidence(data)
	if err != nil {
		return TargetCapabilityEvidenceContract{}, fmt.Errorf("load target capability evidence %q: %w", path, err)
	}
	root, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return TargetCapabilityEvidenceContract{}, fmt.Errorf("resolve target capability evidence root: %w", err)
	}
	if filepath.Base(root) == "config" {
		root = filepath.Dir(root)
	}
	if err := validateTargetCapabilityEvidenceSources(contract, root); err != nil {
		return TargetCapabilityEvidenceContract{}, fmt.Errorf("load target capability evidence %q: %w", path, err)
	}
	return contract, nil
}

// DecodeTargetCapabilityEvidence strictly decodes and validates one passive
// evidence-record registry from JSON bytes.
func DecodeTargetCapabilityEvidence(data []byte) (TargetCapabilityEvidenceContract, error) {
	var contract TargetCapabilityEvidenceContract
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&contract); err != nil {
		return TargetCapabilityEvidenceContract{}, fmt.Errorf("decode contract: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return TargetCapabilityEvidenceContract{}, errors.New("decode contract: trailing JSON value")
	} else if err != nil && !errors.Is(err, io.EOF) {
		return TargetCapabilityEvidenceContract{}, fmt.Errorf("decode contract: trailing data: %w", err)
	}
	if err := ValidateTargetCapabilityEvidence(contract); err != nil {
		return TargetCapabilityEvidenceContract{}, err
	}
	return contract, nil
}

// ValidateTargetCapabilityEvidence enforces the version-3 passive evidence
// record schema without permitting wildcard versions or unsupported claims.
func ValidateTargetCapabilityEvidence(contract TargetCapabilityEvidenceContract) error {
	if contract.ContractVersion != TargetCapabilityEvidenceContractVersion {
		return fmt.Errorf("unsupported target capability contract version %d", contract.ContractVersion)
	}
	if len(contract.Records) == 0 {
		return errors.New("target capability evidence must contain at least one record")
	}
	identities := make(map[string]struct{}, len(contract.Records))
	targets := make(map[string]struct{}, len(contract.Records))
	for index, record := range contract.Records {
		if err := validateTargetCapabilityRecord(record); err != nil {
			return fmt.Errorf("records[%d]: %w", index, err)
		}
		identity := capabilityRecordIdentity(record)
		if _, exists := identities[identity]; exists {
			return fmt.Errorf("records[%d]: duplicate target/surface/version/platform/installed_mode identity %q/%q/%q/%q/%q", index, record.Target, record.Surface, record.Version, record.Platform, record.InstalledMode)
		}
		identities[identity] = struct{}{}
		targets[strings.ToLower(strings.TrimSpace(record.Target))] = struct{}{}
	}
	for target := range supportedCapabilityTargets {
		if _, ok := targets[target]; !ok {
			return fmt.Errorf("target capability evidence has no record for supported target %q", target)
		}
	}
	deferredTargets := make(map[string]struct{}, len(contract.Deferred))
	for index, deferred := range contract.Deferred {
		if err := validateDeferredTargetCapabilityRecord(deferred); err != nil {
			return fmt.Errorf("deferred[%d]: %w", index, err)
		}
		key := strings.ToLower(strings.TrimSpace(deferred.Target))
		if _, exists := deferredTargets[key]; exists {
			return fmt.Errorf("deferred[%d]: duplicate deferred target %q", index, deferred.Target)
		}
		deferredTargets[key] = struct{}{}
	}
	if _, ok := deferredTargets["pi"]; !ok {
		return errors.New("target capability evidence must retain the deferred pi boundary")
	}
	return nil
}

func capabilityRecordIdentity(record TargetCapabilityRecord) string {
	return strings.ToLower(strings.TrimSpace(record.Target)) + "\x00" + strings.ToLower(strings.TrimSpace(record.Surface)) + "\x00" + record.Version + "\x00" + strings.ToLower(strings.TrimSpace(record.Platform)) + "\x00" + strings.ToLower(strings.TrimSpace(record.InstalledMode))
}

func validateTargetCapabilityRecord(record TargetCapabilityRecord) error {
	target := strings.ToLower(strings.TrimSpace(record.Target))
	if _, ok := supportedCapabilityTargets[target]; !ok {
		if target == "pi" {
			return errors.New("pi is deferred and not a build target")
		}
		return fmt.Errorf("unsupported build target %q", record.Target)
	}
	if strings.TrimSpace(record.Surface) == "" {
		return errors.New("surface is required")
	}
	if strings.TrimSpace(record.Version) == "" {
		return errors.New("version is required")
	}
	if !isExactCapabilityVersion(record.Version) {
		return fmt.Errorf("version %q must be one exact tested version, not a wildcard or range", record.Version)
	}
	platform := strings.ToLower(strings.TrimSpace(record.Platform))
	if !canonicalCapabilityPlatformPattern.MatchString(platform) {
		return fmt.Errorf("platform %q must be an exact canonical platform such as darwin-arm64", record.Platform)
	}
	installedMode := strings.ToLower(strings.TrimSpace(record.InstalledMode))
	if _, ok := supportedInstalledModes[installedMode]; !ok {
		return fmt.Errorf("installed_mode %q is not a supported artifact-loading mode", record.InstalledMode)
	}
	if strings.TrimSpace(record.Fallback) == "" {
		return errors.New("fallback is required")
	}
	if len(record.BypassModes) == 0 {
		return errors.New("bypass_modes must contain at least one orthogonal mode")
	}
	for index, mode := range record.BypassModes {
		trimmed := strings.ToLower(strings.TrimSpace(mode))
		if trimmed == "" {
			return fmt.Errorf("bypass_modes[%d] is blank", index)
		}
		if runtimeMode, ok := runtimeCapabilityModeMention(trimmed); ok {
			return fmt.Errorf("bypass_modes[%d] duplicates runtime mode %q; classify it in context.modes", index, runtimeMode)
		}
	}
	if err := validateContextCapability(record.Context, installedMode); err != nil {
		return fmt.Errorf("context: %w", err)
	}
	if err := validateCompletionCapability(record.Completion); err != nil {
		return fmt.Errorf("completion: %w", err)
	}
	return nil
}

func validateContextCapability(contextRecord TargetContextCapabilityRecord, installedMode string) error {
	if strings.TrimSpace(contextRecord.Adapter) == "" {
		return errors.New("adapter is required")
	}
	if strings.TrimSpace(contextRecord.Mechanism) == "" {
		return errors.New("mechanism is required")
	}
	if strings.TrimSpace(contextRecord.RootPolicy) == "" {
		return errors.New("root_policy is required")
	}
	if strings.TrimSpace(contextRecord.SubagentPolicy) == "" {
		return errors.New("subagent_policy is required")
	}
	if strings.TrimSpace(contextRecord.BackgroundPolicy) == "" {
		return errors.New("background_policy is required")
	}
	if len(contextRecord.Modes) == 0 {
		return errors.New("modes must contain at least one mode")
	}
	modeNames := make(map[string]struct{}, len(contextRecord.Modes))
	for index, mode := range contextRecord.Modes {
		if err := validateModeEvidence(mode, installedMode); err != nil {
			return fmt.Errorf("modes[%d]: %w", index, err)
		}
		name := strings.ToLower(strings.TrimSpace(mode.Name))
		if _, exists := modeNames[name]; exists {
			return fmt.Errorf("modes[%d]: duplicate mode name %q", index, mode.Name)
		}
		modeNames[name] = struct{}{}
	}
	return nil
}

func validateModeEvidence(mode ModeEvidence, installedMode string) error {
	name := strings.TrimSpace(mode.Name)
	if name == "" {
		return errors.New("name is required")
	}
	if !canonicalCapabilityModePattern.MatchString(name) {
		return fmt.Errorf("name %q must be a canonical lower-kebab mode name", mode.Name)
	}
	if _, reserved := reservedCapabilityScopeNames[strings.ToLower(name)]; reserved {
		return fmt.Errorf("%s is a policy scope, not a context mode", strings.ToLower(name))
	}
	status := strings.TrimSpace(mode.Status)
	if _, ok := supportedModeStatuses[status]; !ok {
		return fmt.Errorf("unsupported status %q", mode.Status)
	}
	if err := validateEvidence(mode.Evidence); err != nil {
		return err
	}
	switch status {
	case "supported":
		if installedMode == "candidate-build" {
			return errors.New("candidate-build installed mode cannot claim a supported mode")
		}
		if mode.Trigger == "" {
			return errors.New("supported mode requires trigger")
		}
		if !mode.ModelVisible {
			return errors.New("supported mode requires model_visible=true")
		}
		if mode.Evidence.Level != "installed-smoke" {
			return errors.New("supported mode requires installed-smoke evidence")
		}
		if mode.Reason != "" {
			return errors.New("supported mode must not include reason")
		}
	case "candidate":
		if mode.Trigger == "" {
			return errors.New("candidate mode requires trigger")
		}
		if mode.ModelVisible {
			return errors.New("candidate mode requires model_visible=false")
		}
		if mode.Reason == "" {
			return errors.New("candidate mode requires reason")
		}
		if mode.Evidence.Level != "source" && mode.Evidence.Level != "fixture" && mode.Evidence.Level != "candidate-build" {
			return errors.New("candidate mode requires source, fixture, or candidate-build evidence")
		}
	case "unsupported":
		if mode.ModelVisible {
			return errors.New("unsupported mode requires model_visible=false")
		}
		if mode.Reason == "" {
			return errors.New("unsupported mode requires reason")
		}
	}
	return nil
}

func validateCompletionCapability(completion TargetCompletionCapabilityRecord) error {
	if strings.TrimSpace(completion.Status) != "unsupported" {
		return errors.New("completion status must be unsupported")
	}
	if strings.TrimSpace(completion.Reason) == "" {
		return errors.New("reason is required")
	}
	if strings.TrimSpace(completion.ResultSignalFidelity) == "" {
		return errors.New("result_signal_fidelity is required")
	}
	if strings.TrimSpace(completion.DurableIdentity) == "" {
		return errors.New("durable_identity is required")
	}
	if strings.TrimSpace(completion.ChildSuppression) == "" {
		return errors.New("child_suppression is required")
	}
	if err := validateEvidence(completion.Evidence); err != nil {
		return err
	}
	return nil
}

func validateEvidence(evidence TargetCapabilityEvidenceRecord) error {
	if strings.TrimSpace(evidence.Source) == "" {
		return errors.New("evidence source is required")
	}
	if _, err := safeEvidenceRelativePath(evidence.Source); err != nil {
		return err
	}
	if _, ok := supportedEvidenceLevels[strings.TrimSpace(evidence.Level)]; !ok {
		return fmt.Errorf("unsupported evidence level %q", evidence.Level)
	}
	if strings.TrimSpace(evidence.Summary) == "" {
		return errors.New("evidence summary is required")
	}
	return nil
}

func runtimeCapabilityModeMention(value string) (string, bool) {
	normalized := regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(strings.ToLower(strings.TrimSpace(value)), "-")
	normalized = strings.Trim(normalized, "-")
	for mode := range runtimeCapabilityModeNames {
		canonicalMode := regexp.MustCompile(`[^a-z0-9]+`).ReplaceAllString(mode, "-")
		pattern := `(^|-)` + regexp.QuoteMeta(canonicalMode) + `(-|$)`
		if regexp.MustCompile(pattern).MatchString(normalized) {
			return mode, true
		}
	}
	return "", false
}

func safeEvidenceRelativePath(source string) (string, error) {
	pathPart := strings.TrimSpace(strings.SplitN(source, "#", 2)[0])
	if pathPart == "" {
		return "", errors.New("evidence source path is blank")
	}
	if filepath.IsAbs(pathPart) || strings.Contains(pathPart, `\`) {
		return "", fmt.Errorf("evidence source %q must be repository-relative", source)
	}
	clean := filepath.Clean(filepath.FromSlash(pathPart))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("evidence source %q escapes the repository", source)
	}
	for _, component := range strings.Split(filepath.ToSlash(pathPart), "/") {
		if component == ".." {
			return "", fmt.Errorf("evidence source %q contains traversal", source)
		}
	}
	return clean, nil
}

func validateTargetCapabilityEvidenceSources(contract TargetCapabilityEvidenceContract, root string) error {
	for recordIndex, record := range contract.Records {
		for modeIndex, mode := range record.Context.Modes {
			if err := validateEvidenceSourceFile(root, mode.Evidence.Source); err != nil {
				return fmt.Errorf("records[%d].context.modes[%d].evidence: %w", recordIndex, modeIndex, err)
			}
			if mode.Status == "supported" && mode.Evidence.Level == "installed-smoke" {
				var err error
				switch record.Target {
				case "codex":
					err = validateCodexInstalledSmokeEvidence(root, record, mode)
				case "opencode":
					err = validateOpenCodeInstalledSmokeEvidence(root, record, mode)
				default:
					err = validateInstalledSmokeEvidence(root, record, mode)
				}
				if err != nil {
					return fmt.Errorf("records[%d].context.modes[%d].evidence: %w", recordIndex, modeIndex, err)
				}
			}
		}
		if err := validateEvidenceSourceFile(root, record.Completion.Evidence.Source); err != nil {
			return fmt.Errorf("records[%d].completion.evidence: %w", recordIndex, err)
		}
	}
	return nil
}

func validateEvidenceSourceFile(root, source string) error {
	relative, err := safeEvidenceRelativePath(source)
	if err != nil {
		return err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve evidence source root: %w", err)
	}
	current := root
	components := strings.Split(filepath.Clean(relative), string(filepath.Separator))
	for index, component := range components {
		if component == "" || component == "." {
			continue
		}
		current = filepath.Join(current, component)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			return fmt.Errorf("evidence source %q is not retained: %w", source, statErr)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if index == len(components)-1 {
				return fmt.Errorf("evidence source %q is not a regular file", source)
			}
			return fmt.Errorf("evidence source %q traverses a symlink component", source)
		}
		if index == len(components)-1 && !info.Mode().IsRegular() {
			return fmt.Errorf("evidence source %q is not a regular file", source)
		}
	}
	return nil
}

func validateInstalledSmokeEvidence(root string, record TargetCapabilityRecord, mode ModeEvidence) error {
	relative, err := safeEvidenceRelativePath(mode.Evidence.Source)
	if err != nil {
		return err
	}
	if filepath.Ext(relative) != ".json" {
		return fmt.Errorf("installed-smoke evidence source %q must be structured JSON", mode.Evidence.Source)
	}
	data, err := os.ReadFile(filepath.Join(root, relative))
	if err != nil {
		return fmt.Errorf("read installed-smoke evidence %q: %w", mode.Evidence.Source, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var smoke TargetCapabilitySmokeEvidence
	if err := decoder.Decode(&smoke); err != nil {
		return fmt.Errorf("decode installed-smoke evidence: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return errors.New("installed-smoke evidence has trailing JSON")
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("installed-smoke evidence trailing data: %w", err)
	}
	if smoke.EvidenceVersion != 2 {
		return fmt.Errorf("unsupported installed-smoke evidence version %d", smoke.EvidenceVersion)
	}
	if _, err := time.Parse(time.RFC3339, smoke.Timestamp); err != nil || !strings.HasSuffix(smoke.Timestamp, "Z") {
		return errors.New("installed-smoke evidence timestamp must be ISO 8601 UTC")
	}
	if smoke.Target != record.Target || smoke.Surface != record.Surface || smoke.Version != record.Version || smoke.Platform != record.Platform || smoke.InstalledMode != record.InstalledMode {
		return errors.New("installed-smoke evidence identity does not match capability record")
	}
	if smoke.ContextMode != mode.Name {
		return fmt.Errorf("installed-smoke evidence context mode %q does not match %q", smoke.ContextMode, mode.Name)
	}
	if smoke.Adapter != record.Context.Adapter {
		return fmt.Errorf("installed-smoke evidence adapter %q does not match %q", smoke.Adapter, record.Context.Adapter)
	}
	if smoke.Mode != "explicit-plugin-dir" {
		return fmt.Errorf("installed-smoke evidence mode %q is not the explicit candidate plugin mode", smoke.Mode)
	}
	if err := validateInstalledSmokeInvocation(smoke.Invocation); err != nil {
		return err
	}
	if len(smoke.Setup) == 0 || smoke.CandidatePluginPath != "plugins/loaf" {
		return errors.New("installed-smoke evidence setup or candidate plugin path is incomplete")
	}
	if smoke.HookObservation.EventName != "SessionStart:startup" || !smoke.HookObservation.NativeJSON || smoke.HookObservation.HookEventName != "SessionStart" || !smoke.HookObservation.AdditionalContextMarker {
		return errors.New("installed-smoke evidence lacks the expected native SessionStart marker observation")
	}
	if smoke.ExitCode != 0 || !smoke.StderrEmpty || !smoke.ModelVisibleMarkerObserved || !smoke.AssistantMarkerMatch || !regexp.MustCompile(`^LOAF_CLAUDE_STARTUP_SMOKE_[A-F0-9]{12}$`).MatchString(smoke.Marker) {
		return errors.New("installed-smoke evidence does not prove a successful model-visible marker result")
	}
	artifacts := smoke.CandidateArtifacts
	if artifacts.HooksPath != "plugins/loaf/hooks/hooks.json" {
		return fmt.Errorf("installed-smoke hooks path %q is not the candidate Claude hooks artifact", artifacts.HooksPath)
	}
	expectedNativeBinaryPath := filepath.ToSlash(filepath.Join("plugins", "loaf", "bin", "native", record.Platform, "loaf"))
	if artifacts.NativeBinaryPath != expectedNativeBinaryPath {
		return fmt.Errorf("installed-smoke native binary path %q, want %q", artifacts.NativeBinaryPath, expectedNativeBinaryPath)
	}
	for name, value := range map[string]string{"hooks_sha256": artifacts.HooksSHA256, "native_binary_sha256": artifacts.NativeBinarySHA256} {
		if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(value) {
			return fmt.Errorf("installed-smoke %s must be a lowercase SHA-256", name)
		}
	}
	for _, artifact := range []struct {
		name   string
		path   string
		digest string
	}{
		{"hooks", artifacts.HooksPath, artifacts.HooksSHA256},
		{"native binary", artifacts.NativeBinaryPath, artifacts.NativeBinarySHA256},
	} {
		actual, err := sha256File(filepath.Join(root, artifact.path))
		if err != nil {
			return fmt.Errorf("hash installed-smoke %s artifact: %w", artifact.name, err)
		}
		if actual != artifact.digest {
			return fmt.Errorf("installed-smoke %s SHA-256 %s does not match current candidate %s", artifact.name, artifact.digest, actual)
		}
	}
	return nil
}

func validateInstalledSmokeInvocation(invocation TargetCapabilitySmokeInvocation) error {
	if invocation.Command != "claude" || invocation.CWD != "<disposable-repo>" {
		return errors.New("installed-smoke evidence invocation is incomplete or not sanitized")
	}
	args := invocation.Args
	expected := []string{
		"--plugin-dir", "<repo>/plugins/loaf",
		"--strict-mcp-config", "--mcp-config", `{"mcpServers":{}}`,
		"--no-session-persistence",
		"--setting-sources", "",
		"--tools", "",
		"--include-hook-events",
		"--output-format", "stream-json",
		"-p",
	}
	if len(args) != len(expected)+1 {
		return fmt.Errorf("installed-smoke evidence invocation has %d arguments, want exact candidate shape", len(args))
	}
	for index, want := range expected {
		if args[index] != want {
			return fmt.Errorf("installed-smoke evidence invocation argument %d = %q, want %q", index, args[index], want)
		}
	}
	if strings.TrimSpace(args[len(expected)]) == "" {
		return errors.New("installed-smoke evidence invocation prompt is blank")
	}
	return nil
}

func validateOpenCodeInstalledSmokeEvidence(root string, record TargetCapabilityRecord, mode ModeEvidence) error {
	relative, err := safeEvidenceRelativePath(mode.Evidence.Source)
	if err != nil {
		return err
	}
	if filepath.Ext(relative) != ".json" {
		return fmt.Errorf("installed-smoke evidence source %q must be structured JSON", mode.Evidence.Source)
	}
	data, err := os.ReadFile(filepath.Join(root, relative))
	if err != nil {
		return fmt.Errorf("read installed-smoke evidence %q: %w", mode.Evidence.Source, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var smoke TargetCapabilityOpenCodeSmokeEvidence
	if err := decoder.Decode(&smoke); err != nil {
		return fmt.Errorf("decode OpenCode installed-smoke evidence: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return errors.New("OpenCode installed-smoke evidence has trailing JSON")
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("OpenCode installed-smoke evidence trailing data: %w", err)
	}
	if smoke.EvidenceVersion != 2 {
		return fmt.Errorf("unsupported OpenCode installed-smoke evidence version %d", smoke.EvidenceVersion)
	}
	if _, err := time.Parse(time.RFC3339, smoke.Timestamp); err != nil || !strings.HasSuffix(smoke.Timestamp, "Z") {
		return errors.New("OpenCode installed-smoke evidence timestamp must be ISO 8601 UTC")
	}
	if smoke.Target != record.Target || smoke.Surface != record.Surface || smoke.Version != record.Version || smoke.Platform != record.Platform || smoke.InstalledMode != record.InstalledMode {
		return errors.New("OpenCode installed-smoke evidence identity does not match capability record")
	}
	if smoke.ContextMode != mode.Name {
		return fmt.Errorf("OpenCode installed-smoke evidence context mode %q does not match %q", smoke.ContextMode, mode.Name)
	}
	if smoke.Adapter != record.Context.Adapter {
		return fmt.Errorf("OpenCode installed-smoke evidence adapter %q does not match %q", smoke.Adapter, record.Context.Adapter)
	}
	if smoke.Mode != "isolated-xdg" {
		return fmt.Errorf("OpenCode installed-smoke evidence mode %q is not the isolated-XDG mode", smoke.Mode)
	}
	if err := validateOpenCodeInstalledSmokeInvocation(smoke.Invocation); err != nil {
		return err
	}
	if len(smoke.Setup) == 0 {
		return errors.New("OpenCode installed-smoke evidence setup is incomplete")
	}
	if smoke.ExitCode != 0 || !smoke.StderrEmpty || smoke.Stderr != "" || smoke.FailureReason != "" {
		return errors.New("OpenCode installed-smoke evidence does not prove a clean successful invocation")
	}
	if !smoke.ModelVisibleMarkerObserved || !smoke.AssistantMarkerMatch || !smoke.PluginLoaded || !smoke.RootSessionLookupProven || !smoke.NoAuthSupplied || !smoke.CleanupSucceeded {
		return errors.New("OpenCode installed-smoke evidence does not prove the required model-visible plugin observations")
	}
	if !regexp.MustCompile(`^LOAF_OPENCODE_REQUEST_SMOKE_[A-F0-9]{12}$`).MatchString(smoke.Marker) {
		return errors.New("OpenCode installed-smoke evidence marker is not a valid request-smoke marker")
	}
	artifacts := smoke.CandidateArtifacts
	if artifacts.HooksPath != "dist/opencode/plugins/hooks.ts" {
		return fmt.Errorf("OpenCode installed-smoke hooks path %q is not the candidate OpenCode plugin artifact", artifacts.HooksPath)
	}
	expectedNativeBinaryPath := filepath.ToSlash(filepath.Join("bin", "native", record.Platform, "loaf"))
	if artifacts.NativeBinaryPath != expectedNativeBinaryPath {
		return fmt.Errorf("OpenCode installed-smoke native binary path %q, want %q", artifacts.NativeBinaryPath, expectedNativeBinaryPath)
	}
	for name, value := range map[string]string{"hooks_sha256": artifacts.HooksSHA256, "native_binary_sha256": artifacts.NativeBinarySHA256} {
		if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(value) {
			return fmt.Errorf("OpenCode installed-smoke %s must be a lowercase SHA-256", name)
		}
	}
	for _, artifact := range []struct {
		name   string
		path   string
		digest string
	}{
		{"hooks", artifacts.HooksPath, artifacts.HooksSHA256},
		{"native binary", artifacts.NativeBinaryPath, artifacts.NativeBinarySHA256},
	} {
		actual, err := sha256File(filepath.Join(root, artifact.path))
		if err != nil {
			return fmt.Errorf("hash OpenCode installed-smoke %s artifact: %w", artifact.name, err)
		}
		if actual != artifact.digest {
			return fmt.Errorf("OpenCode installed-smoke %s SHA-256 %s does not match current candidate %s", artifact.name, artifact.digest, actual)
		}
	}
	return nil
}

func validateOpenCodeInstalledSmokeInvocation(invocation TargetCapabilitySmokeInvocation) error {
	if invocation.Command != "opencode" || invocation.CWD != "<disposable-repo>" {
		return errors.New("OpenCode installed-smoke invocation is incomplete or not sanitized")
	}
	expected := []string{
		"run", "--format", "json", "--model", "opencode/deepseek-v4-flash-free", "--dir", "<disposable-repo>",
		"Reply with exactly the unique marker present in Loaf continuity context, and nothing else.",
	}
	if len(invocation.Args) != len(expected) {
		return fmt.Errorf("OpenCode installed-smoke invocation has %d arguments, want exact candidate shape", len(invocation.Args))
	}
	for index, want := range expected {
		if invocation.Args[index] != want {
			return fmt.Errorf("OpenCode installed-smoke invocation argument %d = %q, want %q", index, invocation.Args[index], want)
		}
	}
	return nil
}

func validateCodexInstalledSmokeEvidence(root string, record TargetCapabilityRecord, mode ModeEvidence) error {
	relative, err := safeEvidenceRelativePath(mode.Evidence.Source)
	if err != nil {
		return err
	}
	if filepath.Ext(relative) != ".json" {
		return fmt.Errorf("installed-smoke evidence source %q must be structured JSON", mode.Evidence.Source)
	}
	data, err := os.ReadFile(filepath.Join(root, relative))
	if err != nil {
		return fmt.Errorf("read installed-smoke evidence %q: %w", mode.Evidence.Source, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var smoke TargetCapabilityCodexSmokeEvidence
	if err := decoder.Decode(&smoke); err != nil {
		return fmt.Errorf("decode Codex installed-smoke evidence: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return errors.New("Codex installed-smoke evidence has trailing JSON")
	} else if !errors.Is(err, io.EOF) {
		return fmt.Errorf("Codex installed-smoke evidence trailing data: %w", err)
	}
	if smoke.EvidenceVersion != 2 {
		return fmt.Errorf("unsupported Codex installed-smoke evidence version %d", smoke.EvidenceVersion)
	}
	if _, err := time.Parse(time.RFC3339, smoke.Timestamp); err != nil || !strings.HasSuffix(smoke.Timestamp, "Z") {
		return errors.New("Codex installed-smoke evidence timestamp must be ISO 8601 UTC")
	}
	if smoke.Target != record.Target || smoke.Surface != record.Surface || smoke.Version != record.Version || smoke.Platform != record.Platform || smoke.InstalledMode != record.InstalledMode {
		return errors.New("Codex installed-smoke evidence identity does not match capability record")
	}
	if smoke.ContextMode != mode.Name {
		return fmt.Errorf("Codex installed-smoke evidence context mode %q does not match %q", smoke.ContextMode, mode.Name)
	}
	if smoke.Adapter != record.Context.Adapter {
		return fmt.Errorf("Codex installed-smoke evidence adapter %q does not match %q", smoke.Adapter, record.Context.Adapter)
	}
	if smoke.Mode != "isolated-codex-home" {
		return fmt.Errorf("Codex installed-smoke evidence mode %q is not the isolated CODEX_HOME mode", smoke.Mode)
	}
	if err := validateCodexInstalledSmokeInvocation(smoke.Invocation); err != nil {
		return err
	}
	if len(smoke.Setup) == 0 || smoke.HookObservation.EventName != "SessionStart:startup" || !smoke.HookObservation.NativeJSON || smoke.HookObservation.HookEventName != "SessionStart" || !smoke.HookObservation.AdditionalContextMarker {
		return errors.New("Codex installed-smoke evidence lacks the expected native SessionStart marker observation")
	}
	if smoke.ExitCode != 0 || !smoke.ModelVisibleMarkerObserved || !smoke.AssistantMarkerMatch || !regexp.MustCompile(`^LOAF_CODEX_STARTUP_SMOKE_[A-F0-9]{12}$`).MatchString(smoke.Marker) {
		return errors.New("Codex installed-smoke evidence does not prove a successful model-visible marker result")
	}
	if smoke.Stderr != "" && smoke.Stderr != "Reading additional input from stdin..." {
		return fmt.Errorf("Codex installed-smoke stderr contains an unexpected diagnostic %q", smoke.Stderr)
	}
	if smoke.StderrEmpty != (smoke.Stderr == "") {
		return errors.New("Codex installed-smoke stderr_empty does not match retained stderr")
	}
	for name, value := range map[string]string{"hooks_sha256": smoke.CandidateArtifacts.HooksSHA256, "native_binary_sha256": smoke.CandidateArtifacts.NativeBinarySHA256} {
		if !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(value) {
			return fmt.Errorf("Codex installed-smoke %s must be a lowercase SHA-256", name)
		}
	}
	if smoke.CandidateArtifacts.HooksPath != "dist/codex/.codex/hooks.json" {
		return fmt.Errorf("Codex installed-smoke hooks path %q is not the candidate Codex hooks artifact", smoke.CandidateArtifacts.HooksPath)
	}
	expectedNativeBinaryPath := filepath.ToSlash(filepath.Join("bin", "native", record.Platform, "loaf"))
	if smoke.CandidateArtifacts.NativeBinaryPath != expectedNativeBinaryPath {
		return fmt.Errorf("Codex installed-smoke native binary path %q, want %q", smoke.CandidateArtifacts.NativeBinaryPath, expectedNativeBinaryPath)
	}
	for _, artifact := range []struct {
		name   string
		path   string
		digest string
	}{
		{"hooks", smoke.CandidateArtifacts.HooksPath, smoke.CandidateArtifacts.HooksSHA256},
		{"native binary", smoke.CandidateArtifacts.NativeBinaryPath, smoke.CandidateArtifacts.NativeBinarySHA256},
	} {
		actual, err := sha256File(filepath.Join(root, artifact.path))
		if err != nil {
			return fmt.Errorf("hash Codex installed-smoke %s artifact: %w", artifact.name, err)
		}
		if actual != artifact.digest {
			return fmt.Errorf("Codex installed-smoke %s SHA-256 %s does not match current candidate %s", artifact.name, artifact.digest, actual)
		}
	}
	return nil
}

func validateCodexInstalledSmokeInvocation(invocation TargetCapabilitySmokeInvocation) error {
	if invocation.Command != "codex" || invocation.CWD != "<disposable-repo>" {
		return errors.New("Codex installed-smoke invocation is incomplete or not sanitized")
	}
	expected := []string{"exec", "--ephemeral", "--ignore-rules", "--dangerously-bypass-hook-trust", "--sandbox", "read-only", "--json", "-C", "<disposable-repo>"}
	if len(invocation.Args) != len(expected)+1 {
		return fmt.Errorf("Codex installed-smoke invocation has %d arguments, want exact candidate shape", len(invocation.Args))
	}
	for index, want := range expected {
		if invocation.Args[index] != want {
			return fmt.Errorf("Codex installed-smoke invocation argument %d = %q, want %q", index, invocation.Args[index], want)
		}
	}
	if strings.TrimSpace(invocation.Args[len(expected)]) == "" {
		return errors.New("Codex installed-smoke invocation prompt is blank")
	}
	return nil
}

func sha256File(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func validateDeferredTargetCapabilityRecord(deferred DeferredTargetCapabilityRecord) error {
	if strings.TrimSpace(deferred.Target) == "" {
		return errors.New("target is required")
	}
	if strings.ToLower(strings.TrimSpace(deferred.Target)) != "pi" {
		return fmt.Errorf("only pi may be listed as deferred, got %q", deferred.Target)
	}
	if deferred.Status != "deferred" {
		return fmt.Errorf("status must be deferred, got %q", deferred.Status)
	}
	if !deferred.NotABuildTarget {
		return errors.New("deferred target must be marked not_a_build_target")
	}
	if strings.TrimSpace(deferred.Reason) == "" {
		return errors.New("reason is required")
	}
	return nil
}

func isExactCapabilityVersion(version string) bool {
	return capabilityExactVersionPattern.MatchString(version)
}

var capabilityExactVersionPattern = regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)+(?:-[A-Za-z0-9]+)?$`)
