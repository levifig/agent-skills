package cli

import "testing"

func TestReleaseVersionIsPrerelease(t *testing.T) {
	cases := []struct {
		version string
		want    bool
	}{
		{"2.0.0-alpha.14", true},
		{"v2.0.0-alpha.14", true},
		{"2.0.0-beta.1", true},
		{"2.0.0-rc.1", true},
		{"2.0.0-pre.20260625192947", true},
		{"2.0.0", false},
		{"v2.0.0", false},
		{"1.17.4", false},
		{"", false},
		{"not-a-version", false},
	}
	for _, tc := range cases {
		if got := releaseVersionIsPrerelease(tc.version); got != tc.want {
			t.Errorf("releaseVersionIsPrerelease(%q) = %v, want %v", tc.version, got, tc.want)
		}
	}
}
