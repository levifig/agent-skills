// Package credential defines the three structurally distinct private-sync
// credential classes selected by ADR-031.
package credential

import (
	"crypto/rand"
	"errors"
	"net/url"
	"unicode/utf8"

	"github.com/levifig/loaf/vnext/continuity"
	synccrypto "github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
)

const (
	// CredentialVersionV1 is the first fixed-field credential schema.
	CredentialVersionV1 uint16 = 1

	// RecoveryPrefix distinguishes full offline recovery authority.
	RecoveryPrefix = "loafrec1:"
	// TrustedPrefix distinguishes one durable attached environment.
	TrustedPrefix = "loaftrusted1:"
	// EphemeralPrefix distinguishes one expiring non-persisted environment.
	EphemeralPrefix = "loafeph1:"

	maximumCheckpointBytes = 65_536
)

const (
	recoveryKind  = "project-recovery"
	trustedKind   = "trusted-project"
	ephemeralKind = "ephemeral-project"
)

var (
	// ErrCredentialClass identifies a credential prefix/class mismatch.
	ErrCredentialClass = errors.New("sync credential: wrong credential class")
	// ErrCredentialChecksum identifies transport corruption. The checksum is not
	// an authenticity proof.
	ErrCredentialChecksum = errors.New("sync credential: checksum mismatch")
	// ErrUnsupportedCredentialVersion identifies a future credential schema.
	ErrUnsupportedCredentialVersion = errors.New("sync credential: unsupported version")
	// ErrInvalidCredential identifies malformed or noncanonical credential data.
	ErrInvalidCredential = errors.New("sync credential: invalid credential")
	// ErrCredentialExpired identifies expired ephemeral relay or signing access.
	ErrCredentialExpired = errors.New("sync credential: ephemeral authority expired")
	// ErrCredentialRandomSource identifies failure to mint bearer material.
	ErrCredentialRandomSource = errors.New("sync credential: secure random source failed")
	// ErrProtectedCredentialEncoding prevents generic serializers from
	// bypassing the class-specific, checksummed credential codecs.
	ErrProtectedCredentialEncoding = errors.New("sync credential: use the protected class-specific encoder")
)

// RelayTokenID is the public high-entropy lookup identity for a bearer token.
type RelayTokenID [16]byte

// RelayTokenSecret is the independently random bearer proof.
type RelayTokenSecret [32]byte

// RelayBearer carries the exact public lookup ID and secret required by relay
// authorization. Its fields are private to prevent partial construction.
type RelayBearer struct {
	id     RelayTokenID
	secret RelayTokenSecret
}

// NewRelayBearer validates an explicit lookup ID and secret pair.
func NewRelayBearer(id RelayTokenID, secret RelayTokenSecret) (RelayBearer, error) {
	if zeroBytes(id[:]) || zeroBytes(secret[:]) {
		return RelayBearer{}, ErrInvalidCredential
	}
	return RelayBearer{id: id, secret: secret}, nil
}

// GenerateRelayBearer independently generates a lookup ID and bearer secret.
func GenerateRelayBearer() (RelayBearer, error) {
	var id RelayTokenID
	var secret RelayTokenSecret
	if _, err := rand.Read(id[:]); err != nil {
		return RelayBearer{}, ErrCredentialRandomSource
	}
	if _, err := rand.Read(secret[:]); err != nil {
		return RelayBearer{}, ErrCredentialRandomSource
	}
	return RelayBearer{id: id, secret: secret}, nil
}

// ID returns the public relay lookup identity.
func (bearer RelayBearer) ID() RelayTokenID { return bearer.id }

// Secret returns a copy for protected request or credential serialization.
func (bearer RelayBearer) Secret() RelayTokenSecret { return bearer.secret }

// String prevents accidental formatting of bearer authority.
func (RelayBearer) String() string { return "[REDACTED relay bearer]" }

// GoString prevents %#v from formatting bearer authority.
func (RelayBearer) GoString() string { return "credential.RelayBearer([REDACTED])" }

func (bearer RelayBearer) valid() bool {
	return !zeroBytes(bearer.id[:]) && !zeroBytes(bearer.secret[:])
}

// ProjectRecoveryCredential carries full offline recovery/admin authority. It
// must never be installed as an ordinary client credential.
type ProjectRecoveryCredential struct {
	ProjectID               continuity.ProjectID
	RelayURL                string
	RelayGeneration         protocol.RelayGeneration
	ChannelID               protocol.ChannelID
	ProjectRoot             synccrypto.ProjectRoot
	AdminSeed               synccrypto.AdminSeed
	OwnerRelayAuthorization RelayBearer
	WriteGeneration         uint32
	LastSignedCheckpoint    []byte
}

// String prevents accidental formatting of recovery authority.
func (ProjectRecoveryCredential) String() string { return "[REDACTED project recovery credential]" }

// GoString prevents %#v from formatting recovery authority.
func (ProjectRecoveryCredential) GoString() string {
	return "credential.ProjectRecoveryCredential([REDACTED])"
}

// MarshalJSON fails closed so generic serialization cannot expose recovery
// authority. Use EncodeRecovery for the protected transport representation.
func (ProjectRecoveryCredential) MarshalJSON() ([]byte, error) {
	return nil, ErrProtectedCredentialEncoding
}

// Validate checks recovery structure without logging protected values.
func (credential ProjectRecoveryCredential) Validate() error {
	if err := validateCommon(credential.ProjectID, credential.RelayURL, credential.RelayGeneration, credential.ChannelID); err != nil {
		return err
	}
	rootBytes := credential.ProjectRoot.Bytes()
	if _, err := synccrypto.ProjectRootFromBytes(rootBytes[:]); err != nil {
		return ErrInvalidCredential
	}
	adminBytes := credential.AdminSeed.Bytes()
	if _, err := synccrypto.AdminSeedFromBytes(adminBytes[:]); err != nil {
		return ErrInvalidCredential
	}
	if !credential.OwnerRelayAuthorization.valid() || credential.WriteGeneration < 1 || !validCheckpoint(credential.LastSignedCheckpoint) {
		return ErrInvalidCredential
	}
	return nil
}

// TrustedProjectCredential carries one durable environment's authority. It has
// no project-admin seed or relay-owner token.
type TrustedProjectCredential struct {
	ProjectID                     continuity.ProjectID
	RelayURL                      string
	RelayGeneration               protocol.RelayGeneration
	ChannelID                     protocol.ChannelID
	AdminPublicKey                protocol.PublicKey
	Certificate                   protocol.EnvironmentCertificate
	EnvironmentSeed               synccrypto.EnvironmentSeed
	EnvironmentRelayAuthorization RelayBearer
	ProjectRoot                   synccrypto.ProjectRoot
	WriteGeneration               uint32
	MinimumProtocolVersion        uint16
	LastObservedCheckpoint        []byte
}

// String prevents accidental formatting of trusted environment authority.
func (TrustedProjectCredential) String() string { return "[REDACTED trusted project credential]" }

// GoString prevents %#v from formatting trusted environment authority.
func (TrustedProjectCredential) GoString() string {
	return "credential.TrustedProjectCredential([REDACTED])"
}

// MarshalJSON fails closed so generic serialization cannot expose trusted
// environment authority. Use EncodeTrusted for the protected representation.
func (TrustedProjectCredential) MarshalJSON() ([]byte, error) {
	return nil, ErrProtectedCredentialEncoding
}

// Validate checks trusted credential structure and its admin-signed
// certificate.
func (credential TrustedProjectCredential) Validate() error {
	if err := validateCommon(credential.ProjectID, credential.RelayURL, credential.RelayGeneration, credential.ChannelID); err != nil {
		return err
	}
	rootBytes := credential.ProjectRoot.Bytes()
	if _, err := synccrypto.ProjectRootFromBytes(rootBytes[:]); err != nil {
		return ErrInvalidCredential
	}
	if !credential.EnvironmentRelayAuthorization.valid() || credential.WriteGeneration < 1 || credential.MinimumProtocolVersion != protocol.ProtocolVersionV1 || !validCheckpoint(credential.LastObservedCheckpoint) {
		return ErrInvalidCredential
	}
	if credential.Certificate.Mode != protocol.EnvironmentTrusted || credential.Certificate.ExpiresAtMillis != 0 || !credential.Certificate.AllowsGeneration(credential.WriteGeneration) {
		return ErrInvalidCredential
	}
	if err := validateCertificateAuthority(credential.ProjectID, credential.ChannelID, credential.AdminPublicKey, credential.Certificate, credential.EnvironmentSeed); err != nil {
		return err
	}
	return nil
}

// EphemeralProjectCredential carries only explicit finite generation keys and
// one expiring environment identity. It cannot derive a future generation or
// mint/recover project authority.
type EphemeralProjectCredential struct {
	ProjectID                     continuity.ProjectID
	RelayURL                      string
	RelayGeneration               protocol.RelayGeneration
	ChannelID                     protocol.ChannelID
	AdminPublicKey                protocol.PublicKey
	Certificate                   protocol.EnvironmentCertificate
	EnvironmentSeed               synccrypto.EnvironmentSeed
	EnvironmentRelayAuthorization RelayBearer
	RelayTokenExpiresAtMillis     int64
	GenerationKeys                []synccrypto.GenerationKey
}

// String prevents accidental formatting of ephemeral environment authority.
func (EphemeralProjectCredential) String() string { return "[REDACTED ephemeral project credential]" }

// GoString prevents %#v from formatting ephemeral environment authority.
func (EphemeralProjectCredential) GoString() string {
	return "credential.EphemeralProjectCredential([REDACTED])"
}

// MarshalJSON fails closed so generic serialization cannot expose ephemeral
// environment authority. Use EncodeEphemeral for the protected representation.
func (EphemeralProjectCredential) MarshalJSON() ([]byte, error) {
	return nil, ErrProtectedCredentialEncoding
}

// Validate checks ephemeral credential structure, exact finite keys, and its
// admin-signed certificate without applying the current clock.
func (credential EphemeralProjectCredential) Validate() error {
	if err := validateCommon(credential.ProjectID, credential.RelayURL, credential.RelayGeneration, credential.ChannelID); err != nil {
		return err
	}
	if !credential.EnvironmentRelayAuthorization.valid() || credential.RelayTokenExpiresAtMillis < 1 {
		return ErrInvalidCredential
	}
	if credential.Certificate.Mode != protocol.EnvironmentEphemeral || credential.Certificate.ExpiresAtMillis < 1 || credential.RelayTokenExpiresAtMillis > credential.Certificate.ExpiresAtMillis {
		return ErrInvalidCredential
	}
	if err := validateCertificateAuthority(credential.ProjectID, credential.ChannelID, credential.AdminPublicKey, credential.Certificate, credential.EnvironmentSeed); err != nil {
		return err
	}
	if len(credential.GenerationKeys) != len(credential.Certificate.AllowedKeyGenerations) || len(credential.GenerationKeys) < 1 {
		return ErrInvalidCredential
	}
	for index, key := range credential.GenerationKeys {
		if key.ProjectID() != credential.ProjectID || key.CipherSuite() != credential.Certificate.CipherSuite || key.Generation() != credential.Certificate.AllowedKeyGenerations[index] {
			return ErrInvalidCredential
		}
		keyBytes := key.Bytes()
		if _, err := synccrypto.NewGenerationKey(credential.ProjectID, key.Generation(), keyBytes); err != nil {
			return ErrInvalidCredential
		}
	}
	return nil
}

// ValidateAt rejects an ephemeral credential at or after either certificate or
// relay-token expiry. The input is trusted Unix milliseconds.
func (credential EphemeralProjectCredential) ValidateAt(atMillis int64) error {
	if err := credential.Validate(); err != nil {
		return err
	}
	if err := credential.Certificate.ValidateAt(atMillis); err != nil {
		if errors.Is(err, protocol.ErrCertificateExpired) {
			return ErrCredentialExpired
		}
		return ErrInvalidCredential
	}
	if atMillis < 0 {
		return ErrInvalidCredential
	}
	if atMillis >= credential.RelayTokenExpiresAtMillis {
		return ErrCredentialExpired
	}
	return nil
}

func validateCommon(projectID continuity.ProjectID, relayURL string, relayGeneration protocol.RelayGeneration, channelID protocol.ChannelID) error {
	if projectID.Validate() != nil || zeroBytes(relayGeneration[:]) || zeroBytes(channelID[:]) || !validRelayURL(relayURL) {
		return ErrInvalidCredential
	}
	return nil
}

func validateCertificateAuthority(projectID continuity.ProjectID, channelID protocol.ChannelID, adminPublic protocol.PublicKey, certificate protocol.EnvironmentCertificate, environmentSeed synccrypto.EnvironmentSeed) error {
	if certificate.ProjectID != projectID || certificate.ChannelID != channelID || zeroBytes(adminPublic[:]) {
		return ErrInvalidCredential
	}
	environmentSeedBytes := environmentSeed.Bytes()
	if _, err := synccrypto.EnvironmentSeedFromBytes(environmentSeedBytes[:]); err != nil {
		return ErrInvalidCredential
	}
	if err := synccrypto.VerifyEnvironmentCertificate(certificate, adminPublic); err != nil {
		return ErrInvalidCredential
	}
	if synccrypto.EnvironmentPublicKey(environmentSeed) != certificate.EnvironmentPublicKey {
		return ErrInvalidCredential
	}
	return nil
}

func validRelayURL(value string) bool {
	if value == "" || !utf8.ValidString(value) {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return false
	}
	return parsed.String() == value
}

func validCheckpoint(value []byte) bool {
	return len(value) <= maximumCheckpointBytes
}

func zeroBytes(value []byte) bool {
	var combined byte
	for _, item := range value {
		combined |= item
	}
	return combined == 0
}
