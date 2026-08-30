package coordinator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
)

func TestNewRequiresDependenciesAndStoresOnlyThem(t *testing.T) {
	store, err := continuitysqlite.Open(t.TempDir(), testEnvironmentID(250))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	remote := &remoteFixture{endpoint: "https://relay.example.test"}
	coordinator, err := New(store, remote)
	if err != nil {
		t.Fatalf("new coordinator: %v", err)
	}
	if coordinator.store != store || coordinator.remote != remote {
		t.Fatal("coordinator did not retain exactly the supplied dependencies")
	}

	for _, test := range []struct {
		name   string
		store  *continuitysqlite.Store
		remote Remote
	}{
		{name: "nil store", remote: remote},
		{name: "nil remote", store: store},
		{name: "typed nil remote", store: store, remote: (*remoteFixture)(nil)},
		{name: "invalid writer", store: &continuitysqlite.Store{}, remote: remote},
		{name: "invalid endpoint", store: store, remote: &remoteFixture{endpoint: "http://relay.example.test"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := New(test.store, test.remote)
			if got != nil {
				t.Fatal("new coordinator returned an instance for invalid dependencies")
			}
			assertProblem(t, err, CodeInvalid, PhaseConstruction, ActionConfigure)
		})
	}
}

func TestProblemFormattingIsStaticAndSecretFree(t *testing.T) {
	problem := Problem{
		Code:   CodeRemote,
		Phase:  PhaseEnvironmentInventory,
		Action: ActionRetry,
	}
	for _, formatted := range []string{
		problem.Error(),
		problem.String(),
		fmt.Sprintf("%v", problem),
		fmt.Sprintf("%#v", problem),
	} {
		if formatted == "" {
			t.Fatal("problem formatted as an empty string")
		}
		if strings.Contains(formatted, "secret") {
			t.Fatalf("problem format leaked secret marker: %q", formatted)
		}
	}

	hostile := Problem{
		Code:   ProblemCode("secret-code"),
		Phase:  ProblemPhase("secret-phase"),
		Action: ProblemAction("secret-action"),
	}
	for _, formatted := range []string{
		hostile.Error(),
		hostile.String(),
		fmt.Sprintf("%v", hostile),
		fmt.Sprintf("%#v", hostile),
	} {
		if strings.Contains(formatted, "secret") {
			t.Fatalf("unknown problem value leaked through formatting: %q", formatted)
		}
	}
	if errors.Unwrap(problem) != nil {
		t.Fatal("problem unexpectedly unwraps an arbitrary error")
	}
}

func TestNewRequiresCanonicalHTTPSEndpoint(t *testing.T) {
	store, err := continuitysqlite.Open(t.TempDir(), testEnvironmentID(250))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	for _, endpoint := range []string{
		"",
		"http://relay.example.test",
		"https://owner@relay.example.test",
		"https://relay.example.test?token=secret",
		"https://relay.example.test#fragment",
	} {
		remote := &remoteFixture{endpoint: endpoint}
		coordinator, err := New(store, remote)
		if coordinator != nil {
			t.Fatalf("New accepted noncanonical endpoint %q", endpoint)
		}
		assertProblem(t, err, CodeInvalid, PhaseConstruction, ActionConfigure)
		if remote.createCalls != 0 || len(remote.environmentRequests) != 0 || remote.registerCalls != 0 {
			t.Fatalf("invalid endpoint %q reached remote workflow methods", endpoint)
		}
	}
}

func assertProblem(t *testing.T, err error, code ProblemCode, phase ProblemPhase, action ProblemAction) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected a coordinator problem, got context error: %v", err)
	}
	var problem *Problem
	if !errors.As(err, &problem) {
		t.Fatalf("expected *Problem, got %T", err)
	}
	if problem.Code != code || problem.Phase != phase || problem.Action != action {
		t.Fatalf("problem = {%q %q %q}, want {%q %q %q}", problem.Code, problem.Phase, problem.Action, code, phase, action)
	}
}
