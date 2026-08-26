package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/scratchpad"
	"github.com/levifig/loaf/internal/state"
)

func (r Runner) runScratchpad(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeScratchpadHelp(out)
		return nil
	}
	switch args[0] {
	case "append":
		return r.runScratchpadAppend(args[1:], out, runtime)
	case "read":
		return r.runScratchpadRead(args[1:], out, runtime)
	case "list":
		return r.runScratchpadList(args[1:], out, runtime)
	case "claim":
		return r.runScratchpadClaim(args[1:], out, runtime)
	case "release":
		return r.runScratchpadRelease(args[1:], out, runtime)
	default:
		return unknownSubcommandError("scratchpad", args[0])
	}
}

func writeScratchpadHelp(out io.Writer) {
	writeUsageHelp(out, "loaf scratchpad <command>", "Ephemeral agent coordination channel for one effort.",
		"append   Append a scratchpad message",
		"read     Read scratchpad messages for a channel",
		"list     List roster and active claims for a channel",
		"claim    Claim a resource with a lease expiry",
		"release  Release a claimed resource")
}

func (r Runner) runScratchpadAppend(args []string, out io.Writer, runtime state.Runtime) error {
	opts, message, err := parseScratchpadChannelArgs(args, true)
	if err != nil {
		return err
	}
	if strings.TrimSpace(message) == "" {
		return fmt.Errorf("scratchpad append requires a message")
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	envelope, err := scratchpad.AppendMessage(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, scratchpad.AppendOptions{
		Channel:    opts.Channel,
		InstanceID: opts.InstanceID,
		EnvID:      opts.EnvID,
		Who:        opts.Who,
		WorkingRef: opts.WorkingRef,
		Text:       message,
	})
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeJSON(out, envelope)
	}
	fmt.Fprintf(out, "appended %s\n", envelope.ID)
	return nil
}

func (r Runner) runScratchpadRead(args []string, out io.Writer, runtime state.Runtime) error {
	opts, _, err := parseScratchpadChannelArgs(args, false)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	view, err := scratchpad.ReadChannel(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, opts.Channel, opts.Limit)
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeJSON(out, view)
	}
	for _, entry := range view.Messages {
		fmt.Fprintf(out, "%s %s\n", entry.HLC, formatScratchpadPayload(entry))
	}
	return nil
}

func (r Runner) runScratchpadList(args []string, out io.Writer, runtime state.Runtime) error {
	opts, _, err := parseScratchpadChannelArgs(args, false)
	if err != nil {
		return err
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	view, err := scratchpad.ReadChannel(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, opts.Channel, 0)
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeJSON(out, struct {
			Channel      string                  `json:"channel"`
			Roster       []scratchpad.RosterMember `json:"roster"`
			ActiveClaims []scratchpad.ActiveClaim  `json:"active_claims"`
		}{Channel: view.Channel, Roster: view.Roster, ActiveClaims: view.ActiveClaims})
	}
	fmt.Fprintln(out, "Roster:")
	for _, member := range view.Roster {
		fmt.Fprintf(out, "  %s (%s) last_seen=%s ref=%s\n", member.Who, member.InstanceID, member.LastSeen, member.WorkingRef)
	}
	fmt.Fprintln(out, "Active claims:")
	for _, claim := range view.ActiveClaims {
		fmt.Fprintf(out, "  %s held by %s until %s\n", claim.Resource, claim.InstanceID, claim.ExpiresAt)
	}
	return nil
}

func (r Runner) runScratchpadClaim(args []string, out io.Writer, runtime state.Runtime) error {
	opts, _, err := parseScratchpadChannelArgs(args, false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opts.Resource) == "" {
		return fmt.Errorf("scratchpad claim requires --resource")
	}
	if opts.TTL <= 0 {
		return fmt.Errorf("scratchpad claim requires --ttl")
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	envelope, err := scratchpad.Claim(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, scratchpad.ClaimOptions{
		Channel:    opts.Channel,
		InstanceID: opts.InstanceID,
		EnvID:      opts.EnvID,
		Resource:   opts.Resource,
		TTL:        opts.TTL,
	})
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeJSON(out, envelope)
	}
	fmt.Fprintf(out, "claimed %s (%s)\n", opts.Resource, envelope.ID)
	return nil
}

func (r Runner) runScratchpadRelease(args []string, out io.Writer, runtime state.Runtime) error {
	opts, _, err := parseScratchpadChannelArgs(args, false)
	if err != nil {
		return err
	}
	if strings.TrimSpace(opts.Resource) == "" {
		return fmt.Errorf("scratchpad release requires --resource")
	}
	projectRoot, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return err
	}
	envelope, err := scratchpad.Release(context.Background(), projectRoot, state.PathResolver{StateHome: r.StateHome}, scratchpad.ReleaseOptions{
		Channel:    opts.Channel,
		InstanceID: opts.InstanceID,
		EnvID:      opts.EnvID,
		Resource:   opts.Resource,
	})
	if err != nil {
		return err
	}
	if opts.JSON {
		return writeJSON(out, envelope)
	}
	fmt.Fprintf(out, "released %s (%s)\n", opts.Resource, envelope.ID)
	return nil
}

type scratchpadCommonOptions struct {
	Channel    string
	InstanceID string
	EnvID      string
	Who        string
	WorkingRef string
	Resource   string
	TTL        time.Duration
	Limit      int
	JSON       bool
}

func parseScratchpadChannelArgs(args []string, messageLast bool) (scratchpadCommonOptions, string, error) {
	opts := scratchpadCommonOptions{}
	var positional []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--json":
			opts.JSON = true
		case arg == "--channel" && i+1 < len(args):
			i++
			opts.Channel = args[i]
		case arg == "--instance" && i+1 < len(args):
			i++
			opts.InstanceID = args[i]
		case arg == "--env" && i+1 < len(args):
			i++
			opts.EnvID = args[i]
		case arg == "--who" && i+1 < len(args):
			i++
			opts.Who = args[i]
		case arg == "--ref" && i+1 < len(args):
			i++
			opts.WorkingRef = args[i]
		case arg == "--resource" && i+1 < len(args):
			i++
			opts.Resource = args[i]
		case arg == "--ttl" && i+1 < len(args):
			i++
			ttl, err := time.ParseDuration(args[i])
			if err != nil {
				return opts, "", fmt.Errorf("parse --ttl: %w", err)
			}
			opts.TTL = ttl
		case arg == "--limit" && i+1 < len(args):
			i++
			limit, err := strconv.Atoi(args[i])
			if err != nil {
				return opts, "", fmt.Errorf("parse --limit: %w", err)
			}
			opts.Limit = limit
		default:
			positional = append(positional, arg)
		}
	}
	if strings.TrimSpace(opts.Channel) == "" {
		return opts, "", fmt.Errorf("scratchpad requires --channel")
	}
	message := ""
	if messageLast {
		message = strings.TrimSpace(strings.Join(positional, " "))
	}
	return opts, message, nil
}

func formatScratchpadPayload(entry scratchpad.Entry) string {
	switch payload := entry.Payload.(type) {
	case scratchpad.MessagePayload:
		return payload.InstanceID + ": " + payload.Text
	default:
		return entry.Kind
	}
}
