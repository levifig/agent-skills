package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/levifig/loaf/internal/crypto"
	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
	"github.com/levifig/loaf/internal/sync"
)

const (
	ClientTokenEnv = "LOAF_CLIENT_TOKEN"
	SyncURLEnv     = "LOAF_SYNC_URL"

	refusalCodeConfMissing       = "attach-conf-missing"
	refusalCodeCredentialMissing = "attach-credential-missing"
	refusalCodeCredentialInvalid = "attach-credential-invalid"
	refusalCodeIdentityMismatch  = "attach-identity-mismatch"
	refusalCodeEndpointInsecure  = "attach-endpoint-insecure"
	refusalCodeReachFailed       = "attach-reach-failed"
	refusalCodeAuthFailed        = "attach-auth-failed"
	refusalCodeDecryptFailed     = "attach-decrypt-failed"
	refusalCodeCapability        = "attach-capability-unsupported"
	refusalCodeHLCSkew           = "attach-hlc-skew"
)

// AttachInput configures the unattended attach sequence for one environment.
type AttachInput struct {
	Root              project.Root
	Store             Store
	ClientWire        string
	ConnectionName    string
	HTTPClient        *http.Client
	MaxHLCSkewMS      int64
	AllowInsecureHTTP bool
	ProbeStore        *state.Store
}

// AttachResult summarizes a successful attach.
type AttachResult struct {
	ProjectID      string `json:"project_id"`
	ConfID         string `json:"conf_id,omitempty"`
	Endpoint       string `json:"endpoint"`
	ConnectionName string `json:"connection_name"`
	PulledFacts    int    `json:"pulled_facts"`
	DecryptedFacts int    `json:"decrypted_facts"`
}

// UnattendedAttach runs conf→cred→HTTPS→auth→pull→decrypt→capability→HLC skew→identity.
func UnattendedAttach(ctx context.Context, in AttachInput) (AttachResult, error) {
	conf, err := project.ReadProjectConf(in.Root)
	if err != nil {
		return AttachResult{}, newAttachRefusal(refusalCodeConfMissing, "project conf is missing or invalid", "ensure `.agents/loaf.conf` ships with the clone and includes project_id", err)
	}
	wire, err := resolveClientWire(in.Store, in.ClientWire)
	if err != nil {
		return AttachResult{}, err
	}
	cred, err := crypto.DecodeBundledClientCredential(wire)
	if err != nil {
		return AttachResult{}, newAttachRefusal(refusalCodeCredentialInvalid, "client credential is invalid", fmt.Sprintf("paste a bundled credential from `loaf auth link` into %s or pass --wire", ClientTokenEnv), err)
	}
	if cred.ProjectID != conf.ProjectID {
		return AttachResult{}, &RefusalError{
			Code:   refusalCodeIdentityMismatch,
			Cause:  fmt.Sprintf("project conf id %q does not match credential project %q", conf.ProjectID, cred.ProjectID),
			Remedy: "mint a project-scoped token with `loaf auth link --project <id> <name>` for this checkout's conf",
		}
	}
	endpoint := strings.TrimSpace(os.Getenv(SyncURLEnv))
	if endpoint == "" {
		endpoint = strings.TrimSpace(cred.Endpoint)
	}
	if endpoint == "" {
		return AttachResult{}, newAttachRefusal(refusalCodeReachFailed, "sync endpoint is empty", fmt.Sprintf("set %s or include endpoint in the bundled credential", SyncURLEnv), errors.New("endpoint is empty"))
	}
	if !in.AllowInsecureHTTP && !strings.HasPrefix(strings.ToLower(endpoint), "https://") {
		return AttachResult{}, &RefusalError{
			Code:   refusalCodeEndpointInsecure,
			Cause:  "sync endpoint must use HTTPS",
			Remedy: fmt.Sprintf("configure a TLS-terminated relay URL in the bundled credential or %s", SyncURLEnv),
		}
	}
	cred.Endpoint = endpoint

	httpClient := in.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	if err := sync.CheckRelayHealth(ctx, endpoint, httpClient); err != nil {
		return AttachResult{}, classifyReachError(err)
	}

	probeStore := in.ProbeStore
	if probeStore == nil {
		return AttachResult{}, errors.New("attach probe store is required")
	}
	maxSkew := in.MaxHLCSkewMS
	if maxSkew <= 0 {
		maxSkew = int64((24 * time.Hour) / time.Millisecond)
	}
	probe, err := sync.ProbeRemote(ctx, sync.ProbeConfig{
		Credential:   cred,
		HTTPClient:   httpClient,
		MaxHLCSkewMS: maxSkew,
		Receive: func(ctx context.Context, envelope state.FactEnvelope) error {
			if _, err := state.ReceiveFact(ctx, probeStore, envelope, state.ReceiveFactOptions{MaxHLCSkewMS: maxSkew}); err != nil {
				return classifyReceiveError(err)
			}
			return nil
		},
	})
	if err != nil {
		return AttachResult{}, classifyProbeError(err)
	}

	name := strings.TrimSpace(in.ConnectionName)
	if name == "" {
		name = connectionNameFromToken(cred.ConnectionToken)
	}
	if err := in.Store.SaveClientWire(wire); err != nil {
		return AttachResult{}, err
	}
	if err := MarkAttached(in.Store, endpoint, name); err != nil {
		return AttachResult{}, err
	}
	return AttachResult{
		ProjectID:      conf.ProjectID,
		ConfID:         conf.ConfID,
		Endpoint:       endpoint,
		ConnectionName: name,
		PulledFacts:    probe.Pulled,
		DecryptedFacts: probe.Decrypted,
	}, nil
}

func resolveClientWire(store Store, override string) (string, error) {
	override = strings.TrimSpace(override)
	if override != "" {
		return override, nil
	}
	if value := strings.TrimSpace(os.Getenv(ClientTokenEnv)); value != "" {
		return value, nil
	}
	if wire, err := store.LoadClientWire(); err == nil {
		return wire, nil
	} else if !errors.Is(err, ErrClientNotConfigured) {
		return "", err
	}
	return "", &RefusalError{
		Code:   refusalCodeCredentialMissing,
		Cause:  "no bundled client credential is configured for this environment",
		Remedy: fmt.Sprintf("set %s to the output of `loaf auth link` on a trusted admin machine", ClientTokenEnv),
	}
}

func connectionNameFromToken(raw string) string {
	raw = strings.TrimSpace(raw)
	if idx := strings.Index(raw, ":"); idx > 0 {
		return raw[:idx]
	}
	return raw
}

func newAttachRefusal(code, cause, remedy string, err error) *RefusalError {
	msg := cause
	if err != nil && err.Error() != "" {
		msg = fmt.Sprintf("%s: %s", cause, err.Error())
	}
	return &RefusalError{Code: code, Cause: msg, Remedy: remedy}
}

func classifyReachError(err error) error {
	if err == nil {
		return nil
	}
	return newAttachRefusal(refusalCodeReachFailed, "cannot reach sync server", fmt.Sprintf("verify relay URL and TLS termination; optionally set %s", SyncURLEnv), err)
}

func classifyProbeError(err error) error {
	if err == nil {
		return nil
	}
	var refusal *RefusalError
	if errors.As(err, &refusal) {
		return refusal
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "unauthorized"):
		return newAttachRefusal(refusalCodeAuthFailed, "connection token authentication failed", "verify the bundled credential matches this project and has not been revoked", err)
	case strings.Contains(msg, "decrypt"):
		return newAttachRefusal(refusalCodeDecryptFailed, "cannot decrypt remote facts with the bundled project key", "re-mint the client credential with `loaf auth link` and update the environment secret", err)
	case strings.Contains(msg, "unsupported kind"), strings.Contains(msg, "unsupported envelope"):
		return newAttachRefusal(refusalCodeCapability, "remote fact uses an unsupported envelope or kind", "upgrade Loaf to a version that understands the remote stream", err)
	default:
		return newAttachRefusal(refusalCodeReachFailed, "attach probe against sync server failed", "verify relay health, token scope, and network reachability", err)
	}
}

func classifyReceiveError(err error) error {
	if err == nil {
		return nil
	}
	var skew *state.HLCSkewError
	if errors.As(err, &skew) {
		return &RefusalError{
			Code:   refusalCodeHLCSkew,
			Cause:  skew.Error(),
			Remedy: "check relay and client clocks; gross HLC skew refuses attach until time is trustworthy",
		}
	}
	if strings.Contains(strings.ToLower(err.Error()), "unsupported envelope") || strings.Contains(strings.ToLower(err.Error()), "unknown kind") {
		return newAttachRefusal(refusalCodeCapability, "remote fact capability check failed", "upgrade Loaf to a version that understands the remote stream", err)
	}
	return err
}


// AttachRefusalJSON renders an attach refusal for --json callers.
func AttachRefusalJSON(err error) ([]byte, error) {
	if err == nil {
		return nil, fmt.Errorf("not an attach refusal")
	}
	var refusal *RefusalError
	if errors.As(err, &refusal) {
		return json.Marshal(refusal)
	}
	return json.Marshal(map[string]string{
		"code":   "attach-failed",
		"cause":  err.Error(),
		"remedy": "inspect attach logs and retry loaf attach",
	})
}
