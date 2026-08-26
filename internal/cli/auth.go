package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/auth"
	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

type authCommonOptions struct {
	serverDB   string
	jsonOutput bool
}

func (r Runner) runAuth(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeAuthHelp(out)
		return nil
	}
	switch args[0] {
	case "setup":
		return r.runAuthSetup(args[1:], out)
	case "link":
		return r.runAuthLink(args[1:], out, runtime)
	case "list":
		return r.runAuthList(args[1:], out)
	case "revoke":
		return r.runAuthRevoke(args[1:], out)
	case "attach":
		return r.runAuthAttach(args[1:], out)
	default:
		return fmt.Errorf("auth: unknown subcommand %q", args[0])
	}
}

func writeAuthHelp(out io.Writer) {
	writeUsageHelp(out, "loaf auth <setup|link|list|revoke|attach>", "Manage substrate accounts, connection tokens, and attach state.",
		"setup   Create a zero-PII account, mint master + Emergency Kit, store admin wire",
		"link    Mint a named connection token and emit the bundled client wire",
		"list    List named connection tokens with last-seen timestamps",
		"revoke  Revoke a named connection token (effective at next client contact)",
		"attach  Mark this environment attached after successful client provisioning")
}

func parseAuthCommonArgs(args []string) (authCommonOptions, []string, error) {
	opts := authCommonOptions{}
	rest := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--server-db":
			if i+1 >= len(args) {
				return authCommonOptions{}, nil, fmt.Errorf("auth --server-db requires a value")
			}
			i++
			opts.serverDB = strings.TrimSpace(args[i])
		case "--json":
			opts.jsonOutput = true
		default:
			rest = append(rest, args[i])
		}
	}
	return opts, rest, nil
}

func (r Runner) runAuthSetup(args []string, out io.Writer) error {
	if len(args) == 1 && isHelpArg(args) {
		writeUsageHelp(out, "loaf auth setup --endpoint <url> --server-db <path> [--json]", "Create account against a self-hosted sync server and store admin wire locally.")
		return nil
	}
	endpoint := ""
	serverDB := ""
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--endpoint":
			if i+1 >= len(args) {
				return fmt.Errorf("auth setup --endpoint requires a value")
			}
			i++
			endpoint = strings.TrimSpace(args[i])
		case "--server-db":
			if i+1 >= len(args) {
				return fmt.Errorf("auth setup --server-db requires a value")
			}
			i++
			serverDB = strings.TrimSpace(args[i])
		case "--json":
			jsonOutput = true
		default:
			return fmt.Errorf("auth setup: unknown option %q", args[i])
		}
	}
	store, err := r.authStore()
	if err != nil {
		return err
	}
	result, err := auth.Setup(context.Background(), store, auth.SetupInput{
		Endpoint: endpoint,
		ServerDB: serverDB,
	})
	if err != nil {
		return err
	}
	if jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	fmt.Fprintf(out, "auth setup ok\n")
	fmt.Fprintf(out, "endpoint: %s\n", result.Endpoint)
	fmt.Fprintf(out, "account access key: %s\n", result.AccountAccessKey)
	fmt.Fprintf(out, "master fingerprint: %s\n", result.MasterFingerprint)
	fmt.Fprintf(out, "server db: %s\n", result.ServerDB)
	fmt.Fprintln(out, "emergency kit (store offline; shown once):")
	fmt.Fprintln(out, result.EmergencyKit)
	return nil
}

func (r Runner) runAuthLink(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 1 && isHelpArg(args) {
		writeUsageHelp(out, "loaf auth link <name> [--project <id>] [--server-db <path>] [--json]", "Mint a named connection token and print the bundled client wire.")
		return nil
	}
	opts, rest, err := parseAuthCommonArgs(args)
	if err != nil {
		return err
	}
	projectID := ""
	name := ""
	for i := 0; i < len(rest); i++ {
		switch rest[i] {
		case "--project":
			if i+1 >= len(rest) {
				return fmt.Errorf("auth link --project requires a value")
			}
			i++
			projectID = strings.TrimSpace(rest[i])
		default:
			if name != "" {
				return fmt.Errorf("auth link: unexpected argument %q", rest[i])
			}
			name = strings.TrimSpace(rest[i])
		}
	}
	if name == "" {
		return fmt.Errorf("auth link requires a connection name")
	}
	if projectID == "" {
		projectID, err = r.resolveAuthProjectID(runtime)
		if err != nil {
			return err
		}
	}
	store, err := r.authStore()
	if err != nil {
		return err
	}
	result, err := auth.Link(context.Background(), store, auth.LinkInput{
		Name:      name,
		ProjectID: projectID,
		ServerDB:  opts.serverDB,
	})
	if err != nil {
		return err
	}
	if opts.jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(result)
	}
	fmt.Fprintf(out, "auth link ok: name=%s project=%s token=%s\n", result.Name, result.ProjectID, result.ConnectionTokenID)
	fmt.Fprintln(out, result.ClientWire)
	return nil
}

func (r Runner) runAuthList(args []string, out io.Writer) error {
	if len(args) == 1 && isHelpArg(args) {
		writeUsageHelp(out, "loaf auth list [--server-db <path>] [--json]", "List named connection tokens.")
		return nil
	}
	opts, rest, err := parseAuthCommonArgs(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return fmt.Errorf("auth list: unexpected argument %q", rest[0])
	}
	store, err := r.authStore()
	if err != nil {
		return err
	}
	rows, err := auth.ListConnections(context.Background(), store, opts.serverDB)
	if err != nil {
		return err
	}
	if opts.jsonOutput {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(rows)
	}
	if len(rows) == 0 {
		fmt.Fprintln(out, "no connection tokens")
		return nil
	}
	for _, row := range rows {
		lastSeen := "-"
		if row.LastSeenAt != nil {
			lastSeen = row.LastSeenAt.UTC().Format("2006-01-02 15:04")
		}
		revoked := ""
		if row.RevokedAt != nil {
			revoked = " revoked"
		}
		fmt.Fprintf(out, "%s project=%s token=%s last_seen=%s%s\n", row.Name, row.ProjectID, row.TokenID, lastSeen, revoked)
	}
	return nil
}

func (r Runner) runAuthRevoke(args []string, out io.Writer) error {
	if len(args) == 1 && isHelpArg(args) {
		writeUsageHelp(out, "loaf auth revoke <name> [--server-db <path>]", "Revoke a named connection token.")
		return nil
	}
	opts, rest, err := parseAuthCommonArgs(args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("auth revoke requires exactly one connection name")
	}
	store, err := r.authStore()
	if err != nil {
		return err
	}
	if err := auth.RevokeConnection(context.Background(), store, rest[0], opts.serverDB); err != nil {
		return err
	}
	if opts.jsonOutput {
		return json.NewEncoder(out).Encode(map[string]string{"status": "revoked", "name": rest[0]})
	}
	fmt.Fprintf(out, "revoked connection %q\n", rest[0])
	return nil
}

func (r Runner) runAuthAttach(args []string, out io.Writer) error {
	if len(args) == 1 && isHelpArg(args) {
		writeUsageHelp(out, "loaf auth attach --name <connection> [--endpoint <url>]", "Record successful attach for this environment.")
		return nil
	}
	name := ""
	endpoint := ""
	jsonOutput := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 >= len(args) {
				return fmt.Errorf("auth attach --name requires a value")
			}
			i++
			name = strings.TrimSpace(args[i])
		case "--endpoint":
			if i+1 >= len(args) {
				return fmt.Errorf("auth attach --endpoint requires a value")
			}
			i++
			endpoint = strings.TrimSpace(args[i])
		case "--json":
			jsonOutput = true
		default:
			return fmt.Errorf("auth attach: unknown option %q", args[i])
		}
	}
	if name == "" {
		return fmt.Errorf("auth attach requires --name")
	}
	store, err := r.authStore()
	if err != nil {
		return err
	}
	if endpoint == "" {
		var err error
		endpoint, err = auth.AdminEndpoint(store)
		if err != nil {
			return err
		}
	}
	if err := auth.MarkAttached(store, endpoint, name); err != nil {
		return err
	}
	if jsonOutput {
		state, _ := store.LoadAttachState()
		return json.NewEncoder(out).Encode(state)
	}
	fmt.Fprintf(out, "attached as %q via %s\n", name, endpoint)
	return nil
}

func (r Runner) resolveAuthProjectID(runtime state.Runtime) (string, error) {
	root, err := project.ResolveRoot(runtime.RootPath())
	if err != nil {
		return "", err
	}
	resolver := state.PathResolver{StateHome: r.StateHome}
	databasePath, err := resolver.DatabasePath(root)
	if err != nil {
		return "", err
	}
	store, err := state.OpenStore(databasePath)
	if err != nil {
		return "", err
	}
	defer store.Close()
	identity, err := store.LookupProjectIdentityForRoot(context.Background(), root)
	if err != nil {
		return "", fmt.Errorf("resolve project for auth link: %w", err)
	}
	return identity.ID, nil
}
