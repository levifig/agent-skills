package credential

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"strings"

	"github.com/levifig/loaf/vnext/continuity"
	synccrypto "github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
)

const (
	credentialChecksumBytes    = 16
	maximumCredentialBodyBytes = 100_000
	credentialChecksumDomain   = "loaf.sync.credential.checksum.v1"
)

type recoveryWireV1 struct {
	Version              uint16               `json:"version"`
	Kind                 string               `json:"kind"`
	ProjectID            continuity.ProjectID `json:"project_id"`
	RelayURL             string               `json:"relay_url"`
	RelayGeneration      string               `json:"relay_generation"`
	ChannelID            string               `json:"channel_id"`
	ProjectRoot          string               `json:"project_root"`
	AdminSeed            string               `json:"admin_seed"`
	OwnerTokenID         string               `json:"owner_token_id"`
	OwnerTokenSecret     string               `json:"owner_token_secret"`
	WriteGeneration      uint32               `json:"write_generation"`
	LastSignedCheckpoint string               `json:"last_signed_checkpoint"`
}

type trustedWireV1 struct {
	Version                uint16               `json:"version"`
	Kind                   string               `json:"kind"`
	ProjectID              continuity.ProjectID `json:"project_id"`
	RelayURL               string               `json:"relay_url"`
	RelayGeneration        string               `json:"relay_generation"`
	ChannelID              string               `json:"channel_id"`
	AdminPublicKey         string               `json:"admin_public_key"`
	Certificate            string               `json:"certificate"`
	EnvironmentSeed        string               `json:"environment_seed"`
	EnvironmentTokenID     string               `json:"environment_token_id"`
	EnvironmentTokenSecret string               `json:"environment_token_secret"`
	ProjectRoot            string               `json:"project_root"`
	WriteGeneration        uint32               `json:"write_generation"`
	MinimumProtocolVersion uint16               `json:"minimum_protocol_version"`
	LastObservedCheckpoint string               `json:"last_observed_checkpoint"`
}

type ephemeralWireV1 struct {
	Version                   uint16                `json:"version"`
	Kind                      string                `json:"kind"`
	ProjectID                 continuity.ProjectID  `json:"project_id"`
	RelayURL                  string                `json:"relay_url"`
	RelayGeneration           string                `json:"relay_generation"`
	ChannelID                 string                `json:"channel_id"`
	AdminPublicKey            string                `json:"admin_public_key"`
	Certificate               string                `json:"certificate"`
	EnvironmentSeed           string                `json:"environment_seed"`
	EnvironmentTokenID        string                `json:"environment_token_id"`
	EnvironmentTokenSecret    string                `json:"environment_token_secret"`
	RelayTokenExpiresAtMillis int64                 `json:"relay_token_expires_at_millis"`
	GenerationKeys            []generationKeyWireV1 `json:"generation_keys"`
}

type generationKeyWireV1 struct {
	Generation uint32 `json:"generation"`
	Key        string `json:"key"`
}

// EncodeRecovery emits fixed-field canonical JSON plus a corruption checksum.
func EncodeRecovery(credential ProjectRecoveryCredential) (string, error) {
	if err := credential.Validate(); err != nil {
		return "", err
	}
	root := credential.ProjectRoot.Bytes()
	adminSeed := credential.AdminSeed.Bytes()
	ownerID := credential.OwnerRelayAuthorization.ID()
	ownerSecret := credential.OwnerRelayAuthorization.Secret()
	body, err := encodeCanonicalJSON(recoveryWireV1{
		Version:              CredentialVersionV1,
		Kind:                 recoveryKind,
		ProjectID:            credential.ProjectID,
		RelayURL:             credential.RelayURL,
		RelayGeneration:      encodeBytes(credential.RelayGeneration[:]),
		ChannelID:            encodeBytes(credential.ChannelID[:]),
		ProjectRoot:          encodeBytes(root[:]),
		AdminSeed:            encodeBytes(adminSeed[:]),
		OwnerTokenID:         encodeBytes(ownerID[:]),
		OwnerTokenSecret:     encodeBytes(ownerSecret[:]),
		WriteGeneration:      credential.WriteGeneration,
		LastSignedCheckpoint: encodeBytes(credential.LastSignedCheckpoint),
	})
	if err != nil {
		return "", err
	}
	return encodeCredentialFrame(RecoveryPrefix, recoveryKind, body)
}

// DecodeRecovery strictly decodes only a recovery credential.
func DecodeRecovery(encoded string) (ProjectRecoveryCredential, error) {
	body, err := decodeCredentialFrame(encoded, RecoveryPrefix, recoveryKind)
	if err != nil {
		return ProjectRecoveryCredential{}, err
	}
	wire, err := decodeCanonicalJSON[recoveryWireV1](body)
	if err != nil {
		return ProjectRecoveryCredential{}, err
	}
	if wire.Version != CredentialVersionV1 {
		return ProjectRecoveryCredential{}, ErrUnsupportedCredentialVersion
	}
	if wire.Kind != recoveryKind {
		return ProjectRecoveryCredential{}, ErrCredentialClass
	}
	relayGeneration, channel, err := decodeCommonFixed(wire.RelayGeneration, wire.ChannelID)
	if err != nil {
		return ProjectRecoveryCredential{}, err
	}
	rootBytes, err := decodeFixed(wire.ProjectRoot, 32)
	if err != nil {
		return ProjectRecoveryCredential{}, err
	}
	root, err := synccrypto.ProjectRootFromBytes(rootBytes)
	if err != nil {
		return ProjectRecoveryCredential{}, ErrInvalidCredential
	}
	adminBytes, err := decodeFixed(wire.AdminSeed, 32)
	if err != nil {
		return ProjectRecoveryCredential{}, err
	}
	adminSeed, err := synccrypto.AdminSeedFromBytes(adminBytes)
	if err != nil {
		return ProjectRecoveryCredential{}, ErrInvalidCredential
	}
	ownerAuthorization, err := decodeRelayBearer(wire.OwnerTokenID, wire.OwnerTokenSecret)
	if err != nil {
		return ProjectRecoveryCredential{}, err
	}
	checkpoint, err := decodeBounded(wire.LastSignedCheckpoint, maximumCheckpointBytes)
	if err != nil {
		return ProjectRecoveryCredential{}, err
	}
	credential := ProjectRecoveryCredential{
		ProjectID:               wire.ProjectID,
		RelayURL:                wire.RelayURL,
		RelayGeneration:         relayGeneration,
		ChannelID:               channel,
		ProjectRoot:             root,
		AdminSeed:               adminSeed,
		OwnerRelayAuthorization: ownerAuthorization,
		WriteGeneration:         wire.WriteGeneration,
		LastSignedCheckpoint:    checkpoint,
	}
	if err := credential.Validate(); err != nil {
		return ProjectRecoveryCredential{}, err
	}
	return credential, nil
}

// EncodeTrusted emits fixed-field canonical JSON plus a corruption checksum.
func EncodeTrusted(credential TrustedProjectCredential) (string, error) {
	if err := credential.Validate(); err != nil {
		return "", err
	}
	certificate, err := credential.Certificate.MarshalBinary()
	if err != nil {
		return "", ErrInvalidCredential
	}
	environmentSeed := credential.EnvironmentSeed.Bytes()
	root := credential.ProjectRoot.Bytes()
	environmentTokenID := credential.EnvironmentRelayAuthorization.ID()
	environmentTokenSecret := credential.EnvironmentRelayAuthorization.Secret()
	body, err := encodeCanonicalJSON(trustedWireV1{
		Version:                CredentialVersionV1,
		Kind:                   trustedKind,
		ProjectID:              credential.ProjectID,
		RelayURL:               credential.RelayURL,
		RelayGeneration:        encodeBytes(credential.RelayGeneration[:]),
		ChannelID:              encodeBytes(credential.ChannelID[:]),
		AdminPublicKey:         encodeBytes(credential.AdminPublicKey[:]),
		Certificate:            encodeBytes(certificate),
		EnvironmentSeed:        encodeBytes(environmentSeed[:]),
		EnvironmentTokenID:     encodeBytes(environmentTokenID[:]),
		EnvironmentTokenSecret: encodeBytes(environmentTokenSecret[:]),
		ProjectRoot:            encodeBytes(root[:]),
		WriteGeneration:        credential.WriteGeneration,
		MinimumProtocolVersion: credential.MinimumProtocolVersion,
		LastObservedCheckpoint: encodeBytes(credential.LastObservedCheckpoint),
	})
	if err != nil {
		return "", err
	}
	return encodeCredentialFrame(TrustedPrefix, trustedKind, body)
}

// DecodeTrusted strictly decodes only a trusted environment credential.
func DecodeTrusted(encoded string) (TrustedProjectCredential, error) {
	body, err := decodeCredentialFrame(encoded, TrustedPrefix, trustedKind)
	if err != nil {
		return TrustedProjectCredential{}, err
	}
	wire, err := decodeCanonicalJSON[trustedWireV1](body)
	if err != nil {
		return TrustedProjectCredential{}, err
	}
	if wire.Version != CredentialVersionV1 {
		return TrustedProjectCredential{}, ErrUnsupportedCredentialVersion
	}
	if wire.Kind != trustedKind {
		return TrustedProjectCredential{}, ErrCredentialClass
	}
	relayGeneration, channel, err := decodeCommonFixed(wire.RelayGeneration, wire.ChannelID)
	if err != nil {
		return TrustedProjectCredential{}, err
	}
	adminPublic, certificate, environmentSeed, err := decodeEnvironmentAuthority(wire.AdminPublicKey, wire.Certificate, wire.EnvironmentSeed)
	if err != nil {
		return TrustedProjectCredential{}, err
	}
	environmentAuthorization, err := decodeRelayBearer(wire.EnvironmentTokenID, wire.EnvironmentTokenSecret)
	if err != nil {
		return TrustedProjectCredential{}, err
	}
	rootBytes, err := decodeFixed(wire.ProjectRoot, 32)
	if err != nil {
		return TrustedProjectCredential{}, err
	}
	root, err := synccrypto.ProjectRootFromBytes(rootBytes)
	if err != nil {
		return TrustedProjectCredential{}, ErrInvalidCredential
	}
	checkpoint, err := decodeBounded(wire.LastObservedCheckpoint, maximumCheckpointBytes)
	if err != nil {
		return TrustedProjectCredential{}, err
	}
	credential := TrustedProjectCredential{
		ProjectID:                     wire.ProjectID,
		RelayURL:                      wire.RelayURL,
		RelayGeneration:               relayGeneration,
		ChannelID:                     channel,
		AdminPublicKey:                adminPublic,
		Certificate:                   certificate,
		EnvironmentSeed:               environmentSeed,
		EnvironmentRelayAuthorization: environmentAuthorization,
		ProjectRoot:                   root,
		WriteGeneration:               wire.WriteGeneration,
		MinimumProtocolVersion:        wire.MinimumProtocolVersion,
		LastObservedCheckpoint:        checkpoint,
	}
	if err := credential.Validate(); err != nil {
		return TrustedProjectCredential{}, err
	}
	return credential, nil
}

// EncodeEphemeral emits a schema containing explicit generation keys and no
// root, admin-private, or relay-owner field.
func EncodeEphemeral(credential EphemeralProjectCredential) (string, error) {
	if err := credential.Validate(); err != nil {
		return "", err
	}
	certificate, err := credential.Certificate.MarshalBinary()
	if err != nil {
		return "", ErrInvalidCredential
	}
	environmentSeed := credential.EnvironmentSeed.Bytes()
	environmentTokenID := credential.EnvironmentRelayAuthorization.ID()
	environmentTokenSecret := credential.EnvironmentRelayAuthorization.Secret()
	keys := make([]generationKeyWireV1, len(credential.GenerationKeys))
	for index, key := range credential.GenerationKeys {
		material := key.Bytes()
		keys[index] = generationKeyWireV1{Generation: key.Generation(), Key: encodeBytes(material[:])}
	}
	body, err := encodeCanonicalJSON(ephemeralWireV1{
		Version:                   CredentialVersionV1,
		Kind:                      ephemeralKind,
		ProjectID:                 credential.ProjectID,
		RelayURL:                  credential.RelayURL,
		RelayGeneration:           encodeBytes(credential.RelayGeneration[:]),
		ChannelID:                 encodeBytes(credential.ChannelID[:]),
		AdminPublicKey:            encodeBytes(credential.AdminPublicKey[:]),
		Certificate:               encodeBytes(certificate),
		EnvironmentSeed:           encodeBytes(environmentSeed[:]),
		EnvironmentTokenID:        encodeBytes(environmentTokenID[:]),
		EnvironmentTokenSecret:    encodeBytes(environmentTokenSecret[:]),
		RelayTokenExpiresAtMillis: credential.RelayTokenExpiresAtMillis,
		GenerationKeys:            keys,
	})
	if err != nil {
		return "", err
	}
	return encodeCredentialFrame(EphemeralPrefix, ephemeralKind, body)
}

// DecodeEphemeral strictly decodes only an ephemeral environment credential.
func DecodeEphemeral(encoded string) (EphemeralProjectCredential, error) {
	body, err := decodeCredentialFrame(encoded, EphemeralPrefix, ephemeralKind)
	if err != nil {
		return EphemeralProjectCredential{}, err
	}
	wire, err := decodeCanonicalJSON[ephemeralWireV1](body)
	if err != nil {
		return EphemeralProjectCredential{}, err
	}
	if wire.Version != CredentialVersionV1 {
		return EphemeralProjectCredential{}, ErrUnsupportedCredentialVersion
	}
	if wire.Kind != ephemeralKind {
		return EphemeralProjectCredential{}, ErrCredentialClass
	}
	relayGeneration, channel, err := decodeCommonFixed(wire.RelayGeneration, wire.ChannelID)
	if err != nil {
		return EphemeralProjectCredential{}, err
	}
	adminPublic, certificate, environmentSeed, err := decodeEnvironmentAuthority(wire.AdminPublicKey, wire.Certificate, wire.EnvironmentSeed)
	if err != nil {
		return EphemeralProjectCredential{}, err
	}
	environmentAuthorization, err := decodeRelayBearer(wire.EnvironmentTokenID, wire.EnvironmentTokenSecret)
	if err != nil {
		return EphemeralProjectCredential{}, err
	}
	if len(wire.GenerationKeys) < 1 || len(wire.GenerationKeys) > protocol.MaxAllowedKeyGenerations {
		return EphemeralProjectCredential{}, ErrInvalidCredential
	}
	keys := make([]synccrypto.GenerationKey, len(wire.GenerationKeys))
	for index, encodedKey := range wire.GenerationKeys {
		materialBytes, err := decodeFixed(encodedKey.Key, 32)
		if err != nil {
			return EphemeralProjectCredential{}, err
		}
		var material [32]byte
		copy(material[:], materialBytes)
		key, err := synccrypto.NewGenerationKey(wire.ProjectID, encodedKey.Generation, material)
		if err != nil {
			return EphemeralProjectCredential{}, ErrInvalidCredential
		}
		keys[index] = key
	}
	credential := EphemeralProjectCredential{
		ProjectID:                     wire.ProjectID,
		RelayURL:                      wire.RelayURL,
		RelayGeneration:               relayGeneration,
		ChannelID:                     channel,
		AdminPublicKey:                adminPublic,
		Certificate:                   certificate,
		EnvironmentSeed:               environmentSeed,
		EnvironmentRelayAuthorization: environmentAuthorization,
		RelayTokenExpiresAtMillis:     wire.RelayTokenExpiresAtMillis,
		GenerationKeys:                keys,
	}
	if err := credential.Validate(); err != nil {
		return EphemeralProjectCredential{}, err
	}
	return credential, nil
}

func encodeCanonicalJSON[T any](value T) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, ErrInvalidCredential
	}
	encoded := buffer.Bytes()
	if len(encoded) < 2 || encoded[len(encoded)-1] != '\n' {
		return nil, ErrInvalidCredential
	}
	encoded = encoded[:len(encoded)-1]
	if len(encoded) > maximumCredentialBodyBytes {
		return nil, ErrInvalidCredential
	}
	return append([]byte(nil), encoded...), nil
}

func decodeCanonicalJSON[T any](encoded []byte) (T, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, ErrInvalidCredential
	}
	var trailing struct{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		return value, ErrInvalidCredential
	}
	canonical, err := encodeCanonicalJSON(value)
	if err != nil || !bytes.Equal(canonical, encoded) {
		return value, ErrInvalidCredential
	}
	return value, nil
}

func encodeCredentialFrame(prefix, kind string, body []byte) (string, error) {
	if len(body) < 2 || len(body) > maximumCredentialBodyBytes {
		return "", ErrInvalidCredential
	}
	checksum := credentialChecksum(kind, body)
	framed := make([]byte, 0, len(body)+len(checksum))
	framed = append(framed, body...)
	framed = append(framed, checksum[:]...)
	return prefix + base64.RawURLEncoding.EncodeToString(framed), nil
}

func decodeCredentialFrame(encoded, prefix, kind string) ([]byte, error) {
	if !strings.HasPrefix(encoded, prefix) {
		return nil, ErrCredentialClass
	}
	payload := strings.TrimPrefix(encoded, prefix)
	if payload == "" || len(payload) > base64.RawURLEncoding.EncodedLen(maximumCredentialBodyBytes+credentialChecksumBytes) {
		return nil, ErrInvalidCredential
	}
	framed, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || base64.RawURLEncoding.EncodeToString(framed) != payload || len(framed) <= credentialChecksumBytes {
		return nil, ErrInvalidCredential
	}
	body := framed[:len(framed)-credentialChecksumBytes]
	provided := framed[len(framed)-credentialChecksumBytes:]
	expected := credentialChecksum(kind, body)
	if subtle.ConstantTimeCompare(provided, expected[:]) != 1 {
		return nil, ErrCredentialChecksum
	}
	return append([]byte(nil), body...), nil
}

func credentialChecksum(kind string, body []byte) [credentialChecksumBytes]byte {
	transcript := make([]byte, 0, len(credentialChecksumDomain)+len(kind)+len(body)+20)
	transcript = binary.BigEndian.AppendUint32(transcript, uint32(len(credentialChecksumDomain)))
	transcript = append(transcript, credentialChecksumDomain...)
	transcript = binary.BigEndian.AppendUint32(transcript, 2)
	transcript = binary.BigEndian.AppendUint32(transcript, uint32(len(kind)))
	transcript = append(transcript, kind...)
	transcript = binary.BigEndian.AppendUint32(transcript, uint32(len(body)))
	transcript = append(transcript, body...)
	full := sha256.Sum256(transcript)
	var checksum [credentialChecksumBytes]byte
	copy(checksum[:], full[:credentialChecksumBytes])
	return checksum
}

func decodeCommonFixed(encodedRelayGeneration, encodedChannel string) (protocol.RelayGeneration, protocol.ChannelID, error) {
	relayBytes, err := decodeFixed(encodedRelayGeneration, 32)
	if err != nil {
		return protocol.RelayGeneration{}, protocol.ChannelID{}, err
	}
	channelBytes, err := decodeFixed(encodedChannel, 32)
	if err != nil {
		return protocol.RelayGeneration{}, protocol.ChannelID{}, err
	}
	var relayGeneration protocol.RelayGeneration
	var channel protocol.ChannelID
	copy(relayGeneration[:], relayBytes)
	copy(channel[:], channelBytes)
	return relayGeneration, channel, nil
}

func decodeRelayBearer(encodedID, encodedSecret string) (RelayBearer, error) {
	idBytes, err := decodeFixed(encodedID, len(RelayTokenID{}))
	if err != nil {
		return RelayBearer{}, err
	}
	secretBytes, err := decodeFixed(encodedSecret, len(RelayTokenSecret{}))
	if err != nil {
		return RelayBearer{}, err
	}
	var id RelayTokenID
	var secret RelayTokenSecret
	copy(id[:], idBytes)
	copy(secret[:], secretBytes)
	bearer, err := NewRelayBearer(id, secret)
	if err != nil {
		return RelayBearer{}, ErrInvalidCredential
	}
	return bearer, nil
}

func decodeEnvironmentAuthority(encodedAdminPublic, encodedCertificate, encodedEnvironmentSeed string) (protocol.PublicKey, protocol.EnvironmentCertificate, synccrypto.EnvironmentSeed, error) {
	adminBytes, err := decodeFixed(encodedAdminPublic, 32)
	if err != nil {
		return protocol.PublicKey{}, protocol.EnvironmentCertificate{}, synccrypto.EnvironmentSeed{}, err
	}
	var adminPublic protocol.PublicKey
	copy(adminPublic[:], adminBytes)
	certificateBytes, err := decodeBounded(encodedCertificate, protocol.MaxCertificateBytes)
	if err != nil {
		return protocol.PublicKey{}, protocol.EnvironmentCertificate{}, synccrypto.EnvironmentSeed{}, err
	}
	certificate, err := protocol.ParseEnvironmentCertificate(certificateBytes)
	if err != nil {
		return protocol.PublicKey{}, protocol.EnvironmentCertificate{}, synccrypto.EnvironmentSeed{}, ErrInvalidCredential
	}
	seedBytes, err := decodeFixed(encodedEnvironmentSeed, 32)
	if err != nil {
		return protocol.PublicKey{}, protocol.EnvironmentCertificate{}, synccrypto.EnvironmentSeed{}, err
	}
	seed, err := synccrypto.EnvironmentSeedFromBytes(seedBytes)
	if err != nil {
		return protocol.PublicKey{}, protocol.EnvironmentCertificate{}, synccrypto.EnvironmentSeed{}, ErrInvalidCredential
	}
	return adminPublic, certificate, seed, nil
}

func encodeBytes(value []byte) string {
	return base64.RawURLEncoding.EncodeToString(value)
}

func decodeFixed(encoded string, size int) ([]byte, error) {
	decoded, err := decodeBounded(encoded, size)
	if err != nil || len(decoded) != size {
		return nil, ErrInvalidCredential
	}
	return decoded, nil
}

func decodeBounded(encoded string, maximum int) ([]byte, error) {
	if len(encoded) > base64.RawURLEncoding.EncodedLen(maximum) {
		return nil, ErrInvalidCredential
	}
	decoded, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || len(decoded) > maximum || base64.RawURLEncoding.EncodeToString(decoded) != encoded {
		return nil, ErrInvalidCredential
	}
	return decoded, nil
}
