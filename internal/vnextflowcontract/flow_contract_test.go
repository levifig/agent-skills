package vnextflowcontract

import (
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"testing/fstest"
)

func TestFlowContract(t *testing.T) {
	t.Parallel()

	assertNoContractFindings(t, validateFlowContract(os.DirFS("../../vnext/content")))
}

func TestTemplateContractRejectsMissingAndOrphanFields(t *testing.T) {
	t.Parallel()

	fixture := validFlowFixture()
	template := fixture["templates/problem-narrative.md"]
	template.Data = []byte(strings.ReplaceAll(string(template.Data), "<!-- loaf:field handoff -->", "<!-- loaf:field unplanned -->"))
	fixture["templates/problem-narrative.md"] = template
	fixture["templates/orphan.md"] = &fstest.MapFile{Data: []byte("# Orphan\n")}

	findings := validateFlowContract(fixture)
	assertContractFinding(t, findings, "template.marker", "missing field \"handoff\"")
	assertContractFinding(t, findings, "template.marker", "orphan field \"unplanned\"")
	assertContractFinding(t, findings, "template.inventory", "template is not declared")
}

func TestTrackerSkillContractRejectsUndeclaredInventoryAndBrokenLinks(t *testing.T) {
	t.Parallel()

	fixture := validFlowFixture()
	fixture["skills/undeclared/SKILL.md"] = &fstest.MapFile{Data: []byte(validSkill("undeclared", "Explains undeclared behavior. Use when testing inventory."))}
	fixture["skills/loaf-reference/SKILL.codex.yaml"] = &fstest.MapFile{Data: []byte("target: codex\n")}
	declared := fixture["skills/loaf-reference/SKILL.md"]
	declared.Data = append(declared.Data, []byte("\n[Missing](references/missing.md)\n")...)
	fixture["skills/loaf-reference/SKILL.md"] = declared

	findings := validateFlowContract(fixture)
	assertContractFinding(t, findings, "skill.inventory", "skill is not declared")
	assertContractFinding(t, findings, "skill.surface", "sidecar")
	assertContractFinding(t, findings, "content.link", "broken link")
}

func TestFlowContractRejectsUnknownManifestFieldsDeterministically(t *testing.T) {
	t.Parallel()

	fixture := validFlowFixture()
	manifest := fixture[flowManifestPath]
	manifest.Data = []byte(strings.Replace(string(manifest.Data), "\n}", ",\n  \"unknown\": true\n}", 1))
	fixture[flowManifestPath] = manifest

	first := validateFlowContract(fixture)
	second := validateFlowContract(fixture)
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("findings differ between identical runs:\nfirst: %v\nsecond: %v", first, second)
	}
	assertContractFinding(t, first, "flow.manifest", "unknown field")
}

func TestFlowExecutionContractKeepsMainAgentPrimary(t *testing.T) {
	t.Parallel()

	mutations := []struct {
		name     string
		oldValue string
		newValue string
	}{
		{name: "missing main agent", oldValue: `"actor": "main-agent"`, newValue: `"actor": "project-manager"`},
		{name: "provider bypass", oldValue: `"provider_route": "selected-provider-skill"`, newValue: `"provider_route": "direct-connector"`},
		{name: "missing primary fallback", oldValue: `"fallback": "primary"`, newValue: `"fallback": "none"`},
		{name: "different behavior source", oldValue: `"behavior_contract": "skills/project-management/contract.json"`, newValue: `"behavior_contract": "agents/project-manager.contract.json"`},
	}

	for _, mutation := range mutations {
		mutation := mutation
		t.Run(mutation.name, func(t *testing.T) {
			t.Parallel()

			fixture := validFlowFixture()
			manifest := fixture[flowManifestPath]
			manifest.Data = []byte(strings.Replace(string(manifest.Data), mutation.oldValue, mutation.newValue, 1))
			fixture[flowManifestPath] = manifest
			assertContractFinding(t, validateFlowContract(fixture), "flow.execution", "execution")
		})
	}
}

func validFlowFixture() fstest.MapFS {
	return fstest.MapFS{
		flowManifestPath: &fstest.MapFile{Data: []byte(validFlowManifest)},
		"skills/loaf-reference/SKILL.md": &fstest.MapFile{Data: []byte(validSkill(
			"loaf-reference",
			"Explains vNext Flow authority and artifacts. Use when deciding where shared work belongs.",
		))},
		"templates/problem-narrative.md": &fstest.MapFile{Data: []byte(validProblemNarrativeTemplate)},
		"templates/work-contract.md":     &fstest.MapFile{Data: []byte(validWorkContractTemplate)},
		"templates/tracker-update.md":    &fstest.MapFile{Data: []byte(validTrackerUpdateTemplate)},
	}
}

func validSkill(name, description string) string {
	return fmt.Sprintf(`---
name: %s
description: %s
---

# Fixture Skill

	Fixture instructions.

## Contents

- Critical Rules
- Verification
- Quick Reference
- Topics

## Critical Rules

- Preserve the fixture contract.

## Verification

- Validate the fixture.

## Quick Reference

| Input | Result |
|-------|--------|
| Fixture | Finding |

## Topics

No supporting references.
`, name, description)
}

func assertNoContractFindings(t *testing.T, findings []finding) {
	t.Helper()

	if len(findings) == 0 {
		return
	}
	formatted := make([]string, 0, len(findings))
	for _, item := range findings {
		formatted = append(formatted, item.String())
	}
	t.Fatalf("unexpected contract findings:\n%s", strings.Join(formatted, "\n"))
}

func assertContractFinding(t *testing.T, findings []finding, rule, detailFragment string) {
	t.Helper()

	for _, item := range findings {
		if item.Rule == rule && strings.Contains(item.Detail, detailFragment) {
			return
		}
	}
	t.Fatalf("missing %s finding containing %q in %v", rule, detailFragment, findings)
}

const validFlowManifest = `{
  "schema": "loaf-flow/v1",
  "tracker_contract": "project-management/v1",
  "authority": {
    "shared_work": "tracker",
    "service_connections": "harness",
    "flow_ceremonies": "loaf",
    "code": "git"
  },
  "execution": {
    "primary": {
      "actor": "main-agent",
      "behavior_contract": "skills/project-management/contract.json",
      "behavior_skill": "skills/project-management/SKILL.md",
      "provider_route": "selected-provider-skill"
    },
    "optional_profile": {
      "id": "project-manager/v1",
      "contract_path": "agents/project-manager.contract.json",
      "fallback": "primary"
    }
  },
  "local_work_record": false,
  "tracker_proxy": false,
  "tracker_sync": false,
  "skills": [
    {
      "name": "loaf-reference",
      "path": "skills/loaf-reference/SKILL.md",
      "kind": "reference"
    }
  ],
  "ceremonies": [],
  "templates": [
    {
      "id": "problem-narrative",
      "path": "templates/problem-narrative.md",
      "required_fields": ["problem", "affected_people", "current_reality", "desired_outcome", "constraints", "unknowns", "handoff"]
    },
    {
      "id": "work-contract",
      "path": "templates/work-contract.md",
      "required_fields": ["problem", "definition_of_done", "out_of_scope", "verification", "risks"]
    },
    {
      "id": "tracker-update",
      "path": "templates/tracker-update.md",
      "required_fields": ["native_ref", "summary", "evidence", "status_intent", "blockers", "next_step"]
    }
  ],
  "agents": []
}`

const validProblemNarrativeTemplate = `# Problem Narrative

<!-- loaf:field problem -->
## Problem
{{problem}}

<!-- loaf:field affected_people -->
## Affected People
{{affected_people}}

<!-- loaf:field current_reality -->
## Current Reality
{{current_reality}}

<!-- loaf:field desired_outcome -->
## Desired Outcome
{{desired_outcome}}

<!-- loaf:field constraints -->
## Constraints
{{constraints}}

<!-- loaf:field unknowns -->
## Unknowns
{{unknowns}}

<!-- loaf:field handoff -->
## Handoff
{{handoff}}
`

const validWorkContractTemplate = `# Work Contract

<!-- loaf:field problem -->
## Problem
{{problem}}

<!-- loaf:field definition_of_done -->
## Definition of Done
{{definition_of_done}}

<!-- loaf:field out_of_scope -->
## Out of Scope
{{out_of_scope}}

<!-- loaf:field verification -->
## Verification
{{verification}}

<!-- loaf:field risks -->
## Risks
{{risks}}
`

const validTrackerUpdateTemplate = `# Tracker Update

<!-- loaf:field native_ref -->
## Native Reference
{{native_ref}}

<!-- loaf:field summary -->
## Summary
{{summary}}

<!-- loaf:field evidence -->
## Evidence
{{evidence}}

<!-- loaf:field status_intent -->
## Status Intent
{{status_intent}}

<!-- loaf:field blockers -->
## Blockers
{{blockers}}

<!-- loaf:field next_step -->
## Next Step
{{next_step}}
`
