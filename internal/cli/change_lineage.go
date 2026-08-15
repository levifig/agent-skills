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
