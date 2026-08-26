package cli

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/internal/auth"
	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

type authSetupOptions struct {
	endpoint   string
	serverDB   string
	jsonOutput bool
}

type authLinkOptions struct {
	name       string
	projectRef string
	serverDB   string
	jsonOutput bool
}

type authListOptions struct {
	serverDB   string
	jsonOutput bool
}

type authRevokeOptions struct {
	name     string
	serverDB string
}

func (r Runner) runAuth(args []string, out io.Writer, runtime state.Runtime) error {
	if len(args) == 0 || isHelpArg(args) {
		writeAuthHelp(out)
		return nil
	}
	switch args[0] {
	case "--help", "-h", "help":
		writeAuthHelp(out)
		return nil
	case "setup":
		return r.runAuthSetup(args[1:], out)
	case "link":
		return r.runAuthLink(args[1:], out, runtime)
	case "list":
		return r.runAuthList(args[1:], out)
	case "revoke":
		return r.runAuthRevoke(args[1:], out)
	default:
		return fmt.Errorf("auth: unknown subcommand %q", args[0])
	}
}

func writeAuthHelp(out io.Writer) {
	writeUsageHelp(out, "loaf auth <setup|link|list|revoke>", "Manage zero-PII accounts and named connection tokens.",
		"setup   Create a relay account, mint a master key, print the Emergency Kit, and store the admin credential",
		"link    Mint a named connection token and emit the bundled client credential",
		"list    List connection tokens with last-seen timestamps",
		"revoke  Revoke a named connection token (effective at the client's next server contact)",
		"",
		"Shared options:",
		"  --server-db <path>  Sync relay SQLite path (default $XDG_DATA_HOME/loaf/sync.sqlite)",
		"  --json              Emit structured output where supported")
}

func (r Runner) runAuthSetup(args []string, out io.Writer) error {
	opts, err := parseAuthSetupArgs(args)
	if err != nil {
		return err
	}
	dataHome, err := r.resolveDataHome()
	if err != nil {
		return err
	}
	cfg := auth.AdminConfig{DataHome: dataHome, ServerDB: opts.serverDB}
	result, err := auth.SetupAccount(context.Background(), cfg, opts.endpoint)
	if err != nil {
		return err
	}
	if opts.jsonOutput {
		payload := map[string]any{
			"access_key_id": result.AccessKeyID,
			"endpoint":      result.Endpoint,
			"server_db":     result.ServerDB,
			"emergency_kit": result.EmergencyKit,
		}
		return writeJSON(out, payload)
	}
	fmt.Fprintf(out, "account created: %s\n", result.AccessKeyID)
	fmt.Fprintf(out, "endpoint: %s\n", result.Endpoint)
	fmt.Fprintf(out, "server db: %s\n", result.ServerDB)
	fmt.Fprintln(out, "emergency kit (store offline; shown once):")
	fmt.Fprintln(out, result.EmergencyKit)
	return nil
}

func (r Runner) runAuthLink(args []string, out io.Writer, runtime state.Runtime) error {
	opts, err := parseAuthLinkArgs(args)
	if err != nil {
		return err
	}
	dataHome, err := r.resolveDataHome()
	if err != nil {
		return err
	}
	projectID, err := r.resolveAuthProjectID(opts.projectRef, runtime)
	if err != nil {
		return err
	}
	cfg := auth.AdminConfig{DataHome: dataHome, ServerDB: opts.serverDB}
	result, err := auth.LinkConnection(context.Background(), cfg, auth.LinkOptions{Name: opts.name, ProjectID: projectID})
	if err != nil {
		return err
	}
	if opts.jsonOutput {
		return writeJSON(out, result)
	}
	fmt.Fprintf(out, "connection %q minted for project %s\n", result.Name, result.ProjectID)
	fmt.Fprintln(out, result.BundledCredential)
	return nil
}

func (r Runner) runAuthList(args []string, out io.Writer) error {
	opts, err := parseAuthListArgs(args)
	if err != nil {
		return err
	}
	dataHome, err := r.resolveDataHome()
	if err != nil {
		return err
	}
	cfg := auth.AdminConfig{DataHome: dataHome, ServerDB: opts.serverDB}
	tokens, err := auth.ListConnections(context.Background(), cfg)
	if err != nil {
		return err
	}
	if opts.jsonOutput {
		return writeJSON(out, tokens)
	}
	if len(tokens) == 0 {
		fmt.Fprintln(out, "no connection tokens")
		return nil
	}
	for _, token := range tokens {
		status := "active"
		if token.RevokedAt != nil {
			status = "revoked"
		}
		lastSeen := "never"
		if token.LastSeenAt != nil {
			lastSeen = token.LastSeenAt.Format("2006-01-02 15:04:05 UTC")
		}
		fmt.Fprintf(out, "- %s project=%s status=%s last_seen=%s\n", token.Name, token.ProjectID, status, lastSeen)
	}
	return nil
}

func (r Runner) runAuthRevoke(args []string, out io.Writer) error {
	opts, err := parseAuthRevokeArgs(args)
	if err != nil {
		return err
	}
	dataHome, err := r.resolveDataHome()
	if err != nil {
		return err
	}
	cfg := auth.AdminConfig{DataHome: dataHome, ServerDB: opts.serverDB}
	revoked, err := auth.RevokeConnection(context.Background(), cfg, opts.name)
	if err != nil {
		return err
	}
	if !revoked {
		return fmt.Errorf("auth revoke: connection %q not found", opts.name)
	}
	fmt.Fprintf(out, "revoked connection %q\n", opts.name)
	return nil
}

func parseAuthSetupArgs(args []string) (authSetupOptions, error) {
	opts := authSetupOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--endpoint":
			value, err := consumeFlagValue(args, &i, "--endpoint")
			if err != nil {
				return authSetupOptions{}, err
			}
			opts.endpoint = value
		case "--server-db":
			value, err := consumeFlagValue(args, &i, "--server-db")
			if err != nil {
				return authSetupOptions{}, err
			}
			opts.serverDB = value
		case "--json":
			opts.jsonOutput = true
		default:
			return authSetupOptions{}, fmt.Errorf("auth setup: unknown option %q", args[i])
		}
	}
	if strings.TrimSpace(opts.endpoint) == "" {
		return authSetupOptions{}, fmt.Errorf("auth setup requires --endpoint")
	}
	return opts, nil
}

func parseAuthLinkArgs(args []string) (authLinkOptions, error) {
	opts := authLinkOptions{}
	var positionals []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--project":
			value, err := consumeFlagValue(args, &i, "--project")
			if err != nil {
				return authLinkOptions{}, err
			}
			opts.projectRef = value
		case "--server-db":
			value, err := consumeFlagValue(args, &i, "--server-db")
			if err != nil {
				return authLinkOptions{}, err
			}
			opts.serverDB = value
		case "--json":
			opts.jsonOutput = true
		default:
			if strings.HasPrefix(args[i], "-") {
				return authLinkOptions{}, fmt.Errorf("auth link: unknown option %q", args[i])
			}
			positionals = append(positionals, args[i])
		}
	}
	if len(positionals) != 1 {
		return authLinkOptions{}, fmt.Errorf("auth link requires <name>")
	}
	opts.name = positionals[0]
	return opts, nil
}

func parseAuthListArgs(args []string) (authListOptions, error) {
	opts := authListOptions{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--server-db":
			value, err := consumeFlagValue(args, &i, "--server-db")
			if err != nil {
				return authListOptions{}, err
			}
			opts.serverDB = value
		case "--json":
			opts.jsonOutput = true
		default:
			return authListOptions{}, fmt.Errorf("auth list: unknown option %q", args[i])
		}
	}
	return opts, nil
}

func parseAuthRevokeArgs(args []string) (authRevokeOptions, error) {
	opts := authRevokeOptions{}
	var positionals []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--server-db":
			value, err := consumeFlagValue(args, &i, "--server-db")
			if err != nil {
				return authRevokeOptions{}, err
			}
			opts.serverDB = value
		default:
			if strings.HasPrefix(args[i], "-") {
				return authRevokeOptions{}, fmt.Errorf("auth revoke: unknown option %q", args[i])
			}
			positionals = append(positionals, args[i])
		}
	}
	if len(positionals) != 1 {
		return authRevokeOptions{}, fmt.Errorf("auth revoke requires <name>")
	}
	opts.name = positionals[0]
	return opts, nil
}

func (r Runner) resolveAuthProjectID(projectRef string, runtime state.Runtime) (string, error) {
	projectRef = strings.TrimSpace(projectRef)
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
	if projectRef == "" {
		identity, err := store.ProjectIdentityForRoot(context.Background(), root)
		if err != nil {
			return "", err
		}
		return identity.ID, nil
	}
	projects, err := store.ListProjects(context.Background())
	if err != nil {
		return "", err
	}
	for _, identity := range projects.Projects {
		if identity.ID == projectRef || strings.EqualFold(identity.FriendlyName, projectRef) {
			return identity.ID, nil
		}
	}
	return "", fmt.Errorf("auth link: project %q not found", projectRef)
}

func (r Runner) resolveDataHome() (string, error) {
	resolver := state.PathResolver{StateHome: r.StateHome}
	return resolver.ResolvedDataHome()
}
