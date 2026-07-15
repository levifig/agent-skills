package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type initOptions struct {
	symlinks bool
	help     bool
}

type detectedStackItem struct {
	name      string
	indicator string
}

type initProjectInfo struct {
	languages  []detectedStackItem
	frameworks []detectedStackItem
	existing   initExistingStructure
}

type initExistingStructure struct {
	hasAgentsDir bool
	hasAgentsMd  bool
	hasDocsDir   bool
	hasChangelog bool
	hasClaudeDir bool
	hasLoafJSON  bool
}

var initScaffoldDirs = []string{
	".agents",
	".agents/sessions",
	".agents/ideas",
	".agents/handoffs",
	".agents/specs",
	".agents/tasks",
	"docs",
	"docs/knowledge",
	"docs/decisions",
}

func (r Runner) runInit(args []string, out io.Writer, runtimeRoot string) error {
	options, err := parseInitArgs(args)
	if err != nil {
		return err
	}
	if options.help {
		writeInitHelp(out)
		return nil
	}
	info := detectInitProject(runtimeRoot)
	fmt.Fprintln(out)
	fmt.Fprintln(out, "loaf init")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  scanning project...")
	fmt.Fprintln(out)
	writeInitDetected(out, info)
	fmt.Fprintln(out)

	dirs, err := scaffoldInitDirs(runtimeRoot)
	if err != nil {
		return err
	}
	files, err := scaffoldInitFiles(runtimeRoot)
	if err != nil {
		return err
	}
	if len(dirs.created) > 0 || len(files.created) > 0 {
		fmt.Fprintln(out, "  Creating:")
		for _, dir := range dirs.created {
			fmt.Fprintf(out, "    + %s\n", dir)
		}
		for _, file := range files.created {
			fmt.Fprintf(out, "    + %s\n", file)
		}
		fmt.Fprintln(out)
	} else {
		fmt.Fprintln(out, "  Nothing to create - all files exist")
		fmt.Fprintln(out)
	}
	if len(dirs.skipped)+len(files.skipped) > 0 {
		fmt.Fprintln(out, "  Skipped (symlink points outside project):")
		for _, item := range append(dirs.skipped, files.skipped...) {
			fmt.Fprintf(out, "    ! %s\n", item)
		}
		fmt.Fprintln(out)
	}

	if options.symlinks {
		if err := r.offerInitSymlinks(runtimeRoot, out); err != nil {
			return err
		}
	}
	writeInitSkillRecommendations(out, info)
	fmt.Fprintln(out, "  ok Project initialized")
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  Next steps:")
	fmt.Fprintln(out, "    1. Edit AGENTS.md with your project details")
	fmt.Fprintln(out, "    2. Run loaf install to set up your AI tools")
	fmt.Fprintln(out)
	return nil
}

func parseInitArgs(args []string) (initOptions, error) {
	options := initOptions{symlinks: true}
	for _, arg := range args {
		switch arg {
		case "--no-symlinks":
			options.symlinks = false
		case "--help", "-h":
			options.help = true
		default:
			return initOptions{}, fmt.Errorf("unknown init option %q", arg)
		}
	}
	return options, nil
}

func writeInitHelp(out io.Writer) {
	fmt.Fprintln(out, strings.Join([]string{
		"Usage: loaf init [options]",
		"",
		"Initialize a project with Loaf structure.",
		"",
		"Options:",
		"  --no-symlinks  Skip symlink creation prompts",
		"  -h, --help     Show help",
	}, "\n"))
}

type scaffoldResult struct {
	created []string
	skipped []string
}

func scaffoldInitDirs(root string) (scaffoldResult, error) {
	var result scaffoldResult
	for _, dir := range initScaffoldDirs {
		fullPath := filepath.Join(root, filepath.FromSlash(dir))
		if _, err := os.Stat(fullPath); err == nil {
			continue
		}
		if !withinInitProject(root, fullPath) {
			result.skipped = append(result.skipped, dir+"/")
			continue
		}
		if err := os.MkdirAll(fullPath, 0o755); err != nil {
			return result, err
		}
		result.created = append(result.created, dir+"/")
	}
	return result, nil
}

func scaffoldInitFiles(root string) (scaffoldResult, error) {
	var result scaffoldResult
	for _, file := range initScaffoldFiles() {
		fullPath := filepath.Join(root, filepath.FromSlash(file.path))
		if _, err := os.Lstat(fullPath); err == nil {
			continue
		}
		if !withinInitProject(root, fullPath) {
			result.skipped = append(result.skipped, file.path)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			return result, err
		}
		if err := os.WriteFile(fullPath, []byte(file.body()), 0o644); err != nil {
			return result, err
		}
		result.created = append(result.created, file.path)
	}
	return result, nil
}

type initScaffoldFile struct {
	path string
	body func() string
}

func initScaffoldFiles() []initScaffoldFile {
	return []initScaffoldFile{
		{path: "AGENTS.md", body: func() string {
			return `# Project Instructions

> Agent instructions for this project. Customize per your needs.

## Quick Start

<!-- Add build/run commands here -->

## Project Structure

<!-- Describe your project layout -->

## Development Practices

<!-- Add coding conventions, testing approach, etc. -->

## Key Decisions

<!-- Link to ADRs in docs/decisions/ -->
`
		}},
		{path: ".agents/loaf.json", body: func() string {
			body, _ := json.MarshalIndent(map[string]string{
				"version":     "1.0.0",
				"initialized": time.Now().UTC().Format(time.RFC3339),
			}, "", "  ")
			return string(body) + "\n"
		}},
		{path: "docs/VISION.md", body: func() string {
			return `# Vision

## Purpose
<!-- Why does this project exist? What problem does it solve? -->

## Target Users
<!-- Who is this for? -->

## Success Criteria
<!-- How do you know when you've succeeded? -->

## Non-Goals
<!-- What is explicitly out of scope? -->
`
		}},
		{path: "docs/STRATEGY.md", body: func() string {
			return `# Strategy

## Current Focus
<!-- What are you working on right now and why? -->

## Priorities
<!-- Ordered list of what matters most -->

## Constraints
<!-- Budget, timeline, team size, technical limitations -->

## Open Questions
<!-- Unresolved strategic decisions -->
`
		}},
		{path: "docs/ARCHITECTURE.md", body: func() string {
			return `# Architecture

## Overview
<!-- High-level system description -->

## Components
<!-- Key components and their responsibilities -->

## Data Flow
<!-- How data moves through the system -->

## Technology Choices
<!-- Key technology decisions and rationale -->

## Deployment
<!-- How the system is deployed -->
`
		}},
		{path: "CHANGELOG.md", body: func() string {
			return `# Changelog

This project follows [Common Changelog](https://common-changelog.org/) and
[Semantic Versioning](https://semver.org/spec/v2.0.0.html). ` + "`## [Unreleased]`" + `
is a workflow staging section for curated entries before release.

## [Unreleased]
`
		}},
	}
}

func withinInitProject(root string, fullPath string) bool {
	check := fullPath
	for {
		if _, err := os.Lstat(check); err == nil || check == root {
			break
		}
		parent := filepath.Dir(check)
		if parent == check {
			break
		}
		check = parent
	}
	realCheck, err := filepath.EvalSymlinks(check)
	if err != nil {
		return false
	}
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return false
	}
	return realCheck == realRoot || strings.HasPrefix(realCheck, realRoot+string(filepath.Separator))
}

func detectInitProject(root string) initProjectInfo {
	languages := detectInitLanguages(root)
	frameworks := detectInitFrameworks(root, languages)
	return initProjectInfo{
		languages:  languages,
		frameworks: frameworks,
		existing: initExistingStructure{
			hasAgentsDir: pathExistsNative(filepath.Join(root, ".agents")),
			hasAgentsMd:  pathExistsNative(filepath.Join(root, "AGENTS.md")),
			hasDocsDir:   pathExistsNative(filepath.Join(root, "docs")),
			hasChangelog: pathExistsNative(filepath.Join(root, "CHANGELOG.md")),
			hasClaudeDir: pathExistsNative(filepath.Join(root, ".claude")),
			hasLoafJSON:  pathExistsNative(filepath.Join(root, ".agents", "loaf.json")),
		},
	}
}

func detectInitLanguages(root string) []detectedStackItem {
	var languages []detectedStackItem
	if pathExistsNative(filepath.Join(root, "tsconfig.json")) {
		languages = append(languages, detectedStackItem{name: "TypeScript", indicator: "tsconfig.json"})
	} else if fileContains(filepath.Join(root, "package.json"), `"typescript"`) || fileContains(filepath.Join(root, "package.json"), `"ts-node"`) {
		languages = append(languages, detectedStackItem{name: "TypeScript", indicator: "package.json (typescript in deps)"})
	}
	if !hasDetected(languages, "TypeScript") && pathExistsNative(filepath.Join(root, "package.json")) {
		languages = append(languages, detectedStackItem{name: "JavaScript", indicator: "package.json"})
	}
	switch {
	case pathExistsNative(filepath.Join(root, "pyproject.toml")):
		languages = append(languages, detectedStackItem{name: "Python", indicator: "pyproject.toml"})
	case pathExistsNative(filepath.Join(root, "setup.py")):
		languages = append(languages, detectedStackItem{name: "Python", indicator: "setup.py"})
	case pathExistsNative(filepath.Join(root, "requirements.txt")):
		languages = append(languages, detectedStackItem{name: "Python", indicator: "requirements.txt"})
	case pathExistsNative(filepath.Join(root, "uv.lock")):
		languages = append(languages, detectedStackItem{name: "Python", indicator: "uv.lock"})
	case pathExistsNative(filepath.Join(root, "Pipfile")):
		languages = append(languages, detectedStackItem{name: "Python", indicator: "Pipfile"})
	}
	switch {
	case pathExistsNative(filepath.Join(root, "Gemfile")):
		languages = append(languages, detectedStackItem{name: "Ruby", indicator: "Gemfile"})
	case pathExistsNative(filepath.Join(root, ".ruby-version")):
		languages = append(languages, detectedStackItem{name: "Ruby", indicator: ".ruby-version"})
	case pathExistsNative(filepath.Join(root, ".ruby-gemset")):
		languages = append(languages, detectedStackItem{name: "Ruby", indicator: ".ruby-gemset"})
	}
	if pathExistsNative(filepath.Join(root, "go.mod")) {
		languages = append(languages, detectedStackItem{name: "Go", indicator: "go.mod"})
	}
	if pathExistsNative(filepath.Join(root, "Cargo.toml")) {
		languages = append(languages, detectedStackItem{name: "Rust", indicator: "Cargo.toml"})
	}
	return languages
}

func detectInitFrameworks(root string, languages []detectedStackItem) []detectedStackItem {
	var frameworks []detectedStackItem
	if hasDetected(languages, "TypeScript") || hasDetected(languages, "JavaScript") {
		for _, config := range []string{"next.config.js", "next.config.mjs", "next.config.ts"} {
			if pathExistsNative(filepath.Join(root, config)) {
				frameworks = append(frameworks, detectedStackItem{name: "Next.js", indicator: config})
				break
			}
		}
		if !hasDetected(frameworks, "Next.js") && fileContains(filepath.Join(root, "package.json"), `"react"`) {
			frameworks = append(frameworks, detectedStackItem{name: "React", indicator: "package.json (react in deps)"})
		}
	}
	if hasDetected(languages, "Python") {
		pyDeps := safeReadNative(filepath.Join(root, "pyproject.toml")) + "\n" + safeReadNative(filepath.Join(root, "requirements.txt"))
		if strings.Contains(pyDeps, "fastapi") {
			frameworks = append(frameworks, detectedStackItem{name: "FastAPI", indicator: "fastapi in deps"})
		}
		if pathExistsNative(filepath.Join(root, "manage.py")) {
			frameworks = append(frameworks, detectedStackItem{name: "Django", indicator: "manage.py"})
		} else if strings.Contains(pyDeps, "django") {
			frameworks = append(frameworks, detectedStackItem{name: "Django", indicator: "django in deps"})
		}
		if strings.Contains(pyDeps, "flask") {
			frameworks = append(frameworks, detectedStackItem{name: "Flask", indicator: "flask in deps"})
		}
	}
	if hasDetected(languages, "Ruby") {
		if pathExistsNative(filepath.Join(root, "config", "routes.rb")) {
			frameworks = append(frameworks, detectedStackItem{name: "Rails", indicator: "config/routes.rb"})
		} else if pathExistsNative(filepath.Join(root, "bin", "rails")) {
			frameworks = append(frameworks, detectedStackItem{name: "Rails", indicator: "bin/rails"})
		}
	}
	return frameworks
}

func writeInitDetected(out io.Writer, info initProjectInfo) {
	fmt.Fprintln(out, "  Detected:")
	if len(info.languages) == 0 && len(info.frameworks) == 0 {
		fmt.Fprintln(out, "    No languages or frameworks detected")
	} else {
		for _, lang := range info.languages {
			fmt.Fprintf(out, "    ok %s (%s)\n", lang.name, lang.indicator)
		}
		for _, framework := range info.frameworks {
			fmt.Fprintf(out, "    ok %s (%s)\n", framework.name, framework.indicator)
		}
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, "  Existing:")
	for _, item := range []struct {
		exists bool
		label  string
	}{
		{info.existing.hasAgentsDir, ".agents/ directory"},
		{info.existing.hasAgentsMd, "AGENTS.md"},
		{info.existing.hasDocsDir, "docs/ directory"},
		{info.existing.hasChangelog, "CHANGELOG.md"},
		{info.existing.hasClaudeDir, ".claude/ directory"},
		{info.existing.hasLoafJSON, ".agents/loaf.json"},
	} {
		marker := "x"
		if item.exists {
			marker = "ok"
		}
		fmt.Fprintf(out, "    %s %s\n", marker, item.label)
	}
}

func (r Runner) offerInitSymlinks(root string, out io.Writer) error {
	agentsPath := filepath.Join(root, "AGENTS.md")
	if !pathExistsNative(agentsPath) {
		return nil
	}
	if !readerIsTerminal(r.Stdin) {
		return nil
	}
	reader := bufio.NewReader(firstReader(r.Stdin, os.Stdin))
	fmt.Fprintln(out, "  Symlinks:")
	claudePath := filepath.Join(root, ".claude", "CLAUDE.md")
	if !pathExistsNative(claudePath) {
		yes, err := askInitYesNo(reader, out, "    Create .claude/CLAUDE.md -> ../AGENTS.md? [y/N] ")
		if err != nil {
			return err
		}
		if yes {
			if err := os.MkdirAll(filepath.Dir(claudePath), 0o755); err != nil {
				return err
			}
			target, _ := filepath.Rel(filepath.Dir(claudePath), agentsPath)
			if err := os.Symlink(target, claudePath); err != nil {
				return err
			}
			fmt.Fprintln(out, "    ok Created .claude/CLAUDE.md")
		}
	}
	fmt.Fprintln(out)
	return nil
}

func askInitYesNo(reader *bufio.Reader, out io.Writer, question string) (bool, error) {
	fmt.Fprint(out, question)
	answer, err := reader.ReadString('\n')
	if err != nil && len(answer) == 0 {
		return false, err
	}
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(answer)), "y"), nil
}

func readerIsTerminal(reader io.Reader) bool {
	file, ok := firstReader(reader, os.Stdin).(*os.File)
	if !ok {
		return false
	}
	return fileIsTerminal(file)
}

func writeInitSkillRecommendations(out io.Writer, info initProjectInfo) {
	skills := recommendedInitSkills(info)
	if len(skills) == 0 {
		return
	}
	fmt.Fprintln(out, "  Recommended skills:")
	var nonFoundation []string
	for _, skill := range skills {
		if skill != "foundations" {
			nonFoundation = append(nonFoundation, skill)
		}
	}
	if len(nonFoundation) > 0 {
		fmt.Fprintf(out, "    - %s\n", strings.Join(nonFoundation, ", "))
	}
	fmt.Fprintln(out, "    - foundations (always)")
	fmt.Fprintln(out)
}

func recommendedInitSkills(info initProjectInfo) []string {
	skills := map[string]bool{"foundations": true}
	for _, item := range append(append([]detectedStackItem{}, info.languages...), info.frameworks...) {
		for _, skill := range initSkillMap(item.name) {
			skills[skill] = true
		}
	}
	var ordered []string
	for skill := range skills {
		ordered = append(ordered, skill)
	}
	sort.Strings(ordered)
	return ordered
}

func initSkillMap(name string) []string {
	switch name {
	case "TypeScript":
		return []string{"typescript-development"}
	case "Python":
		return []string{"python-development"}
	case "Ruby":
		return []string{"ruby-development"}
	case "Go":
		return []string{"go-development"}
	case "Next.js", "React":
		return []string{"typescript-development", "interface-design"}
	case "FastAPI", "Django":
		return []string{"python-development", "database-design"}
	case "Rails":
		return []string{"ruby-development", "database-design"}
	case "Flask":
		return []string{"python-development"}
	default:
		return nil
	}
}

func hasDetected(items []detectedStackItem, name string) bool {
	for _, item := range items {
		if item.name == name {
			return true
		}
	}
	return false
}

func fileContains(path string, needle string) bool {
	return strings.Contains(safeReadNative(path), needle)
}

func safeReadNative(path string) string {
	body, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(body)
}

func pathExistsNative(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
