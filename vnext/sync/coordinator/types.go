// Package coordinator implements transport-neutral synchronization workflows.
package coordinator

import (
	"context"
	"net/url"
	"reflect"
	"unicode/utf8"

	continuitysqlite "github.com/levifig/loaf/vnext/continuity/sqlite"
	"github.com/levifig/loaf/vnext/sync/relay"
)

// Remote is the complete transport-neutral relay surface used by coordinator
// workflows. Recovery preparation uses only Endpoint, CreateChannel, and
// EnvironmentInventory; later attach phases use the remaining operations.
type Remote interface {
	Endpoint() string
	EnvironmentInventory(context.Context, relay.EnvironmentInventoryRequest) (relay.EnvironmentInventoryPage, error)
	PruneInventory(context.Context, relay.PruneInventoryRequest) (relay.PruneInventoryPage, error)
	Page(context.Context, relay.PageRequest) (relay.Page, error)
	CreateChannel(context.Context, relay.Channel) (relay.ChannelState, error)
	ClassifyEnvironmentRegistration(context.Context, relay.RegisterEnvironmentRequest) (relay.EnvironmentRegistrationStatus, error)
	RegisterEnvironment(context.Context, relay.RegisterEnvironmentRequest) (relay.ChannelState, error)
}

// Coordinator binds local continuity persistence to one transport-neutral
// relay. It retains no credentials or other secret authority.
type Coordinator struct {
	store  *continuitysqlite.Store
	remote Remote
}

// New constructs a coordinator from its two durable dependencies.
func New(store *continuitysqlite.Store, remote Remote) (*Coordinator, error) {
	if store == nil || nilRemote(remote) {
		return nil, newProblem(CodeInvalid, PhaseConstruction, ActionConfigure)
	}
	if store.WriterEnvironmentID().Validate() != nil || !validRemoteEndpoint(remote.Endpoint()) {
		return nil, newProblem(CodeInvalid, PhaseConstruction, ActionConfigure)
	}
	return &Coordinator{store: store, remote: remote}, nil
}

func nilRemote(remote Remote) bool {
	if remote == nil {
		return true
	}
	value := reflect.ValueOf(remote)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func validRemoteEndpoint(value string) bool {
	if value == "" || !utf8.ValidString(value) {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return parsed.String() == value
}
