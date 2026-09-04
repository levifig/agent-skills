package coordinator

import (
	"context"
	"errors"
)

// ProblemCode is a static coordinator failure category.
type ProblemCode string

const (
	CodeInvalid       ProblemCode = "invalid"
	CodeConflict      ProblemCode = "conflict"
	CodeAuthorization ProblemCode = "authorization"
	CodeRemote        ProblemCode = "remote"
	CodeUnavailable   ProblemCode = "unavailable"
	CodeInternal      ProblemCode = "internal"
)

// ProblemPhase identifies a fixed coordinator workflow boundary.
type ProblemPhase string

const (
	PhaseConstruction            ProblemPhase = "construction"
	PhaseRecoveryValidation      ProblemPhase = "recovery_validation"
	PhaseChannelCreation         ProblemPhase = "channel_creation"
	PhaseChannelAuthorization    ProblemPhase = "channel_authorization"
	PhaseEnvironmentInventory    ProblemPhase = "environment_inventory"
	PhaseCredentialGeneration    ProblemPhase = "credential_generation"
	PhaseEnvironmentRegistration ProblemPhase = "environment_registration"
	PhaseAttachDownload          ProblemPhase = "attach_download"
	PhaseAttachActivation        ProblemPhase = "attach_activation"
	PhasePruneInventory          ProblemPhase = "prune_inventory"
)

// ProblemAction is a static recovery action. It never carries user or remote
// values.
type ProblemAction string

const (
	ActionConfigure              ProblemAction = "configure"
	ActionCorrectInput           ProblemAction = "correct_input"
	ActionRepairLocalStore       ProblemAction = "repair_local_store"
	ActionRestartRecovery        ProblemAction = "restart_recovery"
	ActionAuthorizeEmptyChannel  ProblemAction = "authorize_empty_channel"
	ActionCheckRecoveryAuthority ProblemAction = "check_recovery_authority"
	ActionRetry                  ProblemAction = "retry"
	ActionUseExistingCredential  ProblemAction = "use_existing_credential"
	ActionProtectCredential      ProblemAction = "protect_credential"
	ActionRegisterEnvironment    ProblemAction = "register_environment"
	ActionDownload               ProblemAction = "download"
	ActionActivate               ProblemAction = "activate"
)

// Problem is a secret-free coordinator failure. Its fields are deliberately
// closed vocabularies and its formatting sanitizes unknown values.
type Problem struct {
	Code   ProblemCode
	Phase  ProblemPhase
	Action ProblemAction
}

// Error implements error without exposing an underlying cause.
func (problem Problem) Error() string {
	return "sync coordinator: " + safeCode(problem.Code) + "; phase=" + safePhase(problem.Phase) + "; action=" + safeAction(problem.Action)
}

// String implements fmt.Stringer with the same static representation as Error.
func (problem Problem) String() string { return problem.Error() }

// GoString prevents %#v from expanding fields that a caller constructed with
// values outside the closed vocabulary.
func (problem Problem) GoString() string {
	return "coordinator.Problem{Code:" + safeCode(problem.Code) + ", Phase:" + safePhase(problem.Phase) + ", Action:" + safeAction(problem.Action) + "}"
}

func newProblem(code ProblemCode, phase ProblemPhase, action ProblemAction) error {
	return &Problem{Code: code, Phase: phase, Action: action}
}

func safeCode(code ProblemCode) string {
	switch code {
	case CodeInvalid, CodeConflict, CodeAuthorization, CodeRemote, CodeUnavailable, CodeInternal:
		return string(code)
	default:
		return "unknown"
	}
}

func safePhase(phase ProblemPhase) string {
	switch phase {
	case PhaseConstruction, PhaseRecoveryValidation, PhaseChannelCreation, PhaseChannelAuthorization,
		PhaseEnvironmentInventory, PhaseCredentialGeneration, PhaseEnvironmentRegistration,
		PhaseAttachDownload, PhaseAttachActivation, PhasePruneInventory:
		return string(phase)
	default:
		return "unknown"
	}
}

func safeAction(action ProblemAction) string {
	switch action {
	case ActionConfigure, ActionCorrectInput, ActionRepairLocalStore, ActionRestartRecovery,
		ActionAuthorizeEmptyChannel, ActionCheckRecoveryAuthority, ActionRetry,
		ActionUseExistingCredential, ActionProtectCredential, ActionRegisterEnvironment,
		ActionDownload, ActionActivate:
		return string(action)
	default:
		return "unknown"
	}
}

func contextError(ctx context.Context, err error) error {
	if ctx != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return context.DeadlineExceeded
	}
	return nil
}
