package auth

import "strings"

var exemptTopLevelCommands = map[string]bool{
	"attach":  true,
	"auth":    true,
	"build":   true,
	"install": true,
	"upgrade": true,
	"init":    true,
	"setup":   true,
	"config":  true,
	"hooks":   true,
	"harness": true,
	"migrate": true,
	"serve":   true,
	"check":   true,
	"version": true,
	"release": true,
}

var exemptStateSubcommands = map[string]bool{
	"init":    true,
	"migrate": true,
	"path":    true,
	"status":  true,
}

// CommandRequiresAttach reports whether args name a substrate-touching command.
func CommandRequiresAttach(args []string) bool {
	if len(args) == 0 {
		return false
	}
	top := strings.TrimSpace(args[0])
	if top == "" || exemptTopLevelCommands[top] {
		return false
	}
	if top == "state" && len(args) > 1 {
		sub := strings.TrimSpace(args[1])
		if exemptStateSubcommands[sub] {
			return false
		}
	}
	return true
}
