package cli

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Scaffold templates are embedded so `loaf change init` never depends on
// installed content. Each must stay byte-identical to the canonical file under
// content/skills/shape/templates/; drift is gated by TestChangeScaffoldTemplatesMatchCanonical.

//go:embed change_shape_template.md
var changeShapeTemplate string

//go:embed change_brief_template.md
var changeBriefTemplate string

//go:embed change_plan_template.md
var changePlanTemplate string

//go:embed change_design_template.md
var changeDesignTemplate string

//go:embed change_task_template.md
var changeTaskTemplate string

type changeInitOptions struct {
	slug  string
	brief bool
}

func parseChangeInitArgs(args []string) (changeInitOptions, error) {
	options := changeInitOptions{}
	for _, arg := range args {
		switch {
		case arg == "--brief":
			options.brief = true
		case strings.HasPrefix(arg, "-"):
			return options, fmt.Errorf("unknown change init option %q", arg)
		case options.slug != "":
			return options, fmt.Errorf("change init accepts a single <slug> argument")
		default:
			options.slug = arg
		}
	}
	if options.slug == "" {
		return options, fmt.Errorf("change init requires a <slug> argument")
	}
	if !changeSlugRE.MatchString(options.slug) {
		return options, fmt.Errorf("invalid slug %q: use lowercase letters, digits, and single hyphens (e.g. auth-token-rotation)", options.slug)
	}
	return options, nil
}

func stampChangeScaffoldPlaceholders(template string, slug string) string {
	return strings.NewReplacer(
		"change: [slug]", "change: "+slug,
		"[slug]", slug,
	).Replace(template)
}

func writeChangeJSON(path string, slug string, now time.Time) error {
	payload := map[string]string{
		"change":  slug,
		"created": now.Format("2006-01-02"),
		"branch":  slug,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return fmt.Errorf("encode change.json: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write change.json: %w", err)
	}
	return nil
}

func scaffoldChangeFolder(folder string, slug string, brief bool, now time.Time) error {
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return fmt.Errorf("create change folder: %w", err)
	}
	if err := writeChangeJSON(filepath.Join(folder, changeMachineFileJSON), slug, now); err != nil {
		return err
	}
	if brief {
		target := filepath.Join(folder, changeBriefFile)
		if err := os.WriteFile(target, []byte(stampChangeScaffoldPlaceholders(changeBriefTemplate, slug)), 0o644); err != nil {
			return fmt.Errorf("write brief.md: %w", err)
		}
		return nil
	}

	shapePath := filepath.Join(folder, changeContractFileShape)
	if err := os.WriteFile(shapePath, []byte(changeShapeTemplate), 0o644); err != nil {
		return fmt.Errorf("write shape.md: %w", err)
	}
	tasksDir := filepath.Join(folder, "tasks")
	if err := os.MkdirAll(tasksDir, 0o755); err != nil {
		return fmt.Errorf("create tasks/: %w", err)
	}
	// Seed an empty tasks directory with a gitkeep so the scaffold is visible
	// in the first commit before shape fills TASK-NNN files.
	gitkeep := filepath.Join(tasksDir, ".gitkeep")
	if err := os.WriteFile(gitkeep, []byte{}, 0o644); err != nil {
		return fmt.Errorf("seed tasks/: %w", err)
	}
	return nil
}
