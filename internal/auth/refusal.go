package auth

import "fmt"

// UnattachedError reports that the environment has not attached to the substrate.
type UnattachedError struct {
	Code      string `json:"code"`
	Cause     string `json:"cause"`
	Remedy    string `json:"remedy"`
	ProjectID string `json:"project_id,omitempty"`
	Command   string `json:"command,omitempty"`
}

func (e *UnattachedError) Error() string {
	if e == nil {
		return "environment is not attached to the substrate"
	}
	msg := e.Cause
	if msg == "" {
		msg = "this environment is not attached to the Loaf substrate"
	}
	if e.Remedy != "" {
		return fmt.Sprintf("%s\nremedy: %s", msg, e.Remedy)
	}
	return msg
}

// NewUnattachedError builds a refusal for substrate-touching commands.
func NewUnattachedError(projectID, command string) *UnattachedError {
	return &UnattachedError{
		Code:      UnattachedCode,
		Cause:     "this environment is not attached to the Loaf substrate",
		Remedy:    "run `loaf auth setup` on a trusted machine, `loaf auth link <name>` to mint a client credential, store it as LOAF_CLIENT_TOKEN for this project, then complete attach when `loaf auth attach` ships",
		ProjectID: projectID,
		Command:   command,
	}
}
