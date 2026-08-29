package credential

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	synccrypto "github.com/levifig/loaf/vnext/sync/crypto"
	"github.com/levifig/loaf/vnext/sync/protocol"
)

func TestRecoveryCredentialClassesCanonicalRoundTrip(t *testing.T) {
	t.Parallel()

	recovery, trusted, ephemeral := testCredentials(t)
	tests := []struct {
		name   string
		prefix string
		encode func() (string, error)
		decode func(string) (string, error)
	}{
		{
			name:   "recovery",
			prefix: RecoveryPrefix,
			encode: func() (string, error) { return EncodeRecovery(recovery) },
			decode: func(encoded string) (string, error) {
				decoded, err := DecodeRecovery(encoded)
				if err != nil {
					return "", err
				}
				return EncodeRecovery(decoded)
			},
		},
		{
			name:   "trusted",
			prefix: TrustedPrefix,
			encode: func() (string, error) { return EncodeTrusted(trusted) },
			decode: func(encoded string) (string, error) {
				decoded, err := DecodeTrusted(encoded)
				if err != nil {
					return "", err
				}
				return EncodeTrusted(decoded)
			},
		},
		{
			name:   "ephemeral",
			prefix: EphemeralPrefix,
			encode: func() (string, error) { return EncodeEphemeral(ephemeral) },
			decode: func(encoded string) (string, error) {
				decoded, err := DecodeEphemeral(encoded)
				if err != nil {
					return "", err
				}
				return EncodeEphemeral(decoded)
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := test.encode()
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if !strings.HasPrefix(encoded, test.prefix) {
				t.Fatalf("prefix = %q, want %q", encoded[:len(test.prefix)], test.prefix)
			}
			reencoded, err := test.decode(encoded)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if reencoded != encoded {
				t.Fatalf("re-encoding changed canonical wire")
			}
		})
	}
}

func TestCredentialClassesCannotBeCrossDecoded(t *testing.T) {
	t.Parallel()

	recovery, trusted, ephemeral := testCredentials(t)
	recoveryWire, err := EncodeRecovery(recovery)
	if err != nil {
		t.Fatalf("encode recovery: %v", err)
	}
	trustedWire, err := EncodeTrusted(trusted)
	if err != nil {
		t.Fatalf("encode trusted: %v", err)
	}
	ephemeralWire, err := EncodeEphemeral(ephemeral)
	if err != nil {
		t.Fatalf("encode ephemeral: %v", err)
	}

	checks := []func() error{
		func() error { _, err := DecodeTrusted(recoveryWire); return err },
		func() error { _, err := DecodeEphemeral(recoveryWire); return err },
		func() error { _, err := DecodeRecovery(trustedWire); return err },
		func() error { _, err := DecodeEphemeral(trustedWire); return err },
		func() error { _, err := DecodeRecovery(ephemeralWire); return err },
		func() error { _, err := DecodeTrusted(ephemeralWire); return err },
	}
	for index, check := range checks {
		if err := check(); !errors.Is(err, ErrCredentialClass) {
			t.Fatalf("cross decode %d error = %v, want %v", index, err, ErrCredentialClass)
		}
	}
}

func TestEphemeralCredentialHasNoRootAdminOrOwnerAuthorityField(t *testing.T) {
	t.Parallel()

	_, _, ephemeral := testCredentials(t)
	encoded, err := EncodeEphemeral(ephemeral)
	if err != nil {
		t.Fatalf("encode ephemeral: %v", err)
	}
	body, err := decodeCredentialFrame(encoded, EphemeralPrefix, ephemeralKind)
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	for _, forbidden := range [][]byte{[]byte("project_root"), []byte("admin_seed"), []byte("owner_token_id"), []byte("owner_token_secret")} {
		if bytes.Contains(body, forbidden) {
			t.Fatalf("ephemeral body contains forbidden authority field %q", forbidden)
		}
	}

	injected := append([]byte(`{"project_root":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",`), body[1:]...)
	injectedWire, err := encodeCredentialFrame(EphemeralPrefix, ephemeralKind, injected)
	if err != nil {
		t.Fatalf("encode injected frame: %v", err)
	}
	if _, err := DecodeEphemeral(injectedWire); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("injected authority error = %v, want %v", err, ErrInvalidCredential)
	}
}

func TestCredentialDecodeRejectsChecksumUnknownAndNoncanonicalJSON(t *testing.T) {
	t.Parallel()

	recovery, _, _ := testCredentials(t)
	encoded, err := EncodeRecovery(recovery)
	if err != nil {
		t.Fatalf("encode recovery: %v", err)
	}

	corrupt := encoded[:len(encoded)-1] + differentBase64Character(encoded[len(encoded)-1])
	if _, err := DecodeRecovery(corrupt); !errors.Is(err, ErrCredentialChecksum) {
		t.Fatalf("checksum error = %v, want %v", err, ErrCredentialChecksum)
	}

	body, err := decodeCredentialFrame(encoded, RecoveryPrefix, recoveryKind)
	if err != nil {
		t.Fatalf("decode frame: %v", err)
	}
	unknown := append([]byte(`{"unknown":true,`), body[1:]...)
	unknownWire, err := encodeCredentialFrame(RecoveryPrefix, recoveryKind, unknown)
	if err != nil {
		t.Fatalf("encode unknown frame: %v", err)
	}
	if _, err := DecodeRecovery(unknownWire); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("unknown-field error = %v, want %v", err, ErrInvalidCredential)
	}

	noncanonical := append([]byte(" "), body...)
	noncanonicalWire, err := encodeCredentialFrame(RecoveryPrefix, recoveryKind, noncanonical)
	if err != nil {
		t.Fatalf("encode noncanonical frame: %v", err)
	}
	if _, err := DecodeRecovery(noncanonicalWire); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("noncanonical error = %v, want %v", err, ErrInvalidCredential)
	}
}

func TestCredentialDecodersRejectAdversarialJSON(t *testing.T) {
	t.Parallel()

	recovery, trusted, ephemeral := testCredentials(t)
	tests := []struct {
		name           string
		prefix         string
		kind           string
		wire           string
		decode         func(string) error
		omittedField   string
		authorityField string
	}{
		{
			name:           "recovery",
			prefix:         RecoveryPrefix,
			kind:           recoveryKind,
			wire:           mustEncodeCredential(t, func() (string, error) { return EncodeRecovery(recovery) }),
			decode:         func(encoded string) error { _, err := DecodeRecovery(encoded); return err },
			omittedField:   "last_signed_checkpoint",
			authorityField: "environment_seed",
		},
		{
			name:           "trusted",
			prefix:         TrustedPrefix,
			kind:           trustedKind,
			wire:           mustEncodeCredential(t, func() (string, error) { return EncodeTrusted(trusted) }),
			decode:         func(encoded string) error { _, err := DecodeTrusted(encoded); return err },
			omittedField:   "last_observed_checkpoint",
			authorityField: "admin_seed",
		},
		{
			name:           "ephemeral",
			prefix:         EphemeralPrefix,
			kind:           ephemeralKind,
			wire:           mustEncodeCredential(t, func() (string, error) { return EncodeEphemeral(ephemeral) }),
			decode:         func(encoded string) error { _, err := DecodeEphemeral(encoded); return err },
			omittedField:   "relay_token_expires_at_millis",
			authorityField: "project_root",
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			body := rawBodyFromWire(t, test.wire, test.prefix)
			mutations := []struct {
				name   string
				mutate func([]byte) []byte
			}{
				{
					name: "duplicate top-level key",
					mutate: func(value []byte) []byte {
						const versionPrefix = `{"version":1,`
						if !bytes.HasPrefix(value, []byte(versionPrefix)) {
							t.Fatalf("body does not start with %q", versionPrefix)
						}
						return append([]byte(versionPrefix), value[1:]...)
					},
				},
				{
					name: "omitted required field",
					mutate: func(value []byte) []byte {
						return omitTopLevelJSONField(t, value, test.omittedField)
					},
				},
				{
					name: "reordered top-level fields",
					mutate: func(value []byte) []byte {
						return reorderVersionAndKind(t, value, test.kind)
					},
				},
				{
					name: "cross-class authority field",
					mutate: func(value []byte) []byte {
						return prependUnknownTopLevelField(value, test.authorityField)
					},
				},
				{
					name: "trailing JSON value",
					mutate: func(value []byte) []byte {
						return append(append([]byte(nil), value...), []byte(` {}`)...)
					},
				},
			}
			for _, mutation := range mutations {
				mutation := mutation
				t.Run(mutation.name, func(t *testing.T) {
					mutatedBody := mutation.mutate(body)
					encoded, err := encodeCredentialFrame(test.prefix, test.kind, mutatedBody)
					if err != nil {
						t.Fatalf("encode adversarial frame: %v", err)
					}
					if err := test.decode(encoded); err == nil {
						t.Fatal("adversarial credential unexpectedly decoded")
					}
				})
			}

			t.Run("padded frame base64", func(t *testing.T) {
				payload := strings.TrimPrefix(test.wire, test.prefix)
				if payload == test.wire {
					t.Fatalf("wire does not have prefix %q", test.prefix)
				}
				if err := test.decode(test.prefix + payload + "="); err == nil {
					t.Fatal("padded base64 credential unexpectedly decoded")
				}
			})

			if test.name == "ephemeral" {
				t.Run("nested generation-key unknown field", func(t *testing.T) {
					marker := []byte(`{"generation":`)
					index := bytes.Index(body, marker)
					if index < 0 {
						t.Fatalf("body has no generation-key object")
					}
					mutatedBody := append([]byte(nil), body[:index+1]...)
					mutatedBody = append(mutatedBody, []byte(`"unexpected":true,`)...)
					mutatedBody = append(mutatedBody, body[index+1:]...)
					encoded, err := encodeCredentialFrame(test.prefix, test.kind, mutatedBody)
					if err != nil {
						t.Fatalf("encode nested adversarial frame: %v", err)
					}
					if err := test.decode(encoded); err == nil {
						t.Fatal("nested unknown generation-key field unexpectedly decoded")
					}
				})
			}
		})
	}
}

func TestEphemeralCredentialRequiresExactExplicitGenerationKeysAndExpiry(t *testing.T) {
	t.Parallel()

	_, _, ephemeral := testCredentials(t)
	tests := []struct {
		name   string
		mutate func(*EphemeralProjectCredential)
	}{
		{name: "no key", mutate: func(value *EphemeralProjectCredential) { value.GenerationKeys = nil }},
		{name: "duplicate key", mutate: func(value *EphemeralProjectCredential) {
			value.GenerationKeys = append(value.GenerationKeys, value.GenerationKeys[0])
		}},
		{name: "token without expiry", mutate: func(value *EphemeralProjectCredential) { value.RelayTokenExpiresAtMillis = 0 }},
		{name: "token past certificate", mutate: func(value *EphemeralProjectCredential) {
			value.RelayTokenExpiresAtMillis = value.Certificate.ExpiresAtMillis + 1
		}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			candidate := ephemeral
			candidate.GenerationKeys = append([]synccrypto.GenerationKey(nil), ephemeral.GenerationKeys...)
			test.mutate(&candidate)
			if _, err := EncodeEphemeral(candidate); !errors.Is(err, ErrInvalidCredential) {
				t.Fatalf("error = %v, want %v", err, ErrInvalidCredential)
			}
		})
	}
}

func TestTrustedCredentialDisallowsCertificateExpiry(t *testing.T) {
	t.Parallel()

	_, trusted, _ := testCredentials(t)
	adminSeed, err := synccrypto.AdminSeedFromBytes(sequentialBytes(0x21, 32))
	if err != nil {
		t.Fatalf("create admin seed: %v", err)
	}
	certificate := trusted.Certificate
	certificate.ExpiresAtMillis = 2_000_000_000_000
	certificate.AdminSignature = protocol.Signature{}
	certificate, err = synccrypto.SignEnvironmentCertificate(certificate, adminSeed)
	if err != nil {
		t.Fatalf("sign expiring trusted certificate: %v", err)
	}
	trusted.Certificate = certificate
	if _, err := EncodeTrusted(trusted); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("expiring trusted credential error = %v, want %v", err, ErrInvalidCredential)
	}
}

func TestCredentialRequiresHTTPSRelayEndpoint(t *testing.T) {
	t.Parallel()

	recovery, _, _ := testCredentials(t)
	recovery.RelayURL = "http://relay.example.test/v1"
	if _, err := EncodeRecovery(recovery); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("HTTP endpoint error = %v, want %v", err, ErrInvalidCredential)
	}
}

func TestCredentialRejectsZeroEnvironmentSeedEvenWhenPublicKeyMatches(t *testing.T) {
	t.Parallel()

	_, trusted, ephemeral := testCredentials(t)
	adminSeed, err := synccrypto.AdminSeedFromBytes(sequentialBytes(0x21, 32))
	if err != nil {
		t.Fatalf("create admin seed: %v", err)
	}
	zeroSeed := synccrypto.EnvironmentSeed{}
	zeroPublic := synccrypto.EnvironmentPublicKey(zeroSeed)

	trusted.EnvironmentSeed = zeroSeed
	trusted.Certificate.EnvironmentPublicKey = zeroPublic
	trusted.Certificate.AdminSignature = protocol.Signature{}
	trusted.Certificate, err = synccrypto.SignEnvironmentCertificate(trusted.Certificate, adminSeed)
	if err != nil {
		t.Fatalf("sign trusted zero-seed certificate: %v", err)
	}
	if _, err := EncodeTrusted(trusted); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("trusted zero-seed error = %v, want %v", err, ErrInvalidCredential)
	}

	ephemeral.EnvironmentSeed = zeroSeed
	ephemeral.Certificate.EnvironmentPublicKey = zeroPublic
	ephemeral.Certificate.AdminSignature = protocol.Signature{}
	ephemeral.Certificate, err = synccrypto.SignEnvironmentCertificate(ephemeral.Certificate, adminSeed)
	if err != nil {
		t.Fatalf("sign ephemeral zero-seed certificate: %v", err)
	}
	if _, err := EncodeEphemeral(ephemeral); !errors.Is(err, ErrInvalidCredential) {
		t.Fatalf("ephemeral zero-seed error = %v, want %v", err, ErrInvalidCredential)
	}
}

func TestCredentialErrorsDoNotEchoWireOrTokens(t *testing.T) {
	t.Parallel()

	recovery, _, _ := testCredentials(t)
	encoded, err := EncodeRecovery(recovery)
	if err != nil {
		t.Fatalf("encode recovery: %v", err)
	}
	corrupt := encoded[:len(encoded)-1] + differentBase64Character(encoded[len(encoded)-1])
	_, decodeErr := DecodeRecovery(corrupt)
	if decodeErr == nil {
		t.Fatal("decode corrupt credential error = nil")
	}
	ownerSecret := recovery.OwnerRelayAuthorization.Secret()
	if strings.Contains(decodeErr.Error(), corrupt) || bytes.Contains([]byte(decodeErr.Error()), ownerSecret[:]) {
		t.Fatalf("credential error leaked protected input: %q", decodeErr)
	}
}

func TestProtectedCredentialsRejectGenericJSONSerialization(t *testing.T) {
	t.Parallel()

	recovery, trusted, ephemeral := testCredentials(t)
	tests := []struct {
		name       string
		credential any
	}{
		{name: "recovery", credential: recovery},
		{name: "trusted", credential: trusted},
		{name: "ephemeral", credential: ephemeral},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			encoded, err := json.Marshal(test.credential)
			if !errors.Is(err, ErrProtectedCredentialEncoding) {
				t.Fatalf("marshal error = %v, want %v", err, ErrProtectedCredentialEncoding)
			}
			if len(encoded) != 0 {
				t.Fatalf("marshal returned %d protected bytes", len(encoded))
			}
		})
	}
}

func TestRelayBearerCarriesIndependentLookupIDAndSecretLosslessly(t *testing.T) {
	t.Parallel()

	var tokenID RelayTokenID
	copy(tokenID[:], sequentialBytes(0x11, len(tokenID)))
	var tokenSecret RelayTokenSecret
	copy(tokenSecret[:], sequentialBytes(0x31, len(tokenSecret)))
	bearer, err := NewRelayBearer(tokenID, tokenSecret)
	if err != nil {
		t.Fatalf("create bearer: %v", err)
	}
	if bearer.ID() != tokenID || bearer.Secret() != tokenSecret {
		t.Fatal("bearer did not preserve lookup ID and secret")
	}
	if strings.Contains(bearer.String(), base64.RawURLEncoding.EncodeToString(tokenSecret[:])) {
		t.Fatal("bearer String leaked its secret")
	}
}

func testCredentials(t *testing.T) (ProjectRecoveryCredential, TrustedProjectCredential, EphemeralProjectCredential) {
	t.Helper()
	rootMaterial := sequentialBytes(0x01, 32)
	root, err := synccrypto.ProjectRootFromBytes(rootMaterial)
	if err != nil {
		t.Fatalf("create root: %v", err)
	}
	adminSeed, err := synccrypto.AdminSeedFromBytes(sequentialBytes(0x21, 32))
	if err != nil {
		t.Fatalf("create admin seed: %v", err)
	}
	adminPublic := synccrypto.AdminPublicKey(adminSeed)
	environmentSeed, err := synccrypto.EnvironmentSeedFromBytes(sequentialBytes(0x41, 32))
	if err != nil {
		t.Fatalf("create environment seed: %v", err)
	}
	var channel protocol.ChannelID
	copy(channel[:], sequentialBytes(0x61, len(channel)))
	var relayGeneration protocol.RelayGeneration
	copy(relayGeneration[:], sequentialBytes(0x81, len(relayGeneration)))
	certificate, err := synccrypto.SignEnvironmentCertificate(protocol.EnvironmentCertificate{
		Version:               protocol.CertificateVersionV1,
		ProtocolVersion:       protocol.ProtocolVersionV1,
		CipherSuite:           protocol.CipherSuiteXChaCha20Poly1305,
		ProjectID:             "project-1",
		ChannelID:             channel,
		EnvironmentID:         "environment-a",
		EnvironmentPublicKey:  synccrypto.EnvironmentPublicKey(environmentSeed),
		Mode:                  protocol.EnvironmentTrusted,
		MembershipGeneration:  1,
		AllowedKeyGenerations: []uint32{7},
	}, adminSeed)
	if err != nil {
		t.Fatalf("sign trusted certificate: %v", err)
	}
	ephemeralCertificate := certificate
	ephemeralCertificate.Mode = protocol.EnvironmentEphemeral
	ephemeralCertificate.ExpiresAtMillis = 2_000_000_000_000
	ephemeralCertificate.AdminSignature = protocol.Signature{}
	ephemeralCertificate, err = synccrypto.SignEnvironmentCertificate(ephemeralCertificate, adminSeed)
	if err != nil {
		t.Fatalf("sign ephemeral certificate: %v", err)
	}
	generationKey, err := synccrypto.DeriveGenerationKey(root, "project-1", 7)
	if err != nil {
		t.Fatalf("derive generation key: %v", err)
	}
	newBearer := func(idStart, secretStart byte) RelayBearer {
		var tokenID RelayTokenID
		copy(tokenID[:], sequentialBytes(idStart, len(tokenID)))
		var tokenSecret RelayTokenSecret
		copy(tokenSecret[:], sequentialBytes(secretStart, len(tokenSecret)))
		bearer, bearerErr := NewRelayBearer(tokenID, tokenSecret)
		if bearerErr != nil {
			t.Fatalf("create relay bearer: %v", bearerErr)
		}
		return bearer
	}
	commonURL := "https://relay.example.test/v1"
	recovery := ProjectRecoveryCredential{
		ProjectID:               "project-1",
		RelayURL:                commonURL,
		RelayGeneration:         relayGeneration,
		ChannelID:               channel,
		ProjectRoot:             root,
		AdminSeed:               adminSeed,
		OwnerRelayAuthorization: newBearer(0xa1, 0xb1),
		WriteGeneration:         7,
		LastSignedCheckpoint:    []byte("signed-checkpoint"),
	}
	trusted := TrustedProjectCredential{
		ProjectID:                     "project-1",
		RelayURL:                      commonURL,
		RelayGeneration:               relayGeneration,
		ChannelID:                     channel,
		AdminPublicKey:                adminPublic,
		Certificate:                   certificate,
		EnvironmentSeed:               environmentSeed,
		EnvironmentRelayAuthorization: newBearer(0xc1, 0xd1),
		ProjectRoot:                   root,
		WriteGeneration:               7,
		MinimumProtocolVersion:        protocol.ProtocolVersionV1,
		LastObservedCheckpoint:        []byte("signed-checkpoint"),
	}
	ephemeral := EphemeralProjectCredential{
		ProjectID:                     "project-1",
		RelayURL:                      commonURL,
		RelayGeneration:               relayGeneration,
		ChannelID:                     channel,
		AdminPublicKey:                adminPublic,
		Certificate:                   ephemeralCertificate,
		EnvironmentSeed:               environmentSeed,
		EnvironmentRelayAuthorization: newBearer(0xe1, 0xf1),
		RelayTokenExpiresAtMillis:     1_999_999_999_000,
		GenerationKeys:                []synccrypto.GenerationKey{generationKey},
	}
	return recovery, trusted, ephemeral
}

func sequentialBytes(start byte, count int) []byte {
	value := make([]byte, count)
	for index := range value {
		value[index] = start + byte(index)
	}
	return value
}

func differentBase64Character(value byte) string {
	if value == 'A' {
		return "B"
	}
	return "A"
}

func rawBodyFromWire(t *testing.T, wire, prefix string) []byte {
	t.Helper()
	decoded, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(wire, prefix))
	if err != nil {
		t.Fatalf("decode framed body: %v", err)
	}
	return decoded[:len(decoded)-credentialChecksumBytes]
}

func mustEncodeCredential(t *testing.T, encode func() (string, error)) string {
	t.Helper()
	encoded, err := encode()
	if err != nil {
		t.Fatalf("encode credential fixture: %v", err)
	}
	return encoded
}

func omitTopLevelJSONField(t *testing.T, body []byte, field string) []byte {
	t.Helper()
	marker := []byte(`"` + field + `":`)
	fieldStart := bytes.Index(body, marker)
	if fieldStart < 1 || body[fieldStart-1] != ',' {
		t.Fatalf("top-level field %q not found in canonical body", field)
	}
	inString := false
	escaped := false
	depth := 0
	for index := fieldStart + len(marker); index < len(body); index++ {
		value := body[index]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if value == '\\' {
				escaped = true
			} else if value == '"' {
				inString = false
			}
			continue
		}
		switch value {
		case '"':
			inString = true
		case '{', '[':
			depth++
		case '}', ']':
			if depth > 0 {
				depth--
			} else if value == '}' {
				return append(append([]byte(nil), body[:fieldStart-1]...), body[index:]...)
			}
		case ',':
			if depth == 0 {
				return append(append([]byte(nil), body[:fieldStart-1]...), body[index:]...)
			}
		}
	}
	t.Fatalf("top-level field %q has no JSON value", field)
	return nil
}

func reorderVersionAndKind(t *testing.T, body []byte, kind string) []byte {
	t.Helper()
	canonicalPrefix := []byte(`{"version":1,"kind":"` + kind + `",`)
	if !bytes.HasPrefix(body, canonicalPrefix) {
		t.Fatalf("body does not have canonical version/kind prefix")
	}
	reorderedPrefix := []byte(`{"kind":"` + kind + `","version":1,`)
	return append(append([]byte(nil), reorderedPrefix...), body[len(canonicalPrefix):]...)
}

func prependUnknownTopLevelField(body []byte, field string) []byte {
	prefix := []byte(`{"` + field + `":null,`)
	return append(prefix, body[1:]...)
}
