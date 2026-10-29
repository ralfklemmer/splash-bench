package check

import (
	"fmt"
	"regexp"
	"strconv"
)

// Version is a semantic version of the Splash launcher or of macOS.
type Version struct {
	Major, Minor, Patch int
}

// Splash versions that unlock features the kit cares about.
var (
	MinSplashVersion      = Version{1, 3, 0}
	MixedBitVersion       = Version{1, 3, 1}
	ThinkingBudgetVersion = Version{1, 3, 2}
	FlashNextVersion      = Version{1, 4, 0}
)

// MinMacOS is the oldest macOS the official Splash installs on.
var MinMacOS = Version{26, 4, 0}

var (
	versionPattern = regexp.MustCompile(`(\d+)\.(\d+)\.(\d+)`)
	macOSPattern   = regexp.MustCompile(`(\d+)\.(\d+)(?:\.(\d+))?`)
)

// ParseVersion finds the first x.y.z version in output such as
// "Splash 1.3.2".
func ParseVersion(output string) (Version, error) {
	m := versionPattern.FindStringSubmatch(output)
	if m == nil {
		return Version{}, fmt.Errorf("keine Versionsnummer in %q", output)
	}
	return versionFromMatch(m)
}

// ParseMacOSVersion reads `sw_vers -productVersion`, which omits a zero
// patch level ("26.4", "27.0.1").
func ParseMacOSVersion(output string) (Version, error) {
	m := macOSPattern.FindStringSubmatch(output)
	if m == nil {
		return Version{}, fmt.Errorf("keine macOS-Version in %q", output)
	}
	if m[3] == "" {
		m[3] = "0"
	}
	return versionFromMatch(m)
}

func versionFromMatch(m []string) (Version, error) {
	var parts [3]int
	for i := range parts {
		n, err := strconv.Atoi(m[i+1])
		if err != nil {
			return Version{}, fmt.Errorf("parse version %q: %w", m[0], err)
		}
		parts[i] = n
	}
	return Version{parts[0], parts[1], parts[2]}, nil
}

// Less reports whether v is older than other.
func (v Version) Less(other Version) bool {
	if v.Major != other.Major {
		return v.Major < other.Major
	}
	if v.Minor != other.Minor {
		return v.Minor < other.Minor
	}
	return v.Patch < other.Patch
}

// IsZero reports whether v is unset.
func (v Version) IsZero() bool {
	return v == Version{}
}

func (v Version) String() string {
	return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
}

// MissingFeatures lists the Splash features v does not support yet.
func (v Version) MissingFeatures() []string {
	var missing []string
	if v.Less(MixedBitVersion) {
		missing = append(missing, "Mixed-Bit-Modelle (ab "+MixedBitVersion.String()+")")
	}
	if v.Less(ThinkingBudgetVersion) {
		missing = append(missing, "Thinking-Budget (ab "+ThinkingBudgetVersion.String()+")")
	}
	if v.Less(FlashNextVersion) {
		missing = append(missing, "Qwen3.8-Flash-Next (ab "+FlashNextVersion.String()+")")
	}
	return missing
}
