package cli

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
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

// changePublishTempPrefix names temp files used by atomic destination publication
// during captured-folder promotion. Stray temps never count as materialization.
const changePublishTempPrefix = ".loaf-change-"

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

// changeSeedTaskFile is the first packet written by `loaf change init`. The
// author is expected to rename the slug; `first-slice` cites no other work unit.
const changeSeedTaskFile = "TASK-001-first-slice.md"

// stampChangeTaskSeed stamps the embedded task template into a ready-to-edit
// first packet: owning change slug, TASK-001 identity, unchecked boxes and
// bracket placeholders preserved, plus an explicit rename note.
func stampChangeTaskSeed(template string, slug string) string {
	body := strings.NewReplacer(
		"change: [slug]", "change: "+slug,
		"TASK-NNN", "TASK-001",
	).Replace(template)
	const renameNote = "\n\nRename this file before authoring real work — `first-slice` is a seed slug (not a work-unit citation); the author is expected to rename it.\n"
	const h1 = "# TASK-001 — [Title]"
	if idx := strings.Index(body, h1); idx >= 0 {
		insertAt := idx + len(h1)
		body = body[:insertAt] + renameNote + body[insertAt:]
	}
	return body
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
	// Seed a real first task packet so shapers see the delegation format at
	// scaffold time. Unchecked boxes cannot manufacture provenance; rename
	// the seed slug when authoring the first real slice.
	seedPath := filepath.Join(tasksDir, changeSeedTaskFile)
	if err := os.WriteFile(seedPath, []byte(stampChangeTaskSeed(changeTaskTemplate, slug)), 0o644); err != nil {
		return fmt.Errorf("seed %s: %w", changeSeedTaskFile, err)
	}
	return nil
}

// changePromotionOutcome is the three-way matrix for init against an existing folder.
type changePromotionOutcome int

const (
	changePromotionPromote changePromotionOutcome = iota
	changePromotionResume
	changePromotionReject
)

type changePromotionDecision struct {
	outcome changePromotionOutcome
	reason  string
	// needSeed is true when the seed task destination is missing (promote or resume gap).
	needSeed bool
	// needShape is true when shape.md is missing (always true for promote/resume).
	needShape bool
}

// classifyChangePromotion applies Decision 12's state matrix. Only ordinary
// init (not --brief) may promote or resume; brief.md and change.json are never
// written by promotion. folderRel is the repo-relative path for error messages.
func classifyChangePromotion(folderAbs, folderRel, slug string, briefMode bool) changePromotionDecision {
	folderRel = filepath.ToSlash(folderRel)
	if briefMode {
		return changePromotionDecision{
			outcome: changePromotionReject,
			reason:  fmt.Sprintf("change slug %q already exists in %s (re-run without --brief to promote a capture-only folder)", slug, folderRel),
		}
	}

	jsonPath := filepath.Join(folderAbs, changeMachineFileJSON)
	mdPath := filepath.Join(folderAbs, changeMachineFileLegacy)
	_, jsonErr := os.Stat(jsonPath)
	_, mdErr := os.Stat(mdPath)
	jsonPresent := jsonErr == nil
	mdPresent := mdErr == nil
	if !jsonPresent && !os.IsNotExist(jsonErr) {
		return changePromotionDecision{outcome: changePromotionReject, reason: fmt.Sprintf("stat change.json: %v", jsonErr)}
	}
	if !mdPresent && !os.IsNotExist(mdErr) {
		return changePromotionDecision{outcome: changePromotionReject, reason: fmt.Sprintf("stat change.md: %v", mdErr)}
	}
	if jsonPresent && mdPresent {
		return changePromotionDecision{
			outcome: changePromotionReject,
			reason:  fmt.Sprintf("hybrid layout in %s: change.json and change.md both present; refuse promotion", folderRel),
		}
	}
	if !jsonPresent {
		return changePromotionDecision{
			outcome: changePromotionReject,
			reason:  fmt.Sprintf("change slug %q already exists in %s", slug, folderRel),
		}
	}

	jsonBytes, err := os.ReadFile(jsonPath)
	if err != nil {
		return changePromotionDecision{outcome: changePromotionReject, reason: fmt.Sprintf("read change.json: %v", err)}
	}
	meta := parseChangeJSON(string(jsonBytes))
	if len(meta.Findings) > 0 {
		return changePromotionDecision{
			outcome: changePromotionReject,
			reason:  fmt.Sprintf("invalid change.json in %s: %s", folderRel, strings.Join(meta.Findings, "; ")),
		}
	}
	if meta.Change != slug {
		return changePromotionDecision{
			outcome: changePromotionReject,
			reason:  fmt.Sprintf("change.json field \"change\" is %q, want slug %q; refuse promotion", meta.Change, slug),
		}
	}

	// shape.md is the materialization marker: present means fully (or partially
	// authored) shaped, and duplicate rejection is unchanged.
	shapePath := filepath.Join(folderAbs, changeContractFileShape)
	if _, err := os.Stat(shapePath); err == nil {
		return changePromotionDecision{
			outcome: changePromotionReject,
			reason:  fmt.Sprintf("change slug %q already exists in %s", slug, folderRel),
		}
	} else if !os.IsNotExist(err) {
		return changePromotionDecision{outcome: changePromotionReject, reason: fmt.Sprintf("stat shape.md: %v", err)}
	}

	if _, err := os.Stat(filepath.Join(folderAbs, changeBriefFile)); err != nil {
		if os.IsNotExist(err) {
			return changePromotionDecision{
				outcome: changePromotionReject,
				reason:  fmt.Sprintf("cannot promote %s: brief.md is missing (change.json-only folders fail closed)", folderRel),
			}
		}
		return changePromotionDecision{outcome: changePromotionReject, reason: fmt.Sprintf("stat brief.md: %v", err)}
	}

	expectedSeed := []byte(stampChangeTaskSeed(changeTaskTemplate, slug))
	tasksDir := filepath.Join(folderAbs, "tasks")
	entries, err := os.ReadDir(tasksDir)
	if err != nil {
		if os.IsNotExist(err) {
			// Structurally valid brief-only folder: promote.
			return changePromotionDecision{
				outcome:   changePromotionPromote,
				needSeed:  true,
				needShape: true,
			}
		}
		return changePromotionDecision{outcome: changePromotionReject, reason: fmt.Sprintf("read tasks/: %v", err)}
	}

	var realEntries []os.DirEntry
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, changePublishTempPrefix) || strings.HasPrefix(name, ".") {
			// Stray temps and hidden leftovers do not count as tasks content.
			continue
		}
		realEntries = append(realEntries, entry)
	}
	if len(realEntries) == 0 {
		return changePromotionDecision{
			outcome:   changePromotionPromote,
			needSeed:  true,
			needShape: true,
		}
	}

	// Resume only when every real tasks entry is the byte-identical seed.
	for _, entry := range realEntries {
		if entry.IsDir() {
			return changePromotionDecision{
				outcome: changePromotionReject,
				reason:  fmt.Sprintf("cannot promote %s: tasks/ has unexpected directory %q", folderRel, entry.Name()),
			}
		}
		if entry.Name() != changeSeedTaskFile {
			return changePromotionDecision{
				outcome: changePromotionReject,
				reason:  fmt.Sprintf("cannot promote %s: tasks/ content diverged from seed (found %q)", folderRel, entry.Name()),
			}
		}
		got, err := os.ReadFile(filepath.Join(tasksDir, entry.Name()))
		if err != nil {
			return changePromotionDecision{outcome: changePromotionReject, reason: fmt.Sprintf("read tasks/%s: %v", entry.Name(), err)}
		}
		if !bytes.Equal(got, expectedSeed) {
			return changePromotionDecision{
				outcome: changePromotionReject,
				reason:  fmt.Sprintf("cannot promote %s: tasks/%s diverged from seed instantiation", folderRel, changeSeedTaskFile),
			}
		}
	}

	return changePromotionDecision{
		outcome:   changePromotionResume,
		needSeed:  false,
		needShape: true,
	}
}

// completeCapturedChangeFolder promotes or resumes a capture-only folder.
// brief.md and change.json are never written. Destination files are published
// atomically (temp-write then rename, refuse existing destinations); shape.md
// is published last as the promotion marker.
func completeCapturedChangeFolder(folderAbs string, slug string, decision changePromotionDecision) error {
	if decision.outcome != changePromotionPromote && decision.outcome != changePromotionResume {
		return fmt.Errorf("internal: completeCapturedChangeFolder called with non-completable outcome")
	}

	// Publication order: seed task first (when needed), shape.md last (marker).
	if decision.needSeed {
		tasksDir := filepath.Join(folderAbs, "tasks")
		if err := os.MkdirAll(tasksDir, 0o755); err != nil {
			return fmt.Errorf("create tasks/: %w", err)
		}
		seedPath := filepath.Join(tasksDir, changeSeedTaskFile)
		seedBody := []byte(stampChangeTaskSeed(changeTaskTemplate, slug))
		if err := publishChangeFileExclusive(seedPath, seedBody); err != nil {
			return fmt.Errorf("publish %s: %w", changeSeedTaskFile, err)
		}
	}

	if decision.needShape {
		shapePath := filepath.Join(folderAbs, changeContractFileShape)
		if err := publishChangeFileExclusive(shapePath, []byte(changeShapeTemplate)); err != nil {
			return fmt.Errorf("publish shape.md: %w", err)
		}
	}
	return nil
}

// publishChangeFileExclusive writes body to a same-directory temp file, syncs,
// then renames into dest only when dest does not already exist. A destination
// is therefore either absent or complete — never half-written in place.
func publishChangeFileExclusive(dest string, body []byte) error {
	if info, err := os.Stat(dest); err == nil {
		_ = info
		return fmt.Errorf("refusing to overwrite existing file %s", dest)
	} else if !os.IsNotExist(err) {
		return err
	}

	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(dir, changePublishTempPrefix+"*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if err := temp.Chmod(0o644); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(body); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}

	// Claim the destination exclusively before rename so we never replace an
	// existing complete file if one appeared between Stat and rename.
	claim, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("refusing to overwrite existing file %s", dest)
		}
		return err
	}
	if err := claim.Close(); err != nil {
		_ = os.Remove(dest)
		return err
	}
	if err := os.Rename(tempPath, dest); err != nil {
		_ = os.Remove(dest)
		return err
	}
	cleanup = false
	return nil
}

// writeChangePromotionSuccess prints a message distinct from fresh-scaffold output.
func writeChangePromotionSuccess(out io.Writer, rootPath, folderAbs, slug string, decision changePromotionDecision) {
	folderRel := relFromRoot(rootPath, folderAbs)
	verb := "Promoted capture"
	if decision.outcome == changePromotionResume {
		verb = "Resumed capture promotion"
	}
	fmt.Fprintf(out, "%s: %s\n", verb, filepath.ToSlash(filepath.Join(folderRel, changeContractFileShape)))
	fmt.Fprintf(out, "  Preserved brief.md + change.json verbatim; instantiated missing shape.md + tasks/\n")
	fmt.Fprintf(out, "\nNext: work on this change happens on branch %q.\n", slug)
	fmt.Fprintf(out, "  Create or switch to it:   git switch -c %s\n", slug)
	fmt.Fprintf(out, "  Then validate the change:  loaf change check\n")
	fmt.Fprintf(out, "  Or check it from any branch by passing the folder: loaf change check %s\n", folderRel)
}
