package auth

import "strings"

var substrateCommandRoots = map[string]struct{}{
	"brainstorm":   {},
	"bundle":       {},
	"conversation": {},
	"exploration":  {},
	"handoff":      {},
	"housekeeping": {},
	"idea":         {},
	"intake":       {},
	"intent":       {},
	"issue":        {},
	"journal":      {},
	"kb":           {},
	"link":         {},
	"plan":         {},
	"project":      {},
	"release":      {},
	"render":       {},
	"report":       {},
	"scratchpad":   {},
	"search":       {},
	"spark":        {},
	"sync":         {},
	"tag":          {},
	"task":         {},
	"trace":        {},
}

// CommandRequiresAttach reports whether argv names a substrate-touching command.
func CommandRequiresAttach(args []string) bool {
	if len(args) == 0 {
		return false
	}
	if isHelpOrVersionArg(args) {
		return false
	}
	root := strings.ToLower(strings.TrimSpace(args[0]))
	if root == "state" {
		return stateSubcommandRequiresAttach(args[1:])
	}
	_, ok := substrateCommandRoots[root]
	return ok
}

func stateSubcommandRequiresAttach(args []string) bool {
	if len(args) == 0 || isHelpOrVersionArg(args) {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(args[0])) {
	case "init", "path", "backup", "restore", "migrate":
		return false
	default:
		return true
	}
}

// IsAuthNamespace reports whether argv targets the auth command tree.
func IsAuthNamespace(args []string) bool {
	if len(args) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(args[0]), "auth")
}

func isHelpOrVersionArg(args []string) bool {
	for _, arg := range args {
		switch arg {
		case "--help", "-h", "--version", "-v", "help":
			return true
		}
	}
	return false
}

// CheckAttached returns an UnattachedError when the project has no attach record.
func CheckAttached(dataHome, projectID, command string) error {
	attached, err := IsAttached(dataHome, projectID)
	if err != nil {
		return err
	}
	if attached {
		return nil
	}
	return NewUnattachedError(projectID, command)
}
