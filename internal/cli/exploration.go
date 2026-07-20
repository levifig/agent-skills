package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

func (r Runner) runExploration(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeExplorationHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"create":       writeExplorationCreateHelp,
		"checkpoint":   writeExplorationCheckpointHelp,
		"list":         writeExplorationListHelp,
		"context":      writeExplorationContextHelp,
		"conversation": writeExplorationConversationHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "create":
		return r.runExplorationCreate(args[1:], out, runtime)
	case "checkpoint":
		return r.runExplorationCheckpoint(args[1:], out, runtime)
	case "list":
		return r.runExplorationList(args[1:], out, runtime)
	case "context":
		return r.runExplorationContext(args[1:], out, runtime)
	case "conversation":
		return r.runExplorationConversation(args[1:], out, runtime)
	default:
		return unknownSubcommandError("exploration", args[0])
	}
}

func writeExplorationHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf exploration <subcommand> [options]", "Manage relational Exploration continuity: immutable portable checkpoints, no lifecycle status, no current pointer.", []subcommandHelpItem{
		{Name: "create", Summary: "Create an Exploration identity"},
		{Name: "checkpoint", Summary: "Append an immutable portable checkpoint"},
		{Name: "list", Summary: "List Explorations with checkpoint counts"},
		{Name: "context", Summary: "Project portable context with bounded layers"},
		{Name: "conversation", Summary: "Associate a logical conversation (add)"},
	})
}

func writeExplorationCreateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf exploration create --title <title> [--from <intent-or-source>]... [--json]", "Create an Exploration identity; sources map to explores or uses-source edges by kind.",
		"--title      Bounded exploration title",
		"--from       Intent, journal entry, handoff, report, or finding reference; repeatable",
		"--json       Output the created Exploration and project identity as JSON")
}

func writeExplorationCheckpointHelp(out io.Writer) {
	writeUsageHelp(out, "loaf exploration checkpoint <exploration> --purpose <text> --conclusions <text> --unresolved <text> --next <text> [--item <type>:<content>]... [--operation-id <key>] [--json]", "Append one immutable checkpoint; all four core fields are required, trimmed, and capped at 4096 UTF-8 bytes without truncation.",
		"--purpose        Current framing",
		"--conclusions    Conclusions or constraints so far",
		"--unresolved     Unresolved question or decision",
		"--next           Recommended next action",
		"--item           Ordered typed item, e.g. candidate:<text> or evidence:<text>; repeatable",
		"--operation-id   Retry-safe operation key",
		"--json           Output the appended checkpoint and project identity as JSON")
}

func writeExplorationListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf exploration list [--json]", "List Explorations with checkpoint counts and portable-context presence.",
		"--json       Output Explorations and project identity as JSON")
}

func writeExplorationContextHelp(out io.Writer) {
	writeUsageHelp(out, "loaf exploration context <exploration> [--layer items|intents|evidence|conversations --cursor <cursor> --limit <n>] [--json]", "Project portable context: the four-field core is returned whole; every optional layer reports counts, truncation, and an exact expansion command.",
		"--layer      Select one optional layer for expansion",
		"--cursor     Continue the selected layer (requires --layer)",
		"--limit      Maximum 1..100 items for the selected layer (requires --layer)",
		"--json       Output the portable context projection as JSON")
}

func writeExplorationConversationHelp(out io.Writer) {
	writeUsageHelp(out, "loaf exploration conversation add <exploration> <conversation-id> [--json]", "Explicitly associate a logical conversation; membership is never inferred from branch, worktree, or recency.",
		"--json       Output the membership result as JSON")
}

func (r Runner) runExplorationCreate(args []string, out io.Writer, runtime state.Runtime) error {
	options := state.ExplorationCreateOptions{}
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--title":
			value, err := consumeFlagValue(args, &i, "--title")
			if err != nil {
				return err
			}
			options.Title = value
		case "--from":
			value, err := consumeFlagValue(args, &i, "--from")
			if err != nil {
				return err
			}
			options.Sources = append(options.Sources, value)
		default:
			return fmt.Errorf("unknown option %q", args[i])
		}
	}
	if strings.TrimSpace(options.Title) == "" {
		return fmt.Errorf("exploration create requires --title")
	}
	projectRoot, err := r.requireIntentSQLiteState("exploration create", runtime)
	if err != nil {
		return err
	}
	result, err := state.CreateExploration(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeExplorationMutation(out, result)
	return nil
}

func (r Runner) runExplorationCheckpoint(args []string, out io.Writer, runtime state.Runtime) error {
	options := state.ExplorationCheckpointOptions{}
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--purpose":
			value, err := consumeFlagValue(args, &i, "--purpose")
			if err != nil {
				return err
			}
			options.Purpose = value
		case "--conclusions":
			value, err := consumeFlagValue(args, &i, "--conclusions")
			if err != nil {
				return err
			}
			options.Conclusions = value
		case "--unresolved":
			value, err := consumeFlagValue(args, &i, "--unresolved")
			if err != nil {
				return err
			}
			options.Unresolved = value
		case "--next":
			value, err := consumeFlagValue(args, &i, "--next")
			if err != nil {
				return err
			}
			options.NextAction = value
		case "--operation-id":
			value, err := consumeFlagValue(args, &i, "--operation-id")
			if err != nil {
				return err
			}
			options.OperationID = value
		case "--item":
			value, err := consumeFlagValue(args, &i, "--item")
			if err != nil {
				return err
			}
			itemType, content, found := strings.Cut(value, ":")
			if !found || strings.TrimSpace(itemType) == "" || strings.TrimSpace(content) == "" {
				return fmt.Errorf("--item requires <type>:<content>, got %q", value)
			}
			options.Items = append(options.Items, state.CheckpointItemInput{Type: strings.TrimSpace(itemType), Content: strings.TrimSpace(content)})
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown option %q", args[i])
			}
			if options.ExplorationRef != "" {
				return fmt.Errorf("exploration checkpoint accepts one exploration reference")
			}
			options.ExplorationRef = args[i]
		}
	}
	if options.ExplorationRef == "" {
		return fmt.Errorf("exploration checkpoint requires an exploration reference")
	}
	projectRoot, err := r.requireIntentSQLiteState("exploration checkpoint", runtime)
	if err != nil {
		return err
	}
	result, err := state.AppendExplorationCheckpoint(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeExplorationMutation(out, result)
	return nil
}

func (r Runner) runExplorationList(args []string, out io.Writer, runtime state.Runtime) error {
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		return fmt.Errorf("unknown option %q", arg)
	}
	projectRoot, err := r.requireIntentSQLiteState("exploration list", runtime)
	if err != nil {
		return err
	}
	result, err := state.ListExplorations(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	if len(result.Explorations) == 0 {
		fmt.Fprintln(out, "no explorations found")
	}
	for _, item := range result.Explorations {
		ref := item.Alias
		if ref == "" {
			ref = item.ID
		}
		fmt.Fprintf(out, "%s  checkpoints=%d portable=%t  %s\n", ref, item.CheckpointCount, item.PortableContextPresent, item.Title)
	}
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runExplorationContext(args []string, out io.Writer, runtime state.Runtime) error {
	options := state.ExplorationContextOptions{}
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--layer":
			value, err := consumeFlagValue(args, &i, "--layer")
			if err != nil {
				return err
			}
			options.Layer = value
		case "--cursor":
			value, err := consumeFlagValue(args, &i, "--cursor")
			if err != nil {
				return err
			}
			options.Cursor = value
		case "--limit":
			value, err := consumeFlagValue(args, &i, "--limit")
			if err != nil {
				return err
			}
			limit, err := strconv.Atoi(value)
			if err != nil {
				return fmt.Errorf("--limit requires an integer, got %q", value)
			}
			options.Limit = limit
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown option %q", args[i])
			}
			if options.ExplorationRef != "" {
				return fmt.Errorf("exploration context accepts one exploration reference")
			}
			options.ExplorationRef = args[i]
		}
	}
	if options.ExplorationRef == "" {
		return fmt.Errorf("exploration context requires an exploration reference")
	}
	projectRoot, err := r.requireIntentSQLiteState("exploration context", runtime)
	if err != nil {
		return err
	}
	result, err := state.ExplorationContext(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeExplorationContextHuman(out, result)
	return nil
}

func (r Runner) runExplorationConversation(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || args[0] != "add" {
		return fmt.Errorf("usage: loaf exploration conversation add <exploration> <conversation-id> [--json]")
	}
	args = args[1:]
	if isHelpArg(args) {
		writeExplorationConversationHelp(out)
		return nil
	}
	jsonOutput := false
	positional := []string{}
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("unknown option %q", arg)
		}
		positional = append(positional, arg)
	}
	if len(positional) != 2 {
		return fmt.Errorf("exploration conversation add requires <exploration> and <conversation-id>")
	}
	projectRoot, err := r.requireIntentSQLiteState("exploration conversation add", runtime)
	if err != nil {
		return err
	}
	result, err := state.AddExplorationConversation(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, positional[0], positional[1])
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "exploration: %s\nconversation: %s\ncreated: %t\n", result.ExplorationID, result.Conversation.ID, result.Created)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func writeExplorationMutation(out io.Writer, result state.ExplorationMutationResult) {
	ref := result.Exploration.Alias
	if ref == "" {
		ref = result.Exploration.ID
	}
	if result.Created {
		fmt.Fprintln(out, "created")
	} else {
		fmt.Fprintln(out, "reused (operation key already established)")
	}
	fmt.Fprintf(out, "exploration: %s\n", ref)
	fmt.Fprintf(out, "title: %s\n", result.Exploration.Title)
	fmt.Fprintf(out, "portable context present: %t\n", result.Exploration.PortableContextPresent)
	if result.Checkpoint != nil {
		fmt.Fprintf(out, "checkpoint seq: %d\n", result.Checkpoint.Seq)
		fmt.Fprintf(out, "next action: %s\n", result.Checkpoint.NextAction)
	}
	if result.OperationID != "" {
		fmt.Fprintf(out, "operation: %s\n", result.OperationID)
		fmt.Fprintf(out, "digest match: %t\n", result.InputDigestMatches)
	}
	fmt.Fprintf(out, "read: loaf exploration context %s\n", ref)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
}

func writeExplorationContextHuman(out io.Writer, result state.ExplorationContextResult) {
	ref := result.Exploration.Alias
	if ref == "" {
		ref = result.Exploration.ID
	}
	fmt.Fprintf(out, "exploration: %s\n", ref)
	fmt.Fprintf(out, "title: %s\n", result.Exploration.Title)
	fmt.Fprintf(out, "portable context present: %t\n", result.PortableContextPresent)
	if result.Checkpoint != nil {
		fmt.Fprintf(out, "checkpoint seq: %d (%s)\n", result.Checkpoint.Seq, result.Checkpoint.CreatedAt)
		fmt.Fprintf(out, "purpose: %s\n", result.Checkpoint.Purpose)
		fmt.Fprintf(out, "conclusions: %s\n", result.Checkpoint.Conclusions)
		fmt.Fprintf(out, "unresolved: %s\n", result.Checkpoint.Unresolved)
		fmt.Fprintf(out, "next action: %s\n", result.Checkpoint.NextAction)
	} else {
		fmt.Fprintln(out, "no portable checkpoint; source handles alone never imply resumable context")
	}
	for _, layer := range []string{"items", "intents", "evidence", "conversations"} {
		built, ok := result.Layers[layer]
		if !ok {
			continue
		}
		fmt.Fprintf(out, "%s: showing %d of %d\n", layer, built.Shown, built.Available)
		var rendered []map[string]any
		if err := json.Unmarshal(built.Items, &rendered); err == nil {
			for _, item := range rendered {
				fmt.Fprintf(out, "  %s\n", compactContextItem(layer, item))
			}
		}
		if built.Truncated && built.ExpandCommand != "" {
			fmt.Fprintf(out, "  more: %s\n", built.ExpandCommand)
		}
	}
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
}

func compactContextItem(layer string, item map[string]any) string {
	get := func(key string) string {
		value, _ := item[key].(string)
		return value
	}
	switch layer {
	case "items":
		return fmt.Sprintf("%v. %s: %s", item["position"], get("type"), get("content"))
	case "intents":
		return fmt.Sprintf("%s %s (%s) %s", get("relationship"), firstNonEmpty(get("alias"), get("id")), get("disposition"), get("title"))
	case "evidence":
		return fmt.Sprintf("%s %s %s %s", get("relationship"), get("kind"), get("id"), get("title"))
	case "conversations":
		title := get("title")
		handles, _ := item["handles"].([]any)
		return fmt.Sprintf("%s (%d handles)", title, len(handles))
	default:
		return fmt.Sprintf("%v", item)
	}
}

func (r Runner) runConversation(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeConversationHelp(out)
		return nil
	}
	if writeNestedHelp(out, args, map[string]func(io.Writer){
		"create":  writeConversationCreateHelp,
		"show":    writeConversationShowHelp,
		"list":    writeConversationListHelp,
		"handle":  writeConversationHandleHelp,
		"observe": writeConversationObserveHelp,
	}) {
		return nil
	}
	switch args[0] {
	case "create":
		return r.runConversationCreate(args[1:], out, runtime)
	case "show":
		return r.runConversationShow(args[1:], out, runtime)
	case "list":
		return r.runConversationList(args[1:], out, runtime)
	case "handle":
		return r.runConversationHandle(args[1:], out, runtime)
	case "observe":
		return r.runConversationObserve(args[1:], out, runtime)
	default:
		return unknownSubcommandError("conversation", args[0])
	}
}

func writeConversationHelp(out io.Writer) {
	writeCommandGroupHelp(out, "loaf conversation <subcommand> [options]", "Manage logical conversations and machine-local provenance handles. Handles are optional evidence and never imply portable context.", []subcommandHelpItem{
		{Name: "create", Summary: "Create a logical conversation"},
		{Name: "show", Summary: "Show one conversation with handles and log refs"},
		{Name: "list", Summary: "List logical conversations"},
		{Name: "handle", Summary: "Attach a machine-local handle (add)"},
		{Name: "observe", Summary: "Append an immutable availability observation"},
	})
}

func writeConversationCreateHelp(out io.Writer) {
	writeUsageHelp(out, "loaf conversation create --title <label> [--operation-id <key>] [--json]", "Create a logical conversation that may carry multiple harness-local handles.",
		"--title          Conversation label",
		"--operation-id   Retry-safe operation key",
		"--json           Output the created conversation and project identity as JSON")
}

func writeConversationShowHelp(out io.Writer) {
	writeUsageHelp(out, "loaf conversation show <conversation-id> [--json]", "Show one conversation with handles, log refs, and latest observed availability.",
		"--json       Output the conversation and project identity as JSON")
}

func writeConversationListHelp(out io.Writer) {
	writeUsageHelp(out, "loaf conversation list [--json]", "List logical conversations deterministically.",
		"--json       Output conversations and project identity as JSON")
}

func writeConversationHandleHelp(out io.Writer) {
	writeUsageHelp(out, "loaf conversation handle add <conversation-id> --harness <harness> --handle <opaque-local-id> [--locality <machine-or-namespace>] [--log-ref <locator> [--hash <sha256>] [--range <range>]] [--json]", "Attach one machine-local handle and optional bounded log reference; nothing is inferred from the current session.",
		"--harness    Harness name, e.g. codex or claude-code",
		"--handle     Opaque machine-local conversation identifier",
		"--locality   Machine or namespace scope for the handle",
		"--log-ref    Bounded log locator (never transcript content)",
		"--hash       Optional SHA-256 of the referenced log range",
		"--range      Optional bounded range within the log",
		"--json       Output the handle result and project identity as JSON")
}

func writeConversationObserveHelp(out io.Writer) {
	writeUsageHelp(out, "loaf conversation observe (--handle <handle-id> | --log-ref <log-ref-id>) (--available | --unavailable) [--observer <name>] [--locality <scope>] [--note <text>] [--json]", "Append an immutable timestamped availability observation; the observed row itself never mutates.",
		"--handle       Observed conversation handle ID",
		"--log-ref      Observed log reference ID",
		"--available    Record that the source was reachable",
		"--unavailable  Record that the source was not reachable",
		"--observer     Observing agent or probe",
		"--locality     Machine or namespace of the observation",
		"--note         Bounded observation note",
		"--json         Output the observation result and project identity as JSON")
}

func (r Runner) runConversationCreate(args []string, out io.Writer, runtime state.Runtime) error {
	options := state.ConversationCreateOptions{}
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--title":
			value, err := consumeFlagValue(args, &i, "--title")
			if err != nil {
				return err
			}
			options.Title = value
		case "--operation-id":
			value, err := consumeFlagValue(args, &i, "--operation-id")
			if err != nil {
				return err
			}
			options.OperationID = value
		default:
			return fmt.Errorf("unknown option %q", args[i])
		}
	}
	projectRoot, err := r.requireIntentSQLiteState("conversation create", runtime)
	if err != nil {
		return err
	}
	result, err := state.CreateConversation(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "conversation: %s\ntitle: %s\ncreated: %t\n", result.Conversation.ID, result.Conversation.Title, result.Created)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runConversationShow(args []string, out io.Writer, runtime state.Runtime) error {
	ref, jsonOutput, err := parseSingleRefArgs("conversation show", args)
	if err != nil {
		return err
	}
	projectRoot, err := r.requireIntentSQLiteState("conversation show", runtime)
	if err != nil {
		return err
	}
	result, err := state.ShowConversation(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, ref)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	writeConversationDetailHuman(out, result.Conversation)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runConversationList(args []string, out io.Writer, runtime state.Runtime) error {
	jsonOutput := false
	for _, arg := range args {
		if arg == "--json" {
			jsonOutput = true
			continue
		}
		return fmt.Errorf("unknown option %q", arg)
	}
	projectRoot, err := r.requireIntentSQLiteState("conversation list", runtime)
	if err != nil {
		return err
	}
	result, err := state.ListConversations(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome})
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	if len(result.Conversations) == 0 {
		fmt.Fprintln(out, "no conversations found")
	}
	for _, conversation := range result.Conversations {
		fmt.Fprintf(out, "%s  handles=%d  %s\n", conversation.ID, len(conversation.Handles), conversation.Title)
	}
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runConversationHandle(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || args[0] != "add" {
		return fmt.Errorf("usage: loaf conversation handle add <conversation-id> --harness <harness> --handle <id> [options]")
	}
	args = args[1:]
	if isHelpArg(args) {
		writeConversationHandleHelp(out)
		return nil
	}
	options := state.ConversationHandleAddOptions{}
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--harness":
			value, err := consumeFlagValue(args, &i, "--harness")
			if err != nil {
				return err
			}
			options.Harness = value
		case "--handle":
			value, err := consumeFlagValue(args, &i, "--handle")
			if err != nil {
				return err
			}
			options.Handle = value
		case "--locality":
			value, err := consumeFlagValue(args, &i, "--locality")
			if err != nil {
				return err
			}
			options.Locality = value
		case "--log-ref":
			value, err := consumeFlagValue(args, &i, "--log-ref")
			if err != nil {
				return err
			}
			options.LogRef = value
		case "--hash":
			value, err := consumeFlagValue(args, &i, "--hash")
			if err != nil {
				return err
			}
			options.Hash = value
		case "--range":
			value, err := consumeFlagValue(args, &i, "--range")
			if err != nil {
				return err
			}
			options.Range = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return fmt.Errorf("unknown option %q", args[i])
			}
			if options.ConversationRef != "" {
				return fmt.Errorf("conversation handle add accepts one conversation reference")
			}
			options.ConversationRef = args[i]
		}
	}
	if options.ConversationRef == "" {
		return fmt.Errorf("conversation handle add requires a conversation reference")
	}
	projectRoot, err := r.requireIntentSQLiteState("conversation handle add", runtime)
	if err != nil {
		return err
	}
	result, err := state.AddConversationHandle(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "conversation: %s\nhandle: %s\ncreated: %t\n", result.Conversation.ID, result.HandleID, result.Created)
	if result.LogRefID != "" {
		fmt.Fprintf(out, "log ref: %s\n", result.LogRefID)
	}
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func (r Runner) runConversationObserve(args []string, out io.Writer, runtime state.Runtime) error {
	options := state.ConversationObserveOptions{}
	jsonOutput := false
	availabilitySet := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--json":
			jsonOutput = true
		case "--handle":
			value, err := consumeFlagValue(args, &i, "--handle")
			if err != nil {
				return err
			}
			options.SubjectKind = "conversation_handle"
			options.SubjectID = value
		case "--log-ref":
			value, err := consumeFlagValue(args, &i, "--log-ref")
			if err != nil {
				return err
			}
			options.SubjectKind = "conversation_log_ref"
			options.SubjectID = value
		case "--available":
			options.Available = true
			availabilitySet = true
		case "--unavailable":
			options.Available = false
			availabilitySet = true
		case "--observer":
			value, err := consumeFlagValue(args, &i, "--observer")
			if err != nil {
				return err
			}
			options.Observer = value
		case "--locality":
			value, err := consumeFlagValue(args, &i, "--locality")
			if err != nil {
				return err
			}
			options.Locality = value
		case "--note":
			value, err := consumeFlagValue(args, &i, "--note")
			if err != nil {
				return err
			}
			options.Note = value
		default:
			return fmt.Errorf("unknown option %q", args[i])
		}
	}
	if options.SubjectID == "" {
		return fmt.Errorf("conversation observe requires --handle or --log-ref")
	}
	if !availabilitySet {
		return fmt.Errorf("conversation observe requires --available or --unavailable")
	}
	projectRoot, err := r.requireIntentSQLiteState("conversation observe", runtime)
	if err != nil {
		return err
	}
	result, err := state.ObserveConversationSource(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, options)
	if err != nil {
		return err
	}
	if jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "observation: %s\navailable: %t\n", result.ObservationID, options.Available)
	writeProjectMutationContext(out, "", result.DatabaseScope, result.DatabasePath, result.ProjectID, result.ProjectName, result.ProjectCurrentPath)
	return nil
}

func writeConversationDetailHuman(out io.Writer, conversation state.ConversationDetail) {
	fmt.Fprintf(out, "conversation: %s\ntitle: %s\n", conversation.ID, conversation.Title)
	for _, handle := range conversation.Handles {
		availability := "unobserved"
		if handle.Latest != nil {
			if handle.Latest.Available {
				availability = "available at " + handle.Latest.ObservedAt
			} else {
				availability = "unavailable at " + handle.Latest.ObservedAt
			}
		}
		fmt.Fprintf(out, "handle: %s %s (%s) locality=%s availability=%s\n", handle.Harness, handle.Handle, handle.ID, handle.Locality, availability)
		for _, logRef := range handle.LogRefs {
			fmt.Fprintf(out, "  log: %s (%s)\n", logRef.Locator, logRef.ID)
		}
	}
}
