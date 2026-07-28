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

// changeRetargetEvent records a target_release mutation derived from unioned
// history across change.json and change.md surfaces.
type changeRetargetEvent struct {
	Folder  string
	Slug    string
	From    string
	To      string
	Commit  string
	Surface string
}

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

func deriveChangeCohorts(nodes []changeNode) map[string][]changeNode {
	cohorts := map[string][]changeNode{}
	for _, node := range nodes {
		if node.TargetRelease == "" {
			continue
		}
		cohorts[node.TargetRelease] = append(cohorts[node.TargetRelease], node)
	}
	for target, members := range cohorts {
		sort.Slice(members, func(i, j int) bool { return members[i].Folder < members[j].Folder })
		cohorts[target] = members
	}
	return cohorts
}

func changeFolderRelFromMachinePath(path string) string {
	path = filepath.ToSlash(path)
	if strings.HasSuffix(path, "/"+changeMachineFileJSON) || strings.HasSuffix(path, "/"+changeMachineFileLegacy) {
		return filepath.ToSlash(filepath.Dir(path))
	}
	return path
}

// deriveChangeRetargetEvents unions change.json and change.md histories per
// folder and returns target_release mutations (including removal-to-none).
// Retargets are surfaced, never blocked.
func deriveChangeRetargetEvents(rootPath string, outputCommand changeGitOutput) ([]changeRetargetEvent, error) {
	output, err := outputCommand(rootPath, "git", "ls-tree", "-r", "--name-only", "HEAD", "--", "docs/changes")
	if err != nil {
		return nil, fmt.Errorf("inspect Change paths at HEAD: %w", err)
	}
	folders := map[string]bool{}
	for _, path := range strings.Split(strings.TrimSpace(output), "\n") {
		path = filepath.ToSlash(strings.TrimSpace(path))
		base := filepath.Base(path)
		if base == changeMachineFileJSON || base == changeMachineFileLegacy {
			folders[filepath.ToSlash(filepath.Dir(path))] = true
		}
	}
	// Also include folders that only exist in history (deleted) — retarget
	// surfacing for current HEAD nodes is enough for TASK-001 consumers.
	folderList := sortedKeys(folders)
	var events []changeRetargetEvent
	for _, folder := range folderList {
		folderEvents, err := deriveFolderRetargetEvents(rootPath, folder, outputCommand)
		if err != nil {
			return nil, err
		}
		events = append(events, folderEvents...)
	}
	return events, nil
}

func deriveFolderRetargetEvents(rootPath, folder string, outputCommand changeGitOutput) ([]changeRetargetEvent, error) {
	mdPath := filepath.ToSlash(filepath.Join(folder, changeMachineFileLegacy))
	jsonPath := filepath.ToSlash(filepath.Join(folder, changeMachineFileJSON))
	versions, err := loadUnionTargetHistory(rootPath, folder, mdPath, jsonPath, outputCommand)
	if err != nil {
		return nil, err
	}
	var events []changeRetargetEvent
	prev := ""
	havePrev := false
	for _, version := range versions {
		if !havePrev {
			prev = version.Target
			havePrev = true
			continue
		}
		if version.Target == prev {
			continue
		}
		events = append(events, changeRetargetEvent{
			Folder:  folder,
			Slug:    version.Slug,
			From:    prev,
			To:      version.Target,
			Commit:  version.Commit,
			Surface: version.Surface,
		})
		prev = version.Target
	}
	return events, nil
}

type changeTargetVersion struct {
	Commit  string
	Target  string
	Slug    string
	Surface string
}

func loadUnionTargetHistory(rootPath, folder, mdPath, jsonPath string, outputCommand changeGitOutput) ([]changeTargetVersion, error) {
	commitsOutput, err := outputCommand(rootPath, "git", "rev-list", "--full-history", "--topo-order", "--reverse", "HEAD", "--", mdPath, jsonPath)
	if err != nil {
		return nil, fmt.Errorf("read %s target history: %w", folder, err)
	}
	commits := strings.Fields(commitsOutput)
	var versions []changeTargetVersion
	for _, commit := range commits {
		jsonContent, jsonOK, err := readCommittedOptional(rootPath, commit, jsonPath, outputCommand)
		if err != nil {
			return nil, err
		}
		mdContent, mdOK, err := readCommittedOptional(rootPath, commit, mdPath, outputCommand)
		if err != nil {
			return nil, err
		}
		if !jsonOK && !mdOK {
			continue
		}
		version := changeTargetVersion{Commit: commit}
		if jsonOK {
			meta := parseChangeJSON(jsonContent)
			version.Target = meta.TargetRelease
			version.Slug = meta.Change
			version.Surface = changeMachineFileJSON
		} else if mdOK {
			fields, _ := changeFrontmatterFields(mdContent)
			version.Target = changeFieldValue(fields, "target_release")
			version.Slug = changeFieldValue(fields, "change")
			version.Surface = changeMachineFileLegacy
		}
		versions = append(versions, version)
	}
	return versions, nil
}

func readCommittedOptional(rootPath, commit, path string, outputCommand changeGitOutput) (string, bool, error) {
	treePath, err := outputCommand(rootPath, "git", "ls-tree", "--name-only", commit, "--", path)
	if err != nil {
		return "", false, fmt.Errorf("inspect %s at %s: %w", path, shortChangeCommit(commit), err)
	}
	treePath = filepath.ToSlash(strings.TrimSpace(treePath))
	if treePath == "" {
		return "", false, nil
	}
	content, err := outputCommand(rootPath, "git", "show", commit+":"+path)
	if err != nil {
		return "", false, fmt.Errorf("read %s at %s: %w", path, shortChangeCommit(commit), err)
	}
	return content, true, nil
}
