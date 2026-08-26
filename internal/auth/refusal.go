package auth

import (
	"encoding/json"
	"fmt"
	"strings"
)

const UnattachedCode = "environment-unattached"

// RefusalError is a machine-readable attach refusal.
type RefusalError struct {
	Code       string `json:"code"`
	Cause      string `json:"cause"`
	Remedy     string `json:"remedy"`
	Command    string `json:"command,omitempty"`
	Suggestion string `json:"suggestion,omitempty"`
}

func (e *RefusalError) Error() string {
	if e == nil {
		return "environment is unattached"
	}
	msg := strings.TrimSpace(e.Cause)
	if msg == "" {
		msg = "this environment is not attached to the substrate"
	}
	if remedy := strings.TrimSpace(e.Remedy); remedy != "" {
		msg += "\n" + remedy
	}
	return msg
}

// JSON returns the refusal as compact JSON for --json callers.
func (e *RefusalError) JSON() ([]byte, error) {
	if e == nil {
		e = &RefusalError{
			Code:   UnattachedCode,
			Cause:  "this environment is not attached to the substrate",
			Remedy: "run loaf auth setup, then loaf auth link, then attach before substrate commands",
		}
	}
	if e.Code == "" {
		e.Code = UnattachedCode
	}
	return json.Marshal(e)
}

// NewUnattachedRefusal builds the default substrate-touching refusal.
func NewUnattachedRefusal(command string) *RefusalError {
	cmd := strings.TrimSpace(command)
	suggestion := "loaf auth setup --endpoint <url> --server-db <path>"
	if cmd != "" {
		suggestion = fmt.Sprintf("attach this environment before `%s`; start with %s", cmd, suggestion)
	}
	return &RefusalError{
		Code:       UnattachedCode,
		Cause:      "this environment is not attached to the substrate",
		Remedy:     "complete attach with loaf auth setup and loaf auth link, or paste a client bundle from an admin machine",
		Command:    cmd,
		Suggestion: suggestion,
	}
}
