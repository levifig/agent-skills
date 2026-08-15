package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	changeLayoutLegacy = "legacy"
	changeLayoutNew    = "new"
)

const (
	changeMachineFileLegacy = "change.md"
	changeMachineFileJSON   = "change.json"
	changeContractFileShape = "shape.md"
	changeBriefFile         = "brief.md"
)

// assembleChangeNodeFromFolder builds a layout-agnostic node from a working-tree
// Change folder. change.json presence selects the new layout; its absence falls
// back to legacy change.md. A present but malformed change.json fails closed.
func assembleChangeNodeFromFolder(rootPath, folderAbs string) (changeNode, error) {
	folderRel := filepath.ToSlash(relFromRoot(rootPath, folderAbs))
	jsonPath := filepath.Join(folderAbs, changeMachineFileJSON)
	mdPath := filepath.Join(folderAbs, changeMachineFileLegacy)

	jsonContent, jsonErr := os.ReadFile(jsonPath)
	if jsonErr == nil {
		return assembleNewLayoutNode(rootPath, folderAbs, folderRel, string(jsonContent)), nil
	}
	if !os.IsNotExist(jsonErr) {
		return changeNode{}, fmt.Errorf("read %s: %w", filepath.ToSlash(filepath.Join(folderRel, changeMachineFileJSON)), jsonErr)
	}

	mdContent, mdErr := os.ReadFile(mdPath)
	if mdErr != nil {
		if os.IsNotExist(mdErr) {
			return changeNode{}, fmt.Errorf("no change.json or change.md in %s", folderRel)
		}
		return changeNode{}, fmt.Errorf("read %s: %w", filepath.ToSlash(filepath.Join(folderRel, changeMachineFileLegacy)), mdErr)
	}
	return assembleLegacyLayoutNode(folderRel, string(mdContent)), nil
}

func assembleNewLayoutNode(rootPath, folderAbs, folderRel, jsonContent string) changeNode {
	meta := parseChangeJSON(jsonContent)
	node := changeNode{
		Slug:          meta.Change,
		Branch:        meta.Branch,
		Created:       meta.Created,
		TargetRelease: meta.TargetRelease,
		Layout:        changeLayoutNew,
		Folder:        folderRel,
		ChangeFile:    filepath.ToSlash(filepath.Join(folderRel, changeMachineFileJSON)),
		ParseFindings: append([]string{}, meta.Findings...),
		MetaContent:   jsonContent,
	}

	shapePath := filepath.Join(folderAbs, changeContractFileShape)
	if content, err := os.ReadFile(shapePath); err == nil {
		node.ContractFile = filepath.ToSlash(filepath.Join(folderRel, changeContractFileShape))
		node.Content = string(content)
	} else if brief, briefErr := os.ReadFile(filepath.Join(folderAbs, changeBriefFile)); briefErr == nil {
		node.ContractFile = filepath.ToSlash(filepath.Join(folderRel, changeBriefFile))
		node.Content = string(brief)
		node.CapturedOnly = true
	} else {
		_ = rootPath
		node.CapturedOnly = true
	}
	return node
}

func assembleLegacyLayoutNode(folderRel, mdContent string) changeNode {
	fields, _ := changeFrontmatterFields(mdContent)
	node := changeNode{
		Slug:          changeFieldValue(fields, "change"),
		Branch:        changeFieldValue(fields, "branch"),
		Created:       changeFieldValue(fields, "created"),
		Lineage:       changeFieldValue(fields, "lineage"),
		Predecessor:   changeFieldValue(fields, "predecessor"),
		ReleaseAfter:  changeFieldValue(fields, "release-after"),
		TargetRelease: changeFieldValue(fields, "target_release"),
		Layout:        changeLayoutLegacy,
		Folder:        folderRel,
		ChangeFile:    filepath.ToSlash(filepath.Join(folderRel, changeMachineFileLegacy)),
		ContractFile:  filepath.ToSlash(filepath.Join(folderRel, changeMachineFileLegacy)),
		Content:       mdContent,
		MetaContent:   mdContent,
	}
	if node.TargetRelease != "" && !isCanonicalChangeTargetRelease(node.TargetRelease) {
		node.ParseFindings = append(node.ParseFindings,
			fmt.Sprintf("target_release %q must be canonical MAJOR.MINOR.PATCH (no v, leading zeros, prerelease, or build)", node.TargetRelease))
	}
	return node
}

// assembleChangeNodeFromCommittedFiles builds a node from committed path contents
// for one Change folder. jsonContent non-nil (including empty string pointer via
// present flag) means change.json was present at that commit.
func assembleChangeNodeFromCommittedFiles(folderRel string, jsonPresent bool, jsonContent string, mdPresent bool, mdContent string) (changeNode, bool) {
	if jsonPresent {
		node := changeNode{
			Layout:      changeLayoutNew,
			Folder:      folderRel,
			ChangeFile:  filepath.ToSlash(filepath.Join(folderRel, changeMachineFileJSON)),
			MetaContent: jsonContent,
		}
		meta := parseChangeJSON(jsonContent)
		node.Slug = meta.Change
		node.Branch = meta.Branch
		node.Created = meta.Created
		node.TargetRelease = meta.TargetRelease
		node.ParseFindings = append([]string{}, meta.Findings...)
		if mdPresent {
			// Keep-both transitional: lineage fields may still live on change.md
			// during conversion windows; target still comes from JSON when present.
			fields, _ := changeFrontmatterFields(mdContent)
			if node.Lineage == "" {
				node.Lineage = changeFieldValue(fields, "lineage")
			}
			if node.Predecessor == "" {
				node.Predecessor = changeFieldValue(fields, "predecessor")
			}
			if node.ReleaseAfter == "" {
				node.ReleaseAfter = changeFieldValue(fields, "release-after")
			}
			node.Content = mdContent
			node.ContractFile = filepath.ToSlash(filepath.Join(folderRel, changeMachineFileLegacy))
		}
		return node, true
	}
	if mdPresent {
		return assembleLegacyLayoutNode(folderRel, mdContent), true
	}
	return changeNode{}, false
}

func listChangeFolderNames(rootPath string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(rootPath, "docs", "changes"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var folders []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		folderAbs := filepath.Join(rootPath, "docs", "changes", name)
		if changeFolderHasMachineSurface(folderAbs) {
			folders = append(folders, name)
		}
	}
	sort.Strings(folders)
	return folders, nil
}

func changeFolderHasMachineSurface(folderAbs string) bool {
	if _, err := os.Stat(filepath.Join(folderAbs, changeMachineFileJSON)); err == nil {
		return true
	}
	if _, err := os.Stat(filepath.Join(folderAbs, changeMachineFileLegacy)); err == nil {
		return true
	}
	return false
}
