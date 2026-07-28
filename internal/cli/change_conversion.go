package cli

import (
	"fmt"
	"path/filepath"
	"strings"
)

// conversionPreCheckedGrandfathers lists conversion commits that predate the
// unchecked-at-conversion rule and are tracked for remediation rather than
// blocking check forever. See INTENT-20260727-dogfood-conversion-manufactured-task-003-execution.
var conversionPreCheckedGrandfathers = map[string]bool{
	"acbea95001f9187b154d095f4579225b7744fe1d": true,
}

// conversionPreCheckedFindings reports sanctioned atomic conversions whose
// resulting tasks/ tree carried any checked checkbox. Flip-grade execution must
// come from later delivering commits, never be manufactured by conversion.
// Findings surface on loaf change check (not only release preflight): a rule
// that fires only at release arrives too late to be useful.
func conversionPreCheckedFindings(rootPath, folderRel string, outputCommand changeGitOutput) ([]string, error) {
	if outputCommand == nil {
		outputCommand = commandOutput
	}
	folderRel = filepath.ToSlash(strings.TrimSpace(folderRel))
	if folderRel == "" {
		return nil, nil
	}
	mdPath := filepath.ToSlash(filepath.Join(folderRel, changeMachineFileLegacy))
	jsonPath := filepath.ToSlash(filepath.Join(folderRel, changeMachineFileJSON))
	output, err := outputCommand(rootPath, "git", "rev-list", "--full-history", "--topo-order", "HEAD", "--", mdPath, jsonPath)
	if err != nil {
		return nil, fmt.Errorf("enumerate conversion history for %s: %w", folderRel, err)
	}
	var findings []string
	for _, commit := range strings.Fields(output) {
		if conversionPreCheckedGrandfathers[commit] {
			continue
		}
		parentsOutput, err := outputCommand(rootPath, "git", "rev-list", "--parents", "-n", "1", commit)
		if err != nil {
			return nil, fmt.Errorf("read parents for %s: %w", shortChangeCommit(commit), err)
		}
		ancestry := strings.Fields(parentsOutput)
		if len(ancestry) == 0 || ancestry[0] != commit {
			return nil, fmt.Errorf("read parents for %s: unexpected git response %q", shortChangeCommit(commit), strings.TrimSpace(parentsOutput))
		}
		for _, parent := range ancestry[1:] {
			diffOutput, err := outputCommand(rootPath, "git", "diff-tree", "--no-commit-id", "--name-status", "--no-renames", "-r", parent, commit, "--", folderRel)
			if err != nil {
				return nil, fmt.Errorf("compare %s with parent %s: %w", shortChangeCommit(commit), shortChangeCommit(parent), err)
			}
			deletedMD := false
			addedJSON := false
			deletedJSON := false
			for _, line := range strings.Split(diffOutput, "\n") {
				status, path, ok := strings.Cut(strings.TrimSpace(line), "\t")
				path = filepath.ToSlash(strings.TrimSpace(path))
				if !ok || path == "" {
					continue
				}
				base := filepath.Base(path)
				switch {
				case strings.HasPrefix(status, "D") && base == changeMachineFileLegacy && path == mdPath:
					deletedMD = true
				case strings.HasPrefix(status, "A") && base == changeMachineFileJSON && path == jsonPath:
					addedJSON = true
				case strings.HasPrefix(status, "D") && base == changeMachineFileJSON && path == jsonPath:
					deletedJSON = true
				}
			}
			if !(deletedMD && addedJSON && !deletedJSON) {
				continue
			}
			offending, err := conversionCommitCheckedTaskFiles(rootPath, commit, folderRel, outputCommand)
			if err != nil {
				return nil, err
			}
			for _, path := range offending {
				findings = append(findings, fmt.Sprintf("%s: conversion commit %s carries checked task checkbox(es); sanctioned conversion must land with all boxes unchecked", path, shortChangeCommit(commit)))
			}
		}
	}
	return sortedUnique(findings), nil
}

func conversionCommitCheckedTaskFiles(rootPath, commit, folderRel string, outputCommand changeGitOutput) ([]string, error) {
	tasksPrefix := filepath.ToSlash(filepath.Join(folderRel, "tasks")) + "/"
	listOutput, err := outputCommand(rootPath, "git", "ls-tree", "-r", "--name-only", commit, "--", filepath.ToSlash(filepath.Join(folderRel, "tasks")))
	if err != nil {
		return nil, fmt.Errorf("list tasks at %s: %w", shortChangeCommit(commit), err)
	}
	var offending []string
	for _, path := range strings.Split(listOutput, "\n") {
		path = filepath.ToSlash(strings.TrimSpace(path))
		if path == "" || !strings.HasPrefix(path, tasksPrefix) {
			continue
		}
		if !changeTaskFileRE.MatchString(filepath.Base(path)) {
			continue
		}
		content, err := outputCommand(rootPath, "git", "show", commit+":"+path)
		if err != nil {
			return nil, fmt.Errorf("read %s at %s: %w", path, shortChangeCommit(commit), err)
		}
		body := stripMarkdownCodeFences(content)
		for _, m := range changeTaskCheckbox.FindAllStringSubmatch(body, -1) {
			if strings.EqualFold(m[1], "x") {
				offending = append(offending, path)
				break
			}
		}
	}
	return offending, nil
}
