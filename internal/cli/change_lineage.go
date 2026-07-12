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
	var nodes []changeNode
	for _, path := range strings.Split(strings.TrimSpace(output), "\n") {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" || !strings.HasSuffix(path, "/change.md") {
			continue
		}
		content, err := outputCommand(rootPath, "git", "show", "HEAD:"+path)
		if err != nil {
			return nil, fmt.Errorf("read committed %s: %w", path, err)
		}
		fields, _ := changeFrontmatterFields(content)
		nodes = append(nodes, changeNode{
			Slug: changeFieldValue(fields, "change"), Branch: changeFieldValue(fields, "branch"), Lineage: changeFieldValue(fields, "lineage"),
			Predecessor: changeFieldValue(fields, "predecessor"), ReleaseAfter: changeFieldValue(fields, "release-after"),
			Folder: filepath.ToSlash(filepath.Dir(path)), ChangeFile: path, Content: content,
		})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].ChangeFile < nodes[j].ChangeFile })
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
		parsed := parseChangeFrontmatter(node.Content)
		fields, atByteOne := parsed.Fields, parsed.AtByteOne
		if !atByteOne {
			g.addLocalFinding(node, prefixChangeFinding(node.ChangeFile, "frontmatter must open the file at byte one"))
		}
		for _, finding := range parsed.Findings {
			g.addLocalFinding(node, prefixChangeFinding(node.ChangeFile, finding))
		}
		for _, key := range []string{"change", "created", "lineage", "predecessor", "release-after"} {
			if countChangeFields(fields, key) > 1 {
				g.addLocalFinding(node, prefixChangeFinding(node.ChangeFile, fmt.Sprintf("duplicate frontmatter field %q", key)))
			}
		}
		folder := filepath.Base(node.Folder)
		match := changeFolderRE.FindStringSubmatch(folder)
		if match == nil {
			g.addLocalFinding(node, fmt.Sprintf("%s: invalid Change folder identity", node.ChangeFile))
		} else {
			created := changeFieldValue(fields, "created")
			wantCreated := match[1][0:4] + "-" + match[1][4:6] + "-" + match[1][6:8]
			if node.Slug != match[2] {
				g.addLocalFinding(node, fmt.Sprintf("%s: change %q does not match folder slug %q", node.ChangeFile, node.Slug, match[2]))
			}
			if created != wantCreated {
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
			if root.ReleaseAfter == "" {
				g.addGap(lineage, fmt.Sprintf("lineage %q root %q must declare release-after", lineage, root.Slug))
			}
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
			if !ok {
				g.addGap(lineage, fmt.Sprintf("release-after terminal %q is not materialized", terminalNames[0]))
			} else if len(children[terminal.Slug]) != 0 {
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
	for _, gap := range lineageGaps {
		if strings.HasPrefix(gap, "release-after terminal ") {
			report.Warnings = append(report.Warnings, gap)
		}
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

func releaseLineagePreflight(rootPath string) error {
	return releaseLineagePreflightWithOutput(rootPath, commandOutput)
}

func releaseLineagePreflightWithOutput(rootPath string, outputCommand changeGitOutput) error {
	nodes, err := loadChangeNodesAtHEADWithOutput(rootPath, outputCommand)
	if err != nil {
		return fmt.Errorf("release blocked: cannot inspect committed Change graph at HEAD: %w", err)
	}
	if err := requireCompleteChangeHistory(rootPath, outputCommand); err != nil {
		return fmt.Errorf("release blocked: cannot confirm complete Change history: %w", err)
	}
	deleted, err := deletedLineageChangesWithOutput(rootPath, outputCommand)
	if err != nil {
		return fmt.Errorf("release blocked: cannot inspect deleted or renamed Change history at HEAD: %w", err)
	}
	if len(deleted) != 0 {
		return fmt.Errorf("release blocked: retained lineage Change deleted or renamed in HEAD ancestry: %s", strings.Join(deleted, ", "))
	}
	historyFindings, err := dependencyMetadataHistoryFindings(rootPath, nodes, outputCommand)
	if err != nil {
		return fmt.Errorf("release blocked: cannot inspect immutable dependency metadata history: %w", err)
	}
	if len(historyFindings) != 0 {
		return fmt.Errorf("release blocked: immutable dependency metadata changed: %s", strings.Join(historyFindings, "; "))
	}
	for _, node := range nodes {
		if node.Lineage == "" && (node.Predecessor != "" || node.ReleaseAfter != "") {
			return fmt.Errorf("release blocked: %s declares predecessor or release-after without lineage", node.ChangeFile)
		}
	}
	graph := deriveChangeGraph(nodes)
	lineages := map[string]bool{}
	for _, node := range nodes {
		if node.Lineage != "" {
			lineages[node.Lineage] = true
		}
	}
	for _, lineage := range sortedKeys(lineages) {
		if findings := graph.findingsForLineage(lineage); len(findings) != 0 {
			return fmt.Errorf("release blocked: lineage %q is structurally invalid: %s", lineage, strings.Join(findings, "; "))
		}
		for _, node := range nodes {
			if node.Lineage != lineage {
				continue
			}
			doc := evaluateChangeDocAtPath(node.Content, filepath.Base(node.Folder), "", node.ChangeFile)
			if len(doc.Violations) != 0 || !doc.Executable {
				return fmt.Errorf("release blocked: lineage %q contains structurally invalid Change %q", lineage, node.Slug)
			}
		}
		var terminal string
		for _, node := range nodes {
			if node.Lineage == lineage && node.ReleaseAfter != "" {
				terminal = node.ReleaseAfter
			}
		}
		if terminal == "" {
			return fmt.Errorf("release blocked: active lineage %q has no release-after terminal", lineage)
		}
		found := false
		for _, node := range nodes {
			if node.Lineage == lineage && node.Slug == terminal {
				found = true
				doc := evaluateChangeDocAtPath(node.Content, filepath.Base(node.Folder), "", node.ChangeFile)
				if len(doc.Violations) != 0 || !doc.Executable {
					return fmt.Errorf("release blocked: terminal %q is not structurally executable", terminal)
				}
			}
		}
		if !found {
			return fmt.Errorf("release blocked: lineage %q is present in HEAD ancestry but release-after terminal %q is unsatisfied", lineage, terminal)
		}
		if gaps := graph.gapsForLineage(lineage); len(gaps) != 0 {
			return fmt.Errorf("release blocked: lineage %q is incomplete: %s", lineage, strings.Join(gaps, "; "))
		}
	}
	return nil
}

func requireCompleteChangeHistory(rootPath string, outputCommand changeGitOutput) error {
	output, err := outputCommand(rootPath, "git", "rev-parse", "--is-shallow-repository")
	if err != nil {
		return fmt.Errorf("inspect repository history depth: %w", err)
	}
	switch strings.TrimSpace(output) {
	case "false":
		return nil
	case "true":
		return fmt.Errorf("repository is shallow; fetch complete history with `git fetch --unshallow` before releasing")
	default:
		return fmt.Errorf("inspect repository history depth: unexpected git response %q", strings.TrimSpace(output))
	}
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
			for _, line := range strings.Split(diffOutput, "\n") {
				status, path, ok := strings.Cut(strings.TrimSpace(line), "\t")
				path = filepath.ToSlash(strings.TrimSpace(path))
				if !ok || !strings.HasPrefix(status, "D") || !strings.HasSuffix(path, "/change.md") {
					continue
				}
				hadLineage, err := changePathHadLineageInHistory(rootPath, parent, path, outputCommand)
				if err != nil {
					return nil, err
				}
				if !hadLineage {
					continue
				}
				deleted = append(deleted, path)
			}
		}
	}
	return sortedUnique(deleted), nil
}

func changePathHadLineageInHistory(rootPath string, ref string, path string, outputCommand changeGitOutput) (bool, error) {
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
		parsed := parseChangeFrontmatter(content)
		if hasNonEmptyChangeField(parsed.Fields, "lineage") {
			return true, nil
		}
	}
	return false, nil
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
		commitsOutput, err := outputCommand(rootPath, "git", "rev-list", "--full-history", "--topo-order", "--reverse", "HEAD", "--", node.ChangeFile)
		if err != nil {
			return nil, fmt.Errorf("read %s history: %w", node.ChangeFile, err)
		}
		commits := strings.Fields(commitsOutput)
		if len(commits) == 0 {
			return nil, fmt.Errorf("read %s history: no commits found for retained Change", node.ChangeFile)
		}
		versions := make([]dependencyMetadataVersion, 0, len(commits))
		hasDependencyMetadata := false
		for _, commit := range commits {
			content, err := outputCommand(rootPath, "git", "show", commit+":"+node.ChangeFile)
			if err != nil {
				return nil, fmt.Errorf("read %s at %s: %w", node.ChangeFile, commit, err)
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
				return nil, fmt.Errorf("parse %s at %s: %s", node.ChangeFile, shortChangeCommit(version.Commit), strings.Join(version.Problems, "; "))
			}
			if len(version.Duplicate) != 0 {
				return nil, fmt.Errorf("parse %s at %s: duplicate %s field", node.ChangeFile, shortChangeCommit(version.Commit), strings.Join(version.Duplicate, " and "))
			}
		}
		findings = append(findings, immutableDependencyFieldFindings(node.ChangeFile, "lineage", versions, func(version dependencyMetadataVersion) string { return version.Lineage })...)
		findings = append(findings, immutableDependencyFieldFindings(node.ChangeFile, "release-after", versions, func(version dependencyMetadataVersion) string { return version.ReleaseAfter })...)
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
	for _, node := range g.Nodes {
		if node.ChangeFile == path {
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
	var relevant []string
	for _, gap := range gaps {
		if !strings.HasPrefix(gap, "release-after terminal ") {
			relevant = append(relevant, gap)
		}
	}
	return sortedUnique(relevant)
}
