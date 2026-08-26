package state

import (
	"errors"
	"testing"
)

func TestParseAuthorityRefV1Providers(t *testing.T) {
	cases := []struct {
		raw      string
		provider string
		key      string
	}{
		{"linear:ENG-123", AuthorityProviderLinear, "ENG-123"},
		{"branch:issue/loaf-68", AuthorityProviderBranch, "issue/loaf-68"},
		{"pr:42", AuthorityProviderPR, "42"},
	}
	for _, tc := range cases {
		ref, err := ParseAuthorityRef(tc.raw)
		if err != nil {
			t.Fatalf("ParseAuthorityRef(%q) error = %v", tc.raw, err)
		}
		if ref.Provider != tc.provider || ref.Key != tc.key {
			t.Fatalf("ParseAuthorityRef(%q) = %#v, want provider=%s key=%s", tc.raw, ref, tc.provider, tc.key)
		}
	}
}

func TestParseAuthorityRefRefusesUnsupportedProviders(t *testing.T) {
	for _, raw := range []string{"github:owner/repo#1", "gitlab:123", "gitea:abc", "forgejo:xyz"} {
		_, err := ParseAuthorityRef(raw)
		if err == nil {
			t.Fatalf("ParseAuthorityRef(%q) = nil, want UnsupportedAuthorityError", raw)
		}
		var unsupported *UnsupportedAuthorityError
		if !errors.As(err, &unsupported) {
			t.Fatalf("ParseAuthorityRef(%q) error = %T %v, want *UnsupportedAuthorityError", raw, err, err)
		}
	}
}

func TestParseAuthorityRefRequiresProviderQualifiedForm(t *testing.T) {
	for _, raw := range []string{"", "LOAF-82", "ENG-123", "linear:"} {
		if _, err := ParseAuthorityRef(raw); err == nil {
			t.Fatalf("ParseAuthorityRef(%q) = nil, want error", raw)
		}
	}
}

func TestIsAuthorityRef(t *testing.T) {
	if !IsAuthorityRef("linear:ENG-1") {
		t.Fatal("IsAuthorityRef(linear:ENG-1) = false, want true")
	}
	if IsAuthorityRef("LOAF-82") {
		t.Fatal("IsAuthorityRef(LOAF-82) = true, want false")
	}
}
