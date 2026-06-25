package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// DurableRenderSweepOptions describes a committed durable render contract sweep.
type DurableRenderSweepOptions struct {
	TargetContract string
	DryRun         bool
}

// DurableRenderSweepResult summarizes a committed durable render contract sweep.
type DurableRenderSweepResult struct {
	ContractVersion int                      `json:"contract_version,omitempty"`
	Contract        string                   `json:"contract"`
	Scanned         int                      `json:"scanned"`
	Skipped         int                      `json:"skipped"`
	Current         int                      `json:"current"`
	UpgradeNeeded   int                      `json:"upgrade_needed"`
	Upgraded        int                      `json:"upgraded"`
	Drift           int                      `json:"drift"`
	Invalid         int                      `json:"invalid"`
	DryRun          bool                     `json:"dry_run"`
	Files           []DurableRenderSweepFile `json:"files,omitempty"`
}

// DurableRenderSweepFile describes one durable render considered by a sweep.
type DurableRenderSweepFile struct {
	Path         string `json:"path"`
	RelativePath string `json:"relative_path"`
	Kind         string `json:"kind,omitempty"`
	FromContract string `json:"from_contract,omitempty"`
	ToContract   string `json:"to_contract,omitempty"`
	Status       string `json:"status"`
	ContentHash  string `json:"content_hash,omitempty"`
	Error        string `json:"error,omitempty"`
}

// HasBlockingFindings reports whether the sweep found files that require manual repair.
func (r DurableRenderSweepResult) HasBlockingFindings() bool {
	return r.Drift > 0 || r.Invalid > 0
}

// SweepDurableRenderContracts upgrades committed durable renders to the target contract.
func SweepDurableRenderContracts(root project.Root, options DurableRenderSweepOptions) (DurableRenderSweepResult, error) {
	targetContract := strings.TrimSpace(options.TargetContract)
	if targetContract == "" {
		targetContract = durableRenderCurrentContract
	}
	result := DurableRenderSweepResult{
		ContractVersion: StateJSONContractVersion,
		Contract:        targetContract,
		DryRun:          options.DryRun,
	}
	files, err := committedDurableRenderFiles(root)
	if err != nil {
		return DurableRenderSweepResult{}, err
	}
	for _, file := range files {
		sweepFile := sweepDurableRenderFile(root, file, targetContract, options.DryRun)
		switch sweepFile.Status {
		case "skipped":
			result.Skipped++
			continue
		case "current":
			result.Scanned++
			result.Current++
		case "upgrade-needed":
			result.Scanned++
			result.UpgradeNeeded++
		case "upgraded":
			result.Scanned++
			result.UpgradeNeeded++
			result.Upgraded++
		case "drift":
			result.Scanned++
			result.Drift++
		case "invalid":
			result.Scanned++
			result.Invalid++
		}
		result.Files = append(result.Files, sweepFile)
	}
	return result, nil
}

type committedDurableRenderFile struct {
	path string
	rel  string
}

func committedDurableRenderFiles(root project.Root) ([]committedDurableRenderFile, error) {
	var files []committedDurableRenderFile
	for _, dir := range []string{filepath.Join(".agents", "specs"), filepath.Join(".agents", "reports")} {
		fullDir := filepath.Join(root.Path(), dir)
		entries, err := os.ReadDir(fullDir)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read durable render directory %s: %w", filepath.ToSlash(dir), err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			rel := filepath.ToSlash(filepath.Join(dir, entry.Name()))
			files = append(files, committedDurableRenderFile{
				path: filepath.Join(fullDir, entry.Name()),
				rel:  rel,
			})
		}
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].rel < files[j].rel
	})
	return files, nil
}

func sweepDurableRenderFile(root project.Root, file committedDurableRenderFile, targetContract string, dryRun bool) DurableRenderSweepFile {
	result := DurableRenderSweepFile{
		Path:         file.path,
		RelativePath: file.rel,
		ToContract:   targetContract,
	}
	contentBytes, err := os.ReadFile(file.path)
	if err != nil {
		result.Status = "invalid"
		result.Error = fmt.Sprintf("read durable render: %v", err)
		return result
	}
	content := string(contentBytes)
	if !hasDurableRenderFinalStamp(content) {
		result.Status = "skipped"
		return result
	}
	doc, contract, err := parseDurableRenderAnyContract(content)
	if err != nil {
		result.Status = "invalid"
		result.Error = err.Error()
		return result
	}
	result.Kind = doc.Kind
	result.FromContract = contract

	rerendered, err := renderDurableDocumentWithContract(doc, contract)
	if err != nil {
		result.Status = "invalid"
		result.Error = err.Error()
		return result
	}
	if rerendered != content {
		result.Status = "drift"
		result.ContentHash = artifactBodyHash(content)
		result.Error = "committed render is not byte-identical to its deterministic self-render"
		return result
	}
	if contract == targetContract {
		result.Status = "current"
		result.ContentHash = artifactBodyHash(content)
		return result
	}
	upgraded, err := renderDurableDocumentWithContract(doc, targetContract)
	if err != nil {
		result.Status = "invalid"
		result.Error = err.Error()
		return result
	}
	result.ContentHash = artifactBodyHash(upgraded)
	if dryRun {
		result.Status = "upgrade-needed"
		return result
	}
	if err := os.WriteFile(file.path, []byte(upgraded), 0o600); err != nil {
		result.Status = "invalid"
		result.Error = fmt.Sprintf("write upgraded durable render: %v", err)
		return result
	}
	result.Status = "upgraded"
	return result
}

func hasDurableRenderFinalStamp(content string) bool {
	lines := strings.Split(strings.TrimSpace(normalizeLineEndings(content)), "\n")
	if len(lines) == 0 {
		return false
	}
	return strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "<!-- loaf:render ")
}
