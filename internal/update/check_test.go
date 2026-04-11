package update

import "testing"

// TestCompareVersions verifies the ad-hoc version comparator against the
// release-tag shapes BRUV actually uses. The scheme is:
//
//	v<major>.<minor><letter>[-prerelease]
//
// e.g. v1.0b, v1.0b-dev, v1.0.1, v1.1b. The letter suffix acts as a
// tiebreaker after the numeric portion, and a pre-release (-dev, -beta)
// sorts BEFORE the same release without one.
func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int // -1 if a<b, 0 if equal, 1 if a>b
	}{
		// Identity cases — same input both sides.
		{"v1.0b", "v1.0b", 0},
		{"v1.0.0", "v1.0.0", 0},
		{"v1.0b-dev", "v1.0b-dev", 0},

		// Leading "v" is optional.
		{"1.0b", "v1.0b", 0},
		{"v1.0b", "1.0b", 0},

		// Pre-release suffixes sort BEFORE the plain version.
		{"v1.0b-dev", "v1.0b", -1},
		{"v1.0b", "v1.0b-dev", 1},

		// Numeric major bumps dominate letter suffixes.
		{"v1.0b", "v2.0a", -1},
		{"v2.0a", "v1.0b", 1},

		// Minor version bumps on same major.
		{"v1.0b", "v1.1b", -1},
		{"v1.1b", "v1.0b", 1},

		// Patch version differences.
		{"v1.0.0", "v1.0.1", -1},
		{"v1.0.1", "v1.0.0", 1},

		// Letter suffix is a tiebreaker after numeric parts match.
		{"v1.0a", "v1.0b", -1},
		{"v1.0b", "v1.0c", -1},
	}

	for _, tc := range cases {
		got := compareVersions(tc.a, tc.b)
		// Normalise to -1/0/1 because the sign is all that matters.
		switch {
		case got < 0:
			got = -1
		case got > 0:
			got = 1
		}
		if got != tc.want {
			t.Errorf("compareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

// TestSplitSuffix documents the behaviour splitSuffix contributes to the
// full compareVersions logic — specifically how "b-dev" splits and how
// suffix-less strings are handled.
func TestSplitSuffix(t *testing.T) {
	cases := []struct {
		in, char, pre string
	}{
		{"b", "b", ""},
		{"b-dev", "b", "dev"},
		{"b-beta", "b", "beta"},
		{"", "", ""},
		{"-dev", "", "dev"},
	}
	for _, tc := range cases {
		char, pre := splitSuffix(tc.in)
		if char != tc.char || pre != tc.pre {
			t.Errorf("splitSuffix(%q) = (%q, %q), want (%q, %q)", tc.in, char, pre, tc.char, tc.pre)
		}
	}
}
