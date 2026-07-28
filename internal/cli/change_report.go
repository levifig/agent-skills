package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Closed report-kind registry (Decision 16 / V9). Ceremony kinds are reserved
// for a future publish ceremony; informational kinds round it out; note is the
// escape hatch.
var changeReportKinds = []string{
	"approval",
	"review",
	"visual",
	"audit",
	"note",
}

var changeReportKindSet = func() map[string]bool {
	set := make(map[string]bool, len(changeReportKinds))
	for _, kind := range changeReportKinds {
		set[kind] = true
	}
	return set
}()

var changeReportKindCeremony = map[string]bool{
	"approval": true,
	"review":   true,
}

// changeReportClock is replaced in tests to pin YYYYMMDD-HHMMSS filenames.
var changeReportClock = time.Now

type changeReportNewOptions struct {
	slug   string
	kind   string
	folder string
}

func parseChangeReportNewArgs(args []string) (changeReportNewOptions, error) {
	options := changeReportNewOptions{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--kind":
			if i+1 >= len(args) {
				return options, fmt.Errorf("--kind requires a value")
			}
			i++
			options.kind = args[i]
		case strings.HasPrefix(arg, "--kind="):
			options.kind = strings.TrimPrefix(arg, "--kind=")
		case strings.HasPrefix(arg, "-"):
			return options, fmt.Errorf("unknown change report new option %q", arg)
		case options.slug == "":
			options.slug = arg
		case options.folder == "":
			options.folder = arg
		default:
			return options, fmt.Errorf("change report new accepts <slug> and optional [folder]")
		}
	}
	if options.slug == "" {
		return options, fmt.Errorf("change report new requires a <slug> argument")
	}
	if !changeSlugRE.MatchString(options.slug) {
		return options, fmt.Errorf("invalid report slug %q: use lowercase letters, digits, and single hyphens", options.slug)
	}
	if options.kind == "" {
		return options, fmt.Errorf("change report new requires --kind (%s)", strings.Join(changeReportKinds, "/"))
	}
	if !changeReportKindSet[options.kind] {
		return options, fmt.Errorf("unknown report kind %q; registry: %s", options.kind, strings.Join(changeReportKinds, ", "))
	}
	return options, nil
}

func (r Runner) runChangeReport(args []string, out io.Writer, rootPath string) error {
	if len(args) == 0 || isHelpArg(args) {
		writeChangeReportHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"new": writeChangeReportNewHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "new":
		return r.runChangeReportNew(args[1:], out, rootPath)
	default:
		return unknownSubcommandError("change report", args[0])
	}
}

func writeChangeReportHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf change report <subcommand> [options]",
		"Stamp authored HTML reports under a Change's reports/ directory.",
		[]subcommandHelpItem{
			{Name: "new", Summary: "Create a timestamped report shell from the closed kind registry"},
		})
}

func writeChangeReportNewHelp(out io.Writer) {
	writeUsageHelp(out, "loaf change report new <slug> --kind <kind> [folder]",
		"Create reports/YYYYMMDD-HHMMSS-<kind>-<slug>.html with charset, provenance header, and design-token skeleton. Refuses collisions and unknown kinds.",
		"--kind    Required kind from the closed registry: "+strings.Join(changeReportKinds, ", "),
		"[folder]  Change folder (or change.json/change.md) path; resolves from the current branch when omitted")
}

func (r Runner) runChangeReportNew(args []string, out io.Writer, rootPath string) error {
	if isHelpArg(args) {
		writeChangeReportNewHelp(out)
		return nil
	}
	options, err := parseChangeReportNewArgs(args)
	if err != nil {
		return err
	}

	folder, _, err := resolveChangeFolder(rootPath, options.folder)
	if err != nil {
		return err
	}

	now := changeReportClock()
	filename := fmt.Sprintf("%s-%s-%s.html", now.Format("20060102-150405"), options.kind, options.slug)
	reportsDir := filepath.Join(folder, "reports")
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return fmt.Errorf("create reports/: %w", err)
	}
	target := filepath.Join(reportsDir, filename)
	if _, err := os.Stat(target); err == nil {
		return fmt.Errorf("report already exists: %s", relFromRoot(rootPath, target))
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat report: %w", err)
	}

	node, err := assembleChangeNodeFromFolder(rootPath, folder)
	if err != nil {
		return err
	}
	body := stampChangeReportSkeleton(options.kind, options.slug, node.Slug, now)
	if err := os.WriteFile(target, []byte(body), 0o644); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	fmt.Fprintf(out, "Created report: %s\n", relFromRoot(rootPath, target))
	fmt.Fprintln(out)
	writeChangeReportDesignGuidance(out, options.kind)
	return nil
}

func stampChangeReportSkeleton(kind, reportSlug, changeSlug string, now time.Time) string {
	title := changeReportDefaultTitle(kind, reportSlug)
	provenance := fmt.Sprintf(
		"<!-- source: %s · kind %s · slug %s · stamped %s · authored report (snapshot semantics) · never auto-updated -->",
		changeSlug, kind, reportSlug, now.Format("2006-01-02 15:04"),
	)
	kindHint := changeReportKindHint(kind)
	return provenance + "\n" + `<meta charset="utf-8">
<title>` + htmlEscapeText(title) + `</title>
<style>
  :root {
    --bg: #FBFAF6; --panel: #FFFFFF; --line: #E4E0D6; --ink: #23262A; --muted: #6B7076;
    --accent: #9A6A00; --accent-fill: #E9B94E; --accent-soft: #F6EBD2;
    --machine: #44618C; --machine-soft: #E5ECF5;
    --ok: #3E7C4F; --ok-soft: #E2F0E5; --bad: #A94438; --bad-soft: #F6E3E0;
    --state: #7A5EA6; --state-soft: #EEE8F6;
    --mono: ui-monospace, "SF Mono", SFMono-Regular, Menlo, Consolas, monospace;
    --serif: "Iowan Old Style", "Palatino Linotype", Palatino, Georgia, serif;
    --sans: system-ui, -apple-system, "Segoe UI", sans-serif;
  }
  @media (prefers-color-scheme: dark) { :root {
    --bg: #15171B; --panel: #1D2025; --line: #33373D; --ink: #E7E4DD; --muted: #9BA0A6;
    --accent: #E2AC4B; --accent-fill: #B98322; --accent-soft: #2E2714;
    --machine: #8FB0DA; --machine-soft: #232B38;
    --ok: #7CC08D; --ok-soft: #1E2E22; --bad: #E08A7E; --bad-soft: #382220;
    --state: #B9A3DE; --state-soft: #2A2436;
  } }
  :root[data-theme="light"] {
    --bg: #FBFAF6; --panel: #FFFFFF; --line: #E4E0D6; --ink: #23262A; --muted: #6B7076;
    --accent: #9A6A00; --accent-fill: #E9B94E; --accent-soft: #F6EBD2;
    --machine: #44618C; --machine-soft: #E5ECF5;
    --ok: #3E7C4F; --ok-soft: #E2F0E5; --bad: #A94438; --bad-soft: #F6E3E0;
    --state: #7A5EA6; --state-soft: #EEE8F6;
  }
  :root[data-theme="dark"] {
    --bg: #15171B; --panel: #1D2025; --line: #33373D; --ink: #E7E4DD; --muted: #9BA0A6;
    --accent: #E2AC4B; --accent-fill: #B98322; --accent-soft: #2E2714;
    --machine: #8FB0DA; --machine-soft: #232B38;
    --ok: #7CC08D; --ok-soft: #1E2E22; --bad: #E08A7E; --bad-soft: #382220;
    --state: #B9A3DE; --state-soft: #2A2436;
  }
  * { box-sizing: border-box; }
  body { background: var(--bg); color: var(--ink); font-family: var(--sans); line-height: 1.55; margin: 0; }
  main { max-width: 1040px; margin: 0 auto; padding: 44px 24px 90px; }
  h1 { font-family: var(--serif); font-weight: 600; font-size: 1.95rem; margin: 6px 0 8px; text-wrap: balance; }
  h2 { font-family: var(--serif); font-weight: 600; font-size: 1.3rem; margin: 44px 0 10px; }
  .eyebrow { font-size: .72rem; letter-spacing: .14em; text-transform: uppercase; color: var(--accent); font-weight: 700; }
  .stamp { font-family: var(--mono); font-size: .8rem; color: var(--muted); }
  .stamp b { color: var(--ink); }
  code { font-family: var(--mono); font-size: .84em; }
  .panel { background: var(--panel); border: 1px solid var(--line); border-radius: 10px; padding: 18px 20px; }
  footer { margin-top: 56px; color: var(--muted); font-size: .8rem; border-top: 1px solid var(--line); padding-top: 16px; }
</style>

<main>
  <div class="eyebrow">` + htmlEscapeText(changeSlug) + ` · ` + htmlEscapeText(kind) + `</div>
  <h1>` + htmlEscapeText(title) + `</h1>
  <p class="stamp">kind <b>` + htmlEscapeText(kind) + `</b> · slug <b>` + htmlEscapeText(reportSlug) + `</b> · ` + htmlEscapeText(now.Format("2006-01-02 15:04")) + `</p>

  <h2>Body</h2>
  <div class="panel">
    <p>` + htmlEscapeText(kindHint) + `</p>
  </div>

  <footer>Authored snapshot — layout is yours; keep charset, provenance comment, and design tokens intact.</footer>
</main>
`
}

func changeReportDefaultTitle(kind, reportSlug string) string {
	label := strings.ReplaceAll(reportSlug, "-", " ")
	switch kind {
	case "approval":
		return "Approval — " + label
	case "review":
		return "Review — " + label
	case "visual":
		return "Visual — " + label
	case "audit":
		return "Audit — " + label
	default:
		return "Note — " + label
	}
}

func changeReportKindHint(kind string) string {
	switch kind {
	case "approval":
		return "Ceremony approval board: what is being approved, at which commit/digest, and the gate the human must clear. Reserved for publish/shaping ceremonies."
	case "review":
		return "Ceremony review board: findings table with severity and disposition. Reserved so publish can find review artifacts deterministically."
	case "visual":
		return "Informational visual: process diagrams, lifecycle maps, anatomy sketches — teach the model, do not gate it."
	case "audit":
		return "Informational audit: inventory, compliance, or corpus sweep evidence baked at write time."
	default:
		return "Escape-hatch note: use when no registered kind fits; recurring topics signal when a new kind has earned registration."
	}
}

func writeChangeReportDesignGuidance(out io.Writer, kind string) {
	fmt.Fprintln(out, "Design language (keep these invariants; layout is yours):")
	fmt.Fprintln(out, "  - First line: HTML provenance comment (source · kind · slug · stamped time)")
	fmt.Fprintln(out, "  - <meta charset=\"utf-8\"> before title so file:// opens correctly in every browser")
	fmt.Fprintln(out, "  - Shared tokens: --bg --panel --line --ink --muted --accent --accent-fill --accent-soft")
	fmt.Fprintln(out, "                 --machine --machine-soft --ok --ok-soft --bad --bad-soft --state --state-soft")
	fmt.Fprintln(out, "                 --mono --serif --sans; light/dark via prefers-color-scheme and data-theme")
	fmt.Fprintln(out, "  - Eyebrow + serif h1 + mono stamp; panel cards for dense material")
	if changeReportKindCeremony[kind] {
		fmt.Fprintf(out, "  - Kind %q is ceremony-reserved: keep the filename glob YYYYMMDD-HHMMSS-%s-*.html stable\n", kind, kind)
	} else {
		fmt.Fprintf(out, "  - Kind %q is informational; bake data at write time — never a live derived view\n", kind)
	}
	fmt.Fprintln(out, "  - Registry:", strings.Join(changeReportKinds, ", "))
}

func htmlEscapeText(value string) string {
	replacer := strings.NewReplacer(
		`&`, "&amp;",
		`<`, "&lt;",
		`>`, "&gt;",
		`"`, "&quot;",
	)
	return replacer.Replace(value)
}
