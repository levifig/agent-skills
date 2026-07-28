package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

type changeListUnitJSON struct {
	Command  string           `json:"command"`
	Target   string           `json:"target,omitempty"`
	Units    []changeListUnit `json:"units"`
	Warnings []string         `json:"warnings,omitempty"`
}

type changeListUnit struct {
	Slug          string   `json:"slug"`
	Folder        string   `json:"folder"`
	Layout        string   `json:"layout"`
	Branch        string   `json:"branch,omitempty"`
	TargetRelease string   `json:"targetRelease,omitempty"`
	State         string   `json:"state"`
	PathExecuted  bool     `json:"pathExecuted,omitempty"`
	FlipExecuted  bool     `json:"flipExecuted,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

type changeListOptionsV2 struct {
	target     string
	jsonOutput bool
}

func parseChangeListArgsV2(args []string) (changeListOptionsV2, error) {
	options := changeListOptionsV2{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			options.jsonOutput = true
		case arg == "--target":
			if i+1 >= len(args) {
				return options, fmt.Errorf("--target requires a value")
			}
			i++
			options.target = args[i]
		case strings.HasPrefix(arg, "--target="):
			options.target = strings.TrimPrefix(arg, "--target=")
		case arg == "--lineage" || strings.HasPrefix(arg, "--lineage="):
			return options, fmt.Errorf("--lineage retired; use loaf change list [--target <version>] for the units/cohort projection")
		case strings.HasPrefix(arg, "-"):
			return options, fmt.Errorf("unknown change list option %q", arg)
		default:
			return options, fmt.Errorf("change list accepts no positional arguments")
		}
	}
	if options.target != "" && !isCanonicalChangeTargetRelease(options.target) {
		return options, fmt.Errorf("target %q must be canonical MAJOR.MINOR.PATCH", options.target)
	}
	return options, nil
}

func (r Runner) runChangeListUnits(args []string, out io.Writer, rootPath string) error {
	options, err := parseChangeListArgsV2(args)
	if err != nil {
		return err
	}
	nodes, err := loadChangeNodes(rootPath)
	if err != nil {
		return err
	}
	units := []changeListUnit{}
	var listWarnings []string
	for _, node := range nodes {
		if options.target != "" && node.TargetRelease != options.target {
			continue
		}
		status, statusErr := changeFolderExecuted(rootPath, node.Folder, node.Layout, commandOutput)
		state, stateWarnings := deriveChangeStateDetailed(rootPath, node, changeEvidenceGitOutput)
		if statusErr != nil {
			stateWarnings = append(stateWarnings, "execution provenance failed: "+statusErr.Error())
		}
		units = append(units, changeListUnit{
			Slug:          node.Slug,
			Folder:        node.Folder,
			Layout:        node.Layout,
			Branch:        node.Branch,
			TargetRelease: node.TargetRelease,
			State:         state,
			PathExecuted:  status.PathExecuted,
			FlipExecuted:  status.FlipExecuted,
			Warnings:      append([]string{}, stateWarnings...),
		})
		for _, w := range stateWarnings {
			listWarnings = append(listWarnings, fmt.Sprintf("%s: %s", node.Slug, w))
		}
	}
	sort.Slice(units, func(i, j int) bool { return units[i].Folder < units[j].Folder })
	result := changeListUnitJSON{Command: "change list", Target: options.target, Units: units, Warnings: sortedUnique(listWarnings)}
	if options.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "\n%s\n", ansiBold("change list"))
	if options.target != "" {
		fmt.Fprintf(out, "target: %s\n", options.target)
	}
	for _, unit := range units {
		target := unit.TargetRelease
		if target == "" {
			target = "-"
		}
		fmt.Fprintf(out, "  %s  %s  layout=%s  target=%s  state=%s\n",
			unit.Slug, unit.Folder, unit.Layout, target, unit.State)
		for _, w := range unit.Warnings {
			fmt.Fprintf(out, "  %s %s\n", ansiYellow("warn:"), w)
		}
	}
	if len(units) == 0 {
		fmt.Fprintf(out, "  (no changes)\n")
	}
	return nil
}
