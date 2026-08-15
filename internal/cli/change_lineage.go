package cli

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

type changeGitOutput func(cwd string, name string, args ...string) (string, error)

type changeGraph struct {
	Nodes                  []changeNode
	Findings               []string
	Gaps                   []string
	GlobalFindings         []string
	findingsByLineage      map[string][]string
	localFindingsByLineage map[string][]string
	localFindingsByChange  map[string][]string
	gapsByLineage          map[string][]string
}

func loadChangeNodesAtHEAD(rootPath string) ([]changeNode, error) {
	return loadChangeNodesAtHEADWithOutput(rootPath, commandOutput)
}

func loadChangeNodesAtHEADWithOutput(rootPath string, outputCommand changeGitOutput) ([]changeNode, error) {
	output, err := outputCommand(rootPath, "git", "ls-tree", "-r", "--name-only", "HEAD", "--", "docs/changes")
	if err != nil {
		return nil, fmt.Errorf("inspect committed Change paths at HEAD: %w", err)
	}
	type folderFiles struct {
		jsonPresent  bool
		jsonContent  string
		mdPresent    bool
		mdContent    string
		shapePresent bool
		shapeContent string
		briefPresent bool
		briefContent string
	}
	byFolder := map[string]*folderFiles{}
	for _, path := range strings.Split(strings.TrimSpace(output), "\n") {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" {
			continue
		}
		base := filepath.Base(path)
		switch base {
		case changeMachineFileJSON, changeMachineFileLegacy, changeContractFileShape, changeBriefFile:
		default:
			continue
		}
		folder := filepath.ToSlash(filepath.Dir(path))
		entry := byFolder[folder]
		if entry == nil {
			entry = &folderFiles{}
			byFolder[folder] = entry
		}
		content, err := outputCommand(rootPath, "git", "show", "HEAD:"+path)
		if err != nil {
			return nil, fmt.Errorf("read committed %s: %w", path, err)
		}
		switch base {
		case changeMachineFileJSON:
			entry.jsonPresent = true
			entry.jsonContent = content
		case changeMachineFileLegacy:
			entry.mdPresent = true
			entry.mdContent = content
		case changeContractFileShape:
			entry.shapePresent = true
			entry.shapeContent = content
		case changeBriefFile:
			entry.briefPresent = true
			entry.briefContent = content
		}
	}
	folders := make([]string, 0, len(byFolder))
	for folder := range byFolder {
		folders = append(folders, folder)
	}
	sort.Strings(folders)
	var nodes []changeNode
	for _, folder := range folders {
		entry := byFolder[folder]
		node, ok := assembleChangeNodeFromCommittedFiles(folder, entry.jsonPresent, entry.jsonContent, entry.mdPresent, entry.mdContent)
		if !ok {
			continue
		}
		if node.Layout == changeLayoutNew {
			switch {
			case entry.shapePresent:
				node.Content = entry.shapeContent
				node.ContractFile = filepath.ToSlash(filepath.Join(folder, changeContractFileShape))
				node.CapturedOnly = false
			case entry.briefPresent:
				node.Content = entry.briefContent
				node.ContractFile = filepath.ToSlash(filepath.Join(folder, changeBriefFile))
				node.CapturedOnly = true
			case entry.mdPresent && node.Content == "":
				node.Content = entry.mdContent
				node.ContractFile = filepath.ToSlash(filepath.Join(folder, changeMachineFileLegacy))
			default:
				if !entry.shapePresent && !entry.briefPresent {
					node.CapturedOnly = true
				}
			}
		}
		nodes = append(nodes, node)
	}
	return nodes, nil
}

func deriveChangeGraph(nodes []changeNode) changeGraph {
	g := changeGraph{
		Nodes:                  append([]changeNode{}, nodes...),
		findingsByLineage:      map[string][]string{},
		localFindingsByLineage: map[string][]string{},
		localFindingsByChange:  map[string][]string{},
		gapsByLineage:          map[string][]string{},
	}
	bySlug := map[string][]changeNode{}
	byLineage := map[string][]changeNode{}
	for _, node := range nodes {
		for _, finding := range node.ParseFindings {
			g.addLocalFinding(node, prefixChangeFinding(node.ChangeFile, finding))
		}
		var fields []changeFrontmatterField
		if node.Layout == changeLayoutLegacy {
			parsed := parseChangeFrontmatter(node.MetaContent)
			fields = parsed.Fields
			if !parsed.AtByteOne {
				g.addLocalFinding(node, prefixChangeFinding(node.ChangeFile, "frontmatter must open the file at byte one"))
			}
			for _, finding := range parsed.Findings {
				g.addLocalFinding(node, prefixChangeFinding(node.ChangeFile, finding))
			}
			for _, key := range []string{"change", "created", "lineage", "predecessor", "release-after", "target_release"} {
				if countChangeFields(fields, key) > 1 {
					g.addLocalFinding(node, prefixChangeFinding(node.ChangeFile, fmt.Sprintf("duplicate frontmatter field %q", key)))
				}
			}
		}
		folder := filepath.Base(node.Folder)
		match := changeFolderRE.FindStringSubmatch(folder)
		if match == nil {
			g.addLocalFinding(node, fmt.Sprintf("%s: invalid Change folder identity", node.ChangeFile))
		} else {
			created := node.Created
			if created == "" {
				created = changeFieldValue(fields, "created")
			}
			wantCreated := match[1][0:4] + "-" + match[1][4:6] + "-" + match[1][6:8]
			if node.Slug != match[2] {
				g.addLocalFinding(node, fmt.Sprintf("%s: change %q does not match folder slug %q", node.ChangeFile, node.Slug, match[2]))
			}
			if created != "" && created != wantCreated {
				g.addLocalFinding(node, fmt.Sprintf("%s: created %q does not match folder date %q", node.ChangeFile, created, wantCreated))
			}
		}
		if node.Lineage == "" && (node.Predecessor != "" || node.ReleaseAfter != "") {
			g.addLocalFinding(node, fmt.Sprintf("%s: predecessor and release-after require lineage", node.ChangeFile))
		}
		if node.Slug != "" {
			bySlug[node.Slug] = append(bySlug[node.Slug], node)
		}
		if node.Lineage != "" {
			byLineage[node.Lineage] = append(byLineage[node.Lineage], node)
		}
	}
	for slug, duplicates := range bySlug {
		if len(duplicates) > 1 {
			g.addGlobalFinding(fmt.Sprintf("duplicate Change slug %q is globally materialized at %s", slug, joinChangePaths(duplicates)))
		}
	}
	for lineage, lineageNodes := range byLineage {
		lineageBySlug := map[string]changeNode{}
		children := map[string][]string{}
		roots := []string{}
		terminals := map[string]bool{}
		for _, node := range lineageNodes {
			lineageBySlug[node.Slug] = node
			if node.Predecessor == "" {
				roots = append(roots, node.Slug)
			} else {
				children[node.Predecessor] = append(children[node.Predecessor], node.Slug)
			}
			if node.ReleaseAfter != "" {
				terminals[node.ReleaseAfter] = true
			}
		}
		sort.Strings(roots)
		if len(roots) > 1 {
			g.addFinding(lineage, fmt.Sprintf("lineage %q has multiple roots: %s", lineage, strings.Join(roots, ", ")))
		} else if len(roots) == 1 {
			root := lineageBySlug[roots[0]]
			for _, node := range lineageNodes {
				if node.Slug != root.Slug && node.ReleaseAfter != "" {
					g.addFinding(lineage, fmt.Sprintf("Change %q declares release-after; lineage %q root %q must own the declaration", node.Slug, lineage, root.Slug))
				}
			}
		}
		for _, node := range lineageNodes {
			if node.Predecessor == node.Slug && node.Slug != "" {
				g.addFinding(lineage, fmt.Sprintf("Change %q cannot name itself as predecessor", node.Slug))
				continue
			}
			if node.Predecessor == "" {
				continue
			}
			predecessors := bySlug[node.Predecessor]
			if len(predecessors) == 0 {
				g.addGap(lineage, fmt.Sprintf("Change %q predecessor %q is not materialized", node.Slug, node.Predecessor))
			} else if len(predecessors) == 1 && predecessors[0].Lineage != lineage {
				g.addFinding(lineage, fmt.Sprintf("Change %q predecessor %q has lineage %q, want %q", node.Slug, node.Predecessor, predecessors[0].Lineage, lineage))
			}
		}
		for predecessor, successors := range children {
			sort.Strings(successors)
			if len(successors) > 1 {
				g.addFinding(lineage, fmt.Sprintf("Change %q has multiple materialized children: %s", predecessor, strings.Join(successors, ", ")))
			}
		}
		if changeLineageHasCycle(lineageBySlug) {
			g.addFinding(lineage, fmt.Sprintf("lineage %q contains a predecessor cycle", lineage))
		}
		terminalNames := sortedKeys(terminals)
		if len(terminalNames) > 1 {
			g.addFinding(lineage, fmt.Sprintf("lineage %q has conflicting release-after terminals: %s", lineage, strings.Join(terminalNames, ", ")))
		} else if len(terminalNames) == 1 {
			terminal, ok := lineageBySlug[terminalNames[0]]
			if ok && len(children[terminal.Slug]) != 0 {
				g.addFinding(lineage, fmt.Sprintf("release-after %q is not the lineage terminal", terminal.Slug))
			}
		}
	}
	g.sort()
	return g
}

func changeLineageHasCycle(nodes map[string]changeNode) bool {
	visiting, visited := map[string]bool{}, map[string]bool{}
	var visit func(string) bool
	visit = func(slug string) bool {
		if visiting[slug] {
			return true
		}
		if visited[slug] {
			return false
		}
		visited[slug] = true
		visiting[slug] = true
		node := nodes[slug]
		if node.Predecessor != "" {
			if _, ok := nodes[node.Predecessor]; ok && visit(node.Predecessor) {
				return true
			}
		}
		visiting[slug] = false
		return false
	}
	for slug := range nodes {
		if visit(slug) {
			return true
		}
	}
	return false
}

func applyLineageValidation(report changeCheckReport, nodes []changeNode, targetPath, rootPath string, requireExecutable bool) changeCheckReport {
	graph := deriveChangeGraph(nodes)
	target, ok := graph.nodeByPath(targetPath)
	if !ok {
		report.Violations = append(report.Violations, fmt.Sprintf("checked Change %s is absent from the derived graph", targetPath))
		return report
	}
	report.Violations = append(report.Violations, graph.findingsForChange(target)...)
	lineageGaps := graph.gapsForLineage(target.Lineage)
	executionGaps := executionRelevantLineageGaps(lineageGaps)
	report.Gaps = append(report.Gaps, executionGaps...)
	if requireExecutable {
		report.Gaps = append(report.Gaps, committedPredecessorGaps(rootPath, target)...)
	}
	report.Violations = sortedUnique(report.Violations)
	report.Warnings = sortedUnique(report.Warnings)
	report.Gaps = sortedUnique(report.Gaps)
	report.Executable = report.Executable && len(report.Violations) == 0 && len(report.Gaps) == 0
	return report
}

func committedPredecessorGaps(rootPath string, target changeNode) []string {
	if target.Predecessor == "" {
		return nil
	}
	nodes, err := loadChangeNodesAtHEAD(rootPath)
	if err != nil {
		return []string{fmt.Sprintf("cannot inspect committed HEAD Change graph: %v", err)}
	}
	graph := deriveChangeGraph(nodes)
	bySlug := map[string]changeNode{}
	for _, node := range nodes {
		bySlug[node.Slug] = node
	}
	var gaps []string
	seen := map[string]bool{}
	for slug := target.Predecessor; slug != ""; {
		if seen[slug] {
			break
		}
		seen[slug] = true
		node, ok := bySlug[slug]
		if !ok || node.Lineage != target.Lineage {
			gaps = append(gaps, fmt.Sprintf("predecessor %q is not committed and retained in HEAD", slug))
			break
		}
		doc := evaluateChangeDocAtPath(node.Content, filepath.Base(node.Folder), "", node.ChangeFile)
		lineageGaps := executionRelevantLineageGaps(graph.gapsForLineage(node.Lineage))
		if len(doc.Violations) != 0 || !doc.Executable || len(graph.findingsForLineage(node.Lineage)) != 0 || len(lineageGaps) != 0 {
			gaps = append(gaps, fmt.Sprintf("committed predecessor %q is not structurally executable", slug))
		}
		slug = node.Predecessor
	}
	sort.Strings(gaps)
	return gaps
}

func removeChangeGraphGap(gaps []string, ignored string) []string {
	filtered := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		if gap != ignored {
			filtered = append(filtered, gap)
		}
	}
	return filtered
}

func deletedLineageChangesWithOutput(rootPath string, outputCommand changeGitOutput) ([]string, error) {
	output, err := outputCommand(rootPath, "git", "rev-list", "--full-history", "--topo-order", "HEAD", "--", "docs/changes")
	if err != nil {
		return nil, fmt.Errorf("enumerate Change history: %w", err)
	}
	var deleted []string
	for _, commit := range strings.Fields(output) {
		parentsOutput, err := outputCommand(rootPath, "git", "rev-list", "--parents", "-n", "1", commit)
		if err != nil {
			return nil, fmt.Errorf("read parents for %s: %w", shortChangeCommit(commit), err)
		}
		ancestry := strings.Fields(parentsOutput)
		if len(ancestry) == 0 || ancestry[0] != commit {
			return nil, fmt.Errorf("read parents for %s: unexpected git response %q", shortChangeCommit(commit), strings.TrimSpace(parentsOutput))
		}
		for _, parent := range ancestry[1:] {
			diffOutput, err := outputCommand(rootPath, "git", "diff-tree", "--no-commit-id", "--name-status", "--no-renames", "-r", parent, commit, "--", "docs/changes")
			if err != nil {
				return nil, fmt.Errorf("compare %s with parent %s: %w", shortChangeCommit(commit), shortChangeCommit(parent), err)
			}
			type folderDiff struct {
				deletedMD   bool
				deletedJSON bool
				addedJSON   bool
				mdPath      string
				jsonPath    string
			}
			byFolder := map[string]*folderDiff{}
			for _, line := range strings.Split(diffOutput, "\n") {
				status, path, ok := strings.Cut(strings.TrimSpace(line), "\t")
				path = filepath.ToSlash(strings.TrimSpace(path))
				if !ok || path == "" {
					continue
				}
				base := filepath.Base(path)
				if base != changeMachineFileLegacy && base != changeMachineFileJSON {
					continue
				}
				folder := filepath.ToSlash(filepath.Dir(path))
				entry := byFolder[folder]
				if entry == nil {
					entry = &folderDiff{}
					byFolder[folder] = entry
				}
				switch {
				case strings.HasPrefix(status, "D") && base == changeMachineFileLegacy:
					entry.deletedMD = true
					entry.mdPath = path
				case strings.HasPrefix(status, "D") && base == changeMachineFileJSON:
					entry.deletedJSON = true
					entry.jsonPath = path
				case strings.HasPrefix(status, "A") && base == changeMachineFileJSON:
					entry.addedJSON = true
					entry.jsonPath = path
				}
			}
			for folder, entry := range byFolder {
				// Sanctioned atomic conversion: retire change.md and add change.json
				// in the same commit. That is replacement, not retention loss.
				if entry.deletedMD && entry.addedJSON && !entry.deletedJSON {
					continue
				}
				if entry.deletedMD {
					retained, err := changePathHadRetentionSignalInHistory(rootPath, parent, entry.mdPath, outputCommand)
					if err != nil {
						return nil, err
					}
					if retained {
						deleted = append(deleted, entry.mdPath)
					}
				}
				if entry.deletedJSON {
					jsonPath := entry.jsonPath
					if jsonPath == "" {
						jsonPath = filepath.ToSlash(filepath.Join(folder, changeMachineFileJSON))
					}
					retained, err := changePathHadRetentionSignalInHistory(rootPath, parent, jsonPath, outputCommand)
					if err != nil {
						return nil, err
					}
					if retained {
						deleted = append(deleted, jsonPath)
					}
				}
			}
		}
	}
	return sortedUnique(deleted), nil
}

func changePathHadLineageInHistory(rootPath string, ref string, path string, outputCommand changeGitOutput) (bool, error) {
	return changePathHadRetentionSignalInHistory(rootPath, ref, path, outputCommand)
}

func changePathHadRetentionSignalInHistory(rootPath string, ref string, path string, outputCommand changeGitOutput) (bool, error) {
	output, err := outputCommand(rootPath, "git", "rev-list", "--full-history", "--topo-order", ref, "--", path)
	if err != nil {
		return false, fmt.Errorf("enumerate %s history from %s: %w", path, shortChangeCommit(ref), err)
	}
	commits := strings.Fields(output)
	if len(commits) == 0 {
		return false, fmt.Errorf("enumerate %s history from %s: no commits found for deleted path", path, shortChangeCommit(ref))
	}
	for _, commit := range commits {
		treePath, err := outputCommand(rootPath, "git", "ls-tree", "--name-only", commit, "--", path)
		if err != nil {
			return false, fmt.Errorf("inspect %s at %s: %w", path, shortChangeCommit(commit), err)
		}
		treePath = filepath.ToSlash(strings.TrimSpace(treePath))
		if treePath == "" {
			continue
		}
		if treePath != path {
			return false, fmt.Errorf("inspect %s at %s: unexpected git path %q", path, shortChangeCommit(commit), treePath)
		}
		content, err := outputCommand(rootPath, "git", "show", commit+":"+path)
		if err != nil {
			return false, fmt.Errorf("read %s at %s: %w", path, shortChangeCommit(commit), err)
		}
		if strings.HasSuffix(path, "/"+changeMachineFileJSON) {
			if changeJSONDeclaresTarget(content) {
				return true, nil
			}
			continue
		}
		parsed := parseChangeFrontmatter(content)
		if hasNonEmptyChangeField(parsed.Fields, "lineage") || hasNonEmptyChangeField(parsed.Fields, "release-after") || hasNonEmptyChangeField(parsed.Fields, "target_release") {
			return true, nil
		}
	}
	return false, nil
}

func changeJSONDeclaresTarget(content string) bool {
	meta := parseChangeJSON(content)
	if meta.TargetRelease != "" {
		return true
	}
	// Malformed historical versions that still named the field count as a
	// retention signal so delete/re-add cannot launder a declared target away.
	trimmed := strings.TrimSpace(content)
	return strings.Contains(trimmed, `"target_release"`)
}

type dependencyMetadataVersion struct {
	Commit       string
	Lineage      string
	ReleaseAfter string
	Problems     []string
	Duplicate    []string
}

func dependencyMetadataHistoryFindings(rootPath string, nodes []changeNode, outputCommand changeGitOutput) ([]string, error) {
	var findings []string
	for _, node := range nodes {
		// Lineage/release-after freeze still keys on the markdown surface. New
		// layout nodes without a historical change.md simply have no frozen
		// dependency metadata; target_release mutability is handled separately.
		historyPath := filepath.ToSlash(filepath.Join(node.Folder, changeMachineFileLegacy))
		if node.Layout == changeLayoutLegacy {
			historyPath = node.ChangeFile
		}
		commitsOutput, err := outputCommand(rootPath, "git", "rev-list", "--full-history", "--topo-order", "--reverse", "HEAD", "--", historyPath)
		if err != nil {
			return nil, fmt.Errorf("read %s history: %w", historyPath, err)
		}
		commits := strings.Fields(commitsOutput)
		if len(commits) == 0 {
			continue
		}
		versions := make([]dependencyMetadataVersion, 0, len(commits))
		hasDependencyMetadata := false
		for _, commit := range commits {
			content, ok, err := readCommittedOptional(rootPath, commit, historyPath, outputCommand)
			if err != nil {
				return nil, err
			}
			if !ok {
				continue
			}
			parsed := parseChangeFrontmatter(content)
			version := dependencyMetadataVersion{
				Commit:       commit,
				Lineage:      changeFieldValue(parsed.Fields, "lineage"),
				ReleaseAfter: changeFieldValue(parsed.Fields, "release-after"),
				Problems:     changeFrontmatterInspectionProblems(parsed),
			}
			if countChangeFields(parsed.Fields, "lineage") > 1 {
				version.Duplicate = append(version.Duplicate, "lineage")
			}
			if countChangeFields(parsed.Fields, "release-after") > 1 {
				version.Duplicate = append(version.Duplicate, "release-after")
			}
			if version.Lineage != "" || version.ReleaseAfter != "" {
				hasDependencyMetadata = true
			}
			versions = append(versions, version)
		}
		if !hasDependencyMetadata {
			continue
		}
		for _, version := range versions {
			if len(version.Problems) != 0 {
				return nil, fmt.Errorf("parse %s at %s: %s", historyPath, shortChangeCommit(version.Commit), strings.Join(version.Problems, "; "))
			}
			if len(version.Duplicate) != 0 {
				return nil, fmt.Errorf("parse %s at %s: duplicate %s field", historyPath, shortChangeCommit(version.Commit), strings.Join(version.Duplicate, " and "))
			}
		}
		findings = append(findings, immutableDependencyFieldFindings(historyPath, "lineage", versions, func(version dependencyMetadataVersion) string { return version.Lineage })...)
		findings = append(findings, immutableDependencyFieldFindings(historyPath, "release-after", versions, func(version dependencyMetadataVersion) string { return version.ReleaseAfter })...)
	}
	return sortedUnique(findings), nil
}

func immutableDependencyFieldFindings(path string, field string, versions []dependencyMetadataVersion, valueOf func(dependencyMetadataVersion) string) []string {
	frozenValue := ""
	frozenCommit := ""
	var findings []string
	for _, version := range versions {
		value := valueOf(version)
		if frozenValue == "" {
			if value != "" {
				frozenValue = value
				frozenCommit = version.Commit
			}
			continue
		}
		if value != frozenValue {
			findings = append(findings, fmt.Sprintf("%s changed %s from %q (set at %s) to %q at %s", path, field, frozenValue, shortChangeCommit(frozenCommit), value, shortChangeCommit(version.Commit)))
		}
	}
	return findings
}

func changeFrontmatterInspectionProblems(parsed changeFrontmatterParse) []string {
	var problems []string
	if !parsed.AtByteOne {
		problems = append(problems, "frontmatter must open at byte one")
	}
	problems = append(problems, parsed.Findings...)
	return sortedUnique(problems)
}

func shortChangeCommit(commit string) string {
	if len(commit) > 12 {
		return commit[:12]
	}
	return commit
}

func (g changeGraph) nodeByPath(path string) (changeNode, bool) {
	path = filepath.ToSlash(path)
	folder := changeFolderRelFromMachinePath(path)
	for _, node := range g.Nodes {
		if node.ChangeFile == path || node.Folder == path || node.Folder == folder || node.ContractFile == path {
			return node, true
		}
	}
	return changeNode{}, false
}

func (g changeGraph) findingsForLineage(lineage string) []string {
	findings := append([]string{}, g.GlobalFindings...)
	findings = append(findings, g.findingsByLineage[lineage]...)
	findings = append(findings, g.localFindingsByLineage[lineage]...)
	return sortedUnique(findings)
}
func (g changeGraph) findingsForChange(node changeNode) []string {
	if node.Lineage != "" {
		return g.findingsForLineage(node.Lineage)
	}
	return sortedUnique(append(append([]string{}, g.GlobalFindings...), g.localFindingsByChange[node.ChangeFile]...))
}
func (g changeGraph) gapsForLineage(lineage string) []string {
	return sortedUnique(g.gapsByLineage[lineage])
}
func (g *changeGraph) addFinding(lineage, finding string) {
	g.Findings = append(g.Findings, finding)
	g.findingsByLineage[lineage] = append(g.findingsByLineage[lineage], finding)
}
func (g *changeGraph) addLocalFinding(node changeNode, finding string) {
	g.Findings = append(g.Findings, finding)
	g.localFindingsByChange[node.ChangeFile] = append(g.localFindingsByChange[node.ChangeFile], finding)
	if node.Lineage != "" {
		g.localFindingsByLineage[node.Lineage] = append(g.localFindingsByLineage[node.Lineage], finding)
	}
}
func (g *changeGraph) addGlobalFinding(finding string) {
	g.Findings = append(g.Findings, finding)
	g.GlobalFindings = append(g.GlobalFindings, finding)
}
func (g *changeGraph) addGap(lineage, gap string) {
	g.Gaps = append(g.Gaps, gap)
	g.gapsByLineage[lineage] = append(g.gapsByLineage[lineage], gap)
}
func (g *changeGraph) sort() {
	g.Findings = sortedUnique(g.Findings)
	g.Gaps = sortedUnique(g.Gaps)
	g.GlobalFindings = sortedUnique(g.GlobalFindings)
	sort.Slice(g.Nodes, func(i, j int) bool { return g.Nodes[i].ChangeFile < g.Nodes[j].ChangeFile })
}

func countChangeFields(fields []changeFrontmatterField, key string) int {
	count := 0
	for _, field := range fields {
		if strings.EqualFold(field.Key, key) {
			count++
		}
	}
	return count
}
func hasNonEmptyChangeField(fields []changeFrontmatterField, key string) bool {
	for _, field := range fields {
		if strings.EqualFold(field.Key, key) && field.Value != "" {
			return true
		}
	}
	return false
}
func joinChangePaths(nodes []changeNode) string {
	paths := make([]string, 0, len(nodes))
	for _, node := range nodes {
		paths = append(paths, node.ChangeFile)
	}
	sort.Strings(paths)
	return strings.Join(paths, ", ")
}
func sortedUnique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, value := range values {
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func executionRelevantLineageGaps(gaps []string) []string {
	return sortedUnique(gaps)
}
