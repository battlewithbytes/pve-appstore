package engine

import "testing"

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		catalog   string
		installed string
		want      bool
		desc      string
	}{
		// Standard semver — catalog is newer
		{"0.8.0", "0.4.0", true, "newer minor"},
		{"1.0.0", "0.9.9", true, "newer major"},
		{"0.5.1", "0.5.0", true, "newer patch"},
		{"2.0.0", "1.99.99", true, "major bump"},

		// Standard semver — catalog is NOT newer
		{"0.4.0", "0.8.0", false, "older minor (downgrade)"},
		{"0.5.0", "0.5.0", false, "same version"},
		{"0.9.9", "1.0.0", false, "older major"},
		{"0.5.0", "0.5.1", false, "older patch"},

		// With v prefix
		{"v1.2.0", "v1.1.0", true, "v-prefix newer"},
		{"v1.1.0", "v1.2.0", false, "v-prefix older"},
		{"v1.0.0", "1.0.0", false, "mixed v-prefix same"},
		{"1.2.0", "v1.1.0", true, "mixed v-prefix newer"},

		// Pre-release suffix (stripped before comparison)
		{"1.0.1-beta", "1.0.0", true, "pre-release newer"},
		{"1.0.0-rc1", "1.0.0", false, "pre-release same base"},

		// Non-semver fallback (string comparison)
		{"latest", "stable", true, "non-semver different"},
		{"stable", "stable", false, "non-semver same"},

		// Partial versions — fallback
		{"1.0", "0.9", true, "two-part version fallback"},
		{"abc", "abc", false, "non-version same"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			got := isNewerVersion(tt.catalog, tt.installed)
			if got != tt.want {
				t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tt.catalog, tt.installed, got, tt.want)
			}
		})
	}
}
