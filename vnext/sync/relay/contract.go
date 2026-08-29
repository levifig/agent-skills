// Package relay defines the transport-neutral persistence boundary for the
// vNext opaque sync relay. Values in this package are authenticated routing
// metadata or opaque bytes; continuity fact semantics never cross this API.
package relay

import (
	"context"
	"errors"
	"time"
)

const (
	ProtocolVersionV1             uint16 = 1
	CipherSuiteV1                 uint16 = 1
	MinimumCiphertextBytes               = 16
	MaxCiphertextBytes                   = 1_100_016
	MaxEnvelopeBytes                     = 1_102_000
	MaxCertificateBytes                  = 8_192
	MaxAcknowledgementBytes              = 4_096
	MaxRetirementBytes                   = 4_096
	MaxControlObjectBytes                = 1_048_576
	MaxPageSize                          = 256
	MaxPruneTargets                      = 4_096
	MaxPruneAuthorityEnvironments        = 256
	MaxEnvironmentInventoryPage          = 4
	MaxPruneInventoryPage                = 4
)

var (
	ErrInvalidArgument         = errors.New("relay: invalid argument")
	ErrUnauthenticated         = errors.New("relay: unauthenticated")
	ErrNotFound                = errors.New("relay: not found")
	ErrImmutableConflict       = errors.New("relay: immutable conflict")
	ErrSourceGap               = errors.New("relay: source sequence gap")
	ErrPreviousDigest          = errors.New("relay: previous envelope digest mismatch")
	ErrNonceReuse              = errors.New("relay: nonce reuse")
	ErrExpired                 = errors.New("relay: environment expired")
	ErrRetired                 = errors.New("relay: environment retired")
	ErrMembershipChanged       = errors.New("relay: membership generation changed")
	ErrAcknowledgementRequired = errors.New("relay: acknowledgement required")
	ErrRollback                = errors.New("relay: rollback refused")
	ErrGenerationMismatch      = errors.New("relay: relay generation mismatch")
	ErrUnverified              = errors.New("relay: cryptographic verification failed")
	ErrClosed                  = errors.New("relay: store closed")
)

type ChannelID [32]byte
type RelayGeneration [32]byte
type Digest [32]byte
type PublicKey [32]byte
type Nonce [24]byte
type Signature [64]byte
type RelayTokenID [16]byte
type RelayTokenSecret [32]byte

type FactID string
type EnvironmentID string

type EnvironmentMode string

const (
	TrustedEnvironment   EnvironmentMode = "trusted"
	EphemeralEnvironment EnvironmentMode = "ephemeral"
)

type TokenRegistration struct {
	TokenID   RelayTokenID
	TokenHash TokenHash
}

type Channel struct {
	ChannelID       ChannelID
	RelayGeneration RelayGeneration
	AdminPublicKey  PublicKey
	OwnerToken      TokenRegistration
}

type ChannelState struct {
	ChannelID            ChannelID
	RelayGeneration      RelayGeneration
	MembershipGeneration uint32
	Head                 int64
	CreatedAt            time.Time
}

type OwnerAuthorization struct {
	ChannelID       ChannelID
	RelayGeneration RelayGeneration
	TokenID         RelayTokenID
	TokenSecret     RelayTokenSecret
}

type EnvironmentAuthorization struct {
	ChannelID       ChannelID
	RelayGeneration RelayGeneration
	EnvironmentID   EnvironmentID
	CertificateID   Digest
	TokenID         RelayTokenID
	TokenSecret     RelayTokenSecret
}

type Environment struct {
	ChannelID                 ChannelID
	EnvironmentID             EnvironmentID
	Token                     TokenRegistration
	CertificateID             Digest
	CertificateBytes          []byte
	Mode                      EnvironmentMode
	ExpiresAtMillis           int64
	RelayTokenExpiresAtMillis int64
	MembershipGeneration      uint32
}

type RegisterEnvironmentRequest struct {
	Authorization OwnerAuthorization
	Environment   Environment
}

type RetireEnvironmentRequest struct {
	Authorization OwnerAuthorization
	Retirement    Retirement
}

type Retirement struct {
	ChannelID                ChannelID
	RelayGeneration          RelayGeneration
	EnvironmentID            EnvironmentID
	CertificateID            Digest
	MembershipGeneration     uint32
	FinalEnvironmentSequence int64
	FinalEnvelopeDigest      Digest
	RetirementID             Digest
	RetirementBytes          []byte
}

type ChannelAuthority struct {
	ChannelID       ChannelID
	RelayGeneration RelayGeneration
	AdminPublicKey  PublicKey
}

type EnvironmentAuthority struct {
	ChannelAuthority
	EnvironmentID             EnvironmentID
	CertificateID             Digest
	CertificateBytes          []byte
	Mode                      EnvironmentMode
	ExpiresAtMillis           int64
	RelayTokenExpiresAtMillis int64
	MembershipGeneration      uint32
}

type Envelope struct {
	ProtocolVersion        uint16
	CipherSuite            uint16
	ChannelID              ChannelID
	FactID                 FactID
	EnvironmentID          EnvironmentID
	EnvironmentSequence    int64
	KeyGeneration          uint32
	PreviousEnvelopeDigest Digest
	CertificateID          Digest
	Nonce                  Nonce
	Ciphertext             []byte
	Signature              Signature
	EnvelopeDigest         Digest
}

type AppendRequest struct {
	Authorization EnvironmentAuthorization
	Envelope      Envelope
}

type AppendDisposition uint8

const (
	AppendAccepted AppendDisposition = iota + 1
	AppendDuplicate
)

type AppendResult struct {
	Disposition AppendDisposition
	Arrival     Arrival
	RelayHead   int64
}

type Arrival struct {
	Envelope
	ArrivalSequence int64
	CiphertextSize  int64
	ArrivedAt       time.Time
	PruneID         *Digest
	PrunedAt        *time.Time
}

type PageRequest struct {
	Authorization EnvironmentAuthorization
	After         int64
	Limit         int
}

type Page struct {
	RelayGeneration      RelayGeneration
	MembershipGeneration uint32
	Head                 int64
	Arrivals             []Arrival
}

type Acknowledgement struct {
	ChannelID              ChannelID
	EnvironmentID          EnvironmentID
	MembershipGeneration   uint32
	AppliedArrivalSequence int64
	ProducerSequence       int64
	ProducerEnvelopeDigest Digest
	CertificateID          Digest
	AcknowledgementDigest  Digest
	AcknowledgementBytes   []byte
}

type AcknowledgeRequest struct {
	Authorization   EnvironmentAuthorization
	Acknowledgement Acknowledgement
}

type PruneTarget struct {
	FactID              FactID
	EnvironmentID       EnvironmentID
	EnvironmentSequence int64
	ArrivalSequence     int64
	EnvelopeDigest      Digest
	CertificateID       Digest
}

type TombstoneRequest struct {
	Authorization OwnerAuthorization
	Certificate   PruneCertificate
}

type PruneCertificate struct {
	ChannelID            ChannelID
	PruneID              Digest
	MembershipGeneration uint32
	Barrier              int64
	Closure              PruneTarget
	CertificateID        Digest
	CertificateBytes     []byte
	Targets              []PruneTarget
}

// PruneAuthority is the bounded, relay-visible control-plane state against
// which a prune certificate must be verified. Environments and
// Acknowledgements are ordered by environment ID and have exactly matching
// indexes. Bearer lookup IDs and token hashes never cross this boundary.
type PruneAuthority struct {
	Channel          ChannelAuthority
	Environments     []EnvironmentAuthority
	Acknowledgements []Acknowledgement
}

type TombstoneResult struct {
	Duplicate  bool
	Tombstoned int
	RelayHead  int64
}

// InventoryAuthorization is a closed authorization union. Exactly one bearer
// must be present. Owner authorization permits bootstrap and recovery before an
// environment credential is available; active environment authorization is for
// ordinary attach and reconciliation.
type InventoryAuthorization struct {
	Owner       *OwnerAuthorization
	Environment *EnvironmentAuthorization
}

type EnvironmentInventorySnapshot struct {
	MembershipGeneration uint32
	ArrivalHead          int64
}

type EnvironmentInventoryRequest struct {
	Authorization      InventoryAuthorization
	AfterEnvironmentID EnvironmentID
	Snapshot           *EnvironmentInventorySnapshot
	Limit              int
}

type EnvironmentRetirement struct {
	RetiredAt                time.Time
	RelayGeneration          RelayGeneration
	CertificateID            Digest
	MembershipGeneration     uint32
	FinalEnvironmentSequence int64
	FinalEnvelopeDigest      Digest
	RetirementID             Digest
	RetirementBytes          []byte
}

// EnvironmentInventoryRecord intentionally excludes bearer lookup IDs and
// secret hashes. It contains only the opaque, signed verification material a
// joining client needs to authenticate foreign envelopes and retirements.
type EnvironmentInventoryRecord struct {
	EnvironmentID        EnvironmentID
	CertificateID        Digest
	CertificateBytes     []byte
	Mode                 EnvironmentMode
	ExpiresAtMillis      int64
	MembershipGeneration uint32
	ProducerHead         int64
	Retirement           *EnvironmentRetirement
}

type EnvironmentInventoryPage struct {
	Channel      ChannelAuthority
	Snapshot     EnvironmentInventorySnapshot
	Environments []EnvironmentInventoryRecord
	More         bool
}

type PruneInventorySnapshot struct {
	MembershipGeneration uint32
	ArrivalHead          int64
	PruneHead            int64
}

type PruneInventoryRequest struct {
	Authorization InventoryAuthorization
	After         int64
	Snapshot      *PruneInventorySnapshot
	Limit         int
}

type PruneInventoryRecord struct {
	PruneSequence int64
	Certificate   PruneCertificate
	CreatedAt     time.Time
}

type PruneInventoryPage struct {
	Channel  ChannelAuthority
	Snapshot PruneInventorySnapshot
	Prunes   []PruneInventoryRecord
	More     bool
}

type ChannelStore interface {
	CreateChannel(context.Context, Channel) (ChannelState, error)
	RegisterEnvironment(context.Context, RegisterEnvironmentRequest) (ChannelState, error)
	RetireEnvironment(context.Context, RetireEnvironmentRequest) (ChannelState, error)
}

type SyncStore interface {
	Append(context.Context, AppendRequest) (AppendResult, error)
	Page(context.Context, PageRequest) (Page, error)
	Acknowledge(context.Context, AcknowledgeRequest) error
}

type PruneStore interface {
	Tombstone(context.Context, TombstoneRequest) (TombstoneResult, error)
}

type InventoryStore interface {
	EnvironmentInventory(context.Context, EnvironmentInventoryRequest) (EnvironmentInventoryPage, error)
	PruneInventory(context.Context, PruneInventoryRequest) (PruneInventoryPage, error)
}

type Store interface {
	ChannelStore
	SyncStore
	PruneStore
	InventoryStore
	RelayGeneration() RelayGeneration
	Close() error
}

// Verifier is the mandatory cryptographic boundary in front of persistence.
// Implementations belong to the protocol/service layer. The SQLite adapter
// calls these methods only after token authentication and structural checks,
// and maps a refusal to ErrUnverified without persisting the candidate bytes.
type Verifier interface {
	VerifyEnvironmentCertificate(context.Context, ChannelAuthority, Environment) error
	VerifyEnvelope(context.Context, EnvironmentAuthority, Envelope) error
	VerifyAcknowledgement(context.Context, EnvironmentAuthority, Acknowledgement) error
	VerifyRetirement(context.Context, ChannelAuthority, Retirement) error
	VerifyPruneCertificate(context.Context, PruneAuthority, PruneCertificate) error
}
