package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

type issueIdentityOptions struct {
	prefix     string
	authority  string
	align      bool
	all        bool
	dryRun     bool
	jsonOutput bool
}

func writeIssueIdentityHelp(out io.Writer) {
	writeUsageHelp(out, "loaf issue identity [--prefix <prefix>] [--authority <local|linear|github>] [--align [--all]] [--dry-run] [--json]", "Show or define issue identity. New projects record authority and prefix in .agents/loaf.json. --prefix sets the local alias prefix or Linear team key and persists it to project config. Local prefix changes rewrite existing aliases; tracker authorities do not. --align rewrites only a leaked LOAF prefix to the derived slug.",
		"--prefix       Define the issue prefix and persist it to .agents/loaf.json",
		"--authority    Set local, linear, or github identity and persist it to .agents/loaf.json",
		"--align        Rewrite a leaked LOAF prefix to the derived project slug",
		"--all          With --align, rewrite every leaked project in the global database",
		"--dry-run      Rehearse --prefix, --authority, or --align without writing",
		"--json         Output identity or rewrite result as JSON")
}

func parseIssueIdentityArgs(args []string) (issueIdentityOptions, error) {
	var options issueIdentityOptions
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--prefix":
			value, err := consumeFlagValue(args, &i, "--prefix")
			if err != nil {
				return issueIdentityOptions{}, err
			}
			options.prefix = value
		case "--authority":
			value, err := consumeFlagValue(args, &i, "--authority")
			if err != nil {
				return issueIdentityOptions{}, err
			}
			options.authority = value
		case "--align":
			options.align = true
		case "--all":
			options.all = true
		case "--dry-run":
			options.dryRun = true
		case "--json":
			options.jsonOutput = true
		default:
			return issueIdentityOptions{}, fmt.Errorf("unknown issue identity option %q", args[i])
		}
	}
	if (options.prefix != "" || options.authority != "") && options.align {
		return issueIdentityOptions{}, fmt.Errorf("--prefix and --authority cannot be combined with --align")
	}
	if options.all && !options.align {
		return issueIdentityOptions{}, fmt.Errorf("--all requires --align")
	}
	if options.dryRun && options.prefix == "" && options.authority == "" && !options.align {
		return issueIdentityOptions{}, fmt.Errorf("--dry-run requires --prefix, --authority, or --align")
	}
	return options, nil
}

func (r Runner) runIssueIdentity(args []string, out io.Writer, runtime state.Runtime) error {
	options, err := parseIssueIdentityArgs(args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIssueSQLiteState("issue identity", runtime)
	if err != nil {
		return err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	if options.prefix != "" || options.authority != "" {
		result, err := state.DefineIssueIdentity(context.Background(), projectRoot, resolver, state.DefineIssueIdentityOptions{
			Prefix:    options.prefix,
			Authority: options.authority,
			DryRun:    options.dryRun,
		})
		if err != nil {
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, result)
		}
		writeIssueIdentityDefineText(out, result, options)
		return nil
	}
	if options.align {
		result, err := state.AlignLeakedIssuePrefixes(context.Background(), projectRoot, resolver, options.all, options.dryRun)
		if err != nil {
			return err
		}
		if options.jsonOutput {
			return writeJSON(out, result)
		}
		if len(result.Items) == 0 {
			fmt.Fprintln(out, "no leaked LOAF issue prefix")
			return nil
		}
		verb := "rewrote"
		if result.DryRun {
			verb = "would rewrite"
		}
		fmt.Fprintf(out, "%s %d project prefix(es)\n", verb, result.Rewritten)
		for _, item := range result.Items {
			fmt.Fprintf(out, "- %s: %s-* -> %s-* (%d alias(es), %d note(s))\n", item.ProjectName, item.FromPrefix, item.ToPrefix, item.Aliases, item.Notes)
		}
		if result.DryRun {
			fmt.Fprintln(out, "would write .agents/loaf.json")
		} else if result.ConfigWritten {
			fmt.Fprintln(out, "wrote .agents/loaf.json")
		}
		return nil
	}
	identity, err := state.GetIssueIdentity(context.Background(), projectRoot, resolver)
	if err != nil {
		return err
	}
	if options.jsonOutput {
		return writeJSON(out, identity)
	}
	fmt.Fprintf(out, "authority: %s\n", identity.Authority)
	fmt.Fprintf(out, "prefix: %s\n", identity.Prefix)
	fmt.Fprintf(out, "next number: %d\n", identity.NextNumber)
	cfg, err := state.LoadIssueProjectConfig(projectRoot.Path())
	if err != nil {
		fmt.Fprintf(out, "config: unreadable (%v)\n", err)
		return nil
	}
	if cfg.Prefix != "" || cfg.Authority != "" {
		authority := cfg.Authority
		if authority == "" {
			authority = state.DefaultIssueAuthority
		}
		fmt.Fprintf(out, "config: %s %s\n", authority, cfg.Prefix)
		return nil
	}
	fmt.Fprintf(out, "note: .agents/loaf.json has no issue.prefix; persist with `%s`\n", state.IssuePrefixPersistCommand(identity.Prefix))
	return nil
}

func writeIssueIdentityDefineText(out io.Writer, result state.IssuePrefixAlignResult, options issueIdentityOptions) {
	verb := "defined"
	if result.DryRun {
		verb = "would define"
	}
	if options.authority != "" {
		fmt.Fprintf(out, "%s authority %s\n", verb, result.Authority)
	}
	if options.prefix != "" {
		if len(result.Items) == 0 {
			fmt.Fprintf(out, "prefix already %s\n", strings.ToUpper(options.prefix))
		} else {
			item := result.Items[0]
			fmt.Fprintf(out, "%s prefix %s\n", verb, item.ToPrefix)
			if item.Aliases > 0 || item.Notes > 0 {
				fmt.Fprintf(out, "%s-* -> %s-* (%d alias(es), %d note(s))\n", item.FromPrefix, item.ToPrefix, item.Aliases, item.Notes)
			} else if result.Authority != state.IssueAuthorityLocal {
				fmt.Fprintln(out, "tracker owns aliases; local rewrite skipped")
			}
		}
	} else if len(result.Items) > 0 && result.Items[0].FromPrefix != result.Items[0].ToPrefix {
		item := result.Items[0]
		fmt.Fprintf(out, "%s prefix %s\n", verb, item.ToPrefix)
	}
	if result.TeamKeyWritten {
		fmt.Fprintf(out, "linear team key %s\n", result.Prefix)
	}
	if result.DryRun {
		fmt.Fprintln(out, "would write .agents/loaf.json")
	} else if result.ConfigWritten {
		fmt.Fprintln(out, "wrote .agents/loaf.json")
	}
}
