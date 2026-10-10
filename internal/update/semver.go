// Package update checks GitHub Releases for newer HinaTracer versions,
// picks the asset for this platform, verifies it against SHA256SUMS.txt and
// installs it.
package update

import (
	"strconv"
	"strings"
)

// Version is a parsed semantic version (vMAJOR.MINOR.PATCH[-pre][+build]).
type Version struct {
	Major, Minor, Patch int
	Pre                 []string
	Valid               bool
}

// ParseVersion parses "v1.2.3", "1.2.3-beta.1", "1.2" (patch 0). Build
// metadata after '+' is ignored.
func ParseVersion(s string) Version {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(strings.TrimPrefix(s, "v"), "V")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i]
	}
	var pre string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		s, pre = s[:i], s[i+1:]
	}
	parts := strings.Split(s, ".")
	if len(parts) < 1 || len(parts) > 3 {
		return Version{}
	}
	nums := [3]int{}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return Version{}
		}
		nums[i] = n
	}
	v := Version{Major: nums[0], Minor: nums[1], Patch: nums[2], Valid: true}
	if pre != "" {
		v.Pre = strings.Split(pre, ".")
	}
	return v
}

// IsPrerelease reports a pre-release tag such as -beta.1.
func (v Version) IsPrerelease() bool { return len(v.Pre) > 0 }

// Compare returns -1, 0 or 1 following semver precedence rules:
// a pre-release sorts before its release; numeric identifiers compare
// numerically and below alphanumeric ones; more fields win on a tie.
func Compare(a, b Version) int {
	for _, d := range [][2]int{{a.Major, b.Major}, {a.Minor, b.Minor}, {a.Patch, b.Patch}} {
		if d[0] != d[1] {
			if d[0] < d[1] {
				return -1
			}
			return 1
		}
	}
	switch {
	case len(a.Pre) == 0 && len(b.Pre) == 0:
		return 0
	case len(a.Pre) == 0:
		return 1
	case len(b.Pre) == 0:
		return -1
	}
	for i := 0; i < len(a.Pre) && i < len(b.Pre); i++ {
		x, y := a.Pre[i], b.Pre[i]
		xn, xe := strconv.Atoi(x)
		yn, ye := strconv.Atoi(y)
		switch {
		case xe == nil && ye == nil:
			if xn != yn {
				if xn < yn {
					return -1
				}
				return 1
			}
		case xe == nil:
			return -1
		case ye == nil:
			return 1
		default:
			if c := strings.Compare(x, y); c != 0 {
				return c
			}
		}
	}
	switch {
	case len(a.Pre) < len(b.Pre):
		return -1
	case len(a.Pre) > len(b.Pre):
		return 1
	}
	return 0
}

// CompareStrings compares two version strings.
func CompareStrings(a, b string) int { return Compare(ParseVersion(a), ParseVersion(b)) }

// IsDevVersion reports builds that must not be nagged: unparsable,
// 0.0.0, or a "dev" pre-release.
func IsDevVersion(s string) bool {
	v := ParseVersion(s)
	if !v.Valid || (v.Major == 0 && v.Minor == 0 && v.Patch == 0) {
		return true
	}
	for _, p := range v.Pre {
		if strings.Contains(strings.ToLower(p), "dev") {
			return true
		}
	}
	return false
}
